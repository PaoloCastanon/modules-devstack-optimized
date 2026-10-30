#!/usr/bin/env python3
"""Reversible lab Placement route switch with private backup and digest leases."""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import subprocess
import sys
import time
from urllib.request import urlopen

SITE_FILE = Path('/etc/apache2/sites-enabled/placement-api.conf')
ORIGINAL_UNIT = 'devstack@placement-api.service'


def digest(data):
    return hashlib.sha256(data).hexdigest()


def command(values, **kwargs):
    return subprocess.run(values, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, **kwargs)


def active(unit):
    return subprocess.run(['systemctl', 'is-active', '--quiet', unit], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0


def replace_site(path, data, expected, mode, uid, gid):
    # Only the fixed root-owned Apache site is written, with a lease checked
    # inside the privileged process and atomic rename/fsync in the same folder.
    script = '''import hashlib,os,sys,tempfile
p,expected,mode,uid,gid=sys.argv[1:]
if hashlib.sha256(open(p,'rb').read()).hexdigest()!=expected: raise SystemExit(2)
fd,tmp=tempfile.mkstemp(prefix='.placement-b13-',dir=os.path.dirname(p))
try:
 with os.fdopen(fd,'wb') as stream:
  stream.write(sys.stdin.buffer.read());stream.flush();os.fsync(stream.fileno())
  os.fchmod(stream.fileno(),int(mode));os.fchown(stream.fileno(),int(uid),int(gid))
 os.replace(tmp,p)
 d=os.open(os.path.dirname(p),os.O_RDONLY|os.O_DIRECTORY);os.fsync(d);os.close(d)
finally:
 if os.path.exists(tmp):os.unlink(tmp)
'''
    command(['sudo', 'python3', '-c', script, str(path), expected, str(mode), str(uid), str(gid)], input=data)


def wait_version(url, maximum):
    for _ in range(50):
        try:
            with urlopen(url.rstrip('/') + '/', timeout=2) as response:
                value = json.load(response)
                if value['versions'][0]['max_version'] == maximum:
                    return value
        except (OSError, ValueError, KeyError, IndexError):
            pass
        time.sleep(.2)
    raise RuntimeError('public endpoint did not reach expected version')


def run(args):
    if not args.allow_mutations:
        raise ValueError('explicit mutation opt-in required')
    root = args.root
    if not root.is_absolute() or root != root.resolve() or not re.fullmatch(r'tt-placement-[a-z0-9-]+', root.name):
        raise ValueError('canonical owned runtime directory required')
    if root.stat().st_uid != os.getuid() or root.stat().st_mode & 0o077:
        raise ValueError('private runtime ownership mismatch')
    runtime = json.loads((root / 'state.json').read_text())
    if runtime.get('owner_uid') != os.getuid() or not runtime.get('prepared'):
        raise ValueError('runtime not prepared by this owner')
    site = SITE_FILE.resolve()
    current = site.read_bytes()
    state_path = root / 'switch-state.json'

    def save(state):
        state_path.write_text(json.dumps(state))
        state_path.chmod(0o600)

    def restore(state):
        original = (root / 'original-apache-site.conf').read_bytes()
        if digest(original) != state['original_sha']:
            raise ValueError('backup digest mismatch')
        now = site.read_bytes()
        if digest(now) not in (state['original_sha'], state['go_sha']):
            raise ValueError('site digest changed outside this switch')
        command(['sudo', 'systemctl', 'start', ORIGINAL_UNIT])
        if digest(now) != state['original_sha']:
            replace_site(site, original, digest(now), state['mode'], state['uid'], state['gid'])
        command(['sudo', 'apache2ctl', 'configtest'])
        command(['sudo', 'systemctl', 'reload', 'apache2'])
        if not state['proxy_http_was_enabled']:
            command(['sudo', 'a2dismod', 'proxy_http'])
            command(['sudo', 'apache2ctl', 'configtest'])
            command(['sudo', 'systemctl', 'reload', 'apache2'])
        discovery = wait_version(args.public_url, state['original_max_version'])
        if not active(ORIGINAL_UNIT) or digest(site.read_bytes()) != state['original_sha']:
            raise RuntimeError('original restoration verification failed')
        state['active'] = False
        save(state)
        if state.get('rollback_unit'):
            command(['sudo', 'systemctl', 'stop', state['rollback_unit'] + '.timer'])
        return {'original_restored': True, 'original_site_sha256': state['original_sha'],
                'original_unit_active': True, 'discovery': discovery}

    if args.action == 'restore':
        report = restore(json.loads(state_path.read_text()))
        print(json.dumps(report))
        return 0

    if not args.expected_original_sha or digest(current) != args.expected_original_sha:
        raise ValueError('original site digest mismatch')
    if not 5 <= args.lease_seconds <= 1800:
        raise ValueError('bounded recovery lease required')
    if not active(ORIGINAL_UNIT) or not active(runtime['go_unit']):
        raise ValueError('original and prepared Go units must be active')
    if not 1024 <= runtime['listen_port'] <= 65535:
        raise ValueError('invalid loopback backend port')
    wait_version('http://127.0.0.1:' + str(runtime['listen_port']), '1.36')
    discovery = wait_version(args.public_url, '1.39')
    replacement = ('ProxyPass "/placement" "http://127.0.0.1:' + str(runtime['listen_port']) + '" retry=0 acquire=1\n' +
                   'ProxyPassReverse "/placement" "http://127.0.0.1:' + str(runtime['listen_port']) + '"\n').encode()
    modules = subprocess.check_output(['sudo', 'apache2ctl', '-M'], stderr=subprocess.DEVNULL, text=True)
    attrs = site.stat()
    state = {'original_sha': digest(current), 'go_sha': digest(replacement),
             'mode': attrs.st_mode & 0o777, 'uid': attrs.st_uid, 'gid': attrs.st_gid,
             'proxy_http_was_enabled': 'proxy_http_module' in modules,
             'original_max_version': discovery['versions'][0]['max_version'], 'active': False}
    backup = root / 'original-apache-site.conf'
    if backup.exists():
        if digest(backup.read_bytes()) != state['original_sha']:
            raise ValueError('existing backup digest mismatch')
    else:
        backup.write_bytes(current)
        backup.chmod(0o600)
    save(state)
    try:
        if not state['proxy_http_was_enabled']:
            command(['sudo', 'a2enmod', 'proxy_http'])
        replace_site(site, replacement, state['original_sha'], state['mode'], state['uid'], state['gid'])
        command(['sudo', 'apache2ctl', 'configtest'])
        command(['sudo', 'systemctl', 'reload', 'apache2'])
        command(['sudo', 'systemctl', 'stop', ORIGINAL_UNIT])
        discovery = wait_version(args.public_url, '1.36')
        if active(ORIGINAL_UNIT):
            raise RuntimeError('original Placement still active')
        state['active'] = True
        state['rollback_unit'] = 'tt-placement-b13-rollback-' + secrets.token_hex(5)
        save(state)
        command(['sudo', 'systemd-run', '--unit=' + state['rollback_unit'], '--uid=' + str(os.getuid()),
                 '--collect', '--on-active=' + str(args.lease_seconds) + 's', '--timer-property=AccuracySec=1s',
                 sys.executable, str(Path(__file__).resolve()), 'restore', '--root', str(root),
                 '--public-url', args.public_url, '--allow-mutations'])
    except Exception:
        restore(state)
        raise
    print(json.dumps({'go_route_active': True, 'original_unit_stopped': True,
                      'recovery_lease_s': args.lease_seconds,
                      'site_sha256': digest(site.read_bytes()), 'discovery': discovery}))
    return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['activate', 'restore'])
    parser.add_argument('--root', type=Path, required=True)
    parser.add_argument('--expected-original-sha')
    parser.add_argument('--public-url', default='http://192.168.122.10/placement')
    parser.add_argument('--lease-seconds', type=int, default=600)
    parser.add_argument('--allow-mutations', action='store_true')
    args = parser.parse_args()
    try:
        return run(args)
    except Exception as error:
        print('route switch failed: ' + type(error).__name__, file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())

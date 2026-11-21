#!/usr/bin/env python3
"""Permanent replacement of the original Glance by Glance Go in the lab VM.

Unlike the B26 suite (temporary units, private MySQL and a recovery lease), this
installs Glance Go for good:

- ``install``: Glance Go serves the ORIGINAL catalog (its only mode): the official ``glance`` database and its filesystem store directory, so both
  implementations see the same images and files and switching copies nothing.
  Glance Go reads the original's own ``/etc/glance/glance-api.conf``
  (GLANCE_GO_CONFIG_FILE): the same MySQL account ([database] connection), the
  same Keystone service user ([keystone_authtoken]), notifications, store
  directory and public endpoint. It writes image files directly as
  ``<id>``, as glance_store does, and keeps only its transfer bookkeeping in
  ``glance_go_shared`` on the same MySQL server. ``glance-go.service`` is
  enabled, then ``use-go``.
- ``use-go``: /image points to Glance Go; ``devstack@g-api`` is stopped and
  disabled (not removed). Survives reboots.
- ``use-original``: restores the original Apache site and service; Glance Go is
  stopped and disabled, its data kept.
- ``status``: which implementation serves /image.

The B26 suite and the campaigns need the original: run ``use-original`` first.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
import time
from urllib.parse import unquote, urlsplit

HERE = Path(__file__).resolve().parent
sys.path[:0] = [str(HERE), str(HERE.parent / 'benchmarks')]

ROOT = Path('/opt/stack/data/glance-go')
UNIT = 'glance-go.service'
UNIT_FILE = Path('/etc/systemd/system/' + UNIT)
ORIGINAL_UNIT = 'devstack@g-api.service'
SITE_FILE = Path('/etc/apache2/sites-enabled/glance-api.conf')
GLANCE_CONF = Path('/etc/glance/glance-api.conf')
LISTEN_PORT = 19292  # 19293 stays free for the B26 suite's temporary runtime
PUBLIC_URL = 'http://192.168.122.10/image'
KEYSTONE_URL = 'http://192.168.122.10/identity/v3'
SERVICE_USER = 'glance-go'
CORE = {'id', 'name', 'owner', 'status', 'visibility', 'disk_format', 'container_format', 'size', 'virtual_size',
        'checksum', 'os_hash_algo', 'os_hash_value', 'created_at', 'updated_at', 'min_disk', 'min_ram', 'protected',
        'os_hidden', 'tags', 'self', 'file', 'schema', 'direct_url', 'stores', 'locations', 'deleted', 'deleted_at'}


def command(values, **kwargs):
    return subprocess.run(values, check=True, **kwargs)


def sudo_read(path):
    return command(['sudo', '-n', 'cat', str(path)], capture_output=True, text=True).stdout


def digest(data):
    return hashlib.sha256(data if isinstance(data, bytes) else data.encode()).hexdigest()


# -- pure helpers (unit tested) -----------------------------------------------
def database_from_glance_conf(conf_text):
    """Host, port, user and password of DevStack's MySQL, from the original's
    [database] connection (DevStack connects as root)."""
    match = re.search(r'^\s*connection\s*=\s*(\S+)', conf_text.split('[database]', 1)[-1], re.M)
    if not match:
        raise ValueError('original Glance [database] connection not found')
    url = urlsplit(match.group(1))
    if not url.scheme.startswith('mysql') or not url.hostname or not url.username:
        raise ValueError('original Glance must use MySQL')
    return {'host': url.hostname, 'port': url.port or 3306, 'user': unquote(url.username), 'password': unquote(url.password or '')}


def go_site(original_site, port):
    """The original Apache site with only its /image backend port changed, so
    SetEnv and ProxyPass options stay as DevStack wrote them."""
    site, n = re.subn(r'(ProxyPass(?:Reverse)?\s+"/image"\s+"http://127\.0\.0\.1:)\d+(")', r'\g<1>' + str(port) + r'\2', original_site)
    if n == 0:
        raise ValueError('original /image ProxyPass not found')
    if 'ProxyPassReverse "/image"' not in site:
        site = site.rstrip('\n') + '\nProxyPassReverse "/image" "http://127.0.0.1:' + str(port) + '"\n'
    return site


def backend_port(site_text):
    match = re.search(r'^ProxyPass\s+"/image"\s+"http://127\.0\.0\.1:(\d+)"', site_text, re.M)
    return int(match.group(1)) if match else None


def official_store_dir(conf_text):
    """The original's filesystem_store_datadir, without a trailing slash."""
    match = re.search(r'^\s*filesystem_store_datadir\s*=\s*(\S+)', conf_text, re.M)
    if not match:
        raise ValueError('original Glance filesystem_store_datadir not found')
    path = match.group(1).rstrip('/')
    if not path.startswith('/') or os.path.normpath(path) != path:
        raise ValueError('original store directory must be absolute and clean')
    return path


def serve_env_original(root, scope):
    """Only what glance-api.conf does not say: Glance Go's own port and its
    private store root (lock and scope marker)."""
    env = {'GLANCE_GO_LISTEN_ADDR': '127.0.0.1:' + str(LISTEN_PORT), 'GLANCE_GO_CONFIG_FILE': str(GLANCE_CONF),
           'GLANCE_GO_BINARY_ENABLED': 'true', 'GLANCE_GO_STORE_ROOT': str(root / 'shared-store'), 'GLANCE_GO_STORE_SCOPE': scope}
    return ''.join(k + '=' + v + '\n' for k, v in env.items())




def unit_text(root, user):
    return ('[Unit]\nDescription=Glance Go image service (replaces devstack@g-api)\n'
            'After=network-online.target mysql.service devstack@keystone.service\nWants=network-online.target\n\n'
            '[Service]\nUser=' + user + '\nEnvironmentFile=' + str(root / 'serve.env') + '\n'
            'ExecStart=' + str(root / 'bin/glance') + ' serve\nRestart=on-failure\nRestartSec=2\n'
            'TimeoutStopSec=40\nLimitNOFILE=65536\n\n[Install]\nWantedBy=multi-user.target\n')










def admin_token():
    from keystoneauth1 import session
    from keystoneauth1.identity import v3
    auth = v3.Password(auth_url=os.environ['OS_AUTH_URL'].rstrip('/') + ('' if os.environ['OS_AUTH_URL'].rstrip('/').endswith('/v3') else '/v3'),
                       username=os.environ['OS_USERNAME'], password=os.environ['OS_PASSWORD'], project_name=os.environ['OS_PROJECT_NAME'],
                       user_domain_name=os.environ.get('OS_USER_DOMAIN_NAME', 'Default'), project_domain_name=os.environ.get('OS_PROJECT_DOMAIN_NAME', 'Default'))
    s = session.Session(auth=auth, timeout=20)
    return s, s.get_token()


def private(path, text):
    path.touch(mode=0o600)
    path.chmod(0o600)
    path.write_text(text)


def install(args):
    import pymysql
    if not args.binary.is_file():
        raise ValueError('Glance Go binary required')
    command(['sudo', 'install', '-d', '-m', '700', '-o', str(os.getuid()), '-g', str(os.getgid()), str(ROOT)])
    (ROOT / 'bin').mkdir(exist_ok=True)
    conf = sudo_read(GLANCE_CONF)
    store_dir = official_store_dir(conf)
    db = database_from_glance_conf(conf)  # the original's own account creates the bookkeeping database
    with pymysql.connect(host=db['host'], port=db['port'], user=db['user'], password=db['password'], connect_timeout=10) as conn:
        with conn.cursor() as cur:
            cur.execute('CREATE DATABASE IF NOT EXISTS `glance_go_shared`')
        conn.commit()
    report = {'catalog': 'official', 'config_file': str(GLANCE_CONF), 'store_dir': store_dir}
    tmp = ROOT / 'bin/.glance.new'
    tmp.write_bytes(args.binary.read_bytes())
    tmp.chmod(0o700)
    tmp.replace(ROOT / 'bin/glance')
    base = {k: v for k, v in os.environ.items() if not k.startswith('GLANCE_GO_')}
    glance = str(ROOT / 'bin/glance')
    with (ROOT / 'shared-migrate.log').open('w') as log:
        command([glance, 'migrate'], env={**base, 'GLANCE_GO_CONFIG_FILE': str(GLANCE_CONF)}, stdout=log, stderr=subprocess.STDOUT)
    pre = dict(base)
    for line in serve_env_original(ROOT, '').splitlines():
        k, _, v = line.partition('=')
        pre[k] = v
    (ROOT / 'shared-store').mkdir(mode=0o700, exist_ok=True)
    scope = command([glance, 'store-scope'], env=pre, capture_output=True, text=True).stdout.strip()
    pre['GLANCE_GO_STORE_SCOPE'] = scope
    if not any((ROOT / 'shared-store').iterdir()):
        with (ROOT / 'shared-store-init.log').open('w') as log:
            command([glance, 'store-init'], env=pre, stdout=log, stderr=subprocess.STDOUT)
    private(ROOT / 'serve.env', serve_env_original(ROOT, scope))
    command(['sudo', 'tee', str(UNIT_FILE)], input=unit_text(ROOT, os.environ.get('USER', 'stack')), text=True, stdout=subprocess.DEVNULL)
    command(['sudo', 'systemctl', 'daemon-reload'])
    report.update(use_go(restart=True))
    report.update({'installed': True, 'root': str(ROOT), 'unit': UNIT, 'listen': LISTEN_PORT,
                   'binary_sha256': digest((ROOT / 'bin/glance').read_bytes())})
    print(json.dumps(report, indent=1))
    return 0




def wait_up(url):
    from urllib.request import urlopen
    from urllib.error import HTTPError
    for _ in range(100):
        try:
            with urlopen(url + '/v2/images', timeout=2):
                return
        except HTTPError as error:
            error.close()
            if error.code == 401:
                return
        except OSError:
            pass
        time.sleep(0.3)
    raise RuntimeError('Glance Go did not become ready; see journalctl -u ' + UNIT)




def switch_site(target_text):
    from placement_switch import replace_site
    attrs = SITE_FILE.resolve().stat()
    current = sudo_read(SITE_FILE)
    if current != target_text:
        replace_site(SITE_FILE.resolve(), target_text.encode(), digest(current), attrs.st_mode & 0o777, attrs.st_uid, attrs.st_gid)
    command(['sudo', 'apache2ctl', 'configtest'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    command(['sudo', 'systemctl', 'reload', 'apache2'])


def original_site():
    backup = ROOT / 'original-glance-api.conf'
    if not backup.exists():
        current = sudo_read(SITE_FILE)
        if backend_port(current) == LISTEN_PORT:
            raise ValueError('Apache already points to Glance Go and no original backup exists')
        private(backup, current)
    return backup.read_text()


def use_go(restart=False):
    site = original_site()
    command(['sudo', 'systemctl', 'enable', '--now', UNIT], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    if restart:
        command(['sudo', 'systemctl', 'restart', UNIT])
    wait_up('http://127.0.0.1:' + str(LISTEN_PORT))
    switch_site(go_site(site, LISTEN_PORT))
    command(['sudo', 'systemctl', 'disable', '--now', ORIGINAL_UNIT], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    wait_up(PUBLIC_URL)
    return status()


def use_original():
    site = original_site()
    command(['sudo', 'systemctl', 'enable', '--now', ORIGINAL_UNIT], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    switch_site(site)
    wait_up(PUBLIC_URL)
    command(['sudo', 'systemctl', 'disable', '--now', UNIT], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
    return status()


def status():
    def state(unit):
        return subprocess.run(['systemctl', 'is-active', unit], capture_output=True, text=True).stdout.strip()
    port = backend_port(sudo_read(SITE_FILE))
    return {'image_served_by': 'glance-go' if port == LISTEN_PORT else 'original' if port else 'unknown', 'backend_port': port,
            UNIT: state(UNIT), ORIGINAL_UNIT: state(ORIGINAL_UNIT)}


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument('action', choices=['install', 'use-go', 'use-original', 'status'])
    parser.add_argument('--binary', type=Path, help='Glance Go binary (install)')
    parser.add_argument('--allow-mutations', action='store_true')
    args = parser.parse_args()
    if args.action != 'status' and not args.allow_mutations:
        raise SystemExit('explicit --allow-mutations required')
    if args.action == 'install':
        return install(args)
    print(json.dumps(use_go() if args.action == 'use-go' else use_original() if args.action == 'use-original' else status(), indent=1))
    return 0


if __name__ == '__main__':
    sys.exit(main())

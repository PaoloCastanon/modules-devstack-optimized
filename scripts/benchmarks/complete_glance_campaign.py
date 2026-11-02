#!/usr/bin/env python3
"""Durable Glance pilot36 -> frozen official108 -> restoration -> publication.

Joint campaign is explicitly postponed. Never launches it or merges a PR.
"""
import argparse
import datetime
import fcntl
import hashlib
import json
import os
from pathlib import Path
import shutil
import shlex
import subprocess
import sys
import time

from analyze_glance import analyze, publish
from campaign_protocol import storage_check
from complete_placement_campaign import compact, safely_shutdown
from glance_campaign import save, validate_protocol


def cmd(values, **kwargs):
    return subprocess.run(values, check=True, **kwargs)


def freeze(config):
    head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
    if Path.cwd().resolve() != Path(config['worktree']).resolve() or head != config['frozen_head']:
        raise ValueError('designated worktree and frozen head required')
    tracked = subprocess.check_output(['git', 'status', '--porcelain', '--untracked-files=no'], text=True)
    if tracked.strip():
        raise ValueError('frozen measurement sources must be clean')
    protocol = Path(config['protocol'])
    if hashlib.sha256(protocol.read_bytes()).hexdigest() != config['protocol_sha256']:
        raise ValueError('protocol bytes changed after freezing')
    validate_protocol(json.loads(protocol.read_text()))


def start_vm(config):
    storage_check(Path('/var/lib/libvirt/images'), 20 * 1024**3)
    locale = {**os.environ, 'LC_ALL': 'C', 'LANG': 'C'}
    state = subprocess.check_output(['virsh', '-c', 'qemu:///system', 'domstate', 'tt-openstack-lab'], text=True, env=locale).strip()
    if state != 'shut off':
        raise ValueError('idle shut-off VM required before session; inspect existing owner')
    cmd(['virsh', '-c', 'qemu:///system', 'start', 'tt-openstack-lab'], stdout=subprocess.DEVNULL, env=locale)
    readiness = ("import subprocess,sys; active=lambda u:subprocess.run(['systemctl','is-active','--quiet',u]).returncode==0; "
                 "sys.exit(0 if all(active(u) for u in ['devstack@g-api.service','devstack@placement-api.service',"
                 "'devstack@n-cpu.service','devstack@n-sch.service','devstack@keystone.service']) "
                 "and not any(active(u) for u in ['apt-daily.service','apt-daily-upgrade.service']) else 1)")
    for _ in range(120):
        probe = subprocess.run([*config['ssh'], 'python3', '-c', shlex.quote(readiness)], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, timeout=10)
        if probe.returncode == 0:
            time.sleep(5)  # App initialization after systemd enters active.
            return
        time.sleep(2)
    raise ValueError('VM SSH readiness failed; inspect before repeating')


def deploy(config):
    archive = Path(config['private_root']) / 'public-code.tar'
    archive.parent.mkdir(mode=0o700, exist_ok=False)
    with archive.open('xb') as stream:
        cmd(['git', 'archive', config['frozen_head'], 'scripts'], stdout=stream)
    ssh = config['ssh']
    options = ssh[1:-1]
    remote_code = config['remote_code']
    # Only fresh public source directory; credentials and app state are elsewhere.
    import shlex
    cmd([*ssh, 'mkdir', '-m', '700', shlex.quote(remote_code)])
    cmd(['scp', *options, str(archive), ssh[-1] + ':' + remote_code + '/code.tar'], stdout=subprocess.DEVNULL)
    cmd(['scp', *options, config['binary'], ssh[-1] + ':' + remote_code + '/glance'], stdout=subprocess.DEVNULL)
    command = 'set -e; cd ' + shlex.quote(remote_code) + '; tar -xf code.tar --strip-components=1; rm code.tar; chmod 700 glance'
    cmd([*ssh, 'bash', '-c', shlex.quote(command)])
    actual = subprocess.check_output([*ssh, 'sha256sum', shlex.quote(remote_code + '/glance')], text=True).split()[0]
    if actual != config['binary_sha256']:
        raise ValueError('deployed binary differs from frozen binary')
    shutil.rmtree(archive.parent)


def session(config, name, status):
    freeze(config)
    root = Path(config['session_root']) / name
    output = root / 'output'
    root.mkdir(parents=True, exist_ok=False)
    stage = name == 'pilot'
    status.update(stage=name + '_running', completed_runs=0, required_runs=36 if stage else 108, accepted=False)
    save(Path(config['status_file']), status)
    start_vm(config)
    status.update(vm='running', last_run=None)
    save(Path(config['status_file']), status)
    if stage:
        deploy(config)
    suffix = config['run_id'] + '-' + name
    arguments = ['--output', str(output), '--private-root', str(root / 'private'), '--protocol', config['protocol'],
                 '--remote-code', config['remote_code'], '--remote-work', '/home/stack/tt-placement-glance-' + suffix,
                 '--remote-output', '/home/stack/glance-result-' + suffix, '--host', config['ssh'][-1],
                 '--ssh-key', config['ssh_key'], '--known-hosts', config['known_hosts'], '--allow-mutations']
    if stage:
        arguments.append('--pilot')
    with (root / 'campaign.log').open('x') as log:
        child = subprocess.Popen([sys.executable, 'scripts/benchmarks/glance_campaign.py', *arguments], stdout=log, stderr=subprocess.STDOUT)
        while child.poll() is None:
            try:
                m = json.loads((output / 'manifest.json').read_text())
                status.update(completed_runs=len(m['runs']), last_run=m['runs'][-1] if m['runs'] else None,
                              updated_utc=datetime.datetime.now(datetime.timezone.utc).isoformat())
                save(Path(config['status_file']), status)
            except (FileNotFoundError, json.JSONDecodeError):
                pass
            time.sleep(10)
    manifest = json.loads((output / 'manifest.json').read_text())
    status.update(stage=name + '_verifying', completed_runs=len(manifest['runs']), campaign_exit=child.returncode)
    save(Path(config['status_file']), status)
    # Stop operational VM before host analysis/build/CI. Even invalid measurements
    # may shut down when (and only when) restoration is independently proved.
    safely_shutdown(manifest, root / 'vm-final-state.json')
    status['vm'] = 'shut off'
    summary = analyze(output)
    save(root / 'summary.json', summary)
    evidence = Path(config['evidence']) / name
    evidence.mkdir(parents=True, exist_ok=False)
    shutil.copytree(output, evidence / 'observations')
    for file in ['campaign.log', 'summary.json', 'vm-final-state.json']:
        shutil.copy2(root / file, evidence / file)
    compact(evidence)
    if child.returncode or not summary['complete_valid']:
        raise ValueError('Glance ' + name + ' invalid; preserve evidence and repair before another attempt')
    status[name] = {'valid': True, 'windows': len(manifest['runs']), 'cleanup': True, 'vm': 'shut off',
                    'summary': str(root / 'summary.json')}
    save(Path(config['status_file']), status)
    return summary


def finish(config, status, summary):
    freeze(config)
    status.update(stage='publishing', accepted=False)
    save(Path(config['status_file']), status)
    evidence = Path(config['evidence'])
    publish(summary, Path('docs/benchmarks/glance-results.md'))
    with Path('harness/build-log.md').open('a') as log:
        log.write('\n\n### ' + summary['protocol']['protocol'] + ': piloto y campaña oficial completados\n\n'
                  '- Piloto36/36 y oficial108/108, bytes/estado/CPU/PSS válidos; Placement Python fijo.\n'
                  '- Restauración, limpieza, controles y apagado normal verificados tras ambas sesiones.\n'
                  '- Datos independientes, n=3 mediana/rango; no atribución exclusiva al lenguaje.\n'
                  '- Resultado reproducible en docs/benchmarks/glance-results.md y evidencia de esta campaña.\n'
                  '- Campaña conjunta postergada por el usuario; no se ejecutó.\n')
    with (evidence / 'make-check.log').open('x') as log:
        cmd(['make', 'check'], stdout=log, stderr=subprocess.STDOUT)
    compact(evidence)
    cmd(['git', 'add', '--', str(evidence), 'docs/benchmarks/glance-results.md', 'harness/build-log.md'])
    cmd(['git', 'diff', '--cached', '--check'])
    cmd(['git', 'commit', '-m', 'test(benchmarks): complete Glance compact pilot and official evaluation'])
    head = subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip()
    cmd(['git', 'push', 'origin', 'HEAD:feat/glance-compact-v2'])
    status.update(stage='ci_pending', head=head, measurement_head=config['frozen_head'])
    save(Path(config['status_file']), status)
    for _ in range(60):
        result = subprocess.run(['gh', 'pr', 'checks', '18', '--json', 'name,bucket,link'], capture_output=True, text=True)
        try:
            checks = json.loads(result.stdout)
        except ValueError:
            checks = []
        # Verify exact head separately; an old successful check is insufficient.
        pr = json.loads(subprocess.check_output(['gh', 'pr', 'view', '18', '--json', 'headRefOid'], text=True))
        runs = json.loads(subprocess.check_output(['gh', 'api', 'repos/PaoloCastanon/modules-devstack-optimized/actions/runs?head_sha=' + head + '&event=pull_request'], text=True))['workflow_runs']
        exact_pass = any(run['head_sha'] == head and run['status'] == 'completed' and run['conclusion'] == 'success' for run in runs)
        if pr['headRefOid'] == head and exact_pass and checks and all(c['bucket'] == 'pass' for c in checks):
            cmd(['gh', 'pr', 'ready', '18'])
            status.update(stage='complete', accepted=True, ci='PASS', vm='shut off', completed_runs=108,
                          joint='postponed_by_user', finished_utc=datetime.datetime.now(datetime.timezone.utc).isoformat())
            save(Path(config['status_file']), status)
            return
        if any(c['bucket'] in ('fail', 'cancel') for c in checks):
            raise ValueError('remote CI failed; PR stays draft, no final acceptance')
        time.sleep(20)
    raise ValueError('CI still pending; do not repeat accepted measurement, resume publication only')


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--config', type=Path, required=True)
    parser.add_argument('--allow-mutations', action='store_true')
    parser.add_argument('--allow-publish', action='store_true')
    parser.add_argument('--allow-vm-shutdown', action='store_true')
    args = parser.parse_args()
    if not (args.allow_mutations and args.allow_publish and args.allow_vm_shutdown):
        raise ValueError('explicit campaign/publication/shutdown opt-ins required')
    config = json.loads(args.config.read_text())
    # Host lifetime lock supplements the VM session lock; never duplicate jobs.
    with Path('/tmp/tt-glance-compact-supervisor.lock').open('a') as lock:
        fcntl.flock(lock, fcntl.LOCK_EX | fcntl.LOCK_NB)
        status = {'schema': 1, 'stage': 'starting', 'accepted': False, 'pr': config['pr_url'],
                  'measurement_head': config['frozen_head'], 'joint': 'postponed_by_user'}
        if Path(config['status_file']).exists():
            raise ValueError('existing campaign status; inspect/resume instead of duplicating')
        save(Path(config['status_file']), status)
        try:
            freeze(config)
            if hashlib.sha256(Path(config['binary']).read_bytes()).hexdigest() != config['binary_sha256']:
                raise ValueError('frozen binary changed')
            session(config, 'pilot', status)
            official = session(config, 'official', status)
            finish(config, status, official)
            return 0
        except Exception as error:
            status.update(stage='needs_attention', error=type(error).__name__,
                          reason=str(error) if isinstance(error, ValueError) else 'see private/service execution log')
            save(Path(config['status_file']), status)
            return 1


if __name__ == '__main__':
    sys.exit(main())

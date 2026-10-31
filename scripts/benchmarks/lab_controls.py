#!/usr/bin/env python3
"""VM-only, temporary maintenance controls; never runs on the load host."""
import argparse
import datetime
import gzip
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import time

UNITS = ['apt-daily.timer', 'apt-daily-upgrade.timer', 'apt-daily.service', 'apt-daily-upgrade.service']
LOGS = [('/var/log/syslog', 128 * 1024**2), ('/var/log/apache2/horizon_access.log', 64 * 1024**2)]
JOURNAL_DIR = Path('/run/systemd/journald.conf.d')
TAIL_DIR = Path('/var/log/tt-placement-benchmark')


def systemctl(*args):
    return subprocess.run(['systemctl', *args], capture_output=True, text=True, check=True)


def unit_state(unit):
    result = systemctl('show', unit, '--property=ActiveState,UnitFileState,LoadState', '--no-pager')
    return dict(line.split('=', 1) for line in result.stdout.splitlines() if '=' in line)


def save(path, value):
    temporary = path.with_suffix('.next')
    temporary.write_text(json.dumps(value, indent=2) + '\n')
    temporary.chmod(0o600)
    temporary.replace(path)


def restore(path):
    if not path.exists():
        return {'controls_restored': True, 'not_installed': True}
    state = json.loads(path.read_text())
    for unit in state['masked_by_session']:
        systemctl('unmask', '--runtime', unit)
    config = Path(state['journal_config'])
    if config.exists():
        config.unlink()
        systemctl('restart', 'systemd-journald.service')
    for unit, original in state['original_units'].items():
        if unit.endswith('.timer') and original['ActiveState'] == 'active':
            systemctl('start', unit)
    actual = {u: unit_state(u) for u in UNITS}
    for unit, original in state['original_units'].items():
        if actual[unit]['UnitFileState'] != original['UnitFileState']:
            raise ValueError('maintenance unit enablement not restored')
        if unit.endswith('.timer') and actual[unit]['ActiveState'] != original['ActiveState']:
            raise ValueError('maintenance timer activity not restored')
    state.update(controls_restored=True, effective_after_restore=actual)
    save(path, state)
    return state


def enter(path):
    if path.exists():
        raise ValueError('refuse to replace maintenance ledger')
    original = {u: unit_state(u) for u in UNITS}
    if any(original[u]['ActiveState'] not in ('inactive', 'failed') for u in UNITS if u.endswith('.service')):
        raise ValueError('package maintenance already running; wait, never interrupt dpkg')
    label = hashlib.sha256(str(path).encode()).hexdigest()[:12]
    config = JOURNAL_DIR / ('tt-placement-benchmark-' + label + '.conf')
    if config.exists():
        raise ValueError('existing journal control belongs to another session')
    state = {'original_units': original, 'masked_by_session': [], 'journal_config': str(config),
             'controls_restored': False, 'runtime_only': True,
             'log_limits_bytes': dict(LOGS), 'journal_system_max_bytes': 256 * 1024**2,
             'retention': 'operational logs only; request samples retained separately',
             'installed_utc': datetime.datetime.now(datetime.timezone.utc).isoformat()}
    save(path, state)
    try:
        for unit in UNITS:
            if original[unit].get('UnitFileState') not in ('masked', 'masked-runtime'):
                systemctl('mask', '--runtime', unit)
                state['masked_by_session'].append(unit)
                save(path, state)
        for unit in UNITS:
            if unit.endswith('.timer'):
                systemctl('stop', unit)
        if any(unit_state(u)['ActiveState'] not in ('inactive', 'failed') for u in UNITS if u.endswith('.service')):
            raise ValueError('package maintenance raced preflight')
        JOURNAL_DIR.mkdir(exist_ok=True)
        config.write_text('[Journal]\nSystemMaxUse=256M\nRuntimeMaxUse=128M\n')
        config.chmod(0o644)
        systemctl('restart', 'systemd-journald.service')
        state['effective_units'] = {u: unit_state(u) for u in UNITS}
        save(path, state)
        return state
    except BaseException:
        restore(path)
        raise


def bound_logs():
    """Run between windows, retain recent private diagnostics, cap old tails."""
    if TAIL_DIR.is_symlink():
        raise ValueError('invalid log tail directory')
    TAIL_DIR.mkdir(mode=0o700, exist_ok=True)
    result = []
    for name, limit in LOGS:
        path = Path(name)
        if not path.exists():
            continue
        if path.is_symlink() or not path.is_file():
            raise ValueError('unexpected operational log file')
        size = path.stat().st_size
        if size <= limit:
            continue
        archive = TAIL_DIR / (path.name + '-' + str(time.time_ns()) + '.gz')
        with path.open('rb') as source, gzip.open(archive, 'wb', compresslevel=1) as target:
            source.seek(max(0, size - 4 * 1024**2))
            while data := source.read(1024**2):
                target.write(data)
        archive.chmod(0o600)
        with path.open('r+b', buffering=0) as stream:
            stream.truncate(0)
        for old in sorted(TAIL_DIR.glob(path.name + '-*.gz'))[:-4]:
            old.unlink()
        result.append({'path': name, 'bytes_before': size, 'retained_tail_bytes': min(size, 4 * 1024**2)})
    return {'between_windows': True, 'rotated': result}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['enter', 'bound', 'restore'])
    parser.add_argument('--state', type=Path, required=True)
    args = parser.parse_args()
    if os.geteuid() != 0:
        raise ValueError('VM root required for maintenance controls')
    path = args.state
    if (path.name != 'maintenance-controls.json' or path.parent != path.parent.resolve()
            or not re.fullmatch(r'tt-placement-[a-z0-9-]+', path.parent.name) or path.is_symlink()):
        raise ValueError('canonical owned lab state required')
    value = enter(path) if args.action == 'enter' else restore(path) if args.action == 'restore' else bound_logs()
    print(json.dumps(value))


if __name__ == '__main__':
    main()

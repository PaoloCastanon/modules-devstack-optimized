#!/usr/bin/env python3
"""Sample CPU, memory and PSS of named cgroups at a fixed interval until SIGTERM.

Runs as root (PSS needs every process's smaps_rollup). Reads only cgroup
counters and /proc/<pid>/stat|smaps_rollup, never arguments or environment.
A group whose directory is missing (a stopped unit) is recorded as null, so
one sampler can span route switches. Output: one JSON line per sample with a
wall-clock timestamp (microseconds), so a runner can slice it by window.
"""
import argparse
import json
from pathlib import Path
import signal
import sys
import time

CGROUP_ROOT = Path('/sys/fs/cgroup')


def usage_usec(text):
    for line in text.splitlines():
        key, _, value = line.partition(' ')
        if key == 'usage_usec':
            return int(value)
    raise ValueError('usage_usec missing')


def pss_kib(text):
    for line in text.splitlines():
        if line.startswith('Pss:'):
            return int(line.split()[1])
    return 0


def group_pids(path):
    found = set()
    for procs in sorted(path.glob('**/cgroup.procs'))[:256]:
        try:
            found.update(int(pid) for pid in procs.read_text().split())
        except OSError:
            continue
    return sorted(found)


def sample_group(path, proc=Path('/proc')):
    if not path.is_dir():
        return None
    try:
        cpu = usage_usec((path / 'cpu.stat').read_text())
        memory = int((path / 'memory.current').read_text())
    except (OSError, ValueError):
        return None
    pss, pids = 0, group_pids(path)
    for pid in pids:
        try:
            pss += pss_kib((proc / str(pid) / 'smaps_rollup').read_text())
        except OSError:  # exited between listing and reading
            continue
    return {'cpu_usec': cpu, 'memory_bytes': memory, 'pss_kib': pss, 'pids': len(pids)}


def vm_cpu(stat=Path('/proc/stat')):
    values = [int(v) for v in stat.read_text().splitlines()[0].split()[1:]]
    total = sum(values[:8])
    return {'busy_ticks': total - values[3] - values[4], 'total_ticks': total}


def parse_groups(items):
    groups = {}
    for item in items:
        name, _, group = item.partition('=')
        if not name or not group.startswith('/') or '..' in group or name in groups:
            raise ValueError('groups are distinct name=/cgroup/path')
        groups[name] = CGROUP_ROOT / group.lstrip('/')
    return groups


def run(args):
    groups = parse_groups(args.group)
    stop = []
    signal.signal(signal.SIGTERM, lambda *_: stop.append(True))
    signal.signal(signal.SIGINT, lambda *_: stop.append(True))
    with args.output.open('x') as out:
        while not stop:
            began = time.monotonic()
            record = {'ts_us': time.time_ns() // 1000, 'vm': vm_cpu(),
                      'groups': {name: sample_group(path) for name, path in groups.items()}}
            record['read_ms'] = round((time.monotonic() - began) * 1000, 1)
            out.write(json.dumps(record, separators=(',', ':')) + '\n')
            out.flush()
            time.sleep(max(0.0, args.interval - (time.monotonic() - began)))
    return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--group', action='append', required=True, help='name=/cgroup/path (repeatable)')
    parser.add_argument('--interval', type=float, default=1.0)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if not 0.2 <= args.interval <= 10:
        parser.error('interval between 0.2 and 10 s')
    return run(args)


if __name__ == '__main__':
    sys.exit(main())

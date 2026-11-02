#!/usr/bin/env python3
"""Sample explicit Linux service PIDs without collecting arguments/environment."""
import argparse
import datetime
import json
import math
import os
from pathlib import Path
import sys
import time


def snapshot(pid, root=Path('/proc')):
    data = (root / str(pid) / 'stat').read_text().rsplit(')', 1)[1].split()
    result = {'pid': pid, 'start_ticks': int(data[19]),
              'cpu_ticks': int(data[11]) + int(data[12]),
              'rss_bytes': int(data[21]) * os.sysconf('SC_PAGE_SIZE')}
    try:
        lines = (root / str(pid) / 'smaps_rollup').read_text().splitlines()
        pss = next(line for line in lines if line.startswith('Pss:'))
        result['pss_bytes'] = int(pss.split()[1]) * 1024
    except (FileNotFoundError, PermissionError, StopIteration):
        result['pss_bytes'] = None
    return result


def run(args):
    # Validate identities before creating evidence or waiting.
    initial = {pid: snapshot(pid) for pid in args.pids}
    if not math.isfinite(args.duration) or not math.isfinite(args.interval) or args.duration <= 0 or args.interval <= 0 or args.interval > args.duration:
        raise ValueError('positive sampling window required')
    args.output.mkdir(parents=True, exist_ok=False)
    records, valid = [], True
    timestamp = datetime.datetime.now(datetime.timezone.utc).isoformat()
    begin = time.monotonic_ns()
    with (args.output / 'resources.jsonl').open('x') as raw:
        while True:
            now = time.monotonic_ns()
            processes, error_kind = [], None
            for pid in args.pids:
                try:
                    item = snapshot(pid)
                    if item['start_ticks'] != initial[pid]['start_ticks']:
                        raise ValueError('PID reused')
                    processes.append(item)
                except (OSError, ValueError):
                    error_kind = 'service_pid_missing_or_reused'
                    valid = False
            record = {'offset_ns': now - begin, 'processes': processes, 'error_kind': error_kind}
            records.append(record)
            raw.write(json.dumps(record, separators=(',', ':')) + '\n')
            if not valid or (time.monotonic_ns() - begin) / 1e9 >= args.duration:
                break
            time.sleep(min(args.interval, max(0, args.duration - (time.monotonic_ns() - begin) / 1e9)))
    report = {'schema': 1, 'timestamp_utc': timestamp, 'run_id': args.run_id,
              'pids': args.pids, 'clock_ticks_s': os.sysconf('SC_CLK_TCK'),
              'page_bytes': os.sysconf('SC_PAGE_SIZE'), 'samples': len(records),
              'elapsed_s': (time.monotonic_ns() - begin) / 1e9,
              'valid': valid, 'scope': 'explicit PIDs; caller must enumerate all service workers',
              'pss_available': all(p['pss_bytes'] is not None for r in records for p in r['processes'])}
    if valid:
        final = {p['pid']: p for p in records[-1]['processes']}
        report['cpu_seconds'] = sum(final[pid]['cpu_ticks'] - initial[pid]['cpu_ticks'] for pid in args.pids) / os.sysconf('SC_CLK_TCK')
    (args.output / 'report.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps(report))
    return 0 if valid else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--pids', type=int, nargs='+', required=True)
    parser.add_argument('--run-id', required=True)
    parser.add_argument('--duration', type=float, default=120)
    parser.add_argument('--interval', type=float, default=1)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if len(set(args.pids)) != len(args.pids) or any(pid < 1 for pid in args.pids):
        parser.error('distinct positive PIDs required')
    try:
        return run(args)
    except Exception as error:
        print('sampling failed: ' + type(error).__name__, file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())

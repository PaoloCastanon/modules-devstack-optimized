#!/usr/bin/env python3
"""Host Glance compact-v2 coordinator. One frozen matrix, exclusive remote RPC."""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import random
import resource
import shutil
import subprocess
import sys
import threading
import uuid

from campaign_protocol import StorageWatch, storage_check
from glance_http import payload
import glance_window
from placement_campaign import Remote, finalize_remote

SCENARIOS = ['catalog-list', 'catalog-show', 'catalog-lifecycle', 'upload-1m', 'upload-16m', 'upload-64m',
             'download-1m', 'download-16m', 'download-64m']


def save(path, value):
    temp = path.with_suffix('.next')
    temp.write_text(json.dumps(value, indent=2) + '\n')
    temp.replace(path)


def validate_protocol(p):
    if (p['protocol'] not in ('glance-compact-v2', 'glance-compact-v3') or [s['id'] for s in p['scenarios']] != SCENARIOS
            or p['concurrency'] != [1, 16] or p['repetitions'] != 3 or p['warmup_s'] != 10 or p['measurement_s'] != 30
            or p['required_official_windows'] != 108 or p['catalog_seed_count'] != 100
            or p['pilot'] != {'concurrency': [1, 16], 'repetitions': 1, 'warmup_s': 1, 'measurement_s': 4, 'required_windows': 36}
            or p['payload_sizes_bytes'] != [1048576, 16777216, 67108864]):
        raise ValueError('unsupported or incomplete frozen compact Glance matrix')
    if p['protocol'] == 'glance-compact-v3' and p.get('notification_policy') != 'noop':
        raise ValueError('v3 requires explicit API/bytes scope with noop notifications')
    for item in p['scenarios']:
        if item['id'].startswith(('upload-', 'download-')):
            size = {'1m': 1048576, '16m': 16777216, '64m': 67108864}[item['id'].split('-')[1]]
            if item['size_bytes'] != size:
                raise ValueError('scenario payload size disagreement')
    return 108


def schedule(protocol, campaign_id, pilot=False):
    validate_protocol(protocol)
    result, rng = [], random.Random(protocol['order_seed'])
    for scenario in protocol['scenarios']:
        for concurrency in protocol['concurrency']:
            for repetition in range(1, (1 if pilot else protocol['repetitions']) + 1):
                name = f"{scenario['id']}-c{concurrency:02}-r{repetition:02}"
                order = ['python', 'go']; rng.shuffle(order)
                result.append({'pair_id': name, 'scenario': scenario['id'], 'size_bytes': scenario.get('size_bytes'),
                               'concurrency': concurrency, 'repetition': repetition, 'order': order,
                               'namespace': str(uuid.uuid5(uuid.UUID(campaign_id), name))})
    return result


class GuestWatch:
    """Independent SSH stat probe; no disk/SSH calls in request timing paths."""
    def __init__(self, remote):
        self.remote = remote
        self.failed, self.stop = threading.Event(), threading.Event()
        self.samples = []
        self.thread = threading.Thread(target=self.loop, daemon=True)

    def probe(self):
        root = str(self.remote.args.remote_work / 'tt-placement-runtime/store')
        code = ("import json,os,pathlib; p=pathlib.Path(" + repr(root) + "); "
                "v=os.statvfs('/'); print(json.dumps({'free_bytes':v.f_bavail*v.f_frsize,"
                "'store_bytes':sum(x.stat().st_size for x in p.rglob('*') if x.is_file())}))")
        result = subprocess.run([*self.remote.ssh, 'python3', '-c', __import__('shlex').quote(code)],
                                capture_output=True, text=True, timeout=20, check=True)
        sample = json.loads(result.stdout)
        sample['utc'] = glance_window.utc()
        self.samples.append(sample)
        if sample['free_bytes'] < 8 * 1024**3 or sample['store_bytes'] > 4 * 1024**3:
            self.failed.set()

    def loop(self):
        while not self.stop.wait(3):
            try:
                self.probe()
            except (OSError, ValueError, subprocess.SubprocessError):
                self.failed.set()
                return

    def __enter__(self):
        self.probe(); self.thread.start()
        return self

    def __exit__(self, *_):
        self.stop.set(); self.thread.join(timeout=25)


def run(args):
    if not args.allow_mutations:
        raise ValueError('explicit mutation opt-in required')
    args.domain = 'glance'
    for path in [args.protocol, args.known_hosts, args.ssh_key]:
        if not path.is_file():
            raise ValueError('readable protocol/SSH trust/key required')
    p = json.loads(args.protocol.read_text()); validate_protocol(p)
    args.notification_policy = p.get('notification_policy', 'original')
    storage_check(args.storage_path, p['host_free_minimum_bytes'])
    args.output.mkdir(parents=True, exist_ok=False)
    args.private_root.mkdir(mode=0o700, exist_ok=False)
    ident = str(uuid.uuid4()); plan = schedule(p, ident, args.pilot)
    manifest = {'schema': 1, 'campaign_id': ident, 'stage': 'pilot' if args.pilot else 'measurement',
                'started_utc': glance_window.utc(), 'protocol': p, 'protocol_sha256': hashlib.sha256(args.protocol.read_bytes()).hexdigest(),
                'git_base': subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip(),
                'source_sha256': {str(path): hashlib.sha256(path.read_bytes()).hexdigest() for path in sorted(Path('scripts').rglob('*.py'))},
                'sequence': plan, 'runs': [], 'valid': False, 'cleanup': False}
    save(args.output / 'manifest.json', manifest)
    payloads = {size: (args.private_root / f'payload-{size}.bin') for size in p['payload_sizes_bytes']}
    hashes = {size: payload(path, size) for size, path in payloads.items()}
    remote = Remote(args)
    try:
        save(args.output / 'environment.json', remote.call('prepare'))
        for i, pair in enumerate(plan):
            auth = remote.call('token'); token = auth.pop('token')
            save(args.output / (pair['pair_id'] + '-auth.json'), auth['metadata'])
            baseline = None
            for implementation in pair['order']:
                run_id = pair['pair_id'] + '-' + implementation
                print(json.dumps({'event': 'preparing', 'pair': i + 1, 'pairs': len(plan), 'run_id': run_id}), flush=True)
                config = remote.call('seed', values={**pair, 'implementation': implementation, 'run_id': run_id})
                equivalent = {'images': config['seed_snapshot'], 'payload': config.get('payload')}
                if baseline is not None and equivalent != baseline:
                    save(args.output / (run_id + '-inequivalent.json'), equivalent)
                    raise ValueError('paired metadata/hash seeds differ')
                baseline = equivalent
                if pair['size_bytes']:
                    size = pair['size_bytes']
                    if config['payload'] != hashes[size]:
                        raise ValueError('host/VM payload hashes differ')
                    config['payload_file'] = str(payloads[size])
                save(args.output / (run_id + '-seed.json'), config)
                clocks = {'before': remote.clock()}
                remote.call('start-collector', run_id=run_id)
                usage, load = os.times(), os.getloadavg()
                with StorageWatch(args.storage_path, 10 * 1024**3) as storage, GuestWatch(remote) as guest:
                    report = glance_window.run(config, token, args.output / run_id,
                                               duration=p['pilot']['measurement_s'] if args.pilot else p['measurement_s'],
                                               warmup=p['pilot']['warmup_s'] if args.pilot else p['warmup_s'],
                                               abort_check=lambda: storage.failed.is_set() or guest.failed.is_set())
                report['host_storage'] = storage.report()
                report['guest_storage'] = {'samples': guest.samples, 'aborted': guest.failed.is_set()}
                end = os.times()
                report['generator_cpu_seconds'] = end.user + end.system - usage.user - usage.system
                report['generator_max_rss_bytes_process_lifetime'] = resource.getrusage(resource.RUSAGE_SELF).ru_maxrss * 1024
                report['host_load_average'] = {'before': load, 'after': os.getloadavg()}
                remote.call('stop-collector')
                clocks['after'] = remote.clock()
                report['clocks'] = clocks
                report['clock_drift_ns'] = abs(clocks['before']['best']['remote_minus_host_ns'] - clocks['after']['best']['remote_minus_host_ns'])
                report['clock_valid'] = report['clock_drift_ns'] < 100_000_000
                report['post_state'] = remote.call('finish')
                report['valid'] = report['valid'] and report['clock_valid'] and report['post_state']['removed'] and not guest.failed.is_set()
                save(args.output / run_id / 'report.json', report)
                manifest['runs'].append({'run_id': run_id, 'valid': report['valid']})
                save(args.output / 'manifest.json', manifest)
                print(json.dumps({'event': 'completed', 'run_id': run_id, 'valid': report['valid'], 'measurement': report['measurement']}), flush=True)
                if not report['valid']:
                    raise ValueError('invalid Glance window; preserve samples, no acceptance')
            remote.call('route', implementation='python')
        manifest['valid'] = len(manifest['runs']) == len(plan) * 2 and all(r['valid'] for r in manifest['runs'])
    except Exception as error:
        manifest['error'] = type(error).__name__
        manifest['reason'] = str(error) if isinstance(error, (ValueError, RuntimeError)) else 'private diagnostic retained'
    finally:
        try:
            manifest['cleanup'] = finalize_remote(remote, args.output, manifest)
        except Exception as error:
            manifest['cleanup_error'] = type(error).__name__
        manifest['valid'] = manifest['valid'] and manifest['cleanup']
        manifest['finished_utc'] = glance_window.utc()
        save(args.output / 'manifest.json', manifest)
        if manifest['cleanup']:
            shutil.rmtree(args.private_root)
    print(json.dumps({'event': 'campaign_finished', 'valid': manifest['valid'], 'cleanup': manifest['cleanup']}), flush=True)
    return 0 if manifest['valid'] else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ['output', 'private-root', 'protocol', 'remote-code', 'remote-work', 'remote-output', 'ssh-key', 'known-hosts']:
        parser.add_argument('--' + name, type=Path, required=True)
    parser.add_argument('--host', default='stack@192.168.122.10')
    parser.add_argument('--storage-path', type=Path, default=Path('/var/lib/libvirt/images'))
    parser.add_argument('--pilot', action='store_true')
    parser.add_argument('--allow-mutations', action='store_true')
    args = parser.parse_args()
    try:
        return run(args)
    except Exception as error:
        print('Glance campaign failed: ' + type(error).__name__, file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())

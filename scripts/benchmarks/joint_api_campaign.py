#!/usr/bin/env python3
"""Joint Placement x Glance API campaign, host coordinator (protocol joint-api-v1).

Every window runs the Placement generator (placement_window) and the Glance
generator (glance_window) at the same time against the public Apache routes,
in one of the four variants (Placement Python/Go x Glance Python/Go). Both
generators are the reviewed instruments of the individual campaigns, with
the same warm-up, measurement time and concurrency, so each service's result
is comparable with its own campaign: Placement with Glance load and Glance
with Placement load. The VM side (joint_lab.py) seeds both, switches routes
and samples every cgroup at 1 Hz. The first invalid window stops the
campaign; the lab cleanup always runs.

Usage: docs/benchmarks/joint-api-campaign.md.
"""
import argparse
import datetime
import gzip
import hashlib
import json
import os
from pathlib import Path
import random
import resource
import shlex
import shutil
import subprocess
import sys
import threading
import uuid

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
import glance_window  # noqa: E402
from glance_http import payload  # noqa: E402
import placement_window  # noqa: E402
from campaign_protocol import StorageWatch, storage_check  # noqa: E402
from placement_campaign import Remote as PlacementRemote  # noqa: E402

VARIANTS = ('python-python', 'go-python', 'python-go', 'go-go')
PLACEMENT_SCENARIOS = {'provider', 'inventories', 'allocations-read', 'candidates', 'claim-cycle', 'nova-mix'}
GLANCE_SCENARIOS = {'catalog-list', 'catalog-show', 'download-1m', 'download-16m', 'download-64m', 'nova-mix'}


def save(path, value):
    path.write_text(json.dumps(value, indent=2) + '\n')


def validate_protocol(p):
    pairs = p.get('pairs') or []
    if (p.get('schema') != 1 or [c['id'] for c in p.get('configurations', [])] != list(VARIANTS)
            or not pairs or len({x['id'] for x in pairs}) != len(pairs)
            or any(x['placement'] not in PLACEMENT_SCENARIOS or x['glance'] not in GLANCE_SCENARIOS for x in pairs)
            or any(c not in (1, 4, 8, 16) for c in p['concurrency']) or p['repetitions'] < 1
            or not 0 < p['minimum_overlap_fraction'] <= 1):
        raise ValueError('unsupported joint API protocol')
    windows = len(pairs) * len(p['concurrency']) * p['repetitions'] * len(VARIANTS)
    if windows != p['required_official_windows']:
        raise ValueError('protocol window accounting mismatch')
    return windows


def schedule(protocol, campaign_id, pilot=False):
    """Per (pair, concurrency, repetition) a block with the four variants in a
    seeded random order. Placement providers/consumers/class and the Glance
    namespace are derived per window, so no ID is reused (the original Glance
    soft-deletes and keeps used IDs reserved)."""
    reps = protocol['pilot']['repetitions'] if pilot else protocol['repetitions']
    concurrency = protocol['pilot']['concurrency'] if pilot else protocol['concurrency']
    rng = random.Random('%s/%s' % (protocol['order_seed'], 'pilot' if pilot else 'official'))
    plan = []
    for pair in protocol['pairs']:
        for c in concurrency:
            for r in range(1, reps + 1):
                block = f"{pair['id']}-c{c:02d}-r{r:02d}"
                order = list(VARIANTS)
                rng.shuffle(order)
                for variant in order:
                    ns = uuid.uuid5(uuid.UUID(campaign_id), block + '/' + variant)
                    pl, gl = variant.split('-')
                    plan.append({'run_id': block + '-' + variant, 'block': block, 'pair': pair['id'], 'concurrency': c,
                                 'repetition': r, 'variant': variant,
                                 'placement': {'run_id': block + '-' + variant + '-p', 'implementation': pl,
                                               'scenario': pair['placement'], 'concurrency': c,
                                               'providers': [str(uuid.uuid5(ns, 'provider-%d' % i)) for i in range(16)],
                                               'consumers': [str(uuid.uuid5(ns, 'consumer-%d' % i)) for i in range(16)],
                                               'custom': 'CUSTOM_TT_B17_' + ns.hex[:16].upper()},
                                 'glance': {'run_id': block + '-' + variant + '-g', 'implementation': gl,
                                            'scenario': pair['glance'], 'concurrency': c,
                                            'size_bytes': pair.get('size_bytes'), 'namespace': str(uuid.uuid5(ns, 'glance'))}})
    return plan


def overlap(a, b):
    """Shared seconds of two measurement windows given as ISO start/end."""
    parse = datetime.datetime.fromisoformat
    start = max(parse(a['start_utc']), parse(b['start_utc']))
    end = min(parse(a['end_utc']), parse(b['end_utc']))
    return max(0.0, (end - start).total_seconds())


def overlap_ok(placement, glance, minimum):
    if not placement or not glance:
        return False, 0.0
    shared = overlap(placement, glance)
    shortest = min(placement['elapsed_s'], glance['elapsed_s'])
    return shared >= minimum * shortest, round(shared, 3)


class Remote(PlacementRemote):
    """Same private SSH JSON channel and clock probe; the joint lab command."""

    def __init__(self, args):
        self.args = args
        self.ssh = ['ssh', '-o', 'BatchMode=yes', '-o', 'IdentitiesOnly=yes', '-o', 'ServerAliveInterval=30',
                    '-o', 'ServerAliveCountMax=3', '-o', 'UserKnownHostsFile=' + str(args.known_hosts), '-i', str(args.ssh_key), args.host]
        command = 'set -e; set +u; source /opt/stack/devstack/openrc admin admin >/dev/null 2>&1; exec ' + shlex.join([
            '/opt/stack/data/venv/bin/python', str(args.remote_code / 'benchmarks/joint_lab.py'),
            '--work-root', str(args.remote_work), '--output', str(args.remote_output),
            '--placement-binary', str(args.remote_code / 'placement'), '--glance-binary', str(args.remote_code / 'glance'),
            '--allow-mutations'])
        self.stderr = (args.private_root / 'remote-stderr.log').open('x')
        self.process = subprocess.Popen([*self.ssh, 'bash', '-lc', shlex.quote(command)], stdin=subprocess.PIPE,
                                        stdout=subprocess.PIPE, stderr=self.stderr, text=True, bufsize=1)


def run_window(entry, config, tokens, out, protocol, pilot, storage):
    """Both generators at once; returns (placement report, glance report)."""
    timing = protocol['pilot'] if pilot else protocol
    results, errors = {}, {}
    # Both measured phases start together: each generator ends its own warm-up
    # and cleanup, then waits here. A broken gate (the other side failed)
    # lets it go on alone; the overlap check then invalidates the window.
    gate = threading.Barrier(2, timeout=120)

    def wait_gate():
        try:
            gate.wait()
        except threading.BrokenBarrierError:
            pass

    def placement():
        try:
            results['placement'] = placement_window.run(config['placement'], tokens['placement'], out / 'placement',
                                                        'pilot' if pilot else 'measurement', duration=timing['measurement_s'],
                                                        warmup=timing['warmup_s'], timeout=protocol['request_timeout_s'],
                                                        abort_check=storage.failed.is_set, measurement_gate=wait_gate)
        except Exception as error:  # noqa: BLE001 - the window becomes invalid
            errors['placement'] = type(error).__name__
            gate.abort()

    def glance():
        try:
            results['glance'] = glance_window.run(config['glance'], tokens['glance'], out / 'glance',
                                                  duration=timing['measurement_s'], warmup=timing['warmup_s'],
                                                  abort_check=storage.failed.is_set, measurement_gate=wait_gate)
        except Exception as error:  # noqa: BLE001
            errors['glance'] = type(error).__name__
            gate.abort()

    threads = [threading.Thread(target=placement), threading.Thread(target=glance)]
    for t in threads:
        t.start()
    for t in threads:
        t.join()
    return results.get('placement'), results.get('glance'), errors


def compress(directory):
    for raw in directory.glob('*samples.jsonl'):
        with raw.open('rb') as source, gzip.open(str(raw) + '.gz', 'wb') as target:
            shutil.copyfileobj(source, target)
        raw.unlink()


def run(args):
    if not args.allow_mutations:
        raise ValueError('explicit mutation opt-in required')
    for path in [args.known_hosts, args.ssh_key, args.protocol]:
        if not path.is_file():
            raise ValueError('readable protocol/SSH trust/key required')
    protocol = json.loads(args.protocol.read_text())
    validate_protocol(protocol)
    storage_check(args.storage_path, protocol['host_free_minimum_bytes'])
    args.output.mkdir(parents=True, exist_ok=False)
    args.private_root.mkdir(mode=0o700, exist_ok=False)
    ident = str(uuid.uuid4())
    plan = schedule(protocol, ident, args.pilot)
    manifest = {'schema': 1, 'campaign_id': ident, 'stage': 'pilot' if args.pilot else 'measurement',
                'started_utc': glance_window.utc(), 'protocol': protocol,
                'protocol_sha256': hashlib.sha256(args.protocol.read_bytes()).hexdigest(),
                'git_base': subprocess.check_output(['git', 'rev-parse', 'HEAD'], text=True).strip(),
                'source_sha256': {p.name: hashlib.sha256(p.read_bytes()).hexdigest() for p in sorted(HERE.glob('*.py'))},
                'sequence': plan, 'runs': [], 'valid': False, 'cleanup': False}
    save(args.output / 'manifest.json', manifest)  # frozen order before any mutation
    sizes = sorted({x['size_bytes'] for x in protocol['pairs'] if x.get('size_bytes')})
    payloads = {s: args.private_root / ('payload-%d.bin' % s) for s in sizes}
    hashes = {s: payload(path, s) for s, path in payloads.items()}
    remote = Remote(args)
    try:
        save(args.output / 'environment.json', remote.call('prepare'))
        for index, entry in enumerate(plan):
            credential = remote.call('token')
            tokens = {'placement': credential.pop('placement_token'), 'glance': credential.pop('glance_token')}
            run_id, out = entry['run_id'], args.output / entry['run_id']
            out.mkdir()
            print(json.dumps({'event': 'preparing', 'window': index + 1, 'of': len(plan), 'run_id': run_id}), flush=True)
            config = remote.call('seed', values={'run_id': run_id, 'placement': entry['placement'], 'glance': entry['glance']})
            routes = config['routes']
            pl, gl = entry['variant'].split('-')
            if (routes['placement_site_go'] != (pl == 'go') or routes['placement_original_active'] != (pl == 'python')
                    or routes['glance_site_go'] != (gl == 'go') or routes['glance_original_active'] != (gl == 'python')):
                raise ValueError('routes do not match the variant')
            gconfig = config['glance']
            if entry['glance']['size_bytes']:
                size = entry['glance']['size_bytes']
                if gconfig['payload'] != hashes[size]:
                    raise ValueError('host/VM payload hashes differ')
                gconfig['payload_file'] = str(payloads[size])
            gconfig['metadata_timeout_s'] = protocol['metadata_request_timeout_s']
            gconfig['binary_timeout_s'] = protocol['binary_request_timeout_s']
            save(out / 'seed.json', {'placement': {k: v for k, v in config['placement'].items() if k != 'candidates'},
                                     'glance': gconfig, 'routes': routes})
            clocks = {'before': remote.clock()}
            remote.call('start-collector', run_id=run_id)
            usage, load = os.times(), os.getloadavg()
            with StorageWatch(args.storage_path, 10 * 1024**3) as storage:
                preport, greport, errors = run_window(entry, {'placement': config['placement'], 'glance': gconfig},
                                                      tokens, out, protocol, args.pilot, storage)
            end = os.times()
            remote.call('stop-collector')
            clocks['after'] = remote.clock()
            drift = abs(clocks['after']['best']['remote_minus_host_ns'] - clocks['before']['best']['remote_minus_host_ns'])
            generations = preport['measurement']['expected_generations'] if preport and preport.get('measurement') else None
            post = remote.call('finish', generations=generations)
            together, shared = overlap_ok((preport or {}).get('measurement'), (greport or {}).get('measurement'),
                                          protocol['minimum_overlap_fraction'])
            report = {'schema': 1, 'run_id': run_id, 'variant': entry['variant'], 'pair': entry['pair'],
                      'concurrency': entry['concurrency'], 'repetition': entry['repetition'],
                      'placement_valid': bool(preport and preport['valid']), 'glance_valid': bool(greport and greport['valid']),
                      'generator_errors': errors, 'overlap_s': shared, 'overlap_ok': together,
                      'clocks': clocks, 'clock_drift_ns': drift, 'clock_valid': drift < 100_000_000,
                      'generator_cpu_seconds': end.user + end.system - usage.user - usage.system,
                      'generator_max_rss_bytes_process_lifetime': resource.getrusage(resource.RUSAGE_SELF).ru_maxrss * 1024,
                      'host_load_average': {'before': load, 'after': os.getloadavg()},
                      'host_storage': storage.report(), 'post_state_removed': post['removed']}
            report['valid'] = (report['placement_valid'] and report['glance_valid'] and together
                               and report['clock_valid'] and post['removed'] and not errors)
            save(out / 'report.json', report)
            compress(out / 'placement')
            manifest['runs'].append({'run_id': run_id, 'valid': report['valid']})
            save(args.output / 'manifest.json', manifest)
            summary = {s: {'ops_s': round(r['measurement']['correct_ops_s'], 1), 'errors': r['measurement']['errors']}
                       for s, r in (('placement', preport), ('glance', greport)) if r and r.get('measurement')}
            print(json.dumps({'event': 'completed', 'run_id': run_id, 'valid': report['valid'],
                              'overlap_s': shared, **summary}), flush=True)
            if not report['valid']:
                raise ValueError('invalid joint window %s; preserve samples, no acceptance' % run_id)
        manifest['valid'] = len(manifest['runs']) == len(plan) and all(r['valid'] for r in manifest['runs'])
    except Exception as error:
        manifest['error'] = type(error).__name__
        manifest['reason'] = str(error) if isinstance(error, (ValueError, RuntimeError)) else 'private diagnostic retained'
    finally:
        try:
            manifest['cleanup_report'] = remote.call('cleanup')
        except Exception as error:
            manifest['cleanup_error'] = type(error).__name__
        try:
            remote.close()
        finally:
            try:
                remote.copy_public_evidence()
            except Exception as error:  # noqa: BLE001
                manifest['evidence_copy_error'] = type(error).__name__
        late = args.output / 'lab' / 'cleanup.json'
        if 'cleanup_report' not in manifest and late.exists():
            manifest['cleanup_report'] = json.loads(late.read_text())
        manifest['cleanup'] = (manifest.get('cleanup_report') or {}).get('private_root_removed') is True
        manifest['valid'] = manifest['valid'] and manifest['cleanup']
        manifest['finished_utc'] = glance_window.utc()
        save(args.output / 'manifest.json', manifest)
        if manifest['cleanup']:
            shutil.rmtree(args.private_root)
    print(json.dumps({'event': 'campaign_finished', 'valid': manifest['valid'], 'cleanup': manifest['cleanup'],
                      'windows_valid': sum(r['valid'] for r in manifest['runs']), 'windows_required': len(plan)}), flush=True)
    return 0 if manifest['valid'] else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
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
        print('joint API campaign failed: ' + type(error).__name__ + (': ' + str(error) if isinstance(error, ValueError) else ''),
              file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())

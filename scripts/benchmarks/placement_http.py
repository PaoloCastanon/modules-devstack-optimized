#!/usr/bin/env python3
"""B16 read-only HTTP instrument; real credentials, closed-loop workers, raw samples.

This is an instrument, not a claim of B17/B18 acceptance. No token or response
body is saved. Requests are validated before being counted as correct operations.
"""
import argparse
from concurrent.futures import ThreadPoolExecutor
import datetime
import hashlib
import http.client
import json
import math
from pathlib import Path
import ssl
import sys
import threading
import time
from urllib.parse import urlsplit
import uuid

sys.path.insert(0, str(Path(__file__).resolve().parents[1]))
from placement_differential import differences, read_token


def percentile(values, probability):
    if not values:
        return None
    return sorted(values)[max(0, math.ceil(probability * len(values)) - 1)]


def summarize(samples, elapsed):
    if not math.isfinite(elapsed) or elapsed <= 0:
        raise ValueError('positive finite duration required')
    if any(type(s['elapsed_ns']) is not int or s['elapsed_ns'] < 0 for s in samples):
        raise ValueError('invalid duration sample')
    good = [s['elapsed_ns'] / 1e6 for s in samples if s['valid']]
    all_times = [s['elapsed_ns'] / 1e6 for s in samples]
    stats = lambda values: {name: percentile(values, q) for name, q in [('p50', .5), ('p95', .95), ('p99', .99)]}
    return {'requests': len(samples), 'correct': len(good), 'errors': len(samples) - len(good),
            'elapsed_s': elapsed, 'completed_requests_s': len(samples) / elapsed,
            'correct_ops_s': len(good) / elapsed, 'correct_latency_ms': stats(good),
            'all_latency_ms': stats(all_times),
            # At least ten expected observations in the top percentile.
            'p99_tail_supported': len(good) >= 1000}


def valid_body(kind, body, provider_id=None):
    try:
        value = json.loads(body)
    except (ValueError, UnicodeDecodeError):
        return False
    if not isinstance(value, dict):
        return False
    if kind == 'provider':
        return value.get('uuid') == provider_id and type(value.get('generation')) is int
    if kind == 'inventories':
        return type(value.get('resource_provider_generation')) is int and isinstance(value.get('inventories'), dict)
    if kind == 'allocations':
        return isinstance(value.get('allocations'), dict)
    if kind == 'candidates':
        return isinstance(value.get('allocation_requests'), list) and isinstance(value.get('provider_summaries'), dict)
    return False


def run(args):
    args.output.mkdir(parents=True, exist_ok=False)
    endpoint = urlsplit(args.url)
    if endpoint.scheme not in ('http', 'https') or not endpoint.hostname or endpoint.username or endpoint.password or endpoint.query or endpoint.fragment:
        raise ValueError('HTTP(S) base URL without credentials/query/fragment required')
    if not args.path.startswith('/') or args.path.startswith('//') or any(ord(c) <= 32 or ord(c) >= 127 for c in args.path):
        raise ValueError('absolute ASCII request path required')
    if not 1 <= args.concurrency <= 64 or not .05 <= args.duration <= 3600 or not 0 <= args.warmup <= 3600 or not .01 <= args.timeout <= 30:
        raise ValueError('invalid concurrency or duration')
    if args.kind == 'provider' and not args.provider_id:
        raise ValueError('provider identity required')
    if not args.version.startswith('1.') or not args.version[2:].isdigit():
        raise ValueError('invalid microversion')
    token = read_token(args.token_file)
    expected_bytes = args.expected_body_file.read_bytes()
    if len(expected_bytes) > 2 * 1024 * 1024 or not valid_body(args.kind, expected_bytes, args.provider_id):
        raise ValueError('invalid expected response snapshot')
    expected = json.loads(expected_bytes)
    run_id = str(uuid.uuid4())
    metadata = {'schema': 1, 'run_id': run_id, 'implementation': args.implementation,
                'instrument_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
                'expected_body_sha256': hashlib.sha256(expected_bytes).hexdigest(),
                'timestamp_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
                'url': args.url, 'path': args.path, 'version': args.version,
                'kind': args.kind, 'concurrency': args.concurrency,
                'planned_duration_s': args.duration, 'warmup_s': args.warmup,
                'timeout_s': args.timeout, 'load_model': 'closed-loop; one connection per worker',
                'rate_denominator': 'actual interval including drain of started requests',
                'latency_boundary': 'before send to complete bounded response read; validation/logging excluded',
                'credentials': 'private file; header/body never stored', 'stage': args.stage}
    lock = threading.Lock()
    with (args.output / 'samples.jsonl').open('x') as raw:
        def phase(duration, save):
            start = [None]
            utc = [None]
            def released():
                utc[0] = datetime.datetime.now(datetime.timezone.utc)
                start[0] = time.monotonic_ns()
            barrier = threading.Barrier(args.concurrency + 1, action=released)

            def worker(index):
                client = None
                observations = []
                barrier.wait()
                deadline = start[0] + int(duration * 1e9)
                while time.monotonic_ns() < deadline:
                    begin = time.monotonic_ns()
                    status, valid, size, error_kind = None, False, 0, None
                    try:
                        if client is None:
                            if endpoint.scheme == 'https':
                                client = http.client.HTTPSConnection(endpoint.hostname, endpoint.port, timeout=args.timeout, context=ssl.create_default_context())
                            else:
                                client = http.client.HTTPConnection(endpoint.hostname, endpoint.port, timeout=args.timeout)
                        client.request('GET', endpoint.path.rstrip('/') + args.path,
                                       headers={'X-Auth-Token': token, 'Accept': 'application/json',
                                                'OpenStack-API-Version': 'placement ' + args.version})
                        response = client.getresponse()
                        status = response.status
                        body = response.read(2 * 1024 * 1024 + 1)
                        finish = time.monotonic_ns()
                        size = len(body)
                        valid = size <= 2 * 1024 * 1024 and status == 200 and valid_body(args.kind, body, args.provider_id)
                        if valid:
                            valid = not differences(expected, json.loads(body))
                        if not valid:
                            error_kind = 'unexpected_response'
                        if size > 2 * 1024 * 1024:
                            client.close()
                            client = None
                    except (OSError, http.client.HTTPException):
                        finish = time.monotonic_ns()
                        error_kind = 'transport'
                        if client is not None:
                            client.close()
                        client = None
                    item = {'run_id': run_id, 'worker': index, 'offset_ns': begin - start[0],
                            'elapsed_ns': finish - begin, 'status': status,
                            'valid': valid, 'response_bytes': size, 'error_kind': error_kind}
                    observations.append(item)
                    if save:
                        with lock:
                            raw.write(json.dumps(item, separators=(',', ':')) + '\n')
                if client is not None:
                    client.close()
                return observations

            with ThreadPoolExecutor(max_workers=args.concurrency) as executor:
                futures = [executor.submit(worker, index) for index in range(args.concurrency)]
                barrier.wait()
                observations = [item for future in futures for item in future.result()]
            # Count all requests started before the deadline, including their
            # drain, without charging thread teardown/list aggregation as load.
            elapsed = max(duration, max((s['offset_ns'] + s['elapsed_ns']) / 1e9 for s in observations))
            result = summarize(observations, elapsed)
            result['start_utc'] = utc[0].isoformat()
            result['end_utc'] = (utc[0] + datetime.timedelta(seconds=elapsed)).isoformat()
            return result

        metadata['warmup'] = phase(args.warmup, False) if args.warmup else None
        metadata['measurement'] = phase(args.duration, True)
    metadata['valid'] = metadata['measurement']['correct'] > 0 and metadata['measurement']['errors'] == 0 and (metadata['warmup'] is None or metadata['warmup']['errors'] == 0)
    (args.output / 'report.json').write_text(json.dumps(metadata, indent=2) + '\n')
    print(json.dumps({'run_id': run_id, 'valid': metadata['valid'], **metadata['measurement']}))
    return 0 if metadata['valid'] else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--url', required=True)
    parser.add_argument('--path', required=True)
    parser.add_argument('--kind', choices=['provider', 'inventories', 'allocations', 'candidates'], required=True)
    parser.add_argument('--provider-id')
    parser.add_argument('--version', default='1.14')
    parser.add_argument('--implementation', choices=['python', 'go', 'fixture'], required=True)
    parser.add_argument('--stage', choices=['instrument-check', 'pilot', 'measurement'], default='pilot')
    parser.add_argument('--token-file', type=Path, required=True)
    parser.add_argument('--expected-body-file', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--concurrency', type=int, default=1)
    parser.add_argument('--duration', type=float, default=120)
    parser.add_argument('--warmup', type=float, default=30)
    parser.add_argument('--timeout', type=float, default=10)
    args = parser.parse_args()
    try:
        return run(args)
    except Exception as error:
        print('benchmark failed: ' + type(error).__name__, file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())

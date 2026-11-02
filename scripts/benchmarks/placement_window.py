#!/usr/bin/env python3
"""B17 closed-loop instrument. Explicit seeded state; no credentials in evidence."""
from concurrent.futures import ThreadPoolExecutor
import datetime
import hashlib
import http.client
import json
from pathlib import Path
import ssl
import threading
import time
from urllib.parse import urlsplit

from placement_http import summarize


def utc():
    return datetime.datetime.now(datetime.timezone.utc).isoformat()


def ordered_candidates(value):
    value = json.loads(json.dumps(value))
    value['allocation_requests'].sort(key=lambda x: json.dumps(x, sort_keys=True))
    for summary in value['provider_summaries'].values():
        if 'traits' in summary:
            summary['traits'].sort()
    return value


def json_match(actual, expected):
    if isinstance(actual, bool) or isinstance(expected, bool):
        return type(actual) is type(expected) and actual == expected
    if isinstance(expected, dict):
        return isinstance(actual, dict) and actual.keys() == expected.keys() and all(json_match(actual[k], v) for k, v in expected.items())
    if isinstance(expected, list):
        return isinstance(actual, list) and len(actual) == len(expected) and all(json_match(a, b) for a, b in zip(actual, expected))
    if isinstance(expected, int):
        return type(actual) is int and actual == expected
    return actual == expected


def candidates_match(value, expected, mutable, custom):
    try:
        value, expected = ordered_candidates(value), ordered_candidates(expected)
        if mutable:
            # Each worker owns a different provider and can hold at most one
            # VCPU/CUSTOM claim. Concurrent candidates may observe either state.
            for ident, summary in value['provider_summaries'].items():
                resources = summary['resources']
                used = resources['VCPU']['used']
                if type(used) is not int or used not in (0, 1) or resources[custom]['used'] != used:
                    return False
                expected['provider_summaries'][ident]['resources']['VCPU']['used'] = used
                expected['provider_summaries'][ident]['resources'][custom]['used'] = used
        return json_match(value, expected)
    except (ValueError, KeyError, TypeError, AttributeError):
        return False


class Client:
    def __init__(self, url, token, timeout):
        self.endpoint = urlsplit(url)
        if self.endpoint.scheme not in ('http', 'https') or not self.endpoint.hostname or self.endpoint.username or self.endpoint.password or self.endpoint.query or self.endpoint.fragment:
            raise ValueError('HTTP(S) endpoint without credentials required')
        self.token, self.timeout, self.connection = token, timeout, None

    def close(self):
        if self.connection:
            self.connection.close()
        self.connection = None

    def request(self, method, path, version, body=None):
        begin = time.monotonic_ns()
        status, size, value, error = None, 0, None, None
        try:
            if self.connection is None:
                cls = http.client.HTTPSConnection if self.endpoint.scheme == 'https' else http.client.HTTPConnection
                options = {'context': ssl.create_default_context()} if cls is http.client.HTTPSConnection else {}
                self.connection = cls(self.endpoint.hostname, self.endpoint.port, timeout=self.timeout, **options)
            data = None if body is None else json.dumps(body, separators=(',', ':')).encode()
            self.connection.request(method, self.endpoint.path.rstrip('/') + path, body=data,
                                    headers={'X-Auth-Token': self.token, 'Accept': 'application/json',
                                             'Content-Type': 'application/json', 'OpenStack-API-Version': 'placement ' + version})
            response = self.connection.getresponse()
            status = response.status
            raw = response.read(2 * 1024 * 1024 + 1)
            finish = time.monotonic_ns()
            size = len(raw)
            if size > 2 * 1024 * 1024 or self.token.encode() in raw:
                error = 'unsafe_response'
                self.close()
            elif raw:
                try:
                    value = json.loads(raw)
                except (ValueError, UnicodeDecodeError):
                    error = 'invalid_json'
        except (OSError, http.client.HTTPException) as exc:
            finish = time.monotonic_ns()
            error = 'timeout' if isinstance(exc, TimeoutError) else 'transport'
            self.close()
        return {'begin': begin, 'finish': finish, 'status': status, 'response_bytes': size, 'error_kind': error}, value


def plan(scenario, provider, consumer, config, generation):
    prefix = '/resource_providers/' + provider
    claim = {'consumer_generation': None, 'project_id': config['owner']['project_id'],
             'user_id': config['owner']['user_id'],
             'allocations': {provider: {'resources': {'VCPU': 1, config['custom']: 1}}}}
    provider_body = dict(config['snapshots'][provider]['provider'], generation=generation)
    inventories = dict(config['snapshots'][provider]['inventories'], resource_provider_generation=generation)
    full = {'allocations': {provider: {'generation': generation + 1, 'resources': {'VCPU': 1, config['custom']: 1}}},
            'project_id': config['owner']['project_id'], 'user_id': config['owner']['user_id'], 'consumer_generation': 1}
    ops = {'provider': ('GET', prefix, '1.14', None, provider_body, 200),
           'inventories': ('GET', prefix + '/inventories', '1.26', None, inventories, 200),
           'allocations-read': ('GET', '/allocations/' + consumer, '1.28', None, {'allocations': {}}, 200),
           'candidates': ('GET', config['candidates_path'], '1.36', None, config['candidates'], 200),
           'claim': ('PUT', '/allocations/' + consumer, '1.36', claim, None, 204),
           'claim-read': ('GET', '/allocations/' + consumer, '1.28', None, full, 200),
           'release': ('DELETE', '/allocations/' + consumer, '1.0', None, None, 204)}
    names = {'claim-cycle': ['claim', 'claim-read', 'release'],
             'nova-mix': ['candidates'] * 6 + ['provider'] * 4 + ['inventories'] * 2 + ['allocations-read', 'claim', 'claim-read', 'release']}.get(scenario, [scenario])
    return [(name, *ops[name]) for name in names]


def run(config, token, output, stage, duration=120, warmup=30, timeout=10, abort_check=None):
    """Stop issuing at deadline; clean partial cycles outside measured windows."""
    output.mkdir(parents=True, exist_ok=False)
    concurrency = config['concurrency']
    if concurrency not in (1, 4, 8, 16) or not .05 <= duration <= 3600 or not 0 <= warmup <= 3600:
        raise ValueError('invalid window')
    generations = [config['snapshots'][p]['provider']['generation'] for p in config['providers'][:concurrency]]
    report = {'schema': 1, 'run_id': config['run_id'], 'implementation': config['implementation'],
              'scenario': config['scenario'], 'concurrency': concurrency, 'stage': stage,
              'instrument_sha256': hashlib.sha256(Path(__file__).read_bytes()).hexdigest(),
              'planned_duration_s': duration, 'warmup_s': warmup, 'timeout_s': timeout,
              'load_model': 'closed-loop; one connection and consumer/provider per worker',
              'drain': 'finish request started before deadline; partial cycle cleanup outside measured window',
              'latency_boundary': 'before send through complete bounded response; excludes JSON validation and logging'}

    def cleanup_consumers():
        client = Client(config['url'], token, timeout)
        try:
            for index, consumer in enumerate(config['consumers'][:concurrency]):
                response, value = client.request('GET', '/allocations/' + consumer, '1.28')
                if response['status'] != 200 or response['error_kind']:
                    raise ValueError('post-phase state unavailable')
                if value.get('allocations'):
                    if value.get('project_id') != config['owner']['project_id'] or value.get('user_id') != config['owner']['user_id'] or set(value['allocations']) != {config['providers'][index]}:
                        raise ValueError('consumer ownership mismatch')
                    response, _ = client.request('DELETE', '/allocations/' + consumer, '1.0')
                    if response['status'] != 204 or response['error_kind']:
                        raise ValueError('post-phase cleanup failed')
                response, value = client.request('GET', '/allocations/' + consumer, '1.28')
                if response['status'] != 200 or response['error_kind'] or value != {'allocations': {}}:
                    raise ValueError('post-phase empty state failed')
        finally:
            client.close()
        return {'all_consumers_empty': True, 'outside_measurement': True}

    def phase(seconds, save):
        start, start_utc = [None], [None]
        failed = threading.Event()
        def release():
            start_utc[0] = utc()
            start[0] = time.monotonic_ns()
        barrier = threading.Barrier(concurrency + 1, action=release)
        lock = threading.Lock()
        with (output / ('samples.jsonl' if save else 'warmup-samples.jsonl')).open('x') as raw:
            def worker(index):
                client = Client(config['url'], token, timeout)
                samples, cycles = [], 0
                provider, consumer = config['providers'][index], config['consumers'][index]
                barrier.wait()
                deadline = start[0] + int(seconds * 1e9)
                try:
                    while time.monotonic_ns() < deadline and not failed.is_set():
                        if abort_check is not None and abort_check():
                            failed.set()
                            break
                        complete = True
                        for name, method, path, version, body, expected, status in plan(config['scenario'], provider, consumer, config, generations[index]):
                            if time.monotonic_ns() >= deadline:
                                complete = False
                                break
                            result, value = client.request(method, path, version, body)
                            valid = result['error_kind'] is None and result['status'] == status
                            if valid:
                                valid = (candidates_match(value, expected, config['scenario'] == 'nova-mix', config['custom']) if name == 'candidates' else json_match(value, expected))
                            item = {'run_id': config['run_id'], 'worker': index, 'cycle': cycles,
                                    'operation': name, 'method': method, 'path': path, 'version': version,
                                    'offset_ns': result['begin'] - start[0], 'elapsed_ns': result['finish'] - result['begin'],
                                    'status': result['status'], 'response_bytes': result['response_bytes'],
                                    'valid': valid, 'error_kind': result['error_kind'] or (None if valid else 'unexpected_response')}
                            samples.append(item)
                            with lock:
                                raw.write(json.dumps(item, separators=(',', ':')) + '\n')
                            if valid and name == 'claim':
                                generations[index] += 1
                            if not valid:
                                complete = False
                                failed.set()
                                break
                        if not complete:
                            break
                        cycles += 1
                finally:
                    client.close()
                return samples, cycles
            with ThreadPoolExecutor(max_workers=concurrency) as executor:
                futures = [executor.submit(worker, i) for i in range(concurrency)]
                barrier.wait()
                results = [future.result() for future in futures]
        samples = [item for items, _ in results for item in items]
        elapsed = max(seconds, max(((s['offset_ns'] + s['elapsed_ns']) / 1e9 for s in samples), default=0))
        summary = summarize(samples, elapsed)
        summary['status_counts'] = {str(status): sum(s['status'] == status for s in samples) for status in {s['status'] for s in samples}}
        summary['error_counts'] = {str(error): sum(s['error_kind'] == error for s in samples) for error in {s['error_kind'] for s in samples} if error is not None}
        summary.update(start_utc=start_utc[0], end_utc=(datetime.datetime.fromisoformat(start_utc[0]) + datetime.timedelta(seconds=elapsed)).isoformat(),
                       cycles_per_worker=[n for _, n in results], expected_generations=generations[:],
                       operations={name: summarize([s for s in samples if s['operation'] == name], elapsed) for name in sorted({s['operation'] for s in samples})})
        return summary

    report['warmup'] = phase(warmup, False) if warmup else None
    report['warmup_cleanup'] = cleanup_consumers()
    report['measurement'] = phase(duration, True) if report['warmup'] is None or report['warmup']['errors'] == 0 else None
    report['measurement_cleanup'] = cleanup_consumers()
    report['external_abort'] = bool(abort_check is not None and abort_check())
    report['valid'] = (not report['external_abort'] and report['measurement'] is not None
                       and report['measurement']['correct'] > 0 and report['measurement']['errors'] == 0)
    (output / 'report.json').write_text(json.dumps(report, indent=2) + '\n')
    return report

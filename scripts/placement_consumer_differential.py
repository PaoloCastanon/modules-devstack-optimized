#!/usr/bin/env python3
"""Strict B09 modern consumer contract. Uses only fresh providers/consumers."""
import argparse
import json
from email.utils import parsedate_to_datetime
from pathlib import Path
import sys
import uuid
import placement_differential as base

base.HEADERS += ('cache-control', 'last-modified')


def canonical(response, target):
    out = base.canonical(response, target.origin)
    if 'last-modified' in out['headers']:
        parsedate_to_datetime(out['headers']['last-modified'])
        out['headers']['last-modified'] = '<http-date>'
    return out


def run(left, right, output, project, user):
    output.mkdir(parents=True, exist_ok=False)
    provider, consumer = str(uuid.uuid4()), str(uuid.uuid4())
    pp, cp = '/resource_providers/' + provider, '/allocations/' + consumer
    targets = {'python': left, 'go': right}
    eligible = {key: [] for key in targets}
    report = {'schema': 1, 'provider': provider, 'consumer': consumer, 'cases': [], 'cleanup': [], 'cleanup_ok': True,
              'normalization': ['B07 request_id/origin/Vary', 'validated Last-Modified HTTP dates'], 'preflight': {}}
    cases = []

    def body(generation, amount=2):
        return {'project_id': project, 'user_id': user, 'consumer_generation': generation,
                'allocations': {provider: {'generation': 999, 'resources': {'VCPU': amount}}}}

    def add(name, method, path, status, data=None, version='1.36', gen=None):
        cases.append((name, method, path, status, data, version, gen))

    add('new-consumer-zero-conflict', 'PUT', cp, 409, body(0), gen=None)
    add('new-consumer-null-create', 'PUT', cp, 204, body(None), gen=1)
    add('consumer-owner-1.12', 'GET', cp, 200, version='1.12', gen=1)
    add('consumer-generation-1.28', 'GET', cp, 200, version='1.28', gen=1)
    add('provider-consumer-generations', 'GET', pp + '/allocations', 200, version='1.28', gen=1)
    add('existing-null-conflict', 'PUT', cp, 409, body(None), gen=1)
    add('existing-stale-zero-conflict', 'PUT', cp, 409, body(0), gen=1)
    add('existing-future-conflict', 'PUT', cp, 409, body(9), gen=1)
    missing = body(1)
    del missing['consumer_generation']
    add('missing-generation-schema', 'PUT', cp, 400, missing, gen=1)
    invalid = body(True)
    add('boolean-generation-schema', 'PUT', cp, 400, invalid, gen=1)
    valid = body(1)
    valid['mappings'] = {'': [provider], 'compute': [provider]}
    add('mappings-1.34-and-ignored-provider-generation', 'PUT', cp, 204, valid, '1.34', 2)
    invalid = body(2)
    invalid['mappings'] = {}
    add('empty-mappings-schema', 'PUT', cp, 400, invalid, gen=2)
    invalid = body(2)
    invalid['mappings'] = {'compute': ['bad']}
    add('mappings-invalid-uuid', 'PUT', cp, 400, invalid, gen=2)
    add('capacity-rollback-generation', 'PUT', cp, 409, body(2, 20), gen=2)
    add('same-allocation-advances-consumer', 'PUT', cp, 204, body(2), gen=3)
    legacy = {'project_id': project, 'user_id': user, 'allocations': {provider: {'resources': {'VCPU': 3}}}}
    add('provider-map-1.12', 'PUT', cp, 204, legacy, '1.12', 4)
    add('legacy-write-increments-modern-generation', 'GET', cp, 200, version='1.28', gen=4)
    empty = body(3)
    empty['allocations'] = {}
    add('empty-stale-preserves-reservation', 'PUT', cp, 409, empty, gen=4)
    empty = body(4)
    empty['allocations'] = {}
    add('empty-current-removes-consumer', 'PUT', cp, 204, empty, gen=None)
    add('empty-read-omits-owner-and-generation', 'GET', cp, 200)
    add('removed-consumer-old-generation-conflict', 'PUT', cp, 409, body(5))
    add('removed-consumer-null-recreates', 'PUT', cp, 204, body(None), gen=1)
    add('consumer-delete-1.0', 'DELETE', cp, 204, version='1.0')
    add('consumer-delete-repeat', 'DELETE', cp, 404)

    def state(target):
        return {name: target.call('GET', path, headers={'OpenStack-API-Version': 'placement ' + version})
                for name, path, version in [('provider', pp, '1.8'), ('inventory', pp + '/inventories', '1.8'),
                                           ('consumer', cp, '1.36'), ('allocations', pp + '/allocations', '1.28')]}

    try:
        for key, target in targets.items():
            report['preflight'][key] = state(target)
            if report['preflight'][key]['provider']['status'] != 404:
                raise RuntimeError('provider occupied')
            eligible[key].append(pp)
            if json.loads(report['preflight'][key]['consumer']['body_text']) != {'allocations': {}}:
                raise RuntimeError('consumer occupied')
            eligible[key].append(cp)
            assert target.call('POST', '/resource_providers', {'uuid': provider, 'name': 'tt-b09-modern-' + provider})['status'] == 201
            assert target.call('PUT', pp + '/inventories', {'resource_provider_generation': 0, 'inventories': {'VCPU': {'total': 8, 'max_unit': 8}}})['status'] == 200
        for name, method, path, status, data, version, generation in cases:
            record = {'name': name, 'request': {'method': method, 'path': path, 'body': data, 'version': version},
                      'responses': {}, 'before': {}, 'after': {}, 'differences': [], 'expected_status': status}
            report['cases'].append(record)
            for key, target in targets.items():
                record['before'][key] = state(target)
                record['responses'][key] = target.call(method, path, data, {'OpenStack-API-Version': 'placement ' + version})
                record['after'][key] = state(target)
                if record['responses'][key]['status'] != status:
                    record['differences'].append({'path': '$.expected_status.' + key, 'actual': record['responses'][key]['status'], 'expected': status})
                actual = json.loads(record['after'][key]['consumer']['body_text']).get('consumer_generation')
                if actual != generation:
                    record['differences'].append({'path': '$.expected_generation.' + key, 'actual': actual, 'expected': generation})
            for phase in ['before', 'after']:
                for item in record[phase]['python']:
                    record['differences'] += base.differences(canonical(record[phase]['python'][item], left), canonical(record[phase]['go'][item], right), '$.' + phase + '.' + item)
            record['differences'] += base.differences(canonical(record['responses']['python'], left), canonical(record['responses']['go'], right), '$.response')
    except Exception as error:
        report['execution_error'] = type(error).__name__
    finally:
        for key, target in targets.items():
            for path in [cp, pp]:
                if path not in eligible[key]:
                    continue
                try:
                    response = target.call('DELETE', path)
                    check = target.call('GET', path)
                    good = check['status'] == 404 if path == pp else json.loads(check['body_text']) == {'allocations': {}}
                    report['cleanup_ok'] &= response['status'] in (204, 404) and good
                    report['cleanup'].append({'target': key, 'path': path, 'delete': response, 'check': check})
                except Exception as error:
                    report['cleanup_ok'] = False
                    report['cleanup'].append({'target': key, 'error': type(error).__name__})
        report['summary'] = {'planned': len(cases), 'total': len(report['cases']), 'strict_equal': sum(not c['differences'] for c in report['cases'])}
        (output / 'report.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps({'summary': report['summary'], 'cleanup_ok': report['cleanup_ok']}))
    return 0 if not report.get('execution_error') and report['cleanup_ok'] and report['summary']['strict_equal'] == len(cases) else 1


def main():
    p = argparse.ArgumentParser(description=__doc__)
    for flag in ['python-url', 'go-url', 'project-id', 'user-id']:
        p.add_argument('--' + flag, required=True)
    for role in ['admin', 'service', 'reader']:
        p.add_argument('--' + role + '-token-file', required=True)
    p.add_argument('--output', type=Path, required=True)
    p.add_argument('--allow-mutations', action='store_true')
    args = p.parse_args()
    if not args.allow_mutations:
        p.error('explicit mutation opt-in required')
    tokens = {role: base.read_token(getattr(args, role + '_token_file')) for role in ['admin', 'service', 'reader']}
    try:
        return run(base.Target(args.python_url, tokens), base.Target(args.go_url, tokens), args.output, args.project_id, args.user_id)
    except Exception as error:
        print(type(error).__name__, file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())

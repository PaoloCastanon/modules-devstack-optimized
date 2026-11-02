#!/usr/bin/env python3
"""B09-A allocations 1.8 live comparison using pre-existing project/user IDs.

Only fresh providers and consumers mutate. Pre-1.8 writes are excluded because
upstream would create global incomplete-project/user records without HTTP delete.
"""
import argparse
import json
from pathlib import Path
import sys
import uuid
from placement_differential import Target, canonical, differences, read_token, request_headers


def run(left, right, output, project, user):
    output.mkdir(parents=True, exist_ok=False)
    providers = [str(uuid.uuid4()), str(uuid.uuid4())]
    consumers = [str(uuid.uuid4()), str(uuid.uuid4())]
    missing = str(uuid.uuid4())
    paths = ['/resource_providers/' + item for item in providers]
    cp = lambda index: '/allocations/' + consumers[index]
    rp = providers[0]
    targets = {'python': left, 'go': right}
    eligible = {key: [] for key in targets}
    report = {'schema': 1, 'providers': providers, 'consumers': consumers,
              'project_id': project, 'user_id': user,
              'normalization': ['B07 request_id, configured Location origin, Vary token order'],
              'limits': ['positive writes use 1.8 and pre-existing project/user records',
                         'consumer generation 1.28, candidates and Nova substitution remain pending'],
              'cases': [], 'preflight': {}, 'cleanup': [], 'cleanup_ok': True}
    cases = []

    def claim(values):
        return {'allocations': [{'resource_provider': {'uuid': provider}, 'resources': resources}
                                for provider, resources in values], 'project_id': project, 'user_id': user}

    def add(name, method, path, status, body=None, version='1.8', role='admin', generation=None):
        cases.append((name, method, path, status, body, version, role, generation))

    add('consumer-empty-1.0', 'GET', cp(0), 200, version='1.0', generation=1)
    add('consumer-invalid-get-empty', 'GET', '/allocations/not-a-uuid', 200, generation=1)
    add('consumer-delete-empty-1.0', 'DELETE', cp(0), 404, version='1.0', generation=1)
    add('provider-allocations-empty', 'GET', paths[0] + '/allocations', 200, version='1.0', generation=1)
    add('provider-usages-zero', 'GET', paths[0] + '/usages', 200, version='1.0', generation=1)
    add('invalid-consumer-before-schema', 'PUT', '/allocations/not-a-uuid', 400, {}, generation=1)
    add('empty-array-schema', 'PUT', cp(0), 400, claim([]), generation=1)
    add('object-instead-of-array', 'PUT', cp(0), 400,
        {'allocations': {}, 'project_id': project, 'user_id': user}, generation=1)
    add('empty-resources-schema', 'PUT', cp(0), 400, claim([(rp, {})]), generation=1)
    add('zero-resources-schema', 'PUT', cp(0), 400, claim([(rp, {'VCPU': 0})]), generation=1)
    add('invalid-resource-name-schema', 'PUT', cp(0), 400, claim([(rp, {'bad': 2})]), generation=1)
    add('project-required-1.8', 'PUT', cp(0), 400,
        {'allocations': claim([(rp, {'VCPU': 2})])['allocations'], 'user_id': user}, generation=1)
    add('user-required-1.8', 'PUT', cp(0), 400,
        {'allocations': claim([(rp, {'VCPU': 2})])['allocations'], 'project_id': project}, generation=1)
    add('project-extra-before-1.8', 'PUT', cp(0), 400, claim([(rp, {'VCPU': 2})]), '1.7', generation=1)
    add('reader-cannot-claim', 'PUT', cp(0), 403, claim([(rp, {'VCPU': 6})]), role='reader', generation=1)
    add('missing-token', 'GET', cp(0), 401, role='missing', generation=1)
    add('claim-service', 'PUT', cp(0), 204, claim([(rp, {'VCPU': 6})]), role='service', generation=2)
    add('consumer-get-1.0', 'GET', cp(0), 200, version='1.0', generation=2)
    add('consumer-uppercase', 'GET', '/allocations/' + consumers[0].upper(), 200, generation=2)
    add('consumer-compact-lookup-empty', 'GET', '/allocations/' + consumers[0].replace('-', ''), 200, generation=2)
    add('reader-cannot-list', 'GET', cp(0), 403, role='reader', generation=2)
    add('claim-identical-increments-provider', 'PUT', cp(0), 204, claim([(rp, {'VCPU': 6})]), generation=3)
    add('duplicate-provider-last-wins', 'PUT', cp(0), 204,
        claim([(rp, {'VCPU': 2}), (rp, {'VCPU': 4})]), generation=4)
    add('claim-second-consumer', 'PUT', cp(1), 204, claim([(rp, {'VCPU': 4})]), generation=5)
    add('provider-allocations-two-consumers', 'GET', paths[0] + '/allocations', 200, generation=5)
    add('provider-usages-full', 'GET', paths[0] + '/usages', 200, generation=5)
    add('resources-filter-subtracts-usage', 'GET', '/resource_providers?uuid=' + rp + '&resources=VCPU:1', 200, generation=5)
    add('replace-over-capacity', 'PUT', cp(0), 409, claim([(rp, {'VCPU': 5})]), generation=5)
    add('replace-over-max-unit', 'PUT', cp(0), 409, claim([(rp, {'VCPU': 9})]), generation=5)
    add('replace-unknown-class', 'PUT', cp(0), 400, claim([(rp, {'CUSTOM_B09_NOT_CREATED': 1})]), generation=5)
    add('replace-missing-inventory', 'PUT', cp(0), 409, claim([(rp, {'DISK_GB': 1})]), generation=5)
    add('inventory-clear-in-use', 'PUT', paths[0] + '/inventories', 409,
        {'resource_provider_generation': 5, 'inventories': {}}, generation=5)
    add('inventory-delete-in-use', 'DELETE', paths[0] + '/inventories', 409, version='1.5', generation=5)
    add('provider-delete-in-use', 'DELETE', paths[0], 409, generation=5)
    add('replace-multi-provider', 'PUT', cp(0), 204,
        claim([(rp, {'VCPU': 2}), (providers[1], {'VCPU': 2})]), generation=6)
    add('multi-provider-rollback', 'PUT', cp(0), 409,
        claim([(rp, {'VCPU': 2}), (providers[1], {'VCPU': 9})]), generation=6)
    add('missing-provider-preserves-existing-consumer', 'PUT', cp(0), 400,
        claim([(missing, {'VCPU': 2})]), generation=6)
    add('move-consumer-second-provider', 'PUT', cp(0), 204,
        claim([(providers[1], {'VCPU': 4})]), generation=6)
    add('old-provider-usage-released', 'GET', paths[0] + '/usages', 200, generation=6)
    add('move-consumer-back', 'PUT', cp(0), 204, claim([(rp, {'VCPU': 4})]), generation=7)
    add('inventory-shrink-preserves-overage', 'PUT', paths[0] + '/inventories', 200,
        {'resource_provider_generation': 7, 'inventories': {'VCPU': {'total': 3, 'max_unit': 8}, 'MEMORY_MB': {'total': 1024}}}, generation=8)
    add('replace-reduces-overage', 'PUT', cp(0), 204, claim([(rp, {'VCPU': 3})]), generation=9)
    add('replace-cannot-worsen-overage', 'PUT', cp(0), 409, claim([(rp, {'VCPU': 4})]), generation=9)
    add('identical-overage-allowed', 'PUT', cp(0), 204, claim([(rp, {'VCPU': 3})]), generation=10)
    add('delete-second-consumer-no-gen-increment', 'DELETE', cp(1), 204, version='1.0', generation=10)
    add('claim-after-capacity-released', 'PUT', cp(0), 204, claim([(rp, {'VCPU': 3})]), generation=11)
    add('delete-first-consumer-no-gen-increment', 'DELETE', cp(0), 204, version='1.0', generation=11)
    add('delete-repeat-404', 'DELETE', cp(0), 404, generation=11)
    add('consumer-empty-after-delete', 'GET', cp(0), 200, generation=11)
    add('usages-zero-after-delete', 'GET', paths[0] + '/usages', 200, generation=11)
    add('provider-allocations-missing', 'GET', '/resource_providers/' + missing + '/allocations', 404, generation=11)
    add('usages-provider-missing', 'GET', '/resource_providers/' + missing + '/usages', 404, generation=11)

    add('fractional-capacity-inventory', 'PUT', paths[1] + '/inventories', 200,
        {'resource_provider_generation': 3, 'inventories': {'VCPU': {'total': 4, 'reserved': 1, 'allocation_ratio': 1.5, 'min_unit': 2, 'max_unit': 8, 'step_size': 2}}}, generation=11)
    add('fractional-capacity-filter-hit', 'GET', '/resource_providers?uuid=' + providers[1] + '&resources=VCPU:4', 200, generation=11)
    add('fractional-capacity-claim', 'PUT', cp(0), 204, claim([(providers[1], {'VCPU': 4})]), generation=11)
    add('fractional-capacity-overflow', 'PUT', cp(1), 409, claim([(providers[1], {'VCPU': 2})]), generation=11)
    add('step-constraint', 'PUT', cp(0), 409, claim([(providers[1], {'VCPU': 3})]), generation=11)
    add('min-constraint', 'PUT', cp(0), 409, claim([(providers[1], {'VCPU': 1})]), generation=11)
    add('max-constraint', 'PUT', cp(0), 409, claim([(providers[1], {'VCPU': 10})]), generation=11)
    add('fractional-consumer-release', 'DELETE', cp(0), 204, generation=11)

    headers = {'OpenStack-API-Version': 'placement 1.8'}

    def state(target):
        requests = [('consumer-' + str(i), cp(i)) for i in range(2)]
        for i, path in enumerate(paths):
            requests += [(label + str(i), path + suffix) for label, suffix in
                         [('provider-', ''), ('inventories-', '/inventories'),
                          ('allocations-', '/allocations'), ('usages-', '/usages')]]
        return {label: target.call('GET', path, headers=headers) for label, path in requests}

    try:
        for key, target in targets.items():
            report['preflight'][key] = {}
            for path in paths:
                response = target.call('GET', path, headers=headers)
                report['preflight'][key][path] = response
                if response['status'] != 404:
                    raise RuntimeError('provider UUID occupied')
                eligible[key].append(path)
            for i in range(2):
                response = target.call('GET', cp(i), headers=headers)
                report['preflight'][key][cp(i)] = response
                if response['status'] != 200 or json.loads(response['body_text']) != {'allocations': {}}:
                    raise RuntimeError('fresh consumer has allocations')
                eligible[key].append(cp(i))
            for i, path in enumerate(paths):
                if target.call('POST', '/resource_providers', {'uuid': providers[i], 'name': 'tt-b09-' + providers[i]}, headers)['status'] != 201:
                    raise RuntimeError('provider seed failed')
                if target.call('PUT', path + '/inventories', {'resource_provider_generation': 0, 'inventories': {'VCPU': {'total': 8, 'max_unit': 8}, 'MEMORY_MB': {'total': 1024}}}, headers)['status'] != 200:
                    raise RuntimeError('inventory seed failed')
        for name, method, path, status, body, version, role, generation in cases:
            selected = {'OpenStack-API-Version': 'placement ' + version}
            record = {'name': name, 'expected_status': status, 'request': {'method': method, 'path': path, 'body': body,
                      'headers': request_headers(body, selected), 'auth_role': role}, 'responses': {}, 'before': {}, 'after': {}, 'differences': []}
            report['cases'].append(record)
            for key, target in targets.items():
                record['before'][key] = state(target)
                record['responses'][key] = target.call(method, path, body, selected, role)
                record['after'][key] = state(target)
                if record['responses'][key]['status'] != status:
                    record['differences'].append({'path': '$.expected_status.' + key, 'expected': status, 'actual': record['responses'][key]['status']})
                if generation is not None:
                    actual = json.loads(record['after'][key]['provider-0']['body_text']).get('generation')
                    if actual != generation:
                        record['differences'].append({'path': '$.expected_generation.' + key, 'expected': generation, 'actual': actual})
            for phase in ['before', 'after']:
                for item in record[phase]['python']:
                    record['differences'] += differences(canonical(record[phase]['python'][item], left.origin), canonical(record[phase]['go'][item], right.origin), '$.' + phase + '.' + item)
            record['differences'] += differences(canonical(record['responses']['python'], left.origin), canonical(record['responses']['go'], right.origin), '$.response')
            record['same_status'] = record['responses']['python']['status'] == record['responses']['go']['status']
    except Exception as error:
        report['execution_error'] = type(error).__name__
    finally:
        for key, target in targets.items():
            for path in [cp(0), cp(1), *paths]:
                if path not in eligible[key]:
                    continue
                try:
                    deleted = target.call('DELETE', path, headers=headers)
                    after = target.call('GET', path, headers=headers)
                    empty = after['status'] == 404 if path in paths else after['status'] == 200 and json.loads(after['body_text']) == {'allocations': {}}
                    report['cleanup_ok'] &= deleted['status'] in (204, 404) and empty
                    report['cleanup'].append({'target': key, 'path': path, 'delete': deleted, 'after': after})
                except Exception as error:
                    report['cleanup_ok'] = False
                    report['cleanup'].append({'target': key, 'path': path, 'error': type(error).__name__})
        report['summary'] = {'planned': len(cases), 'total': len(report['cases']), 'strict_equal': sum(not item['differences'] for item in report['cases']), 'status_equal': sum(item['same_status'] for item in report['cases'])}
        (output / 'report.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps({'summary': report['summary'], 'cleanup_ok': report['cleanup_ok']}))
    return 0 if not report.get('execution_error') and report['cleanup_ok'] and report['summary']['strict_equal'] == len(cases) else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for flag in ['python-url', 'go-url', 'project-id', 'user-id']:
        parser.add_argument('--' + flag, required=True)
    for role in ['admin', 'service', 'reader']:
        parser.add_argument('--' + role + '-token-file', required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--allow-mutations', action='store_true')
    args = parser.parse_args()
    if not args.allow_mutations:
        parser.error('explicit --allow-mutations required')
    tokens = {role: read_token(getattr(args, role + '_token_file')) for role in ['admin', 'service', 'reader']}
    try:
        return run(Target(args.python_url, tokens), Target(args.go_url, tokens), args.output, args.project_id, args.user_id)
    except Exception as error:
        print('comparison failed: ' + type(error).__name__, file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())

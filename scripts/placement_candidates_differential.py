#!/usr/bin/env python3
"""B10 flat-provider candidate profile; isolated CUSTOM class scopes live queries."""
import argparse
import json
from pathlib import Path
import sys
import uuid
import placement_differential as base
from placement_consumer_differential import canonical as timestamp_canonical


def canonical(response, target):
    out = timestamp_canonical(response, target)
    body = out['body']
    if isinstance(body, dict):
        if isinstance(body.get('allocation_requests'), list):
            body['allocation_requests'].sort(key=lambda item: json.dumps(item, sort_keys=True))
        for summary in body.get('provider_summaries', {}).values():
            if 'traits' in summary:
                summary['traits'].sort()
    return out


def run(left, right, output, project, user):
    output.mkdir(parents=True, exist_ok=False)
    ids = [str(uuid.uuid4()), str(uuid.uuid4())]
    paths = ['/resource_providers/' + value for value in ids]
    consumer = str(uuid.uuid4())
    cp = '/allocations/' + consumer
    resource = 'CUSTOM_B10_' + uuid.uuid4().hex.upper()
    trait = 'CUSTOM_B10_' + uuid.uuid4().hex.upper()
    class_path, trait_path = '/resource_classes/' + resource, '/traits/' + trait
    targets = {'python': left, 'go': right}
    eligible = {key: [] for key in targets}
    report = {'schema': 1, 'providers': ids, 'consumer': consumer, 'class': resource, 'trait': trait,
              'normalization': ['B07 request_id/origin/Vary', 'validated Last-Modified dates', 'sort unordered allocation_requests and summary traits without deduplication'],
              'scope': 'flat providers; CUSTOM resource isolates queries from pre-existing original providers',
              'cases': [], 'preflight': {}, 'seed': {}, 'seed_differences': [], 'cleanup': [], 'cleanup_ok': True}
    cases = []

    def add(name, method, path, status, body=None, version='1.36', role='admin'):
        cases.append((name, method, path, status, body, version, role))

    def query(parameters, amount=1):
        return '/allocation_candidates?resources=' + resource + ':' + str(amount) + parameters

    add('provider-tree-fields-1.14', 'GET', paths[0], 200, version='1.14')
    add('provider-in-tree-filter', 'GET', '/resource_providers?uuid=' + ids[0] + '&in_tree=' + ids[0], 200, version='1.14')
    add('provider-required-filter', 'GET', '/resource_providers?uuid=' + ids[0] + '&required=OWNER_NOVA', 200, version='1.18')
    add('candidate-list-format-1.10', 'GET', query(''), 200, version='1.10')
    add('candidate-map-format-1.12', 'GET', query(''), 200, version='1.12')
    add('candidate-traits-1.17', 'GET', query('&required=' + trait), 200, version='1.17')
    add('candidate-all-inventories-1.27', 'GET', query(''), 200, version='1.27')
    add('candidate-root-fields-1.29', 'GET', query(''), 200, version='1.29')
    add('candidate-mappings-1.34', 'GET', query(''), 200, version='1.34')
    add('nova-root-required-1.36', 'GET', query('&limit=1000&root_required=OWNER_NOVA,!COMPUTE_STATUS_DISABLED'), 200)
    add('aggregate-generation-get', 'GET', paths[0] + '/aggregates', 200, version='1.19')
    add('aggregate-generation-put', 'PUT', paths[0] + '/aggregates', 200,
        {'resource_provider_generation': 2, 'aggregates': []}, '1.19')
    add('aggregate-generation-stale', 'PUT', paths[0] + '/aggregates', 409,
        {'resource_provider_generation': 2, 'aggregates': []}, '1.19')
    add('aggregate-noop-increments', 'PUT', paths[0] + '/aggregates', 200,
        {'resource_provider_generation': 3, 'aggregates': []}, '1.19')
    add('disable-second-root', 'PUT', paths[1] + '/traits', 200,
        {'resource_provider_generation': 2, 'traits': ['OWNER_NOVA', trait, 'COMPUTE_STATUS_DISABLED']}, '1.6')
    add('forbidden-root-excludes-disabled', 'GET', query('&root_required=OWNER_NOVA,!COMPUTE_STATUS_DISABLED'), 200)
    add('required-forbidden-1.22', 'GET', query('&required=' + trait + ',!COMPUTE_STATUS_DISABLED'), 200, version='1.22')
    allocation = {'project_id': project, 'user_id': user, 'consumer_generation': None,
                  'allocations': {ids[0]: {'resources': {resource: 6}}}}
    add('claim-candidate-capacity', 'PUT', cp, 204, allocation)
    add('usage-removes-candidate-with-root-filter', 'GET', query('&root_required=OWNER_NOVA,!COMPUTE_STATUS_DISABLED', 3), 200)
    add('remaining-candidate-without-forbidden-root', 'GET', query('', 3), 200)
    add('limit-one-unique-eligible', 'GET', query('&limit=1', 3), 200)
    add('limit-zero-invalid', 'GET', query('&limit=0'), 400)
    add('candidate-query-extra', 'GET', query('&unknown=1'), 400)
    add('root-traits-conflict', 'GET', query('&root_required=OWNER_NOVA,!OWNER_NOVA'), 400)
    add('root-traits-duplicate-key', 'GET', query('&root_required=OWNER_NOVA&root_required=OWNER_NOVA'), 400)
    add('missing-resource-group', 'GET', '/allocation_candidates?root_required=OWNER_NOVA', 400)
    add('unknown-resource-class', 'GET', '/allocation_candidates?resources=CUSTOM_B10_MISSING:1', 400)
    add('unknown-required-trait', 'GET', query('&required=CUSTOM_B10_MISSING'), 400)
    add('malformed-resource-amount', 'GET', '/allocation_candidates?resources=VCPU:bad', 400)
    add('in-tree-selected-root', 'GET', query('&in_tree=' + ids[0]), 200, version='1.31')
    add('in-tree-invalid', 'GET', query('&in_tree=bad'), 400, version='1.31')
    aggregate = str(uuid.uuid4())
    add('member-of-no-association', 'GET', query('&member_of=' + aggregate), 200, version='1.21')
    add('forbidden-member-of-1.32', 'GET', query('&member_of=!' + aggregate), 200, version='1.32')
    add('granular-merged-capacity', 'GET', '/allocation_candidates?resources1=' + resource + ':2&resources2=' + resource + ':2&group_policy=none', 200)
    add('granular-isolate-flat-tree', 'GET', '/allocation_candidates?resources1=' + resource + ':1&resources2=' + resource + ':1&group_policy=isolate', 200)
    add('granular-missing-policy', 'GET', '/allocation_candidates?resources1=' + resource + ':1&resources2=' + resource + ':1', 400)
    add('same-subtree-with-resourceless-group', 'GET', '/allocation_candidates?resources_C=' + resource + ':1&required_T=' + trait + '&same_subtree=_C,_T&group_policy=none', 200)
    add('same-subtree-missing-group', 'GET', '/allocation_candidates?resources_C=' + resource + ':1&same_subtree=_C,MISSING', 400)
    add('reader-forbidden', 'GET', query(''), 403, role='reader')
    add('missing-token', 'GET', query(''), 401, role='missing')
    zero = {'resource_provider_generation': 3, 'inventories': {resource: {'total': 8, 'reserved': 8}}}
    add('reserved-equal-total-before-1.26', 'PUT', paths[1] + '/inventories', 400, zero, '1.25')
    add('reserved-equal-total-1.26', 'PUT', paths[1] + '/inventories', 200, zero, '1.26')
    add('reserved-inventory-roundtrip', 'GET', paths[1] + '/inventories', 200, version='1.26')
    add('zero-capacity-excludes-provider', 'GET', query(''), 200)

    def state(target):
        return {label: target.call('GET', path, headers={'OpenStack-API-Version': 'placement ' + version})
                for label, path, version in [('provider-0', paths[0], '1.14'), ('provider-1', paths[1], '1.14'),
                                            ('inventory-0', paths[0] + '/inventories', '1.8'),
                                            ('inventory-1', paths[1] + '/inventories', '1.8'), ('consumer', cp, '1.36')]}

    try:
        for key, target in targets.items():
            report['preflight'][key] = {}
            for path, version in [(class_path, '1.7'), (trait_path, '1.6'), (paths[0], '1.14'), (paths[1], '1.14'), (cp, '1.36')]:
                response = target.call('GET', path, headers={'OpenStack-API-Version': 'placement ' + version})
                report['preflight'][key][path] = response
                if path != cp and response['status'] != 404 or path == cp and json.loads(response['body_text']) != {'allocations': {}}:
                    raise RuntimeError('fresh resource occupied')
                eligible[key].append(path)
            seeds = [('PUT', class_path, None, '1.7', 201), ('PUT', trait_path, None, '1.6', 201)]
            for i, path in enumerate(paths):
                seeds += [('POST', '/resource_providers', {'uuid': ids[i], 'name': 'tt-b10-' + ids[i], 'parent_provider_uuid': None}, '1.20', 200),
                          ('PUT', path + '/inventories', {'resource_provider_generation': 0, 'inventories': {resource: {'total': 8, 'max_unit': 8}, 'VCPU': {'total': 8}, 'MEMORY_MB': {'total': 1024}}}, '1.26', 200),
                          ('PUT', path + '/traits', {'resource_provider_generation': 1, 'traits': ['OWNER_NOVA', trait]}, '1.6', 200)]
            report['seed'][key] = []
            for method, path, data, version, status in seeds:
                response = target.call(method, path, data, {'OpenStack-API-Version': 'placement ' + version})
                report['seed'][key].append(response)
                if response['status'] != status:
                    raise RuntimeError('seed failed')
        for i, (a, b) in enumerate(zip(report['seed']['python'], report['seed']['go'])):
            report['seed_differences'] += base.differences(canonical(a, left), canonical(b, right), '$.seed[' + str(i) + ']')
        for name, method, path, status, data, version, role in cases:
            record = {'name': name, 'expected_status': status, 'request': {'method': method, 'path': path, 'body': data, 'version': version, 'role': role},
                      'responses': {}, 'before': {}, 'after': {}, 'differences': []}
            report['cases'].append(record)
            for key, target in targets.items():
                record['before'][key] = state(target)
                record['responses'][key] = target.call(method, path, data, {'OpenStack-API-Version': 'placement ' + version}, role)
                record['after'][key] = state(target)
                if record['responses'][key]['status'] != status:
                    record['differences'].append({'path': '$.expected_status.' + key, 'actual': record['responses'][key]['status'], 'expected': status})
            for phase in ['before', 'after']:
                for item in record[phase]['python']:
                    record['differences'] += base.differences(canonical(record[phase]['python'][item], left), canonical(record[phase]['go'][item], right), '$.' + phase + '.' + item)
            record['differences'] += base.differences(canonical(record['responses']['python'], left), canonical(record['responses']['go'], right), '$.response')
    except Exception as error:
        report['execution_error'] = type(error).__name__
    finally:
        for key, target in targets.items():
            for path, version in [(cp, '1.0'), (paths[0], '1.0'), (paths[1], '1.0'), (class_path, '1.7'), (trait_path, '1.6')]:
                if path not in eligible[key]:
                    continue
                try:
                    deleted = target.call('DELETE', path, headers={'OpenStack-API-Version': 'placement ' + version})
                    after = target.call('GET', path, headers={'OpenStack-API-Version': 'placement ' + version})
                    good = after['status'] == 404 if path != cp else json.loads(after['body_text']) == {'allocations': {}}
                    report['cleanup_ok'] &= deleted['status'] in (204, 404) and good
                    report['cleanup'].append({'target': key, 'path': path, 'delete': deleted, 'check': after})
                except Exception as error:
                    report['cleanup_ok'] = False
                    report['cleanup'].append({'target': key, 'error': type(error).__name__})
        report['summary'] = {'planned': len(cases), 'total': len(report['cases']), 'strict_equal': sum(not c['differences'] for c in report['cases'])}
        (output / 'report.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps({'summary': report['summary'], 'seed_differences': len(report['seed_differences']), 'cleanup_ok': report['cleanup_ok']}))
    return 0 if not report.get('execution_error') and not report['seed_differences'] and report['cleanup_ok'] and report['summary']['strict_equal'] == len(cases) else 1


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

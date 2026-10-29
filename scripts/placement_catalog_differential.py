#!/usr/bin/env python3
"""B08 live comparison. Only fresh CUSTOM catalogues and a fresh provider mutate.

Aggregate additions are covered by isolated MySQL: Placement HTTP cannot remove
orphan aggregate records from the original, so this live suite only writes [].
"""
import argparse
import json
from pathlib import Path
import sys
import uuid
from urllib.parse import quote

from placement_differential import Target, canonical, differences, read_token, request_headers


def normalized(response, target, owned, global_traits=False):
    out = canonical(response, target.origin)
    body = out['body']
    if isinstance(body, dict):
        for key in ('traits', 'aggregates'):
            if isinstance(body.get(key), list):
                # Lists are sets in the pinned upstream contract. Keep duplicates.
                body[key] = sorted(body[key])
        if isinstance(body.get('resource_classes'), list):
            body['resource_classes'] = sorted(
                (item for item in body['resource_classes']
                 if not item['name'].startswith('CUSTOM_') or item['name'] in owned),
                key=lambda item: item['name'])
        if global_traits and isinstance(body.get('traits'), list):
            body['traits'] = [name for name in body['traits']
                              if not name.startswith('CUSTOM_') or name in owned]
    return out


def run(left, right, output):
    output.mkdir(parents=True, exist_ok=False)
    uid = str(uuid.uuid4())
    prefix = 'CUSTOM_B08_' + uuid.uuid4().hex.upper()
    old, new, extra = (prefix + s for s in ('_OLD', '_NEW', '_EXTRA'))
    t1, t2, absent = (prefix + s for s in ('_T1', '_T2', '_ABSENT'))
    owned = {old, new, extra, t1, t2, absent}
    cp = lambda name: '/resource_classes/' + name
    tp = lambda name: '/traits/' + name
    path = '/resource_providers/' + uid
    targets = {'python': left, 'go': right}
    eligible = {key: [] for key in targets}
    report = {'schema': 1, 'provider_uuid': uid, 'owned_names': sorted(owned),
              'normalization': ['B07 request_id/origin/Vary normalization',
                                'sort trait/aggregate sets and class catalogue by name without deduplication',
                                'global catalogues exclude unrelated CUSTOM names; raw responses retained'],
              'limits': ['no positive aggregate creation on original (HTTP cannot delete global aggregate rows)',
                         'no allocations, candidates or Nova substitution'],
              'cases': [], 'preflight': {}, 'cleanup': [], 'cleanup_ok': True}
    cases = []

    def add(label, method, route, status, body=None, version='1.7', role='admin', generation=None):
        cases.append((label, method, route, status, body, version, role, generation))

    add('classes-standard', 'GET', '/resource_classes', 200, version='1.2')
    add('class-standard-show', 'GET', cp('VCPU'), 200, version='1.2')
    add('class-route-before-1.2', 'GET', '/resource_classes', 404, version='1.1')
    add('class-post-invalid-name', 'POST', '/resource_classes', 400, {'name': 'bad'}, '1.2')
    add('class-post', 'POST', '/resource_classes', 201, {'name': old}, '1.2')
    add('class-duplicate', 'POST', '/resource_classes', 409, {'name': old}, '1.2')
    add('class-rename-through-1.6', 'PUT', cp(old), 200, {'name': new}, '1.6')
    add('class-old-missing', 'GET', cp(old), 404)
    add('class-new-show', 'GET', cp(new), 200)
    add('class-ensure-existing-1.7', 'PUT', cp(new), 204)
    add('class-ensure-create-1.7', 'PUT', cp(extra), 201)
    add('class-ensure-idempotent-1.7', 'PUT', cp(extra), 204)
    add('class-rename-duplicate', 'PUT', cp(new), 409, {'name': extra}, '1.6')
    add('class-delete-unused', 'DELETE', cp(extra), 204)
    add('class-delete-missing', 'DELETE', cp(extra), 404)
    add('class-standard-rename-protected', 'PUT', cp('VCPU'), 400, {'name': extra}, '1.6')
    add('class-standard-delete-protected', 'DELETE', cp('VCPU'), 400)
    add('traits-standard', 'GET', '/traits', 200, version='1.6')
    add('trait-standard-show', 'GET', tp('OWNER_NOVA'), 204, version='1.6')
    add('traits-before-1.6', 'GET', '/traits', 404, version='1.5')
    add('trait-invalid-name', 'PUT', tp('bad'), 400, version='1.6')
    add('trait-create', 'PUT', tp(t1), 201, version='1.6')
    add('trait-idempotent', 'PUT', tp(t1), 204, version='1.6')
    add('trait-second-create', 'PUT', tp(t2), 201)
    add('trait-filter-in', 'GET', '/traits?name=in:' + t1 + ',' + t2, 200)
    add('trait-filter-prefix', 'GET', '/traits?name=startswith:' + prefix, 200)
    add('trait-bad-filter', 'GET', '/traits?name=invalid', 400)
    add('trait-bad-associated', 'GET', '/traits?associated=wrong', 400)
    add('trait-query-extra', 'GET', '/traits?extra=1', 400)
    add('trait-standard-delete-protected', 'DELETE', tp('OWNER_NOVA'), 400)
    add('provider-create', 'POST', '/resource_providers', 201, {'uuid': uid, 'name': 'tt-b08-' + uid}, generation=0)
    add('provider-links-1.1', 'GET', path, 200, version='1.1', generation=0)
    add('provider-links-1.6', 'GET', path, 200, version='1.6', generation=0)
    add('provider-traits-empty', 'GET', path + '/traits', 200, generation=0)
    add('provider-traits-before-1.6', 'GET', path + '/traits', 404, version='1.5', generation=0)
    add('provider-traits-set', 'PUT', path + '/traits', 200,
        {'resource_provider_generation': 0, 'traits': [t1, 'OWNER_NOVA']}, generation=1)
    add('provider-traits-noop', 'PUT', path + '/traits', 200,
        {'resource_provider_generation': 1, 'traits': ['OWNER_NOVA', t1]}, generation=1)
    add('provider-traits-stale-noop', 'PUT', path + '/traits', 409,
        {'resource_provider_generation': 0, 'traits': ['OWNER_NOVA', t1]}, generation=1)
    add('provider-traits-unknown', 'PUT', path + '/traits', 400,
        {'resource_provider_generation': 1, 'traits': [absent]}, generation=1)
    add('provider-traits-invalid-item', 'PUT', path + '/traits', 400,
        {'resource_provider_generation': 1, 'traits': [12]}, generation=1)
    add('provider-traits-missing-generation', 'PUT', path + '/traits', 400, {'traits': []}, generation=1)
    add('trait-delete-in-use', 'DELETE', tp(t1), 409, generation=1)
    add('trait-filter-associated', 'GET', '/traits?name=startswith:' + prefix + '&associated=true', 200, generation=1)
    add('trait-filter-unassociated', 'GET', '/traits?name=startswith:' + prefix + '&associated=false', 200, generation=1)
    add('provider-traits-service', 'GET', path + '/traits', 200, role='service', generation=1)
    add('provider-traits-reader', 'PUT', path + '/traits', 403,
        {'resource_provider_generation': 1, 'traits': []}, role='reader', generation=1)
    add('class-reader-forbidden', 'GET', '/resource_classes', 403, role='reader', generation=1)
    add('trait-missing-token', 'GET', '/traits', 401, role='missing', generation=1)
    add('aggregates-empty', 'GET', path + '/aggregates', 200, version='1.1', generation=1)
    add('aggregates-put-empty', 'PUT', path + '/aggregates', 200, [], '1.1', generation=1)
    add('aggregates-bad-uuid', 'PUT', path + '/aggregates', 400, ['bad'], '1.1', generation=1)
    add('aggregates-before-1.1', 'GET', path + '/aggregates', 404, version='1.0', generation=1)
    add('inventories-custom', 'PUT', path + '/inventories', 200,
        {'resource_provider_generation': 1, 'inventories': {new: {'total': 8, 'min_unit': 2, 'max_unit': 6, 'step_size': 2}}}, generation=2)
    add('class-rename-in-use', 'PUT', cp(new), 200, {'name': old}, '1.6', generation=2)
    add('class-rename-in-use-back', 'PUT', cp(old), 200, {'name': new}, '1.6', generation=2)
    add('class-delete-in-use', 'DELETE', cp(new), 409, generation=2)
    add('resources-hit', 'GET', '/resource_providers?uuid=' + uid + '&resources=' + new + ':4', 200, version='1.4', generation=2)
    add('resources-capacity-miss', 'GET', '/resource_providers?uuid=' + uid + '&resources=' + new + ':10', 200, generation=2)
    add('resources-step-miss', 'GET', '/resource_providers?uuid=' + uid + '&resources=' + new + ':3', 200, generation=2)
    add('resources-min-miss', 'GET', '/resource_providers?uuid=' + uid + '&resources=' + new + ':1', 200, generation=2)
    add('resources-unknown-class', 'GET', '/resource_providers?uuid=' + uid + '&resources=' + absent + ':1', 400, generation=2)
    add('resources-bad-amount', 'GET', '/resource_providers?resources=VCPU:bad', 400, generation=2)
    add('resources-before-1.4', 'GET', '/resource_providers?resources=VCPU:1', 400, version='1.3', generation=2)
    aggregate = str(uuid.uuid4())
    add('member-of-empty-result', 'GET', '/resource_providers?uuid=' + uid + '&member_of=' + aggregate, 200, version='1.3', generation=2)
    add('member-of-in-empty-result', 'GET', '/resource_providers?uuid=' + uid + '&member_of=in:' + aggregate, 200, generation=2)
    add('member-of-bad-uuid', 'GET', '/resource_providers?member_of=bad', 400, generation=2)
    add('member-of-repeated', 'GET', '/resource_providers?member_of=' + aggregate + '&member_of=' + aggregate, 400, generation=2)
    add('inventories-delete-before-1.5', 'DELETE', path + '/inventories', 405, version='1.4', generation=2)
    add('inventories-delete-1.5', 'DELETE', path + '/inventories', 204, version='1.5', generation=3)
    add('inventories-delete-empty', 'DELETE', path + '/inventories', 204, generation=4)
    add('traits-delete-provider-set', 'DELETE', path + '/traits', 204, generation=5)
    add('traits-delete-empty-noop', 'DELETE', path + '/traits', 204, generation=5)
    add('trait-delete-released', 'DELETE', tp(t1), 204, generation=5)
    add('trait-missing-after-delete', 'GET', tp(t1), 404, generation=5)
    add('class-delete-released', 'DELETE', cp(new), 204, generation=5)
    add('provider-delete', 'DELETE', path, 204)
    add('provider-traits-missing', 'GET', path + '/traits', 404)
    add('provider-traits-put-missing', 'PUT', path + '/traits', 404, {'resource_provider_generation': 0, 'traits': []})
    add('provider-traits-delete-missing', 'DELETE', path + '/traits', 404)
    add('aggregates-get-missing', 'GET', path + '/aggregates', 404)
    add('aggregates-provider-before-schema', 'PUT', path + '/aggregates', 404, {})
    add('provider-traits-schema-before-provider', 'PUT', path + '/traits', 400, {'traits': []})

    def state(target):
        headers = {'OpenStack-API-Version': 'placement 1.7'}
        return {key: target.call('GET', route, headers=headers) for key, route in
                [('provider', path), ('inventories', path + '/inventories'),
                 ('traits', path + '/traits'), ('aggregates', path + '/aggregates'),
                 ('classes-old', cp(old)), ('classes-new', cp(new)),
                 ('custom-traits', '/traits?name=startswith:' + prefix)]}

    try:
        for key, target in targets.items():
            report['preflight'][key] = {}
            for route in [path, cp(old), cp(new), cp(extra), tp(t1), tp(t2), tp(absent)]:
                response = target.call('GET', route, headers={'OpenStack-API-Version': 'placement 1.7'})
                report['preflight'][key][route] = response
                if response['status'] != 404:
                    raise RuntimeError('fresh resource unavailable; refusing mutations')
                eligible[key].append(route)
        for label, method, route, status, body, version, role, generation in cases:
            headers = {'OpenStack-API-Version': 'placement ' + version}
            record = {'name': label, 'request': {'method': method, 'path': route, 'body': body,
                      'headers': request_headers(body, headers), 'auth_role': role},
                      'expected_status': status, 'responses': {}, 'before': {}, 'after': {}, 'differences': []}
            report['cases'].append(record)
            for key, target in targets.items():
                record['before'][key] = state(target)
                record['responses'][key] = target.call(method, route, body, headers, role)
                record['after'][key] = state(target)
                if record['responses'][key]['status'] != status:
                    record['differences'].append({'path': '$.expected_status.' + key, 'expected': status,
                                                   'actual': record['responses'][key]['status']})
                if generation is not None:
                    actual = json.loads(record['after'][key]['provider']['body_text']).get('generation')
                    if actual != generation:
                        record['differences'].append({'path': '$.expected_generation.' + key,
                                                       'expected': generation, 'actual': actual})
            for phase in ('before', 'after'):
                for item in record[phase]['python']:
                    record['differences'] += differences(
                        normalized(record[phase]['python'][item], left, owned),
                        normalized(record[phase]['go'][item], right, owned), '$.' + phase + '.' + item)
            record['differences'] += differences(normalized(record['responses']['python'], left, owned, route == '/traits'),
                                                  normalized(record['responses']['go'], right, owned, route == '/traits'), '$.response')
            record['same_status'] = record['responses']['python']['status'] == record['responses']['go']['status']
    except Exception as error:
        report['execution_error'] = type(error).__name__
    finally:
        for key, target in targets.items():
            for route in [path, cp(old), cp(new), cp(extra), tp(t1), tp(t2)]:
                if route not in eligible[key]:
                    continue
                try:
                    response = target.call('DELETE', route, headers={'OpenStack-API-Version': 'placement 1.7'})
                    check = target.call('GET', route, headers={'OpenStack-API-Version': 'placement 1.7'})
                    ok = response['status'] in (204, 404) and check['status'] == 404
                    report['cleanup'].append({'target': key, 'path': route, 'delete': response, 'check': check})
                    report['cleanup_ok'] &= ok
                except Exception as error:
                    report['cleanup_ok'] = False
                    report['cleanup'].append({'target': key, 'path': route, 'error': type(error).__name__})
        count = len(report['cases'])
        report['summary'] = {'total': count, 'strict_equal': sum(not item['differences'] for item in report['cases']),
                             'status_equal': sum(item['same_status'] for item in report['cases']),
                             'planned': len(cases)}
        (output / 'report.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps({'summary': report['summary'], 'cleanup_ok': report['cleanup_ok']}))
    return 0 if not report.get('execution_error') and report['cleanup_ok'] and count == len(cases) and report['summary']['strict_equal'] == count else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--python-url', required=True)
    parser.add_argument('--go-url', required=True)
    parser.add_argument('--output', type=Path, required=True)
    for role in ('admin', 'service', 'reader'):
        parser.add_argument('--' + role + '-token-file', required=True)
    parser.add_argument('--allow-mutations', action='store_true')
    args = parser.parse_args()
    if not args.allow_mutations:
        parser.error('explicit --allow-mutations required')
    tokens = {role: read_token(getattr(args, role + '_token_file')) for role in ('admin', 'service', 'reader')}
    try:
        return run(Target(args.python_url, tokens), Target(args.go_url, tokens), args.output)
    except Exception as error:
        print('comparison failed: ' + type(error).__name__, file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())

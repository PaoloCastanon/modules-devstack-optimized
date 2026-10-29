#!/usr/bin/env python3
"""B12 real-token policy comparisons on fresh, reversible Placement resources."""
import argparse
import json
from pathlib import Path
import sys
import uuid
import placement_differential as base
from placement_consumer_differential import canonical as dates


def run(left, right, output, project, user, phase='policies'):
    output.mkdir(parents=True, exist_ok=False)
    provider, consumer = str(uuid.uuid4()), str(uuid.uuid4())
    prefix = 'CUSTOM_B12_' + uuid.uuid4().hex.upper()
    pp, cp = '/resource_providers/' + provider, '/allocations/' + consumer
    class_path, extra_class, trait_path = '/resource_classes/' + prefix, '/resource_classes/' + prefix + '_EXTRA', '/traits/' + prefix
    owned = {prefix, prefix + '_EXTRA'}
    targets = {'python': left, 'go': right}
    eligible = {key: [] for key in targets}
    report = {'schema': 1, 'phase': phase, 'provider': provider, 'consumer': consumer, 'class': prefix,
              'cases': [], 'cleanup': [], 'cleanup_ok': True, 'normalization': ['B07 request_id/origin/Vary', 'validated HTTP dates', 'B08 global catalogues exclude unrelated CUSTOM, sets sorted without deduplication']}

    def canonical(response, target):
        out = dates(response, target)
        body = out['body']
        if isinstance(body, dict):
            if isinstance(body.get('resource_classes'), list):
                body['resource_classes'] = sorted([item for item in body['resource_classes'] if not item['name'].startswith('CUSTOM_') or item['name'] in owned], key=lambda item: item['name'])
            if isinstance(body.get('traits'), list):
                body['traits'] = sorted([name for name in body['traits'] if not name.startswith('CUSTOM_') or name in owned])
        return out

    def call(target, method, path, data=None, version='1.36', role='admin', headers=None):
        selected = {'OpenStack-API-Version': 'placement ' + version}
        selected.update(headers or {})
        return target.call(method, path, data, selected, role)

    def state(target):
        return {label: call(target, 'GET', path, version=version) for label, path, version in
                [('provider', pp, '1.14'), ('inventories', pp + '/inventories', '1.8'), ('consumer', cp, '1.36'),
                 ('traits', pp + '/traits', '1.6'), ('class', class_path, '1.7'), ('extra-class', extra_class, '1.7'), ('trait', trait_path, '1.6')]}

    def record(label, role, method, path, data, version, status, unchanged=False, headers=None):
        item = {'name': label, 'request': {'role': role, 'method': method, 'path': path, 'body': data, 'version': version, 'extra_header_names': list((headers or {}).keys())},
                'expected_status': status, 'before': {}, 'responses': {}, 'after': {}, 'differences': []}
        report['cases'].append(item)
        for key, target in targets.items():
            item['before'][key] = state(target)
            item['responses'][key] = call(target, method, path, data, version, role, headers)
            item['after'][key] = state(target)
            if item['responses'][key]['status'] != status:
                item['differences'].append({'path': '$.expected_status.' + key, 'expected': status, 'actual': item['responses'][key]['status']})
            if unchanged:
                for name in item['before'][key]:
                    item['differences'] += base.differences(canonical(item['before'][key][name], target), canonical(item['after'][key][name], target), '$.unchanged.' + key + '.' + name)
        for where in ['before', 'after']:
            for name in item[where]['python']:
                item['differences'] += base.differences(canonical(item[where]['python'][name], left), canonical(item[where]['go'][name], right), '$.' + where + '.' + name)
        item['differences'] += base.differences(canonical(item['responses']['python'], left), canonical(item['responses']['go'], right), '$.response')

    try:
        for key, target in targets.items():
            for path, version in [(pp, '1.14'), (cp, '1.36'), (class_path, '1.7'), (extra_class, '1.7'), (trait_path, '1.6')]:
                response = call(target, 'GET', path, version=version)
                if path != cp and response['status'] != 404 or path == cp and json.loads(response['body_text']) != {'allocations': {}}:
                    raise RuntimeError('fresh resource occupied')
                eligible[key].append(path)
            assert call(target, 'POST', '/resource_providers', {'uuid': provider, 'name': 'tt-b12-' + provider, 'parent_provider_uuid': None}, '1.20')['status'] == 200
            assert call(target, 'PUT', pp + '/inventories', {'resource_provider_generation': 0, 'inventories': {'VCPU': {'total': 8}}}, '1.26')['status'] == 200
            assert call(target, 'PUT', pp + '/traits', {'resource_provider_generation': 1, 'traits': ['OWNER_NOVA']}, '1.6')['status'] == 200
            assert call(target, 'PUT', class_path, version='1.7')['status'] == 201
            assert call(target, 'PUT', trait_path, version='1.6')['status'] == 201
            assert call(target, 'PUT', cp, {'consumer_generation': None, 'project_id': project, 'user_id': user, 'allocations': {provider: {'resources': {'VCPU': 2}}}})['status'] == 204
        if phase == 'policies':
            reads = [('provider', pp, '1.14'), ('providers', '/resource_providers?uuid=' + provider, '1.18'),
                     ('inventories', pp + '/inventories', '1.26'), ('inventory', pp + '/inventories/VCPU', '1.26'),
                     ('traits', '/traits', '1.6'), ('trait', trait_path, '1.6'), ('provider-traits', pp + '/traits', '1.6'),
                     ('classes', '/resource_classes', '1.7'), ('class', class_path, '1.7'), ('aggregates', pp + '/aggregates', '1.19'),
                     ('consumer', cp, '1.28'), ('provider-allocations', pp + '/allocations', '1.28'), ('usages', pp + '/usages', '1.0'),
                     ('candidates', '/allocation_candidates?resources=VCPU:1&in_tree=' + provider + '&root_required=OWNER_NOVA', '1.36')]
            for role in ['nova', 'service', 'admin']:
                for label, path, version in reads:
                    record(role + '-read-' + label, role, 'GET', path, None, version, 204 if label == 'trait' else 200, True)
            generation = json.loads(call(left, 'GET', pp, version='1.14')['body_text'])['generation']
            record('nova-updates-inventory', 'nova', 'PUT', pp + '/inventories', {'resource_provider_generation': generation, 'inventories': {'VCPU': {'total': 8}}}, '1.26', 200)
            cg = json.loads(call(left, 'GET', cp)['body_text'])['consumer_generation']
            record('nova-updates-allocation', 'nova', 'PUT', cp, {'consumer_generation': cg, 'project_id': project, 'user_id': user, 'allocations': {provider: {'resources': {'VCPU': 2}}}}, '1.36', 204)
            generation = json.loads(call(left, 'GET', pp, version='1.14')['body_text'])['generation']
            cg = json.loads(call(left, 'GET', cp)['body_text'])['consumer_generation']
            writes = [('provider-post', 'POST', '/resource_providers', {'uuid': provider, 'name': 'tt-b12-' + provider}, '1.20'),
                      ('provider-put', 'PUT', pp, {'name': 'forbidden-rename'}, '1.14'), ('provider-delete', 'DELETE', pp, None, '1.0'),
                      ('inventories-put', 'PUT', pp + '/inventories', {'resource_provider_generation': generation, 'inventories': {'VCPU': {'total': 10}}}, '1.26'),
                      ('inventories-delete', 'DELETE', pp + '/inventories', None, '1.26'),
                      ('trait-put', 'PUT', trait_path, None, '1.6'), ('trait-delete', 'DELETE', trait_path, None, '1.6'),
                      ('provider-traits-put', 'PUT', pp + '/traits', {'resource_provider_generation': generation, 'traits': []}, '1.6'),
                      ('provider-traits-delete', 'DELETE', pp + '/traits', None, '1.6'),
                      ('class-post', 'POST', '/resource_classes', {'name': prefix + '_EXTRA'}, '1.7'),
                      ('class-put', 'PUT', class_path, None, '1.7'), ('class-delete', 'DELETE', class_path, None, '1.7'),
                      ('aggregates-put', 'PUT', pp + '/aggregates', {'resource_provider_generation': generation, 'aggregates': []}, '1.19'),
                      ('consumer-put', 'PUT', cp, {'consumer_generation': cg, 'project_id': project, 'user_id': user, 'allocations': {provider: {'resources': {'VCPU': 4}}}}, '1.36'),
                      ('consumer-delete', 'DELETE', cp, None, '1.0')]
            for role in ['reader', 'member', 'unscoped']:
                for label, path, version in reads:
                    record(role + '-denied-' + label, role, 'GET', path, None, version, 403, True)
                for label, method, path, data, version in writes:
                    record(role + '-denied-' + label, role, method, path, data, version, 403, True)
            record('forged-identity-headers', 'reader', 'GET', pp, None, '1.14', 403, True, {'X-Roles': 'admin,service', 'X-Project-Id': 'forged-project', 'X-User-Id': 'forged-user', 'X-Is-Admin-Project': 'true'})
            record('missing-token', 'missing', 'GET', pp, None, '1.14', 401, True)
            record('invalid-token', 'invalid', 'GET', pp, None, '1.14', 401, True)
        else:
            role = 'revoked' if phase == 'revoked' else 'expired'
            status = 200 if phase == 'valid-expiring' else 401
            for label, path, version in [('provider', pp, '1.14'), ('consumer', cp, '1.28'), ('candidates', '/allocation_candidates?resources=VCPU:1&in_tree=' + provider, '1.36')]:
                record(phase + '-' + label, role, 'GET', path, None, version, status, True)
    except Exception as error:
        report['execution_error'] = type(error).__name__
    finally:
        for key, target in targets.items():
            for path, version in [(cp, '1.0'), (pp, '1.0'), (class_path, '1.7'), (extra_class, '1.7'), (trait_path, '1.6')]:
                if path not in eligible[key]:
                    continue
                try:
                    response = call(target, 'DELETE', path, version=version)
                    check = call(target, 'GET', path, version=version)
                    good = check['status'] == 404 if path != cp else json.loads(check['body_text']) == {'allocations': {}}
                    report['cleanup_ok'] &= response['status'] in (204, 404) and good
                    report['cleanup'].append({'target': key, 'path': path, 'delete': response, 'check': check})
                except Exception as error:
                    report['cleanup_ok'] = False
                    report['cleanup'].append({'target': key, 'error': type(error).__name__})
        report['summary'] = {'total': len(report['cases']), 'strict_equal': sum(not c['differences'] for c in report['cases'])}
        (output / 'report.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps({'summary': report['summary'], 'cleanup_ok': report['cleanup_ok']}))
    return 0 if not report.get('execution_error') and report['cleanup_ok'] and report['cases'] and report['summary']['strict_equal'] == len(report['cases']) else 1


def main():
    p = argparse.ArgumentParser(description=__doc__)
    for flag in ['python-url', 'go-url', 'project-id', 'user-id']:
        p.add_argument('--' + flag, required=True)
    p.add_argument('--tokens-dir', type=Path, required=True)
    p.add_argument('--phase', choices=['policies', 'revoked', 'valid-expiring', 'expired'], default='policies')
    p.add_argument('--output', type=Path, required=True)
    p.add_argument('--allow-mutations', action='store_true')
    args = p.parse_args()
    if not args.allow_mutations:
        p.error('explicit mutation opt-in required')
    roles = ['admin', 'service', 'reader', 'member', 'unscoped', 'nova'] if args.phase == 'policies' else ['admin', 'revoked' if args.phase == 'revoked' else 'expired']
    tokens = {role: base.read_token(args.tokens_dir / (role + '.token')) for role in roles}
    try:
        return run(base.Target(args.python_url, tokens), base.Target(args.go_url, tokens), args.output, args.project_id, args.user_id, args.phase)
    except Exception as error:
        print(type(error).__name__, file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())

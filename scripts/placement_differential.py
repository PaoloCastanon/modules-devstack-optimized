#!/usr/bin/env python3
"""Compare selected B07 operations on running Python and Go Placement services.

Mutates only fresh, explicitly named providers created by this run. Tokens are
read from private files and never included in requests/evidence written to disk.
A strict difference is a nonzero exit, including error detail/header differences.
"""
import argparse
import copy
import json
import os
from pathlib import Path
import stat
import sys
import uuid
from decimal import Decimal
from urllib.error import HTTPError
from urllib.parse import quote, urlsplit
from urllib.request import Request, build_opener, HTTPRedirectHandler

HEADERS = ('content-type', 'openstack-api-version', 'vary', 'allow',
           'location', 'www-authenticate')


def read_token(path):
    with open(path, 'r', encoding='utf-8') as source:
        info = os.fstat(source.fileno())
        if not stat.S_ISREG(info.st_mode) or info.st_mode & 0o077 or info.st_size > 16385:
            raise ValueError('token must be a private regular file <=16 KiB')
        token = source.read().strip()
    if not token or len(token) > 16384 or any(ord(c) <= 32 or ord(c) >= 127 for c in token):
        raise ValueError('invalid token file')
    return token


def request_headers(body, overrides):
    selected = {'Accept': 'application/json', 'OpenStack-API-Version': 'placement 1.0'}
    if body is not None:
        selected['Content-Type'] = 'application/json'
    selected.update(overrides or {})
    return selected


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class Target:
    def __init__(self, base, tokens):
        url = urlsplit(base)
        if url.scheme not in ('http', 'https') or not url.hostname or url.username or url.password or url.query or url.fragment:
            raise ValueError('expected an HTTP(S) base without credentials/query/fragment')
        self.base = base.rstrip('/')
        self.origin = (url.scheme, url.netloc)
        self.tokens = tokens
        self.client = build_opener(NoRedirect())

    def call(self, method, path, body=None, headers=None, role='admin'):
        selected = request_headers(body, headers)
        data = None if body is None else (body if isinstance(body, str) else json.dumps(body)) .encode('utf-8')
        # selected is also the saved header allowlist: never put credentials in it.
        outgoing = dict(selected)
        if role != 'missing':
            outgoing['X-Auth-Token'] = self.tokens[role] if role != 'invalid' else 'b07-deliberately-invalid-token'
        request = Request(self.base + path, data=data, headers=outgoing, method=method)
        try:
            response = self.client.open(request, timeout=15)
        except HTTPError as error:
            response = error
        with response:
            text = response.read(2 * 1024 * 1024 + 1).decode('utf-8')
            if len(text.encode('utf-8')) > 2 * 1024 * 1024:
                raise ValueError('response exceeded comparison limit')
            # Defend against an upstream unexpectedly echoing a real credential.
            if any(token and token in text for token in self.tokens.values()):
                raise ValueError('response contained a credential; evidence suppressed')
            saved_headers = {key: response.headers[key] for key in HEADERS if key in response.headers}
            if any(token and token in value for token in self.tokens.values() for value in saved_headers.values()):
                raise ValueError('response header contained a credential; evidence suppressed')
            return {'status': response.status, 'headers': saved_headers, 'body_text': text}


def canonical(response, origin):
    out = copy.deepcopy(response)
    text = out.pop('body_text')
    try:
        body = json.loads(text, parse_float=Decimal)
    except (ValueError, TypeError):
        body = text
    if isinstance(body, dict) and isinstance(body.get('errors'), list):
        for error in body['errors']:
            if isinstance(error, dict) and isinstance(error.get('request_id'), str):
                error['request_id'] = '<request-id>'
    out['body'] = body
    headers = out['headers']
    if 'location' in headers:
        parsed = urlsplit(headers['location'])
        if (parsed.scheme, parsed.netloc) == origin:
            headers['location'] = parsed.path + ('?' + parsed.query if parsed.query else '')
    if 'vary' in headers:
        headers['vary'] = ', '.join(sorted(part.strip().lower() for part in headers['vary'].split(',')))
    return out


def kind(value):
    if value is None:
        return 'null'
    if isinstance(value, bool):
        return 'boolean'
    if isinstance(value, (int, float, Decimal)):
        return 'number'
    if isinstance(value, str):
        return 'string'
    if isinstance(value, dict):
        return 'object'
    if isinstance(value, list):
        return 'array'
    raise TypeError('unsupported comparison value')


def differences(left, right, path='$'):
    """No type coercion, numeric tolerance, generation/error normalization."""
    if kind(left) != kind(right):
        return [{'path': path, 'reason': 'type', 'python': kind(left), 'go': kind(right)}]
    out = []
    if isinstance(left, dict):
        for key in sorted(left.keys() | right.keys()):
            child = path + '.' + key
            if key not in left or key not in right:
                out.append({'path': child, 'reason': 'missing', 'python_present': key in left, 'go_present': key in right})
            else:
                out.extend(differences(left[key], right[key], child))
    elif isinstance(left, list):
        if len(left) != len(right):
            out.append({'path': path, 'reason': 'length', 'python': len(left), 'go': len(right)})
        for index, (a, b) in enumerate(zip(left, right)):
            out.extend(differences(a, b, f'{path}[{index}]'))
    elif left != right:
        out.append({'path': path, 'reason': 'value', 'python': str(left), 'go': str(right)})
    return out


def cases(provider, name):
    path = '/resource_providers/' + provider
    inventory = path + '/inventories'
    put = lambda value: {'resource_provider_generation': 1, 'inventories': {'VCPU': value}}
    return [
        ('create', 'none', 'POST', '/resource_providers', {'uuid': provider, 'name': name}, {}, 'admin'),
        ('show-service', 'provider', 'GET', path, None, {}, 'service'),
        ('list-uuid', 'provider', 'GET', '/resource_providers?uuid=' + provider, None, {}, 'admin'),
        ('list-name', 'provider', 'GET', '/resource_providers?name=' + quote(name), None, {}, 'admin'),
        ('rename', 'provider', 'PUT', path, {'name': name + '-renamed'}, {}, 'admin'),
        ('inventories-empty', 'provider', 'GET', inventory, None, {}, 'admin'),
        ('inventories-replace', 'inventory', 'PUT', inventory, put({'total': 8}), {}, 'admin'),
        ('inventory-show', 'inventory', 'GET', inventory + '/VCPU', None, {}, 'admin'),
        ('inventories-clear', 'inventory', 'PUT', inventory, {'resource_provider_generation': 1, 'inventories': {}}, {}, 'admin'),
        ('delete', 'inventory', 'DELETE', path, None, {}, 'admin'),
        ('duplicate-uuid', 'provider', 'POST', '/resource_providers', {'uuid': provider, 'name': name + '-other'}, {}, 'admin'),
        ('duplicate-name', 'provider', 'POST', '/resource_providers', {'uuid': str(uuid.UUID(int=uuid.UUID(provider).int ^ 1)), 'name': name}, {}, 'admin'),
        ('stale-generation', 'inventory', 'PUT', inventory, {'resource_provider_generation': 0, 'inventories': {}}, {}, 'admin'),
        ('inventory-missing', 'provider', 'GET', inventory + '/VCPU', None, {}, 'admin'),
        ('provider-missing', 'none', 'GET', path, None, {}, 'admin'),
        ('missing-name', 'none', 'POST', '/resource_providers', {'uuid': provider}, {}, 'admin'),
        ('null-name', 'none', 'POST', '/resource_providers', {'uuid': provider, 'name': None}, {}, 'admin'),
        ('unknown-field', 'none', 'POST', '/resource_providers', {'uuid': provider, 'name': name, 'extra': 1}, {}, 'admin'),
        ('rename-generation-field', 'provider', 'PUT', path, {'name': name, 'generation': 0}, {}, 'admin'),
        ('reserved-total', 'inventory', 'PUT', inventory, put({'total': 4, 'reserved': 4}), {}, 'admin'),
        ('zero-total', 'inventory', 'PUT', inventory, put({'total': 0}), {}, 'admin'),
        ('null-total', 'inventory', 'PUT', inventory, put({'total': None}), {}, 'admin'),
        ('unknown-class', 'inventory', 'PUT', inventory, {'resource_provider_generation': 1, 'inventories': {'CUSTOM_B07_UNKNOWN': {'total': 1}}}, {}, 'admin'),
        ('mixed-invalid', 'inventory', 'PUT', inventory, {'resource_provider_generation': 1, 'inventories': {'VCPU': {'total': 8}, 'MEMORY_MB': {'total': 0}}}, {}, 'admin'),
        ('integer-as-decimal', 'inventory', 'PUT', inventory, put({'total': 3.0}), {}, 'admin'),
        ('ratio-precision', 'inventory', 'PUT', inventory, put({'total': 8, 'allocation_ratio': 1.23456789}), {}, 'admin'),
        ('name-case-filter', 'provider', 'GET', '/resource_providers?name=' + quote(name.upper()), None, {}, 'admin'),
        ('name-trailing-space', 'provider', 'GET', '/resource_providers?name=' + quote(name + ' '), None, {}, 'admin'),
        ('duplicate-query', 'provider', 'GET', '/resource_providers?name=not-a-provider&name=' + quote(name), None, {}, 'admin'),
        ('uuid-case-lookup', 'provider', 'GET', '/resource_providers/' + provider.upper(), None, {}, 'admin'),
        ('uuid-compact-query', 'provider', 'GET', '/resource_providers?uuid=' + provider.replace('-', ''), None, {}, 'admin'),
        ('duplicate-name-case', 'provider', 'POST', '/resource_providers', {'uuid': str(uuid.UUID(int=uuid.UUID(provider).int ^ 1)), 'name': name.upper()}, {}, 'admin'),
        ('duplicate-name-space', 'provider', 'POST', '/resource_providers', {'uuid': str(uuid.UUID(int=uuid.UUID(provider).int ^ 1)), 'name': name + ' '}, {}, 'admin'),
        ('reader-forbidden', 'provider', 'GET', path, None, {}, 'reader'),
        ('missing-token', 'provider', 'GET', path, None, {}, 'missing'),
        ('invalid-token', 'provider', 'GET', path, None, {}, 'invalid'),
        ('unsupported-media', 'none', 'POST', '/resource_providers', {'uuid': provider, 'name': name}, {'Content-Type': 'text/plain'}, 'admin'),
        ('unacceptable', 'provider', 'GET', path, None, {'Accept': 'text/plain'}, 'admin'),
    ]


def snapshot(target, path):
    return {key: target.call('GET', path + suffix) for key, suffix in (('provider', ''), ('inventories', '/inventories'))}


def compare_responses(a, b, left, right, path):
    return differences(canonical(a, left.origin), canonical(b, right.origin), path)


def run(left, right, output):
    # Refuse overwrites, including evidence and artifacts from failed earlier runs.
    output.mkdir(parents=True, exist_ok=False)
    report = {'schema': 1, 'microversion': '1.0', 'normalization': ['error request_id values', 'configured origins in Location', 'case-insensitive Vary header tokens'], 'cases': [], 'cleanup': [], 'cleanup_ok': True}
    targets = {'python': left, 'go': right}
    for index in range(len(cases(str(uuid.uuid4()), 'unused'))):
        provider = str(uuid.uuid4())
        name = 'tt-b07-diff-' + provider
        label, seed, method, path, body, headers, role = cases(provider, name)[index]
        provider_path = '/resource_providers/' + provider
        record = {'name': label, 'provider_uuid': provider, 'request': {'method': method, 'path': path, 'body': body, 'headers': request_headers(body, headers), 'body_text': None if body is None else (body if isinstance(body, str) else json.dumps(body)), 'auth_role': role}, 'seed': {}, 'before': {}, 'responses': {}, 'after': {}, 'differences': []}
        eligible = {key: [] for key in targets}
        try:
            for key, target in targets.items():
                ids = [provider, str(uuid.UUID(int=uuid.UUID(provider).int ^ 1))] if label.startswith('duplicate-name') else [provider]
                for owned_id in ids:
                    preflight = target.call('GET', '/resource_providers/' + owned_id)
                    if preflight['status'] != 404:
                        raise RuntimeError('fresh provider UUID unavailable; mutation refused')
                    eligible[key].append(owned_id)
            for key, target in targets.items():
                if seed != 'none':
                    response = target.call('POST', '/resource_providers', {'uuid': provider, 'name': name})
                    record['seed'][key] = [response]
                    if response['status'] != 201:
                        raise RuntimeError('seed create failed')
                    if seed == 'inventory':
                        response = target.call('PUT', provider_path + '/inventories', {'resource_provider_generation': 0, 'inventories': {'VCPU': {'total': 4}}})
                        record['seed'][key].append(response)
                        if response['status'] != 200:
                            raise RuntimeError('seed inventory failed')
                record['before'][key] = snapshot(target, provider_path)
            for key, target in targets.items():
                record['responses'][key] = target.call(method, path, body, headers, role)
                record['after'][key] = snapshot(target, provider_path)
            record['differences'].extend(compare_responses(record['responses']['python'], record['responses']['go'], left, right, '$.response'))
            for phase in ['before', 'after']:
                for item in ['provider', 'inventories']:
                    record['differences'].extend(compare_responses(record[phase]['python'][item], record[phase]['go'][item], left, right, '$.' + phase + '.' + item))
            record['same_status'] = record['responses']['python']['status'] == record['responses']['go']['status']
        except Exception as error:
            # Raw transport/SDK errors may contain request headers: save only type.
            record['execution_error'] = type(error).__name__
            record['same_status'] = False
        finally:
            # IDs are generated by us; never discover/delete unrelated providers.
            # A duplicate-name probe may create the XOR ID on an incorrect server.
            for key, target in targets.items():
                for owned_id in eligible[key]:
                    try:
                        deletion = target.call('DELETE', '/resource_providers/' + owned_id)
                        verification = target.call('GET', '/resource_providers/' + owned_id)
                        ok = deletion['status'] in (204, 404) and verification['status'] == 404
                    except Exception:
                        ok = False
                    report['cleanup'].append({'target': key, 'uuid': owned_id, 'verified_absent': ok})
                    report['cleanup_ok'] &= ok
            record['same_state'] = 'execution_error' not in record and not any(d['path'].startswith(('$.before.', '$.after.')) for d in record['differences'])
            record['strict_match'] = not record['differences'] and 'execution_error' not in record
            report['cases'].append(record)
            (output / (f'{index + 1:02d}-' + label + '.json')).write_text(json.dumps(record, indent=2) + '\n')
            report['summary'] = {'cases': len(report['cases']), 'same_status': sum(c['same_status'] for c in report['cases']), 'strict_match': sum(c['strict_match'] for c in report['cases']), 'same_state': sum(c['same_state'] for c in report['cases'])}
            (output / 'report.json').write_text(json.dumps(report, indent=2) + '\n')
    return report


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--python-url', required=True)
    parser.add_argument('--go-url', required=True)
    for role in ['admin', 'service', 'reader']:
        parser.add_argument('--' + role + '-token-file', required=True, type=Path)
    parser.add_argument('--output', required=True, type=Path)
    parser.add_argument('--allow-mutations', action='store_true', required=True)
    args = parser.parse_args()
    try:
        tokens = {role: read_token(getattr(args, role + '_token_file')) for role in ['admin', 'service', 'reader']}
        left, right = Target(args.python_url, tokens), Target(args.go_url, tokens)
        if left.base == right.base:
            raise ValueError('comparison requires different endpoints')
        report = run(left, right, args.output)
    except Exception as error:
        print('Comparison could not complete: ' + type(error).__name__, file=sys.stderr)
        return 2
    print(json.dumps(report['summary'], sort_keys=True))
    print('cleanup verified:', report['cleanup_ok'])
    return 0 if report['cleanup_ok'] and report['summary']['strict_match'] == report['summary']['cases'] else 1


if __name__ == '__main__':
    sys.exit(main())

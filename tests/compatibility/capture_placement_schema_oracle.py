#!/usr/bin/env python3
"""Read-only schema oracle for the pinned laboratory venv.

Run with the deployed Placement Python; stdout is a sanitized fixture containing
only synthetic requests and validation errors. No database, identity or service
mutations occur. This capture is explicit and is not part of make check.
"""
import importlib.metadata
import pathlib
import subprocess
import sys
import json
from placement import util
from placement.schemas import resource_provider as r, inventory as i
from webob import Request
upstream = subprocess.check_output(['git', '-C', str(pathlib.Path(util.__file__).resolve().parent.parent), 'rev-parse', 'HEAD'], text=True).strip()
if upstream != 'f4a89d3803cb23df22b8714569889ab39c918c34':
    raise SystemExit('Oracle requires the pinned Placement revision')
if importlib.metadata.version('jsonschema') != '4.26.0' or importlib.metadata.version('WebOb') != '1.8.11':
    raise SystemExit('Oracle requires the inspected jsonschema/WebOb versions')
schemas = {'create': r.POST_RESOURCE_PROVIDER_SCHEMA, 'rename': r.PUT_RESOURCE_PROVIDER_SCHEMA, 'inventories': i.PUT_INVENTORY_SCHEMA, 'query': r.GET_RPS_SCHEMA_1_0}
cases = []

def add(name, schema, body):
    text = body if isinstance(body, str) else json.dumps(body, ensure_ascii=True)
    try:
        if schema == 'query':
            from urllib.parse import urlencode
            req = Request.blank('/?' + urlencode(json.loads(text)), headers={'Accept': 'application/json'})
            original_query = req
            util.validate_query_params(req, schemas[schema])
        else:
            util.extract_json(text.encode(), schemas[schema])
    except Exception as e:
        if not hasattr(e, 'status_int'):
            raise
        req = Request.blank('/', headers={'Accept': 'application/json'})
        # Match PlacementWsgify.call_func, including query errors.
        e.json_formatter = util.json_error_formatter
        response = req.get_response(e)
        result = {'name': name, 'schema': schema, 'body': text, 'status': response.status_int, 'response': json.loads(response.text)}
    else:
        result = {'name': name, 'schema': schema, 'body': text, 'status': 200}
    result['instance'] = dict(req.GET) if schema == 'query' and result['status'] == 200 else dict(original_query.GET) if schema == 'query' else json.loads(text)
    cases.append(result)
for schema in ['create', 'rename']:
    for field, value in [('name', None), ('name', True), ('name', 3.1), ('name', []), ('name', {}), ('name', '界' * 201), ('name', 'word ' * 45), ('name', 'can\'t "quote"\n' * 18)]:
        add(schema + '-' + str(len(cases)), schema, {field: value})
    add(schema + '-missing', schema, {})
    add(schema + '-extra', schema, {'uuid': '4446c55f-ee69-4f16-b6c0-b562d5f200ab', 'name': 'custom-name', 'z': True, 'a': None})
    add(schema + '-string', schema, {'name': '"both\' quotes\n tab\t𐐀', 'extra': [1, True, False, None, {'nested': 'hello world ' * 8}]})
for uuid in ['bad', '4446c55fee694f16b6c0b562d5f200ab', '4446C55F-EE69-4F16-B6C0-B562D5F200AB', None, False]:
    add('uuid-' + str(len(cases)), 'create', {'name': 'host', 'uuid': uuid})
    add('query-' + str(len(cases)), 'query', {'uuid': uuid})
for field, values in {'total': [None, False, '4', [], {}, 0, -1, 2147483648, 1.5], 'reserved': [-1, 2147483648, True], 'min_unit': [0, 1.5], 'max_unit': [0], 'step_size': [0], 'allocation_ratio': ['1', True, 3.40283e+38]}.items():
    for value in values:
        inv = {'total': 4, field: value}
        add(field + '-' + str(len(cases)), 'inventories', {'resource_provider_generation': 0, 'inventories': {'MEMORY_MB': inv}})
add('missing-total', 'inventories', {'resource_provider_generation': 0, 'inventories': {'VCPU': {}}})
add('missing-generation', 'inventories', {'inventories': {}})
add('missing-both', 'inventories', {})
add('wrong-inventories', 'inventories', {'resource_provider_generation': 0, 'inventories': []})
add('extra-inventory', 'inventories', {'resource_provider_generation': 0, 'inventories': {'VCPU': {'total': 2, 'extra': 'foo'}}})
add('two-invalid', 'inventories', {'resource_provider_generation': 0, 'inventories': {'VCPU': {'total': 0}, 'MEMORY_MB': {'total': None}}})
add('two-invalid-reversed', 'inventories', {'inventories': {'MEMORY_MB': {'reserved': False, 'total': None}, 'VCPU': {'total': 0}}, 'resource_provider_generation': 0})
add('extra-root', 'inventories', {'inventories': {'VCPU': {'total': 0}}, 'resource_provider_generation': 0, 'extra': 1})
add('unknown-query', 'query', {'name': 'word ' * 25, 'unknown': 'true', 'z': 'a'})
add('duplicate-member', 'create', '{"name":null,"name":"accepted","uuid":"4446c55fee694f16b6c0b562d5f200ab"}')
print(json.dumps({'source': 'Placement f4a89d3 util.extract_json/validate_query_params, jsonschema 4.26.0, WebOb 1.8.11; no HTTP service mutation', 'cases': cases}, indent=2))

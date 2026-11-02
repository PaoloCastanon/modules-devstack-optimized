#!/usr/bin/env python3
"""B14 real Nova boot and allocation release while Placement Go is active.

Run inside the VM with its admin openrc, a service token file, explicit resource
IDs and a NEW output directory. It deletes only the fresh server named by this run.
"""
import argparse
import datetime
import json
import logging
import os
from pathlib import Path
import subprocess
import sys
import time
from urllib.request import HTTPRedirectHandler, Request, build_opener
import uuid


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def run(args):
    if not args.allow_mutations:
        raise ValueError('explicit mutation opt-in required')
    args.output.mkdir(parents=True, exist_ok=False)
    if args.token_file.stat().st_mode & 0o077:
        raise ValueError('private service token required')
    token = args.token_file.read_text().strip()
    if not token or any(ord(c) <= 32 or ord(c) >= 127 for c in token):
        raise ValueError('invalid service token')
    logging.disable(logging.CRITICAL)
    from keystoneauth1 import session
    from keystoneauth1.identity import v3
    from openstack.connection import Connection

    def placement(path, version='1.36'):
        req = Request(args.placement_url.rstrip('/') + path,
                      headers={'X-Auth-Token': token, 'OpenStack-API-Version': 'placement ' + version})
        with build_opener(NoRedirect()).open(req, timeout=15) as response:
            body = response.read(2 * 1024 * 1024 + 1)
            if len(body) > 2 * 1024 * 1024 or token.encode() in body:
                raise ValueError('unsafe response')
            return json.loads(body)

    def checkpoint():
        (args.output / 'report.json').write_text(json.dumps(report, indent=2) + '\n')

    name = 'tt-placement-b14-' + uuid.uuid4().hex
    report = {'schema': 1, 'name': name, 'server_id': None, 'states': [],
              'provider_uuid': args.provider_id, 'image_id': args.image_id,
              'flavor_id': args.flavor_id, 'network_id': args.network_id,
              'boot_active': False, 'allocation_verified': False,
              'server_deleted': False, 'allocations_released': False,
              'usages_restored': False, 'execution_error': None, 'cleanup_error': None}
    checkpoint()
    if subprocess.run(['systemctl', 'is-active', '--quiet', 'devstack@placement-api.service']).returncode == 0:
        raise ValueError('original Placement must be stopped')
    discovery = placement('/', '1.0')
    if discovery['versions'][0]['max_version'] != '1.36':
        raise ValueError('Go route is not active')
    auth_url = os.environ['OS_AUTH_URL'].rstrip('/')
    if not auth_url.endswith('/v3'):
        auth_url += '/v3'
    auth = v3.Password(auth_url=auth_url, username=os.environ['OS_USERNAME'], password=os.environ['OS_PASSWORD'],
                       project_name=os.environ['OS_PROJECT_NAME'], user_domain_name=os.environ.get('OS_USER_DOMAIN_NAME', 'Default'),
                       project_domain_name=os.environ.get('OS_PROJECT_DOMAIN_NAME', 'Default'))
    connection = Connection(session=session.Session(auth=auth, timeout=20), region_name=os.environ.get('OS_REGION_NAME', 'RegionOne'))
    server = None
    base_usages = placement('/resource_providers/' + args.provider_id + '/usages', '1.0')
    report['usages_before'] = base_usages
    try:
        if list(connection.compute.servers(name=name, all_projects=True)):
            raise ValueError('fresh server name already exists')
        image = connection.image.get_image(args.image_id)
        flavor = connection.compute.get_flavor(args.flavor_id)
        network = connection.network.get_network(args.network_id)
        if image.status.lower() != 'active' or network.id != args.network_id or int(flavor.vcpus) < 1:
            raise ValueError('image/flavor/network preflight failed')
        expected = {'VCPU': int(flavor.vcpus), 'MEMORY_MB': int(flavor.ram), 'DISK_GB': int(flavor.disk)}
        report['expected_resources'] = expected
        checkpoint()
        server = connection.compute.create_server(name=name, image_id=args.image_id,
                                                  flavor_id=args.flavor_id, networks=[{'uuid': args.network_id}])
        report['server_id'] = server.id
        checkpoint()
        deadline = time.monotonic() + args.wait_seconds
        while time.monotonic() < deadline:
            current = connection.compute.get_server(server.id)
            report['states'].append({'timestamp_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(), 'status': current.status})
            checkpoint()
            if current.status == 'ACTIVE':
                report['boot_active'] = True
                break
            if current.status == 'ERROR':
                raise RuntimeError('server reached ERROR')
            time.sleep(2)
        if not report['boot_active']:
            raise TimeoutError('server did not reach ACTIVE')
        report['owner'] = {'project_id': current.project_id, 'user_id': current.user_id}
        allocations = placement('/allocations/' + server.id)
        report['allocations_active'] = allocations
        observed = allocations['allocations']
        if set(observed) != {args.provider_id} or observed[args.provider_id]['resources'] != expected:
            raise ValueError('allocation resources/provider mismatch')
        if allocations['project_id'] != current.project_id or allocations['user_id'] != current.user_id or type(allocations.get('consumer_generation')) is not int:
            raise ValueError('allocation owner/generation mismatch')
        report['allocation_verified'] = True
        active_usages = placement('/resource_providers/' + args.provider_id + '/usages', '1.0')
        report['usages_active'] = active_usages
        for resource, amount in expected.items():
            if active_usages['usages'].get(resource, 0) != base_usages['usages'].get(resource, 0) + amount:
                raise ValueError('provider usage mismatch')
        checkpoint()
    except Exception as error:
        report['execution_error'] = type(error).__name__
        checkpoint()
    finally:
        try:
            # Resolve an uncertain create by this run's fresh exact name only.
            if server is None:
                matches = [s for s in connection.compute.servers(name=name, all_projects=True) if s.name == name]
                if len(matches) > 1:
                    raise RuntimeError('ambiguous owned server')
                server = matches[0] if matches else None
            if server is not None:
                actual = connection.compute.get_server(server.id)
                if actual.name != name:
                    raise ValueError('server ownership mismatch')
                connection.compute.delete_server(server.id, ignore_missing=False)
                connection.compute.wait_for_delete(actual, interval=2, wait=args.wait_seconds)
                report['server_deleted'] = True
                deadline = time.monotonic() + 60
                while time.monotonic() < deadline:
                    freed = placement('/allocations/' + server.id, '1.0')
                    if freed == {'allocations': {}}:
                        report['allocations_released'] = True
                        report['allocations_after'] = freed
                        break
                    time.sleep(1)
                after = placement('/resource_providers/' + args.provider_id + '/usages', '1.0')
                report['usages_after'] = after
                report['usages_restored'] = after['usages'] == base_usages['usages']
            else:
                report['no_server_was_created'] = True
        except Exception as error:
            report['cleanup_error'] = type(error).__name__
        checkpoint()
        connection.close()
    good = all(report[k] for k in ['boot_active', 'allocation_verified', 'server_deleted', 'allocations_released', 'usages_restored']) and not report['execution_error'] and not report['cleanup_error']
    print(json.dumps({key: report[key] for key in ['server_id', 'boot_active', 'allocation_verified', 'server_deleted', 'allocations_released', 'usages_restored', 'execution_error', 'cleanup_error']}))
    return 0 if good else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ['provider-id', 'image-id', 'flavor-id', 'network-id']:
        parser.add_argument('--' + name, required=True)
    parser.add_argument('--placement-url', default='http://192.168.122.10/placement')
    parser.add_argument('--token-file', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--wait-seconds', type=int, default=180)
    parser.add_argument('--allow-mutations', action='store_true')
    args = parser.parse_args()
    if not 10 <= args.wait_seconds <= 300:
        parser.error('bounded wait of 10..300 seconds required')
    try:
        return run(args)
    except Exception as error:
        print('boot operation failed: ' + type(error).__name__, file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())

#!/usr/bin/env python3
"""Read-only preflight on the DevStack VM; no DSN, token or environment in output."""
import argparse
import configparser
import hashlib
import json
from pathlib import Path
import subprocess
import sys
from urllib.request import HTTPRedirectHandler, Request, build_opener


class NoRedirect(HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


def run(args):
    args.output.mkdir(mode=0o700, parents=True, exist_ok=False)
    info = args.token_file.stat()
    if info.st_mode & 0o077 or info.st_size > 16385:
        raise ValueError('private token file required')
    token = args.token_file.read_text().strip()

    def get(path, version):
        request = Request(args.url.rstrip('/') + path,
                          headers={'X-Auth-Token': token, 'OpenStack-API-Version': 'placement ' + version})
        with build_opener(NoRedirect()).open(request, timeout=15) as response:
            body = response.read(2 * 1024 * 1024 + 1)
            if len(body) > 2 * 1024 * 1024 or token.encode() in body:
                raise ValueError('unsafe response')
            return json.loads(body)

    def command(values):
        return subprocess.check_output(values, text=True, stderr=subprocess.DEVNULL).strip()

    cfg = configparser.RawConfigParser(strict=False)
    cfg.read('/etc/placement/placement.conf')
    selected = {}
    for section, keys in {'oslo_policy': ['enforce_scope', 'enforce_new_defaults', 'policy_file'],
                          'keystone_authtoken': ['auth_type', 'memcached_servers', 'token_cache_time', 'enforce_token_bind'],
                          'placement_database': ['max_pool_size', 'max_overflow']}.items():
        selected[section] = {key: cfg.get(section, key) for key in keys if cfg.has_option(section, key)}
    nova = configparser.RawConfigParser(strict=False)
    nova.read('/etc/nova/nova.conf')
    selected['nova_placement'] = {key: nova.get('placement', key) for key in ['auth_type', 'username', 'project_name', 'valid_interfaces', 'region_name'] if nova.has_option('placement', key)}
    group = command(['systemctl', 'show', 'devstack@placement-api.service', '--value', '-p', 'ControlGroup'])
    from sqlalchemy import create_engine, text
    engine = create_engine(cfg.get('placement_database', 'connection'))
    try:
        with engine.connect() as connection:
            database = dict(connection.execute(text('SELECT @@server_uuid AS server_uuid, @@version AS version, @@innodb_flush_log_at_trx_commit AS flush_log, @@sync_binlog AS sync_binlog, @@log_bin AS log_bin')).mappings().one())
    finally:
        engine.dispose()
    pids = [int(x) for x in (Path('/sys/fs/cgroup') / group.lstrip('/') / 'cgroup.procs').read_text().split()]
    report = {'schema': 1, 'providers': [], 'config': selected, 'original_mysql': database,
              'original_unit_active': command(['systemctl', 'is-active', 'devstack@placement-api.service']),
              'original_cgroup': group, 'original_pids': pids,
              'apache_site_sha256': hashlib.sha256(Path('/etc/apache2/sites-enabled/placement-api.conf').read_bytes()).hexdigest(),
              'versions': {name: command(['git', '-C', '/opt/stack/' + name, 'rev-parse', 'HEAD']) for name in ['placement', 'nova', 'glance']},
              'cpu_count': command(['nproc']),
              'mysql_version': command(['/usr/sbin/mysqld', '--no-defaults', '--version']),
              'discovery': get('/', '1.0')}
    for provider in get('/resource_providers', '1.14')['resource_providers']:
        prefix = '/resource_providers/' + provider['uuid']
        report['providers'].append({'provider': get(prefix, '1.14'),
                                    'inventories': get(prefix + '/inventories', '1.26'),
                                    'traits': get(prefix + '/traits', '1.6'),
                                    'aggregates': get(prefix + '/aggregates', '1.19'),
                                    'allocations': get(prefix + '/allocations', '1.36')})
    report['flat_providers'] = all(p['provider'].get('parent_provider_uuid') is None for p in report['providers'])
    report['no_provider_allocations'] = all(p['allocations']['allocations'] == {} for p in report['providers'])
    report['no_sharing'] = all('MISC_SHARES_VIA_AGGREGATE' not in p['traits']['traits'] for p in report['providers'])
    (args.output / 'report.json').write_text(json.dumps(report, indent=2) + '\n')
    print(json.dumps({'providers': len(report['providers']), 'flat': report['flat_providers'],
                      'no_allocations': report['no_provider_allocations'], 'no_sharing': report['no_sharing']}))
    return 0 if report['providers'] and report['flat_providers'] and report['no_provider_allocations'] and report['no_sharing'] else 1


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--url', required=True)
    parser.add_argument('--token-file', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    try:
        return run(args)
    except Exception as error:
        print('preflight failed: ' + type(error).__name__, file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())

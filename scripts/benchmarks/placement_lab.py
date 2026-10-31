#!/usr/bin/env python3
"""Private SSH JSON coordinator inside the VM, one exclusive lab session.

stdout is a private RPC channel. Only the token action returns a credential;
the host must store that reply privately, never as public evidence or logs.
"""
import argparse
import configparser
import contextlib
import hashlib
import io
import json
import logging
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import time
from types import SimpleNamespace
import uuid

HERE = Path(__file__).resolve().parent
sys.path[:0] = [str(HERE), str(HERE.parent / 'integration'), str(HERE.parent)]
import lab_identity
import placement_preflight
import placement_runtime
import placement_switch
from placement_suite import exclusive_run, require, save_json, sdk_connection
from placement_differential import Target, read_token


class Lab:
    def __init__(self, args):
        self.args = args
        self.root = args.work_root
        self.auth = self.root / 'tt-placement-auth'
        self.runtime = self.root / 'tt-placement-runtime'
        self.original = None
        self.current = None
        self.collector = None
        self.ledger = []
        self.targets = {}
        self.closed = False

    def action(self, label, module, **values):
        stream = io.StringIO()
        with contextlib.redirect_stdout(stream):
            result = module.run(SimpleNamespace(allow_mutations=True, **values))
        (self.args.output / (label + '.log')).write_text(stream.getvalue())
        require(result == 0, label)

    def controls(self, action):
        result = subprocess.run(['sudo', sys.executable, str(HERE / 'lab_controls.py'), action,
                                 '--state', str(self.root / 'maintenance-controls.json')], capture_output=True, text=True)
        require(result.returncode == 0, 'temporary lab controls ' + action)
        return json.loads(result.stdout)

    def call(self, implementation, method, path, body=None, version='1.36', statuses=(200,)):
        response = self.targets[implementation].call(method, path, body, {'OpenStack-API-Version': 'placement ' + version, 'Content-Type': 'application/json'})
        require(response['status'] in statuses, 'unexpected ' + method + ' status ' + str(response['status']))
        return json.loads(response['body_text']) if response['body_text'] else None

    def owners(self):
        from sqlalchemy import create_engine, text
        cfg = configparser.RawConfigParser(strict=False)
        cfg.read('/etc/placement/placement.conf')
        engine = create_engine(cfg.get('placement_database', 'connection'))
        try:
            with engine.connect() as connection:
                result = {table: sorted(connection.execute(text('SELECT external_id FROM ' + table)).scalars()) for table in ('projects', 'users')}
                result['mysql'] = dict(connection.execute(text('SELECT @@version AS version, @@innodb_buffer_pool_size AS buffer_pool, @@innodb_flush_log_at_trx_commit AS flush_log, @@sync_binlog AS sync_binlog, @@log_bin AS log_bin, @@max_connections AS max_connections')).mappings().one())
        finally:
            engine.dispose()
        return result

    def prepare(self):
        save_json(self.args.output / 'maintenance-controls-before.json', self.controls('enter'))
        self.connection, project, user = sdk_connection()
        require(list(self.connection.compute.servers(all_projects=True)) == [], 'idle lab required')
        self.original_owners = self.owners()
        self.original_mysql_profile_sha = hashlib.sha256(Path('/etc/apparmor.d/usr.sbin.mysqld').read_bytes()).hexdigest()
        require(project in self.original_owners['projects'] and user in self.original_owners['users'], 'reuse existing Placement owners')
        self.owner = {'project_id': project, 'user_id': user}
        self.action('identity-prepare', lab_identity, action='prepare', private_root=self.auth, application_lifetime_hours=24)
        self.action('original-preflight', placement_preflight, url=self.args.url, token_file=self.auth / 'service.token', output=self.args.output / 'original-before')
        self.original = json.loads((self.args.output / 'original-before/report.json').read_text())
        self.action('runtime-prepare', placement_runtime, action='prepare', root=self.runtime, auth_root=self.auth,
                    binary=self.args.binary, mysql_port=13316, listen_port=18779, public_url=self.args.url, keystone_url=self.args.keystone_url)
        self.runtime_state = json.loads((self.runtime / 'state.json').read_text())
        self.refresh()
        save_json(self.args.output / 'original-owners.json', self.original_owners)
        tracker = self.prepare_tracker()
        return {'original': self.original, 'runtime': json.loads((self.runtime / 'public-report.json').read_text()), 'owner': self.owner,
                'environment': self.environment(), 'tracker': tracker}

    def prepare_tracker(self):
        # Give both deployments the same idle real compute in addition to the
        # sixteen synthetic providers. Do not let its first publication happen
        # unpredictably inside an official load window.
        require(len(self.original['providers']) == 1, 'single flat idle compute required')
        original = self.original['providers'][0]
        ident = original['provider']['uuid']
        def restart_nova():
            subprocess.run(['sudo', 'systemctl', 'restart', 'devstack@n-cpu.service', 'devstack@n-sch.service'],
                           check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        def probe(label):
            result = subprocess.run([sys.executable, str(HERE.parent / 'integration/nova_candidates_probe.py'),
                                     '--provider-id', ident, '--output', str(self.args.output / label)], capture_output=True, text=True)
            (self.args.output / (label + '.log')).write_text(result.stdout + result.stderr)
            require(result.returncode == 0, 'real Nova client candidate probe')
        self.route('go')
        try:
            restart_nova()
            deadline = time.monotonic() + 120
            while time.monotonic() < deadline:
                response = self.targets['go'].call('GET', '/resource_providers/' + ident, headers={'OpenStack-API-Version': 'placement 1.14'})
                if response['status'] == 200:
                    state = self.snapshots('go', {'providers': [ident]})[ident]
                    aggregates = self.call('go', 'GET', '/resource_providers/' + ident + '/aggregates', version='1.19')
                    fields = [('inventories', 'inventories'), ('traits', 'traits'), ('allocations', 'allocations')]
                    if all(sorted(state[key][field]) == sorted(original[key][field]) if isinstance(state[key][field], list)
                           else state[key][field] == original[key][field] for key, field in fields):
                        require(sorted(aggregates['aggregates']) == sorted(original['aggregates']['aggregates']), 'compute aggregates equivalent')
                        require(all(state['provider'][key] == original['provider'][key] for key in ['uuid', 'name', 'root_provider_uuid', 'parent_provider_uuid']), 'same compute identity/root')
                        break
                time.sleep(1)
            else:
                raise TimeoutError('compute publication outside load window')
            probe('go-nova-probe')
            save_json(self.args.output / 'go-compute-before-load.json', state)
        finally:
            self.route('python')
            restart_nova()
        probe('python-nova-probe')
        return {'logical_compute_equivalent': True, 'provider_id': ident, 'original_generation': original['provider']['generation'],
                'go_generation': state['provider']['generation'], 'note': 'independent generations retained; tracker publication and restarts outside every load window'}

    def refresh(self):
        self.action('identity-refresh', lab_identity, action='refresh', private_root=self.auth)
        token = read_token(self.auth / 'service.token')
        self.targets = {'python': Target(self.args.url, {'admin': token}), 'go': Target('http://127.0.0.1:18779', {'admin': token})}
        return {'token': token, 'metadata': json.loads((self.auth / 'metadata.json').read_text())}

    def environment(self):
        from oslo_db import options as database_options
        from keystonemiddleware.auth_token import _opts as auth_options
        def cmd(values):
            return subprocess.check_output(values, text=True, stderr=subprocess.DEVNULL).strip()
        cfg = configparser.RawConfigParser(strict=False)
        cfg.read('/etc/placement/placement.conf')
        uwsgi = configparser.RawConfigParser(strict=False)
        uwsgi.read('/etc/placement/placement-uwsgi.ini')
        public = {'kernel': cmd(['uname', '-a']), 'lscpu': cmd(['lscpu']), 'memory': Path('/proc/meminfo').read_text(),
                  'python_sha': self.original['versions']['placement'], 'nova_sha': self.original['versions']['nova'],
                  'original_mysql': self.original_owners['mysql'], 'binary_sha256': hashlib.sha256(self.args.binary.read_bytes()).hexdigest(),
                  'go_build': cmd(['go', 'version', '-m', str(self.args.binary)]) if shutil.which('go') else 'host build manifest',
                  'selected_placement_config': self.original['config'],
                  'original_mysql_apparmor_sha256': self.original_mysql_profile_sha,
                  'go_runtime_settings': {'gomaxprocs': 'default; four VM CPUs; no override', 'sql_max_open_connections': 4,
                                          'sql_max_idle_connections': 2, 'auth_user_token_cache': False,
                                          'go_heap_gc_metrics': 'not exposed; total PSS measured'},
                  'python_registered_library_defaults': {
                      'oslo_db': {opt.name: opt.default for opt in database_options.database_opts if opt.name in ['max_pool_size', 'max_overflow']},
                      'keystonemiddleware': {opt.name: opt.default for opt in auth_options._OPTS if opt.name == 'token_cache_time'}},
                  'uwsgi': {key: uwsgi.get('uwsgi', key) for key in ['processes', 'threads', 'master', 'enable-threads', 'buffer-size', 'lazy-apps', 'thunder-lock', 'max-requests', 'harakiri'] if uwsgi.has_option('uwsgi', key)},
                  'systemd_limits': {unit: cmd(['systemctl', 'show', unit, '-p', 'ControlGroup', '-p', 'CPUQuotaPerSecUSec', '-p', 'MemoryMax', '-p', 'AllowedCPUs', '-p', 'TasksMax']) for unit in ['devstack@placement-api.service', self.runtime_state['go_unit']]}}
        # No connection strings or arbitrary config/environment are serialized.
        import pymysql
        dsn = (self.runtime / 'app.dsn').read_text().strip()
        match = re.fullmatch(r'([a-z0-9_]+):([a-f0-9]+)@tcp\(127\.0\.0\.1:([0-9]+)\)/(placement_go_test_[a-z0-9_]+)\?parseTime=true', dsn)
        require(match is not None, 'owned application DSN')
        name, password, port, schema = match.groups()
        with pymysql.connect(host='127.0.0.1', port=int(port), user=name, password=password, database=schema) as db:
            with db.cursor() as cur:
                cur.execute('SELECT @@server_uuid, @@version, @@port, DATABASE(), @@innodb_buffer_pool_size, @@innodb_flush_log_at_trx_commit, @@sync_binlog, @@log_bin, @@max_connections')
                values = cur.fetchone()
        require(values[0] != self.original['original_mysql']['server_uuid'], 'isolated application database')
        public['own_mysql'] = dict(zip(['server_uuid', 'version', 'port', 'schema', 'buffer_pool', 'flush_log', 'sync_binlog', 'log_bin', 'max_connections'], values))
        return public

    def route(self, implementation):
        active = (self.runtime / 'switch-state.json').exists() and json.loads((self.runtime / 'switch-state.json').read_text()).get('active')
        if active:
            self.action('restore-' + str(time.time_ns()), placement_switch, action='restore', root=self.runtime, public_url=self.args.url)
        if implementation == 'go':
            self.action('activate-' + str(time.time_ns()), placement_switch, action='activate', root=self.runtime,
                        expected_original_sha=self.original['apache_site_sha256'], public_url=self.args.url, lease_seconds=600)
        return {'implementation': implementation, 'original_active': placement_switch.active(placement_switch.ORIGINAL_UNIT),
                'site_sha256': hashlib.sha256(placement_switch.SITE_FILE.read_bytes()).hexdigest()}

    def seed(self, values):
        require(self.current is None, 'finish prior run first')
        save_json(self.args.output / (values['run_id'] + '-log-retention.json'), self.controls('bound'))
        self.current = values
        self.current['authorized_providers'] = []
        self.current['authorized_class'] = False
        self.ledger.append(values)
        save_json(self.root / 'ledger.json', self.ledger, private=True)
        impl = values['implementation']
        custom = values['custom']
        require(re.fullmatch(r'CUSTOM_TT_B17_[A-F0-9]{16}', custom), 'owned class')
        self.route(impl)
        require(self.targets[impl].call('GET', '/resource_classes/' + custom, headers={'OpenStack-API-Version': 'placement 1.2'})['status'] == 404, 'fresh resource class')
        self.current['authorized_class'] = True
        save_json(self.root / 'ledger.json', self.ledger, private=True)
        self.call(impl, 'PUT', '/resource_classes/' + custom, version='1.7', statuses=(201,))
        inventories = {name: {'total': total, 'reserved': 0, 'min_unit': 1, 'max_unit': total, 'step_size': 1, 'allocation_ratio': 1.0}
                       for name, total in [('VCPU', 4), ('MEMORY_MB', 1024), ('DISK_GB', 10), (custom, 64)]}
        for ident in values['providers']:
            require(self.targets[impl].call('GET', '/resource_providers/' + ident)['status'] == 404, 'fresh provider')
            self.current['authorized_providers'].append(ident)
            save_json(self.root / 'ledger.json', self.ledger, private=True)
            self.call(impl, 'POST', '/resource_providers', {'uuid': ident, 'name': 'tt-placement-b17-' + ident}, '1.20')
            self.call(impl, 'PUT', '/resource_providers/' + ident + '/inventories', {'resource_provider_generation': 0, 'inventories': inventories}, '1.26')
            self.call(impl, 'PUT', '/resource_providers/' + ident + '/traits', {'resource_provider_generation': 1, 'traits': ['OWNER_NOVA']}, '1.6', (200, 204))
        self.current['snapshots'] = self.snapshots(impl, values)
        path = '/allocation_candidates?resources=VCPU:1,MEMORY_MB:512,DISK_GB:1,' + custom + ':1&root_required=OWNER_NOVA,!COMPUTE_STATUS_DISABLED'
        candidates = self.call(impl, 'GET', path)
        require(len(candidates['allocation_requests']) == 16 and set(candidates['provider_summaries']) == set(values['providers']), 'isolated candidates set')
        for consumer in values['consumers']:
            require(self.call(impl, 'GET', '/allocations/' + consumer, version='1.28') == {'allocations': {}}, 'fresh consumer')
        return {**values, 'url': self.args.url, 'owner': self.owner, 'snapshots': self.current['snapshots'], 'candidates_path': path, 'candidates': candidates}

    def snapshots(self, impl, values):
        return {ident: {name: self.call(impl, 'GET', '/resource_providers/' + ident + suffix, version=version) for name, suffix, version in
                [('provider', '', '1.14'), ('inventories', '/inventories', '1.26'), ('traits', '/traits', '1.6'), ('allocations', '/allocations', '1.36'), ('usages', '/usages', '1.0')]}
                for ident in values['providers']}

    def cleanup_seed(self, generations=None):
        if self.current is None:
            return {'removed': True}
        values = self.current
        impl = values['implementation']
        # Go stays directly reachable even if its emergency route lease elapsed.
        existing = [ident for ident in values['authorized_providers'] if self.targets[impl].call('GET', '/resource_providers/' + ident)['status'] != 404]
        before = self.snapshots(impl, dict(values, providers=existing))
        if generations is not None:
            for index, ident in enumerate(values['providers'][:len(generations)]):
                require(before[ident]['provider']['generation'] == generations[index], 'post-load exact generation')
        for consumer in values['consumers']:
            value = self.call(impl, 'GET', '/allocations/' + consumer, version='1.28')
            if value.get('allocations'):
                require(value.get('project_id') == self.owner['project_id'] and value.get('user_id') == self.owner['user_id'] and set(value['allocations']).issubset(set(values['providers'])), 'owned allocation before delete')
                self.call(impl, 'DELETE', '/allocations/' + consumer, version='1.0', statuses=(204,))
            require(self.call(impl, 'GET', '/allocations/' + consumer, version='1.28') == {'allocations': {}}, 'consumer empty')
        after = self.snapshots(impl, dict(values, providers=existing))
        require(all(not any(p['usages']['usages'].values()) and p['allocations']['allocations'] == {} for p in after.values()), 'seed usage restored')
        for ident in existing:
            require(after[ident]['provider']['name'] == 'tt-placement-b17-' + ident, 'provider ownership')
            self.call(impl, 'DELETE', '/resource_providers/' + ident, version='1.0', statuses=(204,))
            require(self.targets[impl].call('GET', '/resource_providers/' + ident)['status'] == 404, 'provider removed')
        if values['authorized_class']:
            self.call(impl, 'DELETE', '/resource_classes/' + values['custom'], version='1.2', statuses=(204, 404))
            require(self.targets[impl].call('GET', '/resource_classes/' + values['custom'], headers={'OpenStack-API-Version': 'placement 1.2'})['status'] == 404, 'class removed')
        self.current = None
        return {'removed': True, 'before_cleanup': before, 'after_release': after, 'all_consumers_empty': True}

    def start_collector(self, run_id):
        require(self.collector is None and re.fullmatch(r'[a-z0-9-]+', run_id), 'collector identity')
        groups = {}
        for label, unit in [('python', 'devstack@placement-api.service'), ('go', self.runtime_state['go_unit']),
                            ('mysql-python', 'mysql.service'), ('mysql-go', self.runtime_state['db_unit']),
                            ('apache', 'apache2.service'), ('keystone', 'devstack@keystone.service'),
                            ('nova-compute', 'devstack@n-cpu.service'), ('nova-scheduler', 'devstack@n-sch.service'), ('glance', 'devstack@g-api.service')]:
            group = subprocess.check_output(['systemctl', 'show', '--value', '-p', 'ControlGroup', unit], text=True).strip()
            if group:
                groups[label] = group
        require(self.current['implementation'] in groups, 'measured service cgroup')
        group_file = self.args.output / (run_id + '-groups.json')
        save_json(group_file, groups)
        output = self.args.output / (run_id + '-resources')
        stop = self.root / (run_id + '.stop')
        log = (self.args.output / (run_id + '-collector.log')).open('x')
        process = subprocess.Popen(['sudo', sys.executable, str(HERE / 'placement_resources.py'), '--groups', str(group_file),
                                    '--output', str(output), '--stop-file', str(stop), '--maximum-seconds', '240'], stdout=log, stderr=log)
        self.collector = (process, log, output, stop)
        time.sleep(5)  # idle PSS before the HTTP warm-up
        require(process.poll() is None, 'collector running')
        return {'groups': groups, 'idle_s': 5}

    def stop_collector(self):
        if self.collector is None:
            return {'stopped': True}
        process, log, output, stop = self.collector
        stop.touch(mode=0o600)
        process.wait(timeout=10)
        log.close()
        self.collector = None
        require(process.returncode == 0, 'collector exit')
        return {'resources': str(output), 'stopped': True}

    def cleanup(self):
        if self.closed:
            return {'already_closed': True}
        self.stop_collector()
        try:
            seed = self.cleanup_seed()
        finally:
            if self.original and (self.runtime / 'switch-state.json').exists():
                self.route('python')
        final = None
        if self.original:
            self.action('original-final', placement_preflight, url=self.args.url, token_file=self.auth / 'service.token', output=self.args.output / 'original-after')
            final = json.loads((self.args.output / 'original-after/report.json').read_text())
            require(final['providers'] == self.original['providers'] and final['apache_site_sha256'] == self.original['apache_site_sha256'] and final['original_mysql'] == self.original['original_mysql'], 'original unchanged')
            require(self.owners() == self.original_owners, 'original owner rows unchanged')
            require(list(self.connection.compute.servers(all_projects=True)) == [], 'no servers')
        if self.runtime.exists():
            self.action('runtime-cleanup', placement_runtime, action='cleanup', root=self.runtime)
        if self.auth.exists():
            self.action('identity-cleanup', lab_identity, action='cleanup', private_root=self.auth)
        controls = self.controls('restore')
        save_json(self.args.output / 'maintenance-controls-after.json', controls)
        require(controls['controls_restored'], 'maintenance controls restored')
        trim = subprocess.run(['sudo', 'fstrim', '-v', '/'], capture_output=True, text=True)
        require(trim.returncode == 0, 'guest unused disk blocks reclaimed')
        services = {unit: placement_switch.active(unit) for unit in ['devstack@placement-api.service', 'devstack@n-cpu.service',
                    'devstack@n-sch.service', 'devstack@keystone.service', 'devstack@g-api.service']}
        require(all(services.values()), 'original control services active after cleanup')
        profile_sha = hashlib.sha256(Path('/etc/apparmor.d/usr.sbin.mysqld').read_bytes()).hexdigest()
        require(profile_sha == self.original_mysql_profile_sha, 'original MySQL AppArmor profile unchanged')
        shutil.rmtree(self.root)
        self.closed = True
        result = {'seed_cleanup': seed, 'controls_restored': True, 'guest_trim': trim.stdout.strip(),
                  'original_unchanged': final is not None, 'runtime_removed': not self.runtime.exists(),
                  'identity_removed': not self.auth.exists(), 'private_root_removed': not self.root.exists(),
                  'original_services_active': services, 'original_mysql_apparmor_sha256': profile_sha}
        save_json(self.args.output / 'cleanup.json', result)
        return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--work-root', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--url', default='http://192.168.122.10/placement')
    parser.add_argument('--keystone-url', default='http://192.168.122.10/identity/v3')
    parser.add_argument('--allow-mutations', action='store_true')
    args = parser.parse_args()
    require(args.allow_mutations, 'explicit opt-in')
    require(args.work_root.is_absolute() and args.work_root == args.work_root.resolve() and re.fullmatch(r'tt-placement-[a-z0-9-]+', args.work_root.name), 'owned canonical work root')
    args.output.mkdir(mode=0o700, parents=True, exist_ok=False)
    logging.disable(logging.CRITICAL)
    with exclusive_run(Path('/run/lock/tt-placement-integration.lock')):
        args.work_root.mkdir(mode=0o700, exist_ok=False)
        lab = Lab(args)
        try:
            for line in sys.stdin:
                try:
                    request = json.loads(line)
                    action = request['action']
                    if action == 'ping':
                        value = {'utc_ns': time.time_ns()}
                    elif action == 'prepare':
                        value = lab.prepare()
                    elif action == 'token':
                        value = lab.refresh()
                    elif action == 'seed':
                        value = lab.seed(request['values'])
                    elif action == 'finish':
                        value = lab.cleanup_seed(request.get('generations'))
                    elif action == 'route':
                        value = lab.route(request['implementation'])
                    elif action == 'start-collector':
                        value = lab.start_collector(request['run_id'])
                    elif action == 'stop-collector':
                        value = lab.stop_collector()
                    elif action == 'cleanup':
                        value = lab.cleanup()
                    else:
                        raise ValueError('unknown action')
                    print(json.dumps({'ok': True, 'value': value}), flush=True)
                except Exception as error:
                    print(json.dumps({'ok': False, 'error': type(error).__name__,
                                      'reason': str(error) if isinstance(error, AssertionError) else None}), flush=True)
                if lab.closed:
                    break
        finally:
            try:
                lab.cleanup()
            except Exception as error:
                # Keep ledger/runtime/credentials for repair; no false cleanup.
                save_json(args.output / 'cleanup-error.json', {'error': type(error).__name__, 'private_state_retained': True})


if __name__ == '__main__':
    main()

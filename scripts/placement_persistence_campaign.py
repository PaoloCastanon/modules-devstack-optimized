#!/usr/bin/env python3
"""B11 isolated durability campaign. Creates and destroys its own Docker volume.

Requires real Keystone, private validator/token files, a NEW output directory and
explicit mutation opt-in. Never connects to or writes the original MySQL DB.
"""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import secrets
import select
import shutil
import signal
import socket
import subprocess
import sys
import tempfile
import time
import urllib.request
import uuid
import placement_differential as base
from placement_consumer_differential import canonical


def run(args):
    repo = Path(__file__).resolve().parents[1]
    output = args.output.resolve()
    output.mkdir(parents=True, exist_ok=False)
    token = base.read_token(args.admin_token_file)
    validator_path = Path(args.validator_file)
    if validator_path.stat().st_mode & 0o077:
        raise ValueError('validator file must be private')
    validator = validator_path.read_text()
    credential = json.loads(validator)['application_credential_secret']
    private = Path(tempfile.mkdtemp(prefix='b11-private-'))
    private.chmod(0o700)
    suffix = secrets.token_hex(5)
    name, volume = 'tt-b11-' + suffix, 'tt-b11-data-' + suffix
    container_owned = volume_owned = False
    children, logs = [], []
    sql_child = None
    proof = {'schema': 1, 'mysql': '8.0.46', 'timestamp_utc': datetime.datetime.now(datetime.timezone.utc).isoformat(),
             'scope': 'own Docker MySQL schemas and volume; real Keystone; no original DB access', 'checks': {}}

    def secret(name, data):
        path = private / name
        path.write_text(data)
        path.chmod(0o600)
        return path

    def command(argv, **kwargs):
        return subprocess.run(argv, check=True, **kwargs)

    def sql(query, database=None, config='/tmp/task.cnf', check=True):
        argv = ['docker', 'exec', '-i', name, 'mysql', '--defaults-extra-file=' + config, '--batch', '--skip-column-names']
        if database:
            argv += ['--database=' + database]
        result = subprocess.run(argv, input=query, text=True, capture_output=True)
        if check and result.returncode:
            raise RuntimeError('isolated SQL command failed')
        return result

    def db_ready():
        for _ in range(60):
            result = subprocess.run(['docker', 'exec', name, 'mysql', '--defaults-extra-file=/tmp/task.cnf', '--protocol=TCP', '--host=127.0.0.1', '-Nse', 'SELECT 1'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            if result.returncode == 0:
                return
            time.sleep(.5)
        raise RuntimeError('authenticated MySQL readiness timed out')

    def free_port():
        with socket.socket() as sock:
            sock.bind(('127.0.0.1', 0))
            return sock.getsockname()[1]

    result = 1
    try:
        envfile = secret('mysql.env', 'MYSQL_ROOT_PASSWORD=' + secrets.token_hex(24) + '\n')
        root_password = envfile.read_text().strip().split('=', 1)[1]
        root_cnf = secret('root.cnf', '[client]\nuser=root\npassword=' + root_password + '\n')
        if subprocess.run(['docker', 'volume', 'inspect', volume], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0:
            raise RuntimeError('refusing existing volume')
        command(['docker', 'volume', 'create', volume], stdout=subprocess.DEVNULL)
        volume_owned = True
        published_port=free_port()
        command(['docker', 'create', '--name', name, '--env-file', str(envfile), '--mount', 'type=volume,src=' + volume + ',dst=/var/lib/mysql', '-p', '127.0.0.1:'+str(published_port)+':3306', 'mysql:8.0.46'], stdout=subprocess.DEVNULL)
        container_owned=True
        command(['docker','start',name],stdout=subprocess.DEVNULL)
        command(['docker', 'cp', str(root_cnf), name + ':/tmp/task.cnf'], stdout=subprocess.DEVNULL)
        db_ready()
        port = subprocess.check_output(['docker', 'port', name, '3306/tcp'], text=True).strip().rsplit(':', 1)[1]
        schemas, accounts = {}, {}
        bootstrap = []
        for label in ['primary', 'restored', 'sentinel']:
            schema = 'placement_go_test_b11_' + label + '_' + suffix
            schemas[label] = schema
            bootstrap.append('CREATE DATABASE `' + schema + '`;')
            accounts[label] = {}
            if label == 'sentinel':
                bootstrap.append('CREATE TABLE `' + schema + '`.canary(value VARCHAR(30));')
                bootstrap.append("INSERT INTO `" + schema + "`.canary VALUES ('untouched');")
                continue
            for mode, rights in [('migrate', 'SELECT,INSERT,UPDATE,DELETE,CREATE,ALTER,INDEX,REFERENCES'), ('app', 'SELECT,INSERT,UPDATE,DELETE')]:
                user, password = 'b11_' + label + '_' + mode, secrets.token_hex(24)
                bootstrap += ["CREATE USER '" + user + "'@'%' IDENTIFIED BY '" + password + "';", 'GRANT ' + rights + ' ON `' + schema.replace('_', r'\_') + "`.* TO '" + user + "'@'%';"]
                accounts[label][mode] = secret(label + '-' + mode + '.dsn', user + ':' + password + '@tcp(127.0.0.1:' + port + ')/' + schema + '?parseTime=true\n')
                if label == 'primary' and mode == 'app':
                    cnf = secret('app.cnf', '[client]\nuser=' + user + '\npassword=' + password + '\n')
                    command(['docker', 'cp', str(cnf), name + ':/tmp/app.cnf'], stdout=subprocess.DEVNULL)
        sql('\n'.join(bootstrap))
        command(['go', 'build', '-trimpath', '-o', str(private / 'placement'), './cmd/placement'], cwd=repo)
        validator_file = secret('validator.json', validator)
        backend = free_port()
        public_url = 'http://127.0.0.1:' + str(backend)
        env = os.environ.copy()
        env.update({'PLACEMENT_GO_LISTEN_ADDR': '127.0.0.1:' + str(backend), 'PLACEMENT_GO_PUBLIC_BASE_URL': public_url,
                    'PLACEMENT_GO_KEYSTONE_AUTH_URL': args.keystone_url, 'PLACEMENT_GO_KEYSTONE_CREDENTIAL_FILE': str(validator_file),
                    'PLACEMENT_GO_DATABASE_DSN_FILE': str(accounts['primary']['migrate'])})
        with (output / 'migrate.log').open('w') as log:
            command([str(private / 'placement'), 'migrate'], env=env, stdout=log, stderr=subprocess.STDOUT)
        with (output / 'ddl-account-serve.log').open('w') as log:
            denied = subprocess.run([str(private / 'placement'), 'serve'], env=env, stdout=log, stderr=subprocess.STDOUT, timeout=15)
        if denied.returncode == 0:
            raise RuntimeError('DDL account was allowed to serve')
        proof['checks']['ddl_account_cannot_serve'] = True
        denied = sql('CREATE TABLE forbidden(value INT);', schemas['primary'], '/tmp/app.cnf', False)
        if denied.returncode == 0:
            raise RuntimeError('runtime account could create a table')
        proof['checks']['runtime_cannot_ddl'] = True
        denied = sql('SELECT * FROM `' + schemas['sentinel'] + '`.canary;', schemas['primary'], '/tmp/app.cnf', False)
        if denied.returncode == 0:
            raise RuntimeError('runtime account could read another schema')
        proof['checks']['runtime_cannot_cross_schema'] = True
        env['PLACEMENT_GO_DATABASE_DSN_FILE'] = str(accounts['primary']['app'])

        def start_go():
            log = (output / 'server.log').open('a')
            logs.append(log)
            child = subprocess.Popen([str(private / 'placement'), 'serve'], env=env, stdout=log, stderr=subprocess.STDOUT)
            children.append(child)
            for _ in range(100):
                if child.poll() is not None:
                    raise RuntimeError('serve exited before readiness')
                try:
                    with urllib.request.urlopen(public_url + '/', timeout=1) as response:
                        if response.status == 200:
                            return child
                except Exception:
                    pass
                time.sleep(.1)
            raise RuntimeError('HTTP readiness failed')

        go = start_go()
        target = base.Target(public_url, {'admin': token})
        provider, consumer = str(uuid.uuid4()), str(uuid.uuid4())
        pp, cp = '/resource_providers/' + provider, '/allocations/' + consumer

        def call(method, path, data=None, version='1.36'):
            return target.call(method, path, data, {'OpenStack-API-Version': 'placement ' + version})

        if call('POST', '/resource_providers', {'uuid': provider, 'name': 'b11-durable', 'parent_provider_uuid': None}, '1.20')['status'] != 200:
            raise RuntimeError('provider seed failed')
        if call('PUT', pp + '/inventories', {'resource_provider_generation': 0, 'inventories': {'VCPU': {'total': 8, 'max_unit': 8}}}, '1.26')['status'] != 200:
            raise RuntimeError('inventory seed failed')
        if call('PUT', pp + '/traits', {'resource_provider_generation': 1, 'traits': ['OWNER_NOVA']}, '1.6')['status'] != 200:
            raise RuntimeError('trait seed failed')
        if call('PUT', pp + '/aggregates', {'resource_provider_generation': 2, 'aggregates': []}, '1.19')['status'] != 200:
            raise RuntimeError('aggregate seed failed')
        if call('PUT', cp, {'consumer_generation': None, 'project_id': 'b11-project', 'user_id': 'b11-user', 'allocations': {provider: {'resources': {'VCPU': 4}}}})['status'] != 204:
            raise RuntimeError('allocation seed failed')

        def snapshot():
            requests = [('provider', pp, '1.20'), ('inventory', pp + '/inventories', '1.26'), ('traits', pp + '/traits', '1.6'),
                        ('aggregates', pp + '/aggregates', '1.19'), ('consumer', cp, '1.36'), ('provider-allocations', pp + '/allocations', '1.28'),
                        ('usages', pp + '/usages', '1.0'), ('candidates', '/allocation_candidates?resources=VCPU:1&root_required=OWNER_NOVA', '1.36')]
            responses = {label: call('GET', path, version=version) for label, path, version in requests}
            if any(response['status'] != 200 for response in responses.values()):
                raise RuntimeError('durability snapshot failed')
            return responses

        def same(before, after):
            differences = []
            for key in before:
                differences += base.differences(canonical(before[key], target), canonical(after[key], target), '$.' + key)
            if differences:
                raise RuntimeError('durability snapshots differ')

        def dump(database, schema=False):
            argv = ['docker', 'exec', name, 'mysqldump', '--defaults-extra-file=/tmp/task.cnf', '--no-tablespaces', '--skip-comments', '--compact', '--skip-lock-tables', '--order-by-primary', '--skip-extended-insert', '--skip-dump-date']
            if not schema:
                argv.append('--no-create-info')
            return subprocess.check_output(argv + [database])

        before = snapshot()
        data_before = dump(schemas['primary'])
        backup = dump(schemas['primary'], True)
        (output / 'backup.sql').write_bytes(backup)
        go.kill()
        killed = go.wait(timeout=10)
        go = start_go()
        after_process = snapshot()
        same(before, after_process)
        proof['checks']['go_sigkill_preserves_state'] = killed < 0
        sql_child = subprocess.Popen(['docker', 'exec', '-i', name, 'mysql', '--defaults-extra-file=/tmp/task.cnf', '--database=' + schemas['primary'], '--batch', '--skip-column-names', '--unbuffered'], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True, bufsize=1)
        sql_child.stdin.write("START TRANSACTION; UPDATE consumers SET project_id='uncommitted-fault' WHERE uuid='" + consumer + "'; SELECT 'fault_ready';\n")
        sql_child.stdin.flush()
        readable, _, _ = select.select([sql_child.stdout], [], [], 10)
        if not readable or sql_child.stdout.readline().strip() != 'fault_ready':
            raise RuntimeError('uncommitted fault transaction not confirmed')
        command(['docker', 'kill', '--signal', 'KILL', name], stdout=subprocess.DEVNULL)
        sql_child.stdin.close()
        sql_child.wait(timeout=10)
        sql_child = None
        command(['docker', 'start', name], stdout=subprocess.DEVNULL)
        db_ready()
        after_mysql = snapshot()
        same(before, after_mysql)
        data_after = dump(schemas['primary'])
        if data_before != data_after:
            raise RuntimeError('committed rows or timestamps changed after recovery')
        proof['checks']['mysql_sigkill_preserves_committed_rows'] = True
        proof['checks']['mysql_recovery_rolls_back_uncommitted_owner'] = True
        sql('SET FOREIGN_KEY_CHECKS=0;\n'+backup.decode('utf-8')+'\nSET FOREIGN_KEY_CHECKS=1;',schemas['restored'])
        if dump(schemas['restored']) != data_before:
            raise RuntimeError('restored rows differ')
        go.send_signal(signal.SIGTERM)
        if go.wait(timeout=10) != 0:
            raise RuntimeError('graceful stop failed')
        env['PLACEMENT_GO_DATABASE_DSN_FILE'] = str(accounts['restored']['app'])
        go = start_go()
        after_restore = snapshot()
        same(before, after_restore)
        proof['checks']['backup_restore_verified_by_http_and_all_rows'] = True
        if sql('SELECT value FROM canary;', schemas['sentinel']).stdout.strip() != 'untouched':
            raise RuntimeError('sentinel changed')
        proof['checks']['sentinel_schema_unchanged'] = True
        if call('DELETE', cp, version='1.0')['status'] != 204 or call('DELETE', pp, version='1.0')['status'] != 204:
            raise RuntimeError('restore cleanup failed')
        proof['checks']['restored_resources_cleaned'] = True
        proof['row_data_sha256'] = hashlib.sha256(data_before).hexdigest()
        proof['backup_sha256'] = hashlib.sha256(backup).hexdigest()
        proof['binary_sha256'] = hashlib.sha256((private / 'placement').read_bytes()).hexdigest()
        proof['snapshots'] = {'before': before, 'after_go_sigkill': after_process, 'after_mysql_recovery': after_mysql, 'after_backup_restore': after_restore}
        if not all(proof['checks'].values()):
            raise RuntimeError('a required durability assertion was false')
        result = 0
    except Exception as error:
        proof['execution_error'] = type(error).__name__
        if isinstance(error,RuntimeError):
            proof['failure_stage']=str(error)
    finally:
        if sql_child is not None:
            sql_child.kill()
            sql_child.wait(timeout=10)
        for child in reversed(children):
            if child.poll() is None:
                child.send_signal(signal.SIGTERM)
                try:
                    child.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    child.kill()
                    child.wait()
        for log in logs:
            log.close()
        for path in output.rglob('*'):
            if path.is_file() and any(value and value in path.read_text(errors='replace') for value in [token, credential]):
                path.unlink()
                proof['credential_echo_suppressed'] = True
                result = 2
        if container_owned:
            removed = subprocess.run(['docker', 'rm', '-f', name], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0
            proof['container_removed'] = removed
            if not removed:
                result = 2
        if volume_owned:
            removed = subprocess.run(['docker', 'volume', 'rm', volume], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL).returncode == 0
            proof['volume_removed'] = removed
            if not removed:
                result = 2
        shutil.rmtree(private)
        proof['private_files_removed'] = True
        (output / 'report.json').write_text(json.dumps(proof, indent=2) + '\n')
    print(json.dumps({'checks': proof['checks'], 'execution_error': proof.get('execution_error'), 'exit': result}))
    return result


def main():
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--output', type=Path, required=True)
    p.add_argument('--keystone-url', required=True)
    p.add_argument('--validator-file', required=True)
    p.add_argument('--admin-token-file', required=True)
    p.add_argument('--allow-mutations', action='store_true')
    args = p.parse_args()
    if not args.allow_mutations:
        p.error('explicit mutation opt-in required')
    try:
        return run(args)
    except Exception as error:
        print(type(error).__name__, file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())

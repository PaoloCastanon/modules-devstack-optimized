#!/usr/bin/env python3
"""Own native MySQL + Go runtime on the lab VM, separate from DevStack MySQL.

Requires explicit opt-in and a new private tt-placement runtime directory. Uses
only its own datadir/socket/loopback port, schema, accounts and systemd units.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import secrets
import shutil
import socket
import subprocess
import sys
import time


def command(args, **kwargs):
    return subprocess.run(args, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL, **kwargs)


def probe_available_port(port):
    with socket.socket() as test:
        # Match daemon bind semantics: permit old TIME_WAIT connections, never
        # SO_REUSEPORT or a currently listening service on the same address.
        test.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
        test.bind(('127.0.0.1', port))


def run(args):
    if not args.allow_mutations:
        raise ValueError('explicit mutation opt-in required')
    root = args.root
    domain = getattr(args, 'domain', 'placement')
    if domain not in ('placement', 'glance'):
        raise ValueError('supported isolated service domain required')
    if not root.is_absolute() or root != root.resolve() or not re.fullmatch(r'tt-placement-[a-z0-9-]+', root.name):
        raise ValueError('absolute canonical tt-placement runtime path required')
    if args.action == 'prepare':
        for port in [args.mysql_port, args.listen_port]:
            probe_available_port(port)
        if args.mysql_port == args.listen_port or args.mysql_port == 3306:
            raise ValueError('distinct non-original loopback ports required')
        if not args.binary.is_file() or not (args.auth_root / 'validator.json').is_file():
            raise ValueError('binary and validator required')
        root.mkdir(mode=0o700, parents=False, exist_ok=False)
        state = {'id': secrets.token_hex(5), 'owner_uid': os.getuid(), 'units': [], 'domain': domain,
                 'schema': domain + '_go_test_b13_' + secrets.token_hex(5),
                 'mysql_port': args.mysql_port, 'listen_port': args.listen_port}
    else:
        if root.stat().st_uid != os.getuid() or root.stat().st_mode & 0o077:
            raise ValueError('private directory mismatch')
        state = json.loads((root / 'state.json').read_text())
        if state['owner_uid'] != os.getuid():
            raise ValueError('runtime ownership mismatch')
        domain = state.get('domain', 'placement')

    def save(name, data):
        p = root / name
        p.write_text(data if isinstance(data, str) else json.dumps(data))
        p.chmod(0o600)

    def checkpoint():
        save('state.json', state)

    checkpoint()

    def start_unit(name, values, properties=()):
        state['units'].append(name)
        checkpoint()
        command(['sudo', 'systemd-run', '--unit=' + name, '--uid=' + str(os.getuid()),
                 '--collect', '--property=Restart=on-failure', '--property=RestartSec=1',
                 *properties, *values])

    if args.action == 'prepare':
        import pymysql
        # Confine only this instance; leave DevStack's mysqld profile unchanged.
        profile = 'tt-placement-b13-' + state['id']
        state['apparmor_profile'] = profile
        state['apparmor_file'] = '/etc/apparmor.d/' + profile
        checkpoint()
        profile_text = ('#include <tunables/global>\nprofile ' + profile + ' {\n' +
                        '  #include <abstractions/base>\n  #include <abstractions/nameservice>\n' +
                        '  network tcp,\n  network unix,\n  capability sys_resource,\n' +
                        '  /usr/sbin/mysqld mrix,\n  /usr/lib/mysql/plugin/ r,\n' +
                        '  /usr/lib/mysql/plugin/** mr,\n  /usr/share/mysql/ r,\n' +
                        '  /usr/share/mysql/** r,\n  /proc/*/status r,\n' +
                        '  /proc/sys/vm/overcommit_memory r,\n  /sys/devices/system/** r,\n' +
                        '  /{,var/}run/systemd/notify w,\n  /etc/ssl/openssl.cnf r,\n' +
                        '  ' + str(root) + '/ r,\n  ' + str(root) + '/** rwk,\n}\n')
        command(['sudo', 'tee', state['apparmor_file']], input=profile_text, text=True)
        command(['sudo', 'apparmor_parser', '-r', state['apparmor_file']])
        confined = ['aa-exec', '-p', profile, '--', '/usr/sbin/mysqld']
        datadir = root / 'mysql'
        datadir.mkdir(mode=0o700)
        (root / 'tmp').mkdir(mode=0o700)
        save('mysql.cnf', '[mysqld]\ndatadir=' + str(datadir) + '\ntmpdir=' + str(root / 'tmp') + '\nsocket=' + str(root / 'mysql.sock') +
             '\npid-file=' + str(root / 'mysql.pid') + '\nport=' + str(args.mysql_port) +
             '\nbind-address=127.0.0.1\nmysqlx=0\nskip-name-resolve=1\n' +
             'log-error=' + str(root / 'mysql.log') + '\ninnodb-buffer-pool-size=134217728\n' +
             'innodb-flush-log-at-trx-commit=1\nsync-binlog=1\nlog-bin=' + str(root / 'binlog') + '\n')
        with (root / 'initialize.log').open('w') as log:
            subprocess.run([*confined, '--defaults-file=' + str(root / 'mysql.cnf'), '--initialize-insecure'], check=True, stdout=log, stderr=subprocess.STDOUT)
        # Bootstrap without a TCP listener while the new root password is empty.
        with (root / 'bootstrap-process.log').open('w') as log:
            bootstrap = subprocess.Popen([*confined, '--defaults-file=' + str(root / 'mysql.cnf'), '--skip-networking'], stdout=log, stderr=subprocess.STDOUT)
            try:
                connection = None
                for _ in range(150):
                    try:
                        connection = pymysql.connect(unix_socket=str(root / 'mysql.sock'), user='root', connect_timeout=1)
                        break
                    except pymysql.MySQLError:
                        if bootstrap.poll() is not None:
                            raise RuntimeError('own bootstrap MySQL exited')
                        time.sleep(.2)
                if connection is None:
                    raise RuntimeError('own bootstrap MySQL readiness failed')
                with connection:
                    with connection.cursor() as cur:
                        schema = state['schema']
                        cur.execute('CREATE DATABASE `' + schema + '`')
                        for mode, rights in [('migrate', 'SELECT,INSERT,UPDATE,DELETE,CREATE,ALTER,INDEX,REFERENCES'), ('app', 'SELECT,INSERT,UPDATE,DELETE')]:
                            user, password = 'b13_' + mode + '_' + state['id'], secrets.token_hex(24)
                            cur.execute("CREATE USER %s@'127.0.0.1' IDENTIFIED BY %s", (user, password))
                            cur.execute('GRANT ' + rights + ' ON `' + schema.replace('_', r'\_') + "`.* TO %s@'127.0.0.1'", (user,))
                            save(mode + '.dsn', user + ':' + password + '@tcp(127.0.0.1:' + str(args.mysql_port) + ')/' + schema + '?parseTime=true\n')
                        root_password = secrets.token_hex(24)
                        cur.execute("ALTER USER 'root'@'localhost' IDENTIFIED BY %s", (root_password,))
                        save('root.cnf', '[client]\nuser=root\npassword=' + root_password + '\nsocket=' + str(root / 'mysql.sock') + '\n')
                    connection.commit()
            finally:
                bootstrap.terminate()
                bootstrap.wait(timeout=40)
        state['db_unit'] = 'tt-placement-b13-db-' + state['id']
        start_unit(state['db_unit'], ['/usr/sbin/mysqld', '--defaults-file=' + str(root / 'mysql.cnf')], ['--property=AppArmorProfile=' + profile])
        for _ in range(100):
            if (root / 'mysql.sock').exists():
                # defaults-file excludes ~/.my.cnf, which may force DevStack's
                # host/port/password despite an extra config's socket value.
                result = subprocess.run(['mysql', '--defaults-file=' + str(root / 'root.cnf'), '--protocol=SOCKET', '--batch', '--skip-column-names', '-e', 'SELECT 1'], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
                if result.returncode == 0:
                    break
            time.sleep(.2)
        else:
            raise RuntimeError('own MySQL readiness failed')
        shutil.copy(args.binary, root / domain)
        (root / domain).chmod(0o700)
        prefix = domain.upper() + '_GO_'
        env = {key: value for key, value in os.environ.items() if not key.startswith(prefix)}
        env.update({prefix + 'DATABASE_DSN_FILE': str(root / 'migrate.dsn')})
        with (root / 'migrate.log').open('w') as log:
            subprocess.run([str(root / domain), 'migrate'], env=env, check=True, stdout=log, stderr=subprocess.STDOUT)
        settings = {prefix + 'LISTEN_ADDR': '127.0.0.1:' + str(args.listen_port),
                    prefix + 'PUBLIC_BASE_URL': args.public_url,
                    prefix + 'DATABASE_DSN_FILE': str(root / 'app.dsn'),
                    prefix + 'KEYSTONE_AUTH_URL': args.keystone_url,
                    prefix + 'KEYSTONE_CREDENTIAL_FILE': str(args.auth_root / 'validator.json')}
        if domain == 'glance':
            import uuid
            own_env = {**env, **settings}
            result = subprocess.run([str(root / domain), 'store-scope'], env=own_env, check=True, capture_output=True, text=True)
            scope = result.stdout.strip()
            if str(uuid.UUID(scope)) != scope or uuid.UUID(scope).version != 5:
                raise ValueError('verified store scope required')
            (root / 'store').mkdir(mode=0o700)
            settings.update({'GLANCE_GO_BINARY_ENABLED': 'true', 'GLANCE_GO_STORE_ROOT': str(root / 'store'),
                             'GLANCE_GO_STORE_SCOPE': scope, 'GLANCE_GO_MAX_CONCURRENT_TRANSFERS': '16',
                             'GLANCE_GO_UPLOAD_TIMEOUT': '120s', 'GLANCE_GO_DOWNLOAD_TIMEOUT': '120s'})
            with (root / 'store-init.log').open('w') as log:
                subprocess.run([str(root / domain), 'store-init'], env={**env, **settings}, check=True, stdout=log, stderr=subprocess.STDOUT)
        save('serve.env', ''.join(key + '=' + value + '\n' for key, value in settings.items()))
        state['go_unit'] = 'tt-placement-b13-go-' + state['id']
        start_unit(state['go_unit'], [str(root / domain), 'serve'], ['--property=EnvironmentFile=' + str(root / 'serve.env')])
        from urllib.request import urlopen
        from urllib.error import HTTPError
        for _ in range(100):
            try:
                with urlopen('http://127.0.0.1:' + str(args.listen_port) + ('/v2/images' if domain == 'glance' else '/'), timeout=1) as response:
                    if response.status == 200:
                        break
            except HTTPError as error:
                if domain == 'glance' and error.code == 401:
                    break
            except OSError:
                pass
            time.sleep(.2)
        else:
            raise RuntimeError('Go readiness failed')
        state['prepared'] = True
        checkpoint()
        report = {'prepared': True, 'db_unit': state['db_unit'], 'go_unit': state['go_unit'],
                  'schema': state['schema'], 'mysql_port': args.mysql_port, 'listen_port': args.listen_port,
                  'binary_sha256': hashlib.sha256((root / domain).read_bytes()).hexdigest(),
                  'separate_mysql_datadir': str(datadir), 'runtime_has_ddl': False, 'domain': domain,
                  'separate_store': domain == 'glance'}
        save('public-report.json', report)
        print(json.dumps(report))
        return 0

    if args.action in ('restart-go', 'replace-go'):
        if args.action == 'replace-go':
            if not args.binary or not args.binary.is_file():
                raise ValueError('replacement binary required')
            shutil.copy(args.binary, root / (domain + '.next'))
            (root / (domain + '.next')).chmod(0o700)
            (root / (domain + '.next')).replace(root / domain)
        command(['sudo', 'systemctl', 'restart', state['go_unit']])
        print(json.dumps({'go_restarted': True, 'binary_sha256': hashlib.sha256((root / domain).read_bytes()).hexdigest()}))
        return 0

    # Do not dismantle a backend while Apache still points at it.
    sites = [p.read_text() for p in Path('/etc/apache2/sites-enabled').glob('*.conf')]
    if any('127.0.0.1:' + str(state['listen_port']) in site for site in sites):
        raise ValueError('restore original route before runtime cleanup')
    for unit in reversed(state['units']):
        command(['sudo', 'systemctl', 'stop', unit])
    for unit in state['units']:
        value = subprocess.run(['systemctl', 'is-active', unit], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        if value.returncode == 0:
            raise RuntimeError('own unit still active')
    if state.get('apparmor_file'):
        command(['sudo', 'apparmor_parser', '-R', state['apparmor_file']])
        command(['sudo', 'rm', '--', state['apparmor_file']])
    shutil.rmtree(root)
    print(json.dumps({'own_units_stopped': True, 'own_datadir_private_files_removed': True}))
    return 0


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('action', choices=['prepare', 'restart-go', 'replace-go', 'cleanup'])
    parser.add_argument('--root', type=Path, required=True)
    parser.add_argument('--domain', choices=['placement', 'glance'], default='placement')
    parser.add_argument('--auth-root', type=Path)
    parser.add_argument('--binary', type=Path)
    parser.add_argument('--mysql-port', type=int, default=13316)
    parser.add_argument('--listen-port', type=int, default=18779)
    parser.add_argument('--public-url', default='http://192.168.122.10/placement')
    parser.add_argument('--keystone-url', default='http://192.168.122.10/identity/v3')
    parser.add_argument('--allow-mutations', action='store_true')
    args = parser.parse_args()
    try:
        return run(args)
    except Exception as error:
        print('runtime operation failed: ' + type(error).__name__, file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())

"""Execute the playbook's actual Python task with isolated MySQL/sudo doubles."""

import contextlib
import io
import json
import os
from pathlib import Path
import re
import subprocess
import unittest
from unittest.mock import patch

import yaml


ROOT = Path(__file__).resolve().parents[2]
PLAY = yaml.safe_load((ROOT / 'infra/lab-vm/ansible/site.yml').read_text())[0]
TASKS = PLAY['tasks']
TASK = next(task for task in TASKS if task.get('register') == 'mysql_authentication')
CODE = TASK['ansible.builtin.command']['argv'][2]
# The contract must also hold when Python assertions are disabled.
COMPILED = compile(CODE, '<mysql-auth-task>', 'exec', optimize=2)
PASSWORD = 'SyntheticSecret12345'
ENV_FILE = '/home/stack/modules-devstack-optimized/infra/devstack/.env'


class Database:
    """Stateful double: socket auth is local; TCP requires the configured password."""

    def __init__(self, plugin='auth_socket', password=None, failure=None,
                 exception=None, tcp_output='1\n', after=None):
        self.plugin = plugin
        self.password = password
        self.failure = failure
        self.exception = exception
        self.tcp_output = tcp_output
        self.after = after
        self.calls = []
        self.alterations = 0
        self.inspections = 0

    def run(self, argv, *, input, text, capture_output, env, timeout):
        assert text and capture_output and timeout == 30
        assert '--no-defaults' in argv and '--user=root' in argv
        assert PASSWORD not in ' '.join(argv)
        tcp = '--protocol=TCP' in argv
        if tcp:
            assert argv[:6] == ['sudo', '-n', '-u', 'stack',
                                '--preserve-env=MYSQL_PWD', 'mysql']
            assert '--host=127.0.0.1' in argv
            assert input == 'SELECT 1;'
            stage = 'tcp'
        else:
            assert argv[0] == 'mysql' and '--protocol=socket' in argv
            if input.startswith('ALTER USER'):
                stage = 'alter'
            else:
                assert input == "SELECT plugin FROM mysql.user WHERE user='root' AND host='localhost';"
                self.inspections += 1
                stage = 'inspect' if self.inspections % 2 else 'verify'
        self.calls.append((stage, argv[:], input, dict(env)))
        if stage == self.failure:
            if self.exception:
                raise self.exception
            # Raw client diagnostics must never be forwarded, even if they echo SQL.
            return subprocess.CompletedProcess(argv, 1, PASSWORD, input + PASSWORD)
        if (tcp or self.plugin != 'auth_socket') and env['MYSQL_PWD'] != self.password:
            return subprocess.CompletedProcess(argv, 1, '', 'Access denied: ' + PASSWORD)
        if stage == 'alter':
            match = re.fullmatch(
                r"ALTER USER 'root'@'localhost' IDENTIFIED WITH caching_sha2_password BY '([A-Za-z0-9]+)';",
                input,
            )
            assert match is not None
            self.plugin = 'caching_sha2_password'
            self.password = match.group(1)
            self.alterations += 1
            output = ''
        elif tcp:
            output = self.tcp_output
        else:
            output = (self.after if stage == 'verify' and self.after is not None
                      else self.plugin) + '\n'
        return subprocess.CompletedProcess(argv, 0, output, '')


class MySQLAuthenticationTests(unittest.TestCase):
    def execute(self, database, content=None, read_error=None):
        stdout, stderr = io.StringIO(), io.StringIO()
        if content is None:
            content = f'# Generated credentials\nDATABASE_PASSWORD={PASSWORD}\nHOST_IP=127.0.0.1\n'

        def read_credentials(path):
            self.assertEqual(str(path), ENV_FILE)
            if read_error:
                raise read_error
            return content

        status = 0
        # No executable, credential file, service or connection is used by the task.
        with patch.object(Path, 'read_text', read_credentials), \
                patch.object(subprocess, 'run', database.run), \
                patch.dict(os.environ, {'MYSQL_PWD': 'InheritedWrongPassword'}), \
                contextlib.redirect_stdout(stdout), contextlib.redirect_stderr(stderr):
            try:
                exec(COMPILED, {'__name__': '__main__'})
            except SystemExit as error:
                status = error.code
            self.assertEqual(os.environ['MYSQL_PWD'], 'InheritedWrongPassword')
        output, errors = stdout.getvalue(), stderr.getvalue()
        self.assertNotIn(PASSWORD, output + errors)
        self.assertNotIn('InheritedWrongPassword', output + errors)
        return status, output, errors

    def assert_failure(self, database, message, **kwargs):
        status, output, errors = self.execute(database, **kwargs)
        self.assertEqual(status, 1)
        self.assertEqual(output, '')
        self.assertEqual(errors, message + '\n')

    def test_socket_authentication_becomes_password_authentication(self):
        database = Database()
        status, output, errors = self.execute(database)
        self.assertEqual((status, errors), (0, ''))
        self.assertEqual(json.loads(output), {
            'changed': True, 'before': 'auth_socket',
            'after': 'caching_sha2_password', 'tcp': 'ok',
        })
        self.assertEqual(database.password, PASSWORD)
        self.assertEqual(database.alterations, 1)
        self.assertEqual([call[0] for call in database.calls],
                         ['inspect', 'alter', 'tcp', 'verify'])

    def test_second_run_is_idempotent(self):
        database = Database()
        self.assertEqual(self.execute(database)[0], 0)
        status, output, errors = self.execute(database)
        self.assertEqual((status, errors), (0, ''))
        self.assertFalse(json.loads(output)['changed'])
        self.assertEqual(database.alterations, 1)
        self.assertEqual([call[0] for call in database.calls[4:]],
                         ['inspect', 'tcp', 'verify'])

    def test_existing_password_plugins_are_preserved(self):
        for plugin in ('caching_sha2_password', 'mysql_native_password'):
            with self.subTest(plugin=plugin):
                database = Database(plugin, PASSWORD)
                status, output, errors = self.execute(database)
                self.assertEqual((status, errors), (0, ''))
                self.assertEqual(json.loads(output), {
                    'changed': False, 'before': plugin, 'after': plugin, 'tcp': 'ok',
                })
                self.assertEqual(database.alterations, 0)

    def test_wrong_existing_password_is_not_reset(self):
        for plugin in ('caching_sha2_password', 'mysql_native_password'):
            with self.subTest(plugin=plugin):
                database = Database(plugin, 'OtherPassword123')
                self.assert_failure(database, 'Cannot inspect existing MySQL root authentication')
                self.assertEqual(len(database.calls), 1)
                self.assertEqual(database.alterations, 0)
                self.assertEqual(database.password, 'OtherPassword123')

    def test_unexpected_or_missing_plugin_is_rejected(self):
        for plugin in ('', 'unknown_plugin', 'auth_socket\nmysql_native_password'):
            with self.subTest(plugin=plugin):
                database = Database(plugin, PASSWORD)
                self.assert_failure(database, 'Unexpected MySQL authentication plugin')
                self.assertEqual(len(database.calls), 1)
                self.assertEqual(database.alterations, 0)

    def test_invalid_password_never_reaches_client(self):
        for password in ('', 'short', 'CHANGE_ME', "QuotePassword123'", 'Has Spaces12345',
                         'NonASCIIá12345', 'TrailingNewline123\n'):
            with self.subTest(password=password):
                database = Database()
                # Embedded newlines in values are represented as a literal shell escape.
                value = password.replace('\n', r'\n')
                self.assert_failure(database, 'Invalid DevStack database password',
                                    content='DATABASE_PASSWORD=' + value)
                self.assertEqual(database.calls, [])

    def test_missing_credentials_and_file_errors_are_sanitized(self):
        database = Database()
        self.assert_failure(database, 'Cannot read DevStack database credentials',
                            content='ADMIN_PASSWORD=irrelevant\n')
        for error in (FileNotFoundError(PASSWORD), PermissionError(PASSWORD)):
            with self.subTest(error=type(error).__name__):
                self.assert_failure(database, 'Cannot read DevStack database credentials',
                                    read_error=error)
        self.assertEqual(database.calls, [])

    def test_client_failures_stop_at_each_stage_without_diagnostics(self):
        messages = {
            'inspect': 'Cannot inspect existing MySQL root authentication',
            'alter': 'Cannot prepare MySQL authentication',
            'tcp': 'DevStack database login failed',
            'verify': 'Cannot verify MySQL authentication plugin',
        }
        for stage, message in messages.items():
            with self.subTest(stage=stage):
                database = Database(failure=stage)
                self.assert_failure(database, message)
                self.assertEqual(database.calls[-1][0], stage)
        for error in (FileNotFoundError(PASSWORD),
                      subprocess.TimeoutExpired(['mysql', PASSWORD], 30),
                      subprocess.SubprocessError(PASSWORD)):
            for stage, message in messages.items():
                with self.subTest(stage=stage, error=type(error).__name__):
                    database = Database(failure=stage, exception=error)
                    self.assert_failure(database, message)
                    self.assertEqual(database.calls[-1][0], stage)

    def test_tcp_must_return_one(self):
        for output in ('', '0\n', '1\n1\n', PASSWORD):
            with self.subTest(output=output):
                database = Database(tcp_output=output)
                self.assert_failure(database, 'DevStack database login failed')
                self.assertEqual(database.calls[-1][0], 'tcp')

    def test_final_plugin_must_match_expected_transition(self):
        for plugin, password, after in (('auth_socket', None, 'auth_socket'),
                                       ('mysql_native_password', PASSWORD, 'caching_sha2_password')):
            with self.subTest(plugin=plugin):
                database = Database(plugin, password, after=after)
                self.assert_failure(database, 'Unexpected MySQL authentication transition')

    def test_secret_only_uses_child_environment_and_sql_stdin(self):
        database = Database()
        self.assertEqual(self.execute(database)[0], 0)
        for stage, argv, sql, env in database.calls:
            self.assertNotIn(PASSWORD, ' '.join(argv))
            self.assertEqual(env['MYSQL_PWD'], PASSWORD)
            if stage == 'alter':
                self.assertIn(PASSWORD, sql)
            else:
                self.assertNotIn(PASSWORD, sql)

    def test_playbook_orders_and_protects_preparation(self):
        position = TASKS.index(TASK)
        generation = next(i for i, task in enumerate(TASKS)
                          if task['name'] == 'Generate unique DevStack credentials once')
        installation = next(i for i, task in enumerate(TASKS)
                            if task['name'] == 'Install pinned DevStack on the dedicated VM')
        self.assertLess(generation, position)
        self.assertLess(position, installation)
        self.assertTrue(PLAY['become'])
        self.assertNotIn('become_user', TASK)
        self.assertTrue(TASK['no_log'])
        self.assertEqual(TASK['changed_when'], '(mysql_authentication.stdout | from_json).changed')
        self.assertFalse(TASK.get('ignore_errors', False))
        self.assertNotIn('when', TASK)  # Also validates credentials on repeat provisioning.
        report = TASKS[position + 1]['ansible.builtin.debug']['msg']
        self.assertNotIn('password', report.lower())
        self.assertNotIn('stderr', report)
        self.assertNotIn('cmd', report)


if __name__ == '__main__':
    unittest.main()

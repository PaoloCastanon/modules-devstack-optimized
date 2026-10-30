import importlib.util
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch

SCRIPT = Path(__file__).resolve().parents[2] / 'scripts/integration/placement_suite.py'
SPEC = importlib.util.spec_from_file_location('placement_suite', SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class SuiteTests(unittest.TestCase):
    def test_opt_in_precedes_files_sdk_and_privilege(self):
        with self.assertRaisesRegex(ValueError, 'opt-in'):
            MODULE.run(SimpleNamespace(allow_mutations=False))

    def test_existing_evidence_refused_before_preparation(self):
        with tempfile.TemporaryDirectory() as folder:
            with self.assertRaises(FileExistsError):
                MODULE.run(SimpleNamespace(allow_mutations=True, output=Path(folder)))

    def test_partial_or_failed_cleanup_cannot_be_a_pass(self):
        report = {'cases': {name: {'passed': True} for name in MODULE.CASES},
                  'cleanup': {name: True for name in MODULE.CLEANUP}, 'execution_error': None}
        self.assertTrue(MODULE.suite_passed(report))
        report['cases'].pop(MODULE.CASES[-1])
        self.assertFalse(MODULE.suite_passed(report))
        report['cases'][MODULE.CASES[-1]] = {'passed': True}
        report['cleanup'][MODULE.CLEANUP[0]] = False
        self.assertFalse(MODULE.suite_passed(report))

    def test_capacity_outcomes_require_consistent_usage_and_no_extra_errors(self):
        self.assertTrue(MODULE.capacity_outcome([204, 204, 204, 204, 409], 16, 4, 16))
        for statuses, used in [([204] * 5, 20), ([204] * 4 + [503], 16), ([204] * 4 + [409], 12)]:
            self.assertFalse(MODULE.capacity_outcome(statuses, 16, 4, used))

    def test_cleanup_requires_exact_name_and_project(self):
        class Compute:
            def get_server(self, ident):
                return SimpleNamespace(id=ident, name=self.name, project_id=self.project)
        connection = SimpleNamespace(compute=Compute())
        for name, project in [('foreign-name', 'p'), ('tt-placement-owned', 'foreign-project')]:
            connection.compute.name, connection.compute.project = name, project
            with self.assertRaisesRegex(ValueError, 'ownership'):
                MODULE.verified_server(connection, 'id', 'tt-placement-owned', 'p')

    def test_failed_server_cleanup_retains_backend_credentials_and_ledger(self):
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder) / 'tt-placement-private'
            root.mkdir()
            runtime, auth = root / 'tt-placement-runtime', root / 'tt-placement-auth'
            runtime.mkdir()
            auth.mkdir()
            (root / 'ledger.json').write_text('{"owned_server":"id"}')
            suite = MODULE.Suite.__new__(MODULE.Suite)
            suite.args = SimpleNamespace(work_root=root)
            suite.runtime_root, suite.auth_root = runtime, auth
            suite.connection = SimpleNamespace(close=lambda: None)
            suite.public = suite.direct = suite.original = None
            suite.servers, suite.flavors, suite.consumers = [{'id': 'owned-server'}], [], []
            suite.report = {'cases': {}, 'cleanup': {key: False for key in MODULE.CLEANUP}, 'cleanup_errors': []}
            suite.checkpoint = lambda: None
            suite.remove_server = Mock(side_effect=TimeoutError('Nova unavailable'))
            suite.action = Mock(side_effect=AssertionError('must retain backend and credentials'))
            with patch.object(MODULE.placement_switch, 'active', return_value=True):
                suite.cleanup()
            suite.action.assert_not_called()
            self.assertFalse(suite.report['cleanup']['runtime_removed'])
            self.assertFalse(suite.report['cleanup']['identity_removed'])
            self.assertTrue(runtime.exists() and auth.exists() and root.exists())
            self.assertEqual((root / 'ledger.json').read_text(), '{"owned_server":"id"}')

    def test_two_coordinators_cannot_hold_the_same_lab_lease(self):
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / 'suite.lock'
            with MODULE.exclusive_run(path):
                with self.assertRaises(BlockingIOError):
                    with MODULE.exclusive_run(path):
                        self.fail('second coordinator acquired shared laboratory')
            with MODULE.exclusive_run(path):
                pass

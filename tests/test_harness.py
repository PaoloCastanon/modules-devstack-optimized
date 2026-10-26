"""Regression checks for gates that must fail instead of reporting false success."""

import contextlib
import importlib.util
import io
from pathlib import Path
import subprocess
import tempfile
import unittest
from unittest.mock import patch

SPEC = importlib.util.spec_from_file_location(
    'harness', Path(__file__).resolve().parents[1] / 'scripts/harness.py'
)
harness = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(harness)


class HarnessTests(unittest.TestCase):
    def setUp(self):
        self.scratch = tempfile.TemporaryDirectory()
        self.addCleanup(self.scratch.cleanup)
        self.root = Path(self.scratch.name)

    def write(self, name, content='content\n'):
        path = self.root / name
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content)
        return path

    def prepare_go(self):
        self.write('go.mod', 'module example.com/test\n\ngo 1.22\n')
        self.write('internal/placement/provider.go', 'package placement\n')
        self.write('internal/placement/provider_test.go', 'package placement\n')

    def run_go(self):
        with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            return harness.verify_go(self.root)

    def test_absent_go_is_explicitly_pending_without_running_tools(self):
        output = io.StringIO()
        with patch.object(harness.subprocess, 'run') as run, contextlib.redirect_stdout(output):
            self.assertEqual(harness.verify_go(self.root), 0)
        run.assert_not_called()
        self.assertIn('PENDING Go', output.getvalue())
        self.assertIn('no Go tests/build were run', output.getvalue())

    def test_go_source_without_module_fails(self):
        self.write('cmd/placement/main.go', 'package main\n')
        self.assertEqual(self.run_go(), 1)

    def test_module_without_go_source_fails(self):
        self.write('go.mod')
        self.assertEqual(self.run_go(), 1)

    def test_runtime_without_tests_fails(self):
        self.write('go.mod')
        self.write('internal/placement/provider.go')
        self.assertEqual(self.run_go(), 1)

    def test_each_present_domain_requires_tests(self):
        self.prepare_go()
        self.write('internal/glance/image.go')
        self.assertEqual(self.run_go(), 1)

    def test_missing_tool_fails(self):
        self.prepare_go()
        with patch.object(harness.shutil, 'which', return_value=None):
            self.assertEqual(self.run_go(), 1)

    def test_unformatted_source_prevents_tests_and_build(self):
        self.prepare_go()
        with patch.object(harness.shutil, 'which', return_value='/tool'), patch.object(
            harness.subprocess, 'run', return_value=subprocess.CompletedProcess([], 0, 'file.go\n', '')
        ) as run:
            self.assertEqual(self.run_go(), 1)
            self.assertEqual(run.call_count, 1)

    def test_test_failure_is_propagated_and_prevents_build(self):
        self.prepare_go()
        results = [subprocess.CompletedProcess([], 0, '', ''),
                   subprocess.CompletedProcess([], 0), subprocess.CompletedProcess([], 2)]
        with patch.object(harness.shutil, 'which', return_value='/tool'), patch.object(
            harness.subprocess, 'run', side_effect=results
        ) as run:
            self.assertEqual(self.run_go(), 2)
            self.assertEqual(run.call_count, 3)

    def test_all_go_gates_execute_when_sources_exist(self):
        self.prepare_go()
        with patch.object(harness.shutil, 'which', return_value='/tool'), patch.object(
            harness.subprocess, 'run', return_value=subprocess.CompletedProcess([], 0, '', '')
        ) as run:
            self.assertEqual(self.run_go(), 0)
        self.assertEqual([call.args[0][:2] for call in run.call_args_list],
                         [['gofmt', '-l'], ['go', 'vet'], ['go', 'test'], ['go', 'build']])
        self.assertIn('-race', run.call_args_list[2].args[0])

    def test_missing_navigation_files_fail(self):
        self.assertEqual(len(harness.check_navigation(self.root)), len(harness.ENTRY_POINTS))

    def test_navigation_rejects_broken_and_external_local_links(self):
        for name in harness.ENTRY_POINTS:
            self.write(name)
        self.write('README.md', '[missing](absent.md)\n[outside](../outside.md)\n')
        errors = harness.check_navigation(self.root)
        self.assertTrue(any('broken local link' in error for error in errors))
        self.assertTrue(any('outside repository' in error for error in errors))

    def test_valid_links_and_remote_references_need_no_network(self):
        for name in harness.ENTRY_POINTS:
            self.write(name)
        self.write('README.md', '[plan](PLANS.md#section)\n[web](https://example.com/page)\n')
        self.assertEqual(harness.check_navigation(self.root), [])

    def test_go_files_in_dependency_and_cache_directories_are_ignored(self):
        self.write('vendor/example/main.go')
        self.write('infra/.cache/main.go')
        self.assertEqual(harness.go_files(self.root), [])


if __name__ == '__main__':
    unittest.main()

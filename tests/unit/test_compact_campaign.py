"""Compact scope, disk aborts and truthful publication are acceptance boundaries."""
import json
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

SCRIPTS = Path(__file__).resolve().parents[2] / 'scripts/benchmarks'
sys.path.insert(0, str(SCRIPTS))
import campaign_protocol


class CompactTests(unittest.TestCase):
    def test_already_off_vm_does_not_issue_another_shutdown(self):
        import complete_placement_campaign as complete
        units = ['devstack@placement-api.service', 'devstack@n-cpu.service', 'devstack@n-sch.service', 'devstack@keystone.service', 'devstack@g-api.service']
        report = {k: True for k in ['original_unchanged', 'runtime_removed', 'identity_removed', 'private_root_removed', 'controls_restored']}
        report['original_services_active'] = dict.fromkeys(units, True)
        with tempfile.TemporaryDirectory() as folder, patch.object(complete.subprocess, 'check_output', return_value='shut off\n'), patch.object(complete, 'cmd') as mutation:
            output = Path(folder) / 'vm.json'
            complete.safely_shutdown({'cleanup': True, 'cleanup_report': report}, output)
            mutation.assert_not_called()
            self.assertFalse(json.loads(output.read_text())['forced'])

    def test_resume_refuses_duplicate_windows_and_different_measurement_revision(self):
        import complete_placement_campaign as complete
        p = json.loads((SCRIPTS.parents[1] / 'docs/benchmarks/placement-protocol-compact-v2.json').read_text())
        m = {'stage': 'measurement', 'git_base': 'frozen', 'protocol': p, 'valid': True, 'cleanup': True,
             'runs': [{'run_id': f'window-{i}', 'valid': True} for i in range(72)]}
        complete.validate_resume_manifest(m, p, 'frozen')
        with self.assertRaises(ValueError):
            complete.validate_resume_manifest(m, p, 'different')
        m['runs'][-1]['run_id'] = m['runs'][0]['run_id']
        with self.assertRaises(ValueError):
            complete.validate_resume_manifest(m, p, 'frozen')

    def test_profile_count_and_no_acceptance_for_inconsistent_matrix(self):
        p = json.loads((SCRIPTS.parents[1] / 'docs/benchmarks/placement-protocol-compact-v2.json').read_text())
        self.assertEqual(campaign_protocol.accounting(p)['runs'], 72)
        p['concurrency'].append(1)
        with self.assertRaises(ValueError):
            campaign_protocol.accounting(p)

    def test_space_refusal_and_running_watch(self):
        from types import SimpleNamespace
        with patch.object(campaign_protocol.shutil, 'disk_usage', return_value=SimpleNamespace(free=100)):
            with self.assertRaises(ValueError):
                campaign_protocol.storage_check(Path('/'), 200)
            with campaign_protocol.StorageWatch(Path('/'), 50, interval=.01) as watch:
                with patch.object(campaign_protocol.shutil, 'disk_usage', return_value=SimpleNamespace(free=25)):
                    self.assertTrue(watch.failed.wait(1))
            self.assertTrue(watch.report()['aborted'])

    def test_three_repetitions_do_not_claim_bootstrap_confidence(self):
        from analyze_placement import bootstrap
        result = bootstrap([1, 2, 100])
        self.assertNotIn('ci95_bootstrap_median', result)
        self.assertEqual(result['median'], 2)
        self.assertEqual((result['min'], result['max']), (1, 100))

    def test_publication_uses_actual_count_and_observed_range(self):
        from publish_placement_results import table_lines
        summary = {'b17_accepted': True, 'b18_accepted': True, 'runs_required': 72,
                   'protocol': {'protocol': 'placement-compact-v2', 'repetitions': 3,
                                'concurrency': [1, 16]}, 'groups': []}
        text = '\n'.join(table_lines(summary, 'B17', []))
        self.assertIn('72', text)
        self.assertIn('observed range', text)
        self.assertNotIn('240', text)
        self.assertNotIn('95%', text)
        summary['b18_accepted'] = False
        with self.assertRaises(ValueError):
            table_lines(summary, 'B17', [])

    def test_late_remote_cleanup_is_collected_but_does_not_validate_failed_run(self):
        from types import SimpleNamespace
        from placement_campaign import finalize_remote
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            class Remote:
                def call(self, *_):
                    raise RuntimeError('first cleanup failed')
                def close(self):
                    pass
                def copy_public_evidence(self):
                    (root / 'lab').mkdir()
                    (root / 'lab/cleanup.json').write_text(json.dumps({'private_root_removed': True}))
            manifest = {'valid': False}
            self.assertTrue(finalize_remote(Remote(), root, manifest))
            self.assertTrue(manifest['cleanup_recovered_after_close'])
            self.assertFalse(manifest['valid'])

    def test_log_retention_preserves_recent_diagnostics_with_bounded_archives(self):
        import gzip
        import lab_controls
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            source = root / 'syslog'
            tails = root / 'tails'
            data = b'recent diagnostic\n' * 100
            source.write_bytes(data)
            with patch.object(lab_controls, 'LOGS', [(str(source), 100)]), patch.object(lab_controls, 'TAIL_DIR', tails):
                for _ in range(6):
                    source.write_bytes(data)
                    lab_controls.bound_logs()
            self.assertEqual(source.stat().st_size, 0)
            self.assertEqual(len(list(tails.glob('*.gz'))), 4)
            for archive in tails.glob('*.gz'):
                self.assertEqual(gzip.decompress(archive.read_bytes()), data)

    def test_existing_package_job_is_never_stopped(self):
        import lab_controls
        with tempfile.TemporaryDirectory() as folder:
            path = Path(folder) / 'maintenance-controls.json'
            with patch.object(lab_controls, 'unit_state', return_value={'ActiveState': 'active'}), patch.object(lab_controls, 'systemctl') as mutation:
                with self.assertRaisesRegex(ValueError, 'already running'):
                    lab_controls.enter(path)
                mutation.assert_not_called()
                self.assertFalse(path.exists())

    def test_control_setup_failure_restores_only_session_owned_masks(self):
        import lab_controls
        from types import SimpleNamespace
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            original = {u: {'ActiveState': 'inactive', 'UnitFileState': 'static', 'LoadState': 'loaded'} for u in lab_controls.UNITS}
            protected = 'apt-daily.service'
            original[protected]['UnitFileState'] = 'masked-runtime'
            current = {u: dict(s) for u, s in original.items()}
            calls = []
            def command(*args):
                calls.append(args)
                if args == ('restart', 'systemd-journald.service') and len([x for x in calls if x == args]) == 1:
                    raise RuntimeError('injected setup failure')
                if args[0] in ('mask', 'unmask'):
                    current[args[-1]]['UnitFileState'] = 'masked-runtime' if args[0] == 'mask' else original[args[-1]]['UnitFileState']
                return SimpleNamespace(stdout='')
            with patch.object(lab_controls, 'JOURNAL_DIR', root), patch.object(lab_controls, 'unit_state', side_effect=lambda u: dict(current[u])), patch.object(lab_controls, 'systemctl', side_effect=command):
                with self.assertRaisesRegex(RuntimeError, 'injected'):
                    lab_controls.enter(root / 'maintenance-controls.json')
            self.assertEqual(current, original)
            self.assertNotIn(('unmask', '--runtime', protected), calls)
            self.assertTrue(json.loads((root / 'maintenance-controls.json').read_text())['controls_restored'])

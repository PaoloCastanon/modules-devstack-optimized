"""B18 process attribution, statistical units and publication guards."""
import json
from pathlib import Path
import sys
import tempfile
import unittest

SCRIPTS = Path(__file__).resolve().parents[2] / "scripts/benchmarks"
sys.path.insert(0, str(SCRIPTS))
import placement_resources as resources


class ResourceTests(unittest.TestCase):
    def test_archiving_preserves_raw_logs_and_request_bytes(self):
        import gzip
        import complete_placement_campaign as complete
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            warning = b'Public SDK warning: \n\n'
            samples = b'{"elapsed_ns":12,"valid":true}\n'
            (root / 'probe.log').write_bytes(warning)
            (root / 'samples.jsonl').write_bytes(samples)
            complete.compact(root)
            self.assertEqual(gzip.decompress((root / 'probe.log.raw.gz').read_bytes()), warning)
            self.assertEqual(gzip.decompress((root / 'samples.jsonl.gz').read_bytes()), samples)
            self.assertEqual((root / 'probe.log').read_bytes(), b'Public SDK warning:\n')
            self.assertIn('probe.log', json.loads((root / 'text-normalization.json').read_text()))

    def test_shutdown_requires_actual_original_service_verification(self):
        from unittest.mock import patch
        import complete_placement_campaign as complete
        manifest = {'cleanup': True, 'cleanup_report': {key: True for key in
                    ['original_unchanged', 'runtime_removed', 'identity_removed', 'private_root_removed']}}
        with tempfile.TemporaryDirectory() as folder, patch.object(complete, 'cmd') as mutation, patch.object(complete.subprocess, 'check_output', return_value='shut off'):
            with self.assertRaises(ValueError):
                complete.safely_shutdown(manifest, Path(folder) / 'vm.json')
            mutation.assert_not_called()

    def test_cpu_interval_excludes_warmup_and_detects_pid_reuse_and_missing_pss(self):
        def row(second, ticks, pid=10, start=42):
            return {'utc_ns': second * 1_000_000_000, 'read_finished_utc_ns': second * 1_000_000_000 + 100,
                    'collector_cpu_s': second / 1000, 'groups': {'python': {'error': None, 'processes': [
                        {'pid': pid, 'start_ticks': start, 'cpu_ticks': ticks, 'pss_bytes': 1000 + second, 'rss_bytes': 2000}]}}}
        records = [row(0, 1000), row(1, 1100), row(2, 1200), row(3, 1400), row(4, 1600), row(5, 9999)]
        for second, item in enumerate(records):
            item['groups']['keystone'] = {'error': None, 'processes': [{'pid': 20, 'start_ticks': 30, 'cpu_ticks': second * 999999, 'pss_bytes': 100_000_000, 'rss_bytes': 100_000_000}]}
        result = resources.interval_summary(records, 'python', 1_000_000_000, 4_500_000_000,
                                             [1_500_000_000, 3_500_000_000, 4_900_000_000], ticks=100)
        self.assertEqual(result['cpu_seconds'], 5)
        self.assertEqual(result['covered_s'], 3)
        self.assertEqual(result['correct_requests_common_interval'], 2)
        self.assertEqual(result['cpu_s_per_correct_request'], 2.5)
        self.assertEqual(result['pss_peak_observed_bytes'], 1004)
        records[2]['groups']['python']['processes'][0]['start_ticks'] += 1
        with self.assertRaises(ValueError):
            resources.interval_summary(records, 'python', 1_000_000_000, 4_500_000_000, [], ticks=100)
        records[2]['groups']['python']['processes'][0]['start_ticks'] -= 1
        records[2]['groups']['python']['processes'][0]['pss_bytes'] = None
        with self.assertRaises(ValueError):
            resources.interval_summary(records, 'python', 1_000_000_000, 4_500_000_000, [], ticks=100)

    def test_bootstrap_repetitions_and_publication_refuse_partial_acceptance(self):
        import analyze_placement
        import publish_placement_results
        result = analyze_placement.bootstrap([1, 2, 3, 4, 100], count=1000)
        self.assertEqual(result['n'], 5)
        self.assertEqual(result['median'], 3)
        self.assertEqual(result, analyze_placement.bootstrap([1, 2, 3, 4, 100], count=1000))
        with tempfile.TemporaryDirectory() as folder:
            output = Path(folder) / 'results'
            with self.assertRaises(ValueError):
                publish_placement_results.render({'b17_accepted': True, 'b18_accepted': False}, output)
            self.assertFalse(output.exists())

"""Joint Placement x Glance API campaign: plan, protocol guards, overlap, lab units and analysis (offline)."""
import gzip
import json
from pathlib import Path
import re
import sys
import tempfile
import unittest
import uuid

ROOT = Path(__file__).resolve().parents[2]
sys.path[:0] = [str(ROOT / 'scripts/benchmarks'), str(ROOT / 'scripts/integration')]
import analyze_joint_api as analyze  # noqa: E402
import joint_api_campaign as campaign  # noqa: E402
import joint_lab  # noqa: E402

PROTOCOL = json.loads((ROOT / 'docs/benchmarks/joint-api-protocol-v1.json').read_text())
CAMPAIGN = str(uuid.UUID(int=7))


class PlanTest(unittest.TestCase):
    def test_official_plan_matches_accounting(self):
        self.assertEqual(campaign.validate_protocol(PROTOCOL), 72)
        plan = campaign.schedule(PROTOCOL, CAMPAIGN)
        self.assertEqual(len(plan), 72)
        self.assertEqual(len({p['run_id'] for p in plan}), 72)

    def test_each_block_has_the_four_variants_contiguously(self):
        plan = campaign.schedule(PROTOCOL, CAMPAIGN)
        for i in range(0, len(plan), 4):
            block = plan[i:i + 4]
            self.assertEqual(len({p['block'] for p in block}), 1)
            self.assertEqual(sorted(p['variant'] for p in block), sorted(campaign.VARIANTS))

    def test_ids_never_reused_and_class_is_owned(self):
        plan = campaign.schedule(PROTOCOL, CAMPAIGN)
        providers = [x for p in plan for x in p['placement']['providers']]
        self.assertEqual(len(providers), len(set(providers)))
        self.assertEqual(len({p['glance']['namespace'] for p in plan}), 72)
        for p in plan:
            self.assertRegex(p['placement']['custom'], r'^CUSTOM_TT_B17_[A-F0-9]{16}$')
            self.assertRegex(p['placement']['run_id'], r'^[a-z0-9-]+$')
            pl, gl = p['variant'].split('-')
            self.assertEqual((p['placement']['implementation'], p['glance']['implementation']), (pl, gl))
            self.assertEqual(p['placement']['concurrency'], p['glance']['concurrency'])

    def test_plan_is_seeded_and_pilot_smaller(self):
        self.assertEqual(campaign.schedule(PROTOCOL, CAMPAIGN), campaign.schedule(PROTOCOL, CAMPAIGN))
        self.assertEqual(len(campaign.schedule(PROTOCOL, CAMPAIGN, pilot=True)), PROTOCOL['pilot']['required_windows'])

    def test_protocol_guards(self):
        bad = dict(PROTOCOL, required_official_windows=71)
        with self.assertRaises(ValueError):
            campaign.validate_protocol(bad)
        bad = dict(PROTOCOL, pairs=[dict(PROTOCOL['pairs'][0], glance='upload-1m')])
        with self.assertRaises(ValueError):
            campaign.validate_protocol(bad)


class OverlapTest(unittest.TestCase):
    def window(self, start, seconds):
        import datetime
        t0 = datetime.datetime(2026, 11, 28, tzinfo=datetime.timezone.utc) + datetime.timedelta(seconds=start)
        return {'start_utc': t0.isoformat(), 'end_utc': (t0 + datetime.timedelta(seconds=seconds)).isoformat(), 'elapsed_s': seconds}

    def test_overlap(self):
        self.assertEqual(campaign.overlap_ok(self.window(0, 30), self.window(1, 30), 0.9), (True, 29.0))
        self.assertEqual(campaign.overlap_ok(self.window(0, 30), self.window(5, 30), 0.9)[0], False)
        self.assertEqual(campaign.overlap_ok(None, self.window(0, 30), 0.9), (False, 0.0))


class LabTest(unittest.TestCase):
    def test_collector_units_cover_both_services_and_controls(self):
        units = dict(joint_lab.collector_units({'go_unit': 'pg', 'db_unit': 'pdb'}, {'go_unit': 'gg'}))
        for label in ('placement-python', 'placement-go', 'placement-go-mysql', 'glance-python', 'glance-go',
                      'mysql', 'apache', 'keystone', 'nova-compute'):
            self.assertIn(label, units)
        with self.assertRaises(AssertionError):
            joint_lab.collector_units({'go_unit': 'pg', 'db_unit': ''}, {'go_unit': 'gg'})


T0 = 1_800_000_000_000_000_000


def iso(ns):
    import datetime
    return datetime.datetime.fromtimestamp(ns / 1e9, datetime.timezone.utc).isoformat()


def records(labels, cpu_per_s, seconds=60):
    """1 Hz rows from T0; each group one process with linear CPU ticks."""
    rows = []
    for i in range(seconds):
        t = T0 + i * 1_000_000_000
        rows.append({'utc_ns': t, 'read_finished_utc_ns': t + 1000, 'monotonic_ns': t,
                     'vm_cpu_stat_ticks': [i * 100, 0, i * 100, i * 200, 0, 0, 0, 0, 0, 0],
                     'vm_loadavg': [1.0, 1.0, 1.0], 'vm_mem_available_kib': 4_000_000, 'collector_cpu_s': i * 0.001,
                     'groups': {label: {'processes': [{'pid': 10 + n, 'start_ticks': 1, 'cpu_ticks': int(i * cpu_per_s[label]),
                                                       'pss_bytes': (n + 1) * 10 * 1024**2, 'rss_bytes': 1}], 'error': None}
                                for n, label in enumerate(labels)}})
    return rows


class AnalyzeTest(unittest.TestCase):
    def build(self, root, variant, rep, ops):
        run_id = 'nova-c01-r%02d-%s' % (rep, variant)
        pl, gl = variant.split('-')
        start, end = T0 + 15 * 10**9, T0 + 45 * 10**9
        (root / run_id / 'placement').mkdir(parents=True)
        (root / run_id / 'glance').mkdir(parents=True)
        clock = {'best': {'remote_minus_host_ns': 0, 'rtt_ns': 1000}}
        (root / run_id / 'report.json').write_text(json.dumps({'clocks': {'before': clock, 'after': clock}, 'overlap_s': 30.0}))
        for service, name in (('placement', 'samples.jsonl.gz'), ('glance', 'measurement-samples.jsonl.gz')):
            samples = [{'valid': True, 'operation': 'op', 'offset_ns': i * 10**9 // ops, 'elapsed_ns': 2_000_000,
                        'ttfb_ns': 1_000_000} for i in range(ops * 30)]
            with gzip.open(root / run_id / service / name, 'wt') as stream:
                stream.write(''.join(json.dumps(s) + '\n' for s in samples))
            m = {'start_utc': iso(start), 'end_utc': iso(end), 'elapsed_s': 30.0, 'requests': len(samples), 'correct': len(samples),
                 'errors': 0, 'correct_ops_s': len(samples) / 30, 'p99_tail_supported': False}
            lat = {'p50': 2.0, 'p95': 2.0, 'p99': 2.0}
            if service == 'placement':
                m.update(correct_latency_ms=lat, cycles_per_worker=[len(samples)])
            else:
                m.update(latency_ms=lat, cycles=len(samples) // 3, validated_payload_mib_s=16.0)
            (root / run_id / service / 'report.json').write_text(json.dumps(
                {'measurement': m, 'warmup': {'start_utc': iso(T0 + 8 * 10**9)}}))
        labels = ['placement-' + pl, 'glance-' + gl, 'mysql']
        res = root / 'lab' / (run_id + '-resources')
        res.mkdir(parents=True)
        cpu = {'placement-' + pl: 10 if pl == 'python' else 1, 'glance-' + gl: 20, 'mysql': 5}
        (res / 'resources.jsonl').write_text(''.join(json.dumps(r) + '\n' for r in records(labels, cpu)))
        (res / 'collector.json').write_text(json.dumps({'clock_ticks_s': 100}))
        return {'run_id': run_id, 'pair': 'nova', 'concurrency': 1, 'repetition': rep, 'variant': variant}

    def test_window_metrics_cells_and_paired_ratios(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            seq = [self.build(root, v, r, 10) for r in (1, 2) for v in ('python-python', 'go-go')]
            (root / 'manifest.json').write_text(json.dumps({'campaign_id': CAMPAIGN, 'stage': 'measurement', 'valid': True,
                                                            'cleanup': True, 'sequence': seq,
                                                            'runs': [{'run_id': s['run_id'], 'valid': True} for s in seq]}))
            result = analyze.analyze(root)
        self.assertTrue(result['accepted'], result['errors'])
        row = result['runs'][0]
        self.assertAlmostEqual(row['placement']['correct_ops_s'], 10.0)
        self.assertEqual(row['placement']['pss_mean_mib'], 10.0)
        self.assertEqual(row['placement']['idle_pss_mib'], 10.0)
        self.assertEqual(row['stack_pss_mib'], 30.0)
        self.assertIn('mysql', row['controls'])
        self.assertAlmostEqual(row['vm_busy_fraction'], 0.5)
        go = next(c for c in result['cells'] if c['variant'] == 'go-go')
        py = next(c for c in result['cells'] if c['variant'] == 'python-python')
        self.assertLess(go['placement']['cpu_ms_per_correct_request']['median'], py['placement']['cpu_ms_per_correct_request']['median'])
        ratio = result['ratios_vs_python_python'][0]
        self.assertEqual(ratio['placement:cpu_ms_per_correct_request']['n'], 2)
        self.assertAlmostEqual(ratio['placement:cpu_ms_per_correct_request']['median'], 0.1, places=2)
        self.assertIn('go-go', analyze.markdown(result))

    def test_invalid_window_is_reported_not_dropped(self):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            seq = [self.build(root, 'go-go', 1, 5)]
            (root / 'manifest.json').write_text(json.dumps({'campaign_id': CAMPAIGN, 'stage': 'measurement', 'valid': False,
                                                            'cleanup': True, 'sequence': seq,
                                                            'runs': [{'run_id': seq[0]['run_id'], 'valid': False}]}))
            result = analyze.analyze(root)
        self.assertFalse(result['accepted'])
        self.assertEqual(result['errors'][0]['error'], 'invalid window')
        self.assertTrue(re.search('Ventanas con error', analyze.markdown(result)))


class GateTest(unittest.TestCase):
    def run_with(self, placement, glance):
        import threading
        storage = type('S', (), {'failed': threading.Event()})()
        saved = campaign.placement_window.run, campaign.glance_window.run
        campaign.placement_window.run, campaign.glance_window.run = placement, glance
        try:
            return campaign.run_window({}, {'placement': {}, 'glance': {}}, {'placement': 't', 'glance': 't'},
                                       Path('/nonexistent'), PROTOCOL, True, storage)
        finally:
            campaign.placement_window.run, campaign.glance_window.run = saved

    def test_measured_phases_start_together(self):
        import time
        starts = {}

        def slow(*args, measurement_gate=None, **kw):
            time.sleep(0.3)  # longer warm-up cleanup
            measurement_gate()
            starts['placement'] = time.monotonic()
            return {'valid': True}

        def fast(*args, measurement_gate=None, **kw):
            measurement_gate()
            starts['glance'] = time.monotonic()
            return {'valid': True}

        p, g, errors = self.run_with(slow, fast)
        self.assertEqual(errors, {})
        self.assertLess(abs(starts['placement'] - starts['glance']), 0.05)

    def test_failed_side_releases_the_other(self):
        def broken(*args, measurement_gate=None, **kw):
            raise ValueError('warm-up failed')

        def waits(*args, measurement_gate=None, **kw):
            measurement_gate()
            return {'valid': True}

        p, g, errors = self.run_with(broken, waits)
        self.assertEqual(errors, {'placement': 'ValueError'})
        self.assertEqual(g, {'valid': True})


if __name__ == '__main__':
    unittest.main()

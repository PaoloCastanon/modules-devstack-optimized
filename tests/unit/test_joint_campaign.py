"""Joint Placement x Glance campaign: plan, trace parsing, acceptance and analysis (offline)."""
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
sys.path[:0] = [str(ROOT / 'scripts/integration'), str(ROOT / 'scripts/benchmarks')]


def load(path, name):
    spec = importlib.util.spec_from_file_location(name, ROOT / path)
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


joint = load('scripts/integration/joint_campaign.py', 'joint_campaign')
sampler = load('scripts/benchmarks/cgroup_sampler.py', 'cgroup_sampler')
analyze = load('scripts/benchmarks/analyze_joint.py', 'analyze_joint')
PROTOCOL = json.loads((ROOT / 'docs/benchmarks/joint-protocol-compact-v3.json').read_text())
IMAGE = '3f0b7c9e-1d2a-4b5c-8d6e-7f8091a2b3c4'
T0 = 1_700_000_000_000_000


def entry(ts, message):
    return json.dumps({'__REALTIME_TIMESTAMP': str(ts), 'MESSAGE': message})


class PlanTest(unittest.TestCase):
    def test_official_plan_covers_the_matrix_once_per_repetition(self):
        plan = joint.block_plan(PROTOCOL)
        self.assertEqual(len(plan), PROTOCOL['required_official_blocks'])
        self.assertEqual(sum(b['concurrency'] for b in plan), PROTOCOL['required_measured_boots'])
        for rep in (1, 2, 3):
            cells = {(b['variant'], b['concurrency']) for b in plan if b['repetition'] == rep}
            self.assertEqual(len(cells), 8)
        self.assertEqual([b['index'] for b in plan], list(range(1, 25)))

    def test_variants_are_contiguous_so_each_repetition_switches_four_times(self):
        plan = joint.block_plan(PROTOCOL)
        runs = [b['variant'] for i, b in enumerate(plan) if i == 0 or plan[i - 1]['variant'] != b['variant']]
        self.assertEqual(len(runs), 12)

    def test_plan_is_seeded_and_pilot_is_separate(self):
        self.assertEqual(joint.block_plan(PROTOCOL), joint.block_plan(PROTOCOL))
        pilot = joint.block_plan(PROTOCOL, pilot=True)
        self.assertEqual(len(pilot), PROTOCOL['pilot']['required_blocks'])
        self.assertEqual(sum(b['concurrency'] for b in pilot), PROTOCOL['pilot']['required_measured_boots'])

    def test_variant_services(self):
        self.assertEqual(joint.variant_services(PROTOCOL, 'go-python'), ('go', 'python'))
        with self.assertRaises(AssertionError):
            joint.variant_services(PROTOCOL, 'go-rust')

    def test_base_cache_name_matches_nova(self):
        # nova.virt.imagecache.get_cache_fname: sha1 of the image id
        self.assertEqual(joint.base_cache_name('abc'), 'a9993e364706816aba3e25717850c26c9cd0d89d')


class TraceTest(unittest.TestCase):
    def test_uwsgi_lines_parse_with_ids_and_window(self):
        line = ('[pid: 1|app: 0|req: 5/10] 127.0.0.1 () {34 vars in 738 bytes} [Fri Nov 27 21:48:11 2026] '
                'GET /v2/images/%s/file => generated 21430272 bytes in 55 msecs (HTTP/1.1 200) 4 headers' % IMAGE)
        lines = [entry(T0, line), entry(T0 + 10**7, line), entry(T0, 'DEBUG unrelated')]
        events = joint.uwsgi_events(lines, T0 - 1, T0 + 1)
        self.assertEqual(len(events), 1)
        self.assertEqual(events[0]['route'], '/v2/images/{id}/file')
        self.assertEqual(events[0]['path_ids'], [IMAGE])
        self.assertTrue(joint.glance_trace_ok(events, [IMAGE], 21430272))
        self.assertFalse(joint.glance_trace_ok(events, [IMAGE], 21430273))  # partial body
        self.assertFalse(joint.glance_trace_ok(events, [IMAGE, IMAGE.replace('3f', '4f')], 1))

    def test_placement_uwsgi_query_and_prefix(self):
        line = '] GET /placement/allocation_candidates?resources=VCPU:1 => generated 9 bytes in 3 msecs (HTTP/1.1 200)'
        events = joint.uwsgi_events([entry(T0, line)], T0, T0)
        self.assertEqual(events[0]['route'], '/allocation_candidates')

    def test_go_events_and_placement_acceptance(self):
        lines = [entry(T0, json.dumps({'msg': 'request completed', 'method': 'GET', 'route': '/allocation_candidates',
                                       'status': 200, 'duration_ms': 2, 'user_id': 'u'})),
                 entry(T0 + 1, json.dumps({'msg': 'request completed', 'method': 'PUT', 'route': '/allocations/{consumer_uuid}',
                                           'status': 204, 'duration_ms': 3})),
                 entry(T0 + 2, 'not json')]
        events = joint.go_events(lines, T0, T0 + 5)
        summary = joint.trace_summary(events)
        self.assertEqual(summary['requests'], 2)
        self.assertTrue(joint.placement_trace_ok(summary, 1))
        self.assertFalse(joint.placement_trace_ok(summary, 2))  # one claim per server
        self.assertFalse(joint.placement_trace_ok(joint.trace_summary([]), 1))

    def test_go_glance_downloads_count_without_ids(self):
        events = [{'method': 'GET', 'route': '/v2/images/{id}/file', 'status': 200, 'response_bytes': 10, 'path_ids': []}]
        self.assertTrue(joint.glance_trace_ok(events, [IMAGE], 10))
        self.assertFalse(joint.glance_trace_ok(events, [IMAGE, 'other'], 10))


class ResourceTest(unittest.TestCase):
    def sample(self, ts, cpu, pss, busy, total, group=True):
        return {'ts_us': ts, 'vm': {'busy_ticks': busy, 'total_ticks': total},
                'groups': {'glance-go': {'cpu_usec': cpu, 'pss_kib': pss} if group else None}}

    def test_window_delta_and_peak(self):
        samples = [self.sample(T0, 0, 1024, 0, 100), self.sample(T0 + 5, 500_000, 4096, 30, 200),
                   self.sample(T0 + 10, 1_000_000, 2048, 50, 300)]
        out = joint.window_resources(samples, T0 + 1, T0 + 9)
        self.assertEqual(out['groups']['glance-go'], {'cpu_s': 1.0, 'pss_max_mib': 4.0, 'pss_mean_mib': 4.0})
        self.assertEqual(out['vm_busy_fraction'], 0.25)

    def test_stopped_group_and_missing_bracket(self):
        samples = [self.sample(T0, 0, 0, 0, 1, group=False), self.sample(T0 + 10, 0, 0, 1, 2, group=False)]
        self.assertIsNone(joint.window_resources(samples, T0 + 1, T0 + 9)['groups']['glance-go']['cpu_s'])
        self.assertIsNone(joint.window_resources(samples[:1], T0 + 1, T0 + 9))

    def test_sampler_groups_and_counters(self):
        with self.assertRaises(ValueError):
            sampler.parse_groups(['a=/x', 'a=/y'])
        with self.assertRaises(ValueError):
            sampler.parse_groups(['a=../x'])
        self.assertEqual(sampler.usage_usec('usage_usec 42\nuser_usec 1'), 42)
        with tempfile.TemporaryDirectory() as tmp:
            self.assertIsNone(sampler.sample_group(Path(tmp) / 'missing'))


class AnalyzeTest(unittest.TestCase):
    def block(self, rep, variant, n, active, valid=True):
        pl, gl = variant.split('-')
        return {'repetition': rep, 'variant': variant, 'placement': pl, 'glance': gl, 'concurrency': n, 'valid': valid,
                'makespan_s': max(active), 'servers': [{'active_s': a} for a in active],
                'traces': {'placement-' + pl: {'requests': 4, 'server_ms_total': 10.0},
                           'glance-' + gl: {'requests': 3, 'server_ms_total': 20.0}},
                'resources': {'vm_busy_fraction': 0.5, 'groups': {'placement-' + pl: {'cpu_s': 1.0, 'pss_max_mib': 10.0},
                                                                   'glance-' + gl: {'cpu_s': 2.0, 'pss_max_mib': 20.0}}}}

    def test_cells_ratios_and_invalid_blocks(self):
        blocks = [self.block(1, 'python-python', 1, [10.0]), self.block(1, 'go-go', 1, [8.0]),
                  self.block(2, 'python-python', 1, [12.0]), self.block(2, 'go-go', 1, [9.0]),
                  self.block(3, 'go-go', 1, [99.0], valid=False)]
        result = analyze.analyze(blocks)
        cell = result['cells']['go-go/1']
        self.assertEqual(cell['blocks'], 2)
        self.assertEqual(cell['boot_s']['median'], 8.5)
        self.assertEqual(result['invalid_blocks'], 1)
        ratio = result['ratios']['go-go/1']
        self.assertEqual(ratio['pairs'], 2)
        self.assertEqual(ratio['boot_median_ratio'], {'median': 0.775, 'min': 0.75, 'max': 0.8})
        self.assertIn('go-go', analyze.markdown(result))


class SharedDatabaseTest(unittest.TestCase):
    def test_bookkeeping_database_created_with_the_original_account(self):
        import types
        runtime = load('scripts/integration/placement_runtime.py', 'placement_runtime_under_test')
        executed, connected = [], {}

        class Cursor:
            def __enter__(self):
                return self

            def __exit__(self, *exc):
                return False

            def execute(self, sql):
                executed.append(sql)

        class Conn(Cursor):
            def cursor(self):
                return Cursor()

            def commit(self):
                executed.append('COMMIT')

        def connect(**kw):
            connected.update(kw)
            return Conn()

        fake = types.ModuleType('pymysql')
        fake.connect = connect
        previous = sys.modules.get('pymysql')
        sys.modules['pymysql'] = fake
        try:
            runtime.ensure_shared_database('[database]\nconnection = mysql+pymysql://root:p%40ss@127.0.0.1/glance?charset=utf8\n')
        finally:
            if previous is None:
                del sys.modules['pymysql']
            else:
                sys.modules['pymysql'] = previous
        self.assertEqual(connected['user'], 'root')
        self.assertEqual(connected['password'], 'p@ss')
        self.assertEqual(executed, ['CREATE DATABASE IF NOT EXISTS `glance_go_shared`', 'COMMIT'])


PROTOCOL_V4 = json.loads((ROOT / 'docs/benchmarks/joint-protocol-compact-v4.json').read_text())
SERVER = '0a1b2c3d-4e5f-4a6b-8c7d-9e0f1a2b3c4d'


class ProtocolV4Test(unittest.TestCase):
    def test_plan_accounting(self):
        plan = joint.block_plan(PROTOCOL_V4)
        self.assertEqual(len(plan), PROTOCOL_V4['required_official_blocks'])
        self.assertEqual(sum(b['concurrency'] for b in plan), PROTOCOL_V4['required_measured_boots'])
        pilot = joint.block_plan(PROTOCOL_V4, pilot=True)
        self.assertEqual(len(pilot), PROTOCOL_V4['pilot']['required_blocks'])
        self.assertEqual(sum(b['concurrency'] for b in pilot), PROTOCOL_V4['pilot']['required_measured_boots'])
        self.assertEqual(PROTOCOL_V4['configurations'], PROTOCOL['configurations'])


def coloured(ts, text):
    """journald stores ANSI-coloured lines as a byte array."""
    return json.dumps({'__REALTIME_TIMESTAMP': str(ts), 'MESSAGE': list(text.encode())})


class PhaseTest(unittest.TestCase):
    def test_compute_log_marks_from_coloured_journal(self):
        base = joint.base_cache_name(IMAGE)
        pre = 'INFO nova.compute.manager [\x1b[01;36mNone req-1 \x1b[00;36madmin admin] \x1b[01;35m[instance: %s] ' % SERVER
        s0 = 1_800_000_000_000_000
        lines = [coloured(s0 + 1_000_000, pre + 'Starting instance...\x1b[00m'),
                 coloured(s0 + 1_500_000, pre + 'Start spawning the instance on the hypervisor.\x1b[00m'),
                 coloured(s0 + 2_000_000, 'DEBUG oslo_concurrency.lockutils [\x1b[01;36mNone] \x1b[01;35mLock "%s" "released" by '
                                          '"nova.virt.libvirt.imagebackend.Image.cache.<locals>.fetch_func_sync" :: held 0.812s\x1b[00m' % base),
                 entry(s0, 'Lock "%s" "released" by "other.create_qcow2_image" :: held 9.0s' % base),
                 coloured(s0 + 4_000_000, pre + 'Took 2.50 seconds to spawn the instance on the hypervisor.\x1b[00m'),
                 coloured(s0 + 4_100_000, pre + 'Took 3.10 seconds to build instance.\x1b[00m'),
                 coloured(s0, 'INFO [instance: 11111111-2222-4333-8444-555555555555] Took 9.00 seconds to build instance.')]
        out = joint.compute_log_phases(lines, {SERVER}, [IMAGE])
        self.assertEqual(out['image_fetch_s'], {IMAGE: 0.812})
        mark = out['instances'][SERVER]
        self.assertEqual((mark['spawn_s'], mark['build_s'], mark['compute_start_us']), (2.5, 3.1, s0 + 1_000_000))
        phases = joint.boot_phases(s0 / 1e6, 4.5, mark, 0.812)
        self.assertEqual((phases['to_compute_s'], phases['prepare_s'], phases['post_to_built_s']), (1.0, 0.5, 4.1))
        self.assertEqual((phases['active_poll_lag_s'], phases['spawn_without_fetch_s']), (0.4, 1.688))
        self.assertTrue(all(phases.get(k) is not None for k in joint.REQUIRED_PHASES))
        self.assertNotIn('to_compute_s', joint.boot_phases(None, None, {}, None))


class AnalyzeV4Test(unittest.TestCase):
    def test_phases_cpu_per_boot_idle_and_stack_ratios(self):
        def block(rep, variant, cpu, pss):
            pl, gl = variant.split('-')
            groups = {'placement-' + pl: {'cpu_s': cpu, 'pss_max_mib': pss, 'pss_mean_mib': pss},
                      'glance-' + gl: {'cpu_s': cpu, 'pss_max_mib': pss, 'pss_mean_mib': pss}}
            return {'repetition': rep, 'variant': variant, 'placement': pl, 'glance': gl, 'concurrency': 2, 'valid': True,
                    'makespan_s': 5.0, 'traces': {}, 'resources': {'vm_busy_fraction': 0.5, 'groups': groups},
                    'idle_resources': {'groups': groups},
                    'servers': [{'active_s': 5.0, 'phases': {'to_compute_s': 1.0, 'build_s': 3.0, 'image_fetch_s': 0.5,
                                                              'spawn_s': 2.0, 'active_poll_lag_s': 0.4}}] * 2}
        result = analyze.analyze([block(1, 'python-python', 1.0, 200.0), block(1, 'go-go', 0.1, 20.0)])
        cell = result['cells']['go-go/2']
        self.assertEqual(cell['placement']['cpu_s_per_boot']['median'], 0.05)
        self.assertEqual(cell['stack']['pss_mean_mib']['median'], 40.0)
        self.assertEqual(cell['phases']['build_s']['median'], 3.0)
        ratio = result['ratios']['go-go/2']
        self.assertEqual(ratio['stack_cpu_per_boot_ratio']['median'], 0.1)
        self.assertEqual(ratio['stack_pss_mean_ratio']['median'], 0.1)
        text = analyze.markdown(result)
        self.assertIn('Descarga imagen', text)
        self.assertIn('PSS reposo', text)


if __name__ == '__main__':
    unittest.main()

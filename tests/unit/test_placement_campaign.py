"""Behavior tests for timed claims, paired seeds and process attribution."""
import copy
import json
from pathlib import Path
import sys
import tempfile
import threading
import unittest
import uuid

SCRIPTS = Path(__file__).resolve().parents[2] / 'scripts/benchmarks'
sys.path.insert(0, str(SCRIPTS))
import placement_campaign as campaign
import placement_window as window


class CampaignTests(unittest.TestCase):
    def test_missing_worktree_trust_file_fails_before_creating_outputs_or_ssh(self):
        from types import SimpleNamespace
        from unittest.mock import patch
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder)
            args = SimpleNamespace(allow_mutations=True, known_hosts=root / 'missing-local-cache',
                                   ssh_key=root / 'key', protocol=root / 'protocol', output=root / 'output', private_root=root / 'private')
            with patch.object(campaign, 'Remote') as remote:
                with self.assertRaises(FileNotFoundError):
                    campaign.run(args)
                remote.assert_not_called()
            self.assertFalse(args.output.exists())
            self.assertFalse(args.private_root.exists())

    def test_full_matrix_has_five_independent_pairs_and_balanced_orders(self):
        protocol = json.loads((SCRIPTS.parents[1] / 'docs/benchmarks/protocol-v1.json').read_text())
        ident = str(uuid.uuid4())
        plan = campaign.schedule(protocol, ident)
        self.assertEqual(plan, campaign.schedule(protocol, ident))
        self.assertEqual(len(plan), 120)
        self.assertEqual(len({p for pair in plan for p in pair['providers']}), 1920)
        for scenario in protocol['scenarios']:
            for concurrency in protocol['concurrency']:
                self.assertEqual(sum(p['scenario'] == scenario and p['concurrency'] == concurrency for p in plan), 5)
        self.assertGreater(sum(p['order'][0] == 'python' for p in plan), 40)
        self.assertGreater(sum(p['order'][0] == 'go' for p in plan), 40)

    def test_concurrent_candidates_validate_correlated_usage_and_exact_resources(self):
        self.assertFalse(window.json_match({'generation': True}, {'generation': 1}))
        self.assertFalse(window.json_match({'generation': 1.0}, {'generation': 1}))
        expected = {'allocation_requests': [], 'provider_summaries': {'p': {'resources': {'VCPU': {'capacity': 4, 'used': 0}, 'CUSTOM_X': {'capacity': 64, 'used': 0}}}}}
        value = copy.deepcopy(expected)
        value['provider_summaries']['p']['resources']['VCPU']['used'] = 1
        self.assertFalse(window.candidates_match(value, expected, True, 'CUSTOM_X'))
        value['provider_summaries']['p']['resources']['CUSTOM_X']['used'] = 1
        self.assertTrue(window.candidates_match(value, expected, True, 'CUSTOM_X'))
        self.assertFalse(window.candidates_match(value, expected, False, 'CUSTOM_X'))
        value['provider_summaries']['p']['resources']['VCPU']['capacity'] = 5
        self.assertFalse(window.candidates_match(value, expected, True, 'CUSTOM_X'))

    def test_cleanup_never_adopts_existing_provider_or_class(self):
        from types import SimpleNamespace
        from placement_lab import Lab
        with tempfile.TemporaryDirectory() as folder:
            lab = Lab(SimpleNamespace(work_root=Path(folder), output=Path(folder)))
            lab.owner = {'project_id': 'p', 'user_id': 'u'}
            lab.current = {'implementation': 'python', 'providers': ['foreign'], 'consumers': [],
                           'custom': 'CUSTOM_TT_B17_1234567890ABCDEF', 'authorized_providers': [], 'authorized_class': False}
            lab.targets = {'python': SimpleNamespace(call=lambda *a, **kw: self.fail('unowned resource accessed'))}
            lab.snapshots = lambda impl, values: self.assertEqual(values['providers'], []) or {}
            self.assertTrue(lab.cleanup_seed()['removed'])
            lab.current = {'implementation': 'python', 'providers': ['owned'], 'consumers': [],
                           'custom': 'CUSTOM_TT_B17_1234567890ABCDEF', 'authorized_providers': ['owned'], 'authorized_class': False}
            lab.targets = {'python': SimpleNamespace(call=lambda method, *a, **kw: {'status': 200} if method == 'GET' else self.fail('unowned deletion'))}
            lab.snapshots = lambda impl, values: {'owned': {'provider': {'name': 'someone-else'}, 'usages': {'usages': {}}, 'allocations': {'allocations': {}}}}
            with self.assertRaisesRegex(AssertionError, 'provider ownership'):
                lab.cleanup_seed()

    def test_all_loads_over_real_http_and_fast_wrong_claim_is_rejected(self):
        from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
        from urllib.parse import urlsplit
        provider = str(uuid.uuid4())
        config = {'run_id': 'unit', 'implementation': 'fixture', 'scenario': 'provider', 'concurrency': 1,
                  'providers': [provider], 'consumers': [str(uuid.uuid4())], 'custom': 'CUSTOM_X',
                  'owner': {'project_id': 'project', 'user_id': 'user'},
                  'snapshots': {provider: {'provider': {'uuid': provider, 'generation': 2},
                                            'inventories': {'resource_provider_generation': 2, 'inventories': {'VCPU': {'total': 4}}}}},
                  'candidates_path': '/allocation_candidates?resources=VCPU:1',
                  'candidates': {'allocation_requests': [], 'provider_summaries': {}}}
        state = {'generation': 2, 'claim': None, 'lie': False}
        class Endpoint(BaseHTTPRequestHandler):
            protocol_version = 'HTTP/1.1'
            def handle_call(self):
                body = self.rfile.read(int(self.headers.get('Content-Length', 0)))
                path = urlsplit(self.path).path
                status, value = 200, None
                if path == '/resource_providers/' + provider:
                    value = {'uuid': provider, 'generation': state['generation']}
                elif path.endswith('/inventories'):
                    value = {'resource_provider_generation': state['generation'], 'inventories': {'VCPU': {'total': 4}}}
                elif path == '/allocation_candidates':
                    value = config['candidates']
                elif path.startswith('/allocations/'):
                    if self.command == 'PUT':
                        value = json.loads(body)
                        if value['consumer_generation'] is not None or state['claim'] is not None:
                            status = 409
                        else:
                            status = 204
                            if not state['lie']:
                                state['generation'] += 1
                                state['claim'] = {'allocations': {provider: {'generation': state['generation'], 'resources': value['allocations'][provider]['resources']}},
                                                  'project_id': value['project_id'], 'user_id': value['user_id'], 'consumer_generation': 1}
                        value = None
                    elif self.command == 'DELETE':
                        state['claim'] = None
                        status = 204
                    else:
                        value = state['claim'] or {'allocations': {}}
                raw = b'' if value is None else json.dumps(value).encode()
                self.send_response(status)
                self.send_header('Content-Length', str(len(raw)))
                self.end_headers()
                self.wfile.write(raw)
            do_GET = do_PUT = do_DELETE = handle_call
            def log_message(self, *args):
                pass
        server = ThreadingHTTPServer(('127.0.0.1', 0), Endpoint)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        config['url'] = 'http://127.0.0.1:' + str(server.server_port)
        try:
            with tempfile.TemporaryDirectory() as folder:
                for scenario in ['provider', 'inventories', 'allocations-read', 'candidates', 'claim-cycle', 'nova-mix']:
                    config['scenario'] = scenario
                    state.update(generation=2, claim=None)
                    report = window.run(config, 'unit-private-token', Path(folder) / scenario, 'instrument-check', duration=.35, warmup=.15)
                    self.assertTrue(report['valid'], scenario + ': ' + json.dumps(report))
                    self.assertIsNone(state['claim'])
                    self.assertEqual(report['measurement']['errors'], 0)
                    raw = (Path(folder) / scenario / 'samples.jsonl').read_text()
                    self.assertNotIn('unit-private-token', raw + json.dumps(report))
                    self.assertTrue(all(json.loads(line)['offset_ns'] < 350_000_000 for line in raw.splitlines()))
                state.update(generation=2, claim=None, lie=True)
                config['scenario'] = 'claim-cycle'
                report = window.run(config, 'unit-private-token', Path(folder) / 'wrong-claim', 'instrument-check', duration=.35, warmup=0)
                self.assertFalse(report['valid'])
                self.assertGreater(report['measurement']['errors'], 0)
        finally:
            server.shutdown()
            server.server_close()
            thread.join()

"""Independent HTTP fixture: full byte validation and bounded, private samples."""
import hashlib
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import json
from pathlib import Path
import sys
import tempfile
import threading
import unittest
import uuid

sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'scripts/benchmarks'))
sys.path.insert(0, str(Path(__file__).resolve().parents[2] / 'scripts/integration'))
import glance_http


class GlanceBenchmarkTests(unittest.TestCase):
    def test_runtime_rejects_unsupported_domain_before_any_command(self):
        from types import SimpleNamespace
        from unittest.mock import patch
        import placement_runtime
        with patch.object(placement_runtime, 'command') as mutation:
            with self.assertRaises(ValueError):
                placement_runtime.run(SimpleNamespace(allow_mutations=True, root=Path('/not-used'), domain='unknown'))
            mutation.assert_not_called()

    def test_glance_route_is_only_fixed_image_prefix_and_bounded_port(self):
        import glance_switch
        text = glance_switch.replacement(19293).decode()
        self.assertIn('"/image"', text)
        self.assertNotIn('/placement', text)
        for port in [True, 80, 65536, '19293']:
            with self.assertRaises(ValueError):
                glance_switch.replacement(port)

    def test_payload_is_reproducible_and_refuses_existing_file(self):
        with tempfile.TemporaryDirectory() as folder:
            a, b = Path(folder) / 'a', Path(folder) / 'b'
            first = glance_http.payload(a, 100_003)
            self.assertEqual(first, glance_http.payload(b, 100_003))
            self.assertEqual(first['sha256'], hashlib.sha256(a.read_bytes()).hexdigest())
            with self.assertRaises(FileExistsError):
                glance_http.payload(a, 100_003)

    def test_streamed_round_trip_and_wrong_200_are_distinguished(self):
        state = {'body': b'', 'corrupt': False}
        class Handler(BaseHTTPRequestHandler):
            protocol_version = 'HTTP/1.1'
            def log_message(self, *_):
                pass
            def do_PUT(self):
                state['body'] = self.rfile.read(int(self.headers['Content-Length']))
                self.send_response(204); self.send_header('Content-Length', '0'); self.end_headers()
            def do_GET(self):
                body = state['body'] if not state['corrupt'] else b'x' * len(state['body'])
                self.send_response(200); self.send_header('Content-Length', str(len(body))); self.end_headers(); self.wfile.write(body)
        server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
        try:
            with tempfile.TemporaryDirectory() as folder:
                p = Path(folder) / 'bytes'
                expected = glance_http.payload(p, 250_003)
                client = glance_http.Client(f'http://127.0.0.1:{server.server_port}/image', 'PRIVATE-TOKEN')
                sample, _ = client.request('PUT', '/v2/images/own/file', upload=p, content_type='application/octet-stream')
                self.assertEqual(sample['status'], 204)
                self.assertIsNone(sample['error_kind'])
                self.assertEqual(hashlib.sha256(state['body']).hexdigest(), expected['sha256'])
                sample, _ = client.request('GET', '/v2/images/own/file', expected_payload=expected)
                self.assertIsNone(sample['error_kind'])
                state['corrupt'] = True
                sample, _ = client.request('GET', '/v2/images/own/file', expected_payload=expected)
                self.assertEqual(sample['error_kind'], 'payload_mismatch')
                self.assertNotIn('PRIVATE-TOKEN', json.dumps(sample))
                client.close()
        finally:
            server.shutdown(); server.server_close(); thread.join()

    def test_metadata_never_coerces_size_or_ignores_state(self):
        expected = {'size': 1024, 'status': 'active', 'tags': ['owned', 'seed']}
        self.assertTrue(glance_http.metadata_matches({'size': 1024, 'status': 'active', 'tags': ['seed', 'owned']}, expected))
        self.assertFalse(glance_http.metadata_matches({'size': 1024.0, 'status': 'active', 'tags': ['seed', 'owned']}, expected))
        self.assertFalse(glance_http.metadata_matches({'size': 1024, 'status': 'saving', 'tags': ['seed', 'owned']}, expected))

    def test_real_cycle_removes_owned_images_and_never_deletes_wrong_owner(self):
        import gzip
        import glance_window
        images, mode, deletions = {}, {'wrong_owner': False}, []
        class Handler(BaseHTTPRequestHandler):
            protocol_version = 'HTTP/1.1'
            def log_message(self, *_):
                pass
            def reply(self, status, body=None):
                data = json.dumps(body).encode() if body is not None else b''
                self.send_response(status); self.send_header('Content-Length', str(len(data))); self.end_headers(); self.wfile.write(data)
            def do_GET(self):
                ident = self.path.rsplit('/', 1)[-1]
                self.reply(200, images[ident]) if ident in images else self.reply(404)
            def do_POST(self):
                image = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
                image.update(owner='foreign' if mode['wrong_owner'] else 'project', status='queued')
                images[image['id']] = image
                self.reply(201, image)
            def do_PATCH(self):
                patch = json.loads(self.rfile.read(int(self.headers['Content-Length'])))
                ident = self.path.rsplit('/', 1)[-1]
                images[ident]['name'] = patch[0]['value']; self.reply(200, images[ident])
            def do_DELETE(self):
                ident = self.path.rsplit('/', 1)[-1]; deletions.append(ident); images.pop(ident); self.reply(204)
        server = ThreadingHTTPServer(('127.0.0.1', 0), Handler)
        thread = threading.Thread(target=server.serve_forever, daemon=True); thread.start()
        try:
            with tempfile.TemporaryDirectory() as folder:
                config = {'url': f'http://127.0.0.1:{server.server_port}/image', 'run_id': 'fixture',
                          'scenario': 'catalog-lifecycle', 'implementation': 'fixture', 'concurrency': 1,
                          'namespace': str(uuid.uuid4()), 'tag': 'owned', 'owner': 'project'}
                out = Path(folder) / 'ok'
                result = glance_window.run(config, 'PRIVATE-TOKEN', out, duration=.05, warmup=0)
                self.assertTrue(result['valid'])
                self.assertFalse(images)
                self.assertGreater(result['measurement']['cycles'], 0)
                with gzip.open(out / 'measurement-samples.jsonl.gz', 'rt') as f:
                    self.assertNotIn('PRIVATE-TOKEN', f.read())
                mode['wrong_owner'] = True
                before = len(deletions)
                config['namespace'] = str(uuid.uuid4())
                result = glance_window.run(config, 'PRIVATE-TOKEN', Path(folder) / 'foreign', duration=.05, warmup=0)
                self.assertFalse(result['valid'])
                self.assertFalse(result['cleanup_ok'])
                self.assertEqual(len(deletions), before)
                self.assertIn('unproven image ownership', result['cleanup_errors'][0])
        finally:
            server.shutdown(); server.server_close(); thread.join()


class GlanceCampaignTests(unittest.TestCase):
    @staticmethod
    def protocol():
        return json.loads((Path(__file__).resolve().parents[2] / 'docs/benchmarks/glance-protocol-compact-v2.json').read_text())

    def test_matrix_is_complete_unique_paired_and_reproducible(self):
        import glance_campaign
        p, namespace = self.protocol(), str(uuid.uuid4())
        plan = glance_campaign.schedule(p, namespace)
        self.assertEqual(len(plan), 54)
        self.assertEqual(plan, glance_campaign.schedule(p, namespace))
        self.assertEqual(len({row['namespace'] for row in plan}), 54)
        self.assertTrue(all(set(row['order']) == {'python', 'go'} for row in plan))
        self.assertEqual(len(glance_campaign.schedule(p, namespace, True)), 18)
        p['scenarios'][-1]['size_bytes'] = 1
        with self.assertRaisesRegex(ValueError, 'payload size'):
            glance_campaign.schedule(p, namespace)

    def test_cleanup_never_adopts_foreign_uuid_owner_tag_or_name(self):
        from glance_lab import owned_descriptor
        namespace = uuid.uuid4()
        values = {'namespace': str(namespace), 'tag': 'own-tag', 'concurrency': 16, 'seed_count': 100}
        image = {'id': str(uuid.uuid5(namespace, 'measurement-15-12')), 'name': 'tt-glance-measurement-w15-c12-updated',
                 'tags': ['own-tag'], 'owner': 'own-project'}
        self.assertTrue(owned_descriptor(values, image, 'own-project'))
        for key, value in [('id', str(uuid.uuid4())), ('name', 'tt-glance-measurement-w16-c12'),
                           ('owner', 'foreign'), ('tags', ['foreign'])]:
            self.assertFalse(owned_descriptor(values, {**image, key: value}, 'own-project'))
        seed = {**image, 'name': 'tt-glance-seed-99', 'id': str(uuid.uuid5(namespace, 'seed-99'))}
        self.assertTrue(owned_descriptor(values, seed, 'own-project'))
        self.assertFalse(owned_descriptor(values, {**seed, 'name': 'tt-glance-seed-100', 'id': str(uuid.uuid5(namespace, 'seed-100'))}, 'own-project'))

    def test_analyzer_reconstructs_cycles_and_pilot_is_never_official_acceptance(self):
        """Synthetic resource fixture only: never represented as lab evidence."""
        import datetime
        import gzip
        import analyze_glance
        import glance_campaign
        from glance_campaign import save
        p, campaign = self.protocol(), str(uuid.uuid4())
        plan = glance_campaign.schedule(p, campaign, True)
        timestamp = datetime.datetime(2026, 10, 31, tzinfo=datetime.timezone.utc)
        t = int(timestamp.timestamp() * 1e9)
        def utc(seconds):
            return (timestamp + datetime.timedelta(seconds=seconds)).isoformat()
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp); (root / 'lab').mkdir()
            manifest = {'campaign_id': campaign, 'protocol': p, 'stage': 'pilot', 'sequence': plan, 'valid': True,
                        'cleanup': True, 'git_base': 'fixture', 'protocol_sha256': 'fixture', 'runs': []}
            for pair in plan:
                for impl in ('python', 'go'):
                    ident = pair['pair_id'] + '-' + impl
                    folder = root / ident; folder.mkdir()
                    ops = (['fresh-id', 'create', 'patch', 'verify', 'delete', 'verify-absent'] if pair['scenario'] == 'catalog-lifecycle'
                           else ['fresh-id', 'create', 'upload', 'verify-upload', 'delete', 'verify-absent'] if pair['scenario'].startswith('upload-')
                           else ['download'] if pair['scenario'].startswith('download-') else [pair['scenario']])
                    size = pair['size_bytes'] or 0
                    samples = [{'run_id': ident, 'worker': 0, 'cycle': 0, 'operation': op, 'offset_ns': (index+1)*100_000_000,
                                'elapsed_ns': 10_000_000, 'ttfb_ns': 1_000_000 if op == 'download' else None,
                                'valid': True, 'error_kind': None, 'request_bytes': size if op == 'upload' else 0,
                                'response_bytes': size if op == 'download' else 0} for index, op in enumerate(ops)]
                    with gzip.open(folder / 'measurement-samples.jsonl.gz', 'wt') as f:
                        for row in samples:
                            f.write(json.dumps(row)+'\n')
                    report = {'run_id': ident, 'scenario': pair['scenario'], 'implementation': impl, 'concurrency': pair['concurrency'],
                              'valid': True, 'cleanup_ok': True, 'clock_valid': True, 'clock_drift_ns': 0,
                              'clocks': {key: {'best': {'remote_minus_host_ns': 0, 'rtt_ns': 1000}} for key in ('before','after')},
                              'warmup': {'start_utc': utc(-1)},
                              'measurement': {'start_utc': utc(0), 'end_utc': utc(4), 'elapsed_s': 4, 'requests': len(ops),
                                              'correct': len(ops), 'cycles': 1, 'errors': 0, 'failed': False,
                                              'correct_ops_s': len(ops)/4, 'complete_cycles_s': .25,
                                              'validated_payload_bytes': size, 'validated_payload_mib_s': size/4/1024**2,
                                              'latency_ms': dict.fromkeys(['p50','p95','p99'],10.0)}}
                    save(folder / 'report.json', report)
                    resources = root / 'lab' / (ident+'-resources'); resources.mkdir()
                    save(resources / 'collector.json', {'clock_ticks_s': 100, 'self_terminated_at_limit': False})
                    with (resources / 'resources.jsonl').open('w') as f:
                        for sec in range(-5, 7):
                            row = {'utc_ns': t+sec*10**9, 'read_finished_utc_ns': t+sec*10**9+1000000, 'collector_cpu_s': (sec+5)*.001,
                                   'groups': {impl: {'error': None, 'processes': [{'pid': 123,'start_ticks': 2,'cpu_ticks': (sec+5)*100,
                                                                                'pss_bytes': 1024**2,'rss_bytes': 2*1024**2}]}}}
                            f.write(json.dumps(row)+'\n')
                    manifest['runs'].append({'run_id': ident, 'valid': True})
            save(root / 'manifest.json', manifest)
            result = analyze_glance.analyze(root)
            self.assertTrue(result['pilot_validated'], result['errors'])
            self.assertFalse(result['accepted'])
            self.assertEqual(result['runs_observed'],36)
            with self.assertRaises(ValueError):
                analyze_glance.publish(result, root / 'must-not-publish.md')
            first = root / manifest['runs'][0]['run_id'] / 'report.json'
            report = json.loads(first.read_text()); report['measurement']['requests'] += 1; save(first, report)
            result = analyze_glance.analyze(root)
            self.assertFalse(result['complete_valid'])
            self.assertIn('counts', result['errors'][0]['error'])

    def test_catalog_same_ids_wrong_order_is_rejected_over_real_http(self):
        import glance_window
        namespace = str(uuid.uuid4())
        images = [{'id': str(uuid.uuid4()), 'name': name, 'owner': 'p', 'tags': ['t'], 'status': 'queued'} for name in ['a','b']]
        mode = {'reverse': False}
        class Handler(BaseHTTPRequestHandler):
            protocol_version = 'HTTP/1.1'
            def log_message(self, *_):
                pass
            def do_GET(self):
                body = json.dumps({'images': list(reversed(images)) if mode['reverse'] else images}).encode()
                self.send_response(200); self.send_header('Content-Length', str(len(body))); self.end_headers(); self.wfile.write(body)
        server = ThreadingHTTPServer(('127.0.0.1',0), Handler)
        thread = threading.Thread(target=server.serve_forever,daemon=True); thread.start()
        try:
            with tempfile.TemporaryDirectory() as tmp:
                config = {'run_id':'catalog-fixture','scenario':'catalog-list','implementation':'fixture','concurrency':1,
                          'namespace':namespace,'tag':'t','owner':'p','images':images,'url':f'http://127.0.0.1:{server.server_port}/image'}
                result = glance_window.run(config,'private',Path(tmp)/'correct',duration=.05,warmup=0)
                self.assertTrue(result['valid'])
                mode['reverse'] = True
                result = glance_window.run(config,'private',Path(tmp)/'reversed',duration=.05,warmup=0)
                self.assertFalse(result['valid'])
                self.assertEqual(result['measurement']['errors'],1)
        finally:
            server.shutdown(); server.server_close(); thread.join()

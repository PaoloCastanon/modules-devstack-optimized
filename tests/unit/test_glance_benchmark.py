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

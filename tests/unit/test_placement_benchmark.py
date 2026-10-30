import importlib.util
import json
from pathlib import Path
import tempfile
import subprocess
import sys
import threading
import unittest

SCRIPT = Path(__file__).resolve().parents[2] / 'scripts/benchmarks/placement_http.py'
SPEC = importlib.util.spec_from_file_location('placement_benchmark', SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class BenchmarkTests(unittest.TestCase):
    def test_goodput_does_not_count_fast_failures(self):
        samples = [{'elapsed_ns': 10_000_000, 'valid': True, 'status': 200},
                   {'elapsed_ns': 1_000_000, 'valid': False, 'status': 403},
                   {'elapsed_ns': 2_000_000, 'valid': False, 'status': None}]
        result = MODULE.summarize(samples, 2)
        self.assertEqual(result['correct_ops_s'], .5)
        self.assertEqual(result['completed_requests_s'], 1.5)
        self.assertEqual(result['errors'], 2)
        self.assertEqual(result['correct_latency_ms']['p95'], 10)
        self.assertFalse(result['p99_tail_supported'])

    def test_nearest_rank_and_invalid_samples(self):
        self.assertEqual(MODULE.percentile([1, 2, 3, 4, 100], .95), 100)
        self.assertIsNone(MODULE.percentile([], .99))
        with self.assertRaises(ValueError):
            MODULE.summarize([{'elapsed_ns': -1, 'valid': True, 'status': 200}], 1)

    def test_invalid_provider_body_is_not_a_success(self):
        self.assertTrue(MODULE.valid_body('provider', b'{"uuid":"p","generation":0}', 'p'))
        self.assertFalse(MODULE.valid_body('provider', b'{"uuid":"other","generation":0}', 'p'))
        self.assertFalse(MODULE.valid_body('provider', b'{"uuid":"p","generation":true}', 'p'))
        self.assertFalse(MODULE.valid_body('provider', b'not-json', 'p'))

    def test_existing_output_is_refused_before_http_or_credentials(self):
        from types import SimpleNamespace
        with tempfile.TemporaryDirectory() as folder:
            with self.assertRaises(FileExistsError):
                MODULE.run(SimpleNamespace(output=Path(folder)))

    def test_real_http_validates_state_and_never_saves_credential(self):
        from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
        payload = [b'{"uuid":"p","generation":0}']
        class Endpoint(BaseHTTPRequestHandler):
            protocol_version = 'HTTP/1.1'
            def do_GET(self):
                self.send_response(200)
                self.send_header('Content-Length', str(len(payload[0])))
                self.end_headers()
                self.wfile.write(payload[0])
            def log_message(self, *args):
                pass
        server = ThreadingHTTPServer(('127.0.0.1', 0), Endpoint)
        thread = threading.Thread(target=server.serve_forever, daemon=True)
        thread.start()
        try:
            with tempfile.TemporaryDirectory() as folder:
                root = Path(folder)
                token = root / 'token'
                token.write_text('private-instrument-token')
                token.chmod(0o600)
                expected = root / 'expected.json'
                expected.write_bytes(payload[0])
                for name, expected_exit in [('valid', 0), ('wrong-state', 1)]:
                    if name == 'wrong-state':
                        payload[0] = b'{"uuid":"p","generation":99,"echo":"private-instrument-token"}'
                    output = root / name
                    result = subprocess.run([sys.executable, str(SCRIPT), '--url', 'http://127.0.0.1:' + str(server.server_port),
                                             '--path', '/resource_providers/p', '--kind', 'provider', '--provider-id', 'p',
                                             '--token-file', str(token), '--expected-body-file', str(expected),
                                             '--implementation', 'fixture', '--stage', 'instrument-check',
                                             '--duration', '.08', '--warmup', '0', '--concurrency', '2', '--output', str(output)],
                                            capture_output=True, text=True)
                    self.assertEqual(result.returncode, expected_exit, result.stderr)
                    report = json.loads((output / 'report.json').read_text())
                    raw = (output / 'samples.jsonl').read_text()
                    self.assertEqual(len(raw.splitlines()), report['measurement']['requests'])
                    self.assertNotIn('private-instrument-token', raw + json.dumps(report) + result.stdout + result.stderr)
                    self.assertEqual(report['valid'], expected_exit == 0)
                    if expected_exit:
                        self.assertEqual(report['measurement']['correct_ops_s'], 0)
        finally:
            server.shutdown()
            server.server_close()
            thread.join()

    def test_sampler_reads_real_process_and_rejects_nonfinite_window(self):
        import os
        sampler = SCRIPT.with_name('sample_processes.py')
        with tempfile.TemporaryDirectory() as folder:
            output = Path(folder) / 'sample'
            result = subprocess.run([sys.executable, str(sampler), '--pids', str(os.getpid()), '--run-id', 'unit',
                                     '--duration', '.06', '--interval', '.02', '--output', str(output)], capture_output=True, text=True)
            self.assertEqual(result.returncode, 0, result.stderr)
            report = json.loads((output / 'report.json').read_text())
            self.assertGreaterEqual(report['samples'], 2)
            self.assertGreaterEqual(report['cpu_seconds'], 0)
            bad = Path(folder) / 'bad'
            result = subprocess.run([sys.executable, str(sampler), '--pids', str(os.getpid()), '--run-id', 'unit',
                                     '--duration', 'nan', '--output', str(bad)], capture_output=True, text=True)
            self.assertEqual(result.returncode, 2)
            self.assertFalse(bad.exists())

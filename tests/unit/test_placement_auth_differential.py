from pathlib import Path
import importlib.util
import subprocess
import sys
import tempfile
import unittest

SCRIPT = Path(__file__).resolve().parents[2] / 'scripts/placement_auth_differential.py'
sys.path.insert(0, str(SCRIPT.parent))
SPEC = importlib.util.spec_from_file_location('auth_differential', SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(MODULE)


class AuthCampaignGuardTests(unittest.TestCase):
    def test_existing_evidence_refused_before_http(self):
        with tempfile.TemporaryDirectory() as folder:
            with self.assertRaises(FileExistsError):
                MODULE.run(None, None, Path(folder), 'project', 'user')

    def test_mutation_opt_in_required_before_tokens_or_http(self):
        with tempfile.TemporaryDirectory() as folder:
            output = Path(folder) / 'new'
            result = subprocess.run([sys.executable, str(SCRIPT), '--output', str(output),
                                     '--python-url', 'http://127.0.0.1:1', '--go-url', 'http://127.0.0.1:2',
                                     '--project-id', 'project', '--user-id', 'user', '--tokens-dir', 'missing'],
                                    capture_output=True, text=True)
            self.assertEqual(result.returncode, 2)
            self.assertIn('explicit mutation opt-in', result.stderr)
            self.assertFalse(output.exists())

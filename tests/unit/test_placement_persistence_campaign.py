import importlib.util
from pathlib import Path
import subprocess
import tempfile
from types import SimpleNamespace
import unittest

SCRIPT = Path(__file__).resolve().parents[2] / 'scripts/placement_persistence_campaign.py'
SPEC = importlib.util.spec_from_file_location('persistence', SCRIPT)
MODULE = importlib.util.module_from_spec(SPEC)
# Module dependencies live alongside the script, as when called through its CLI.
import sys
sys.path.insert(0, str(SCRIPT.parent))
SPEC.loader.exec_module(MODULE)


class PersistenceGuardTests(unittest.TestCase):
    def test_existing_evidence_is_refused_before_reading_credentials(self):
        with tempfile.TemporaryDirectory() as folder:
            with self.assertRaises(FileExistsError):
                MODULE.run(SimpleNamespace(output=Path(folder)))

    def test_opt_in_is_required_before_files_or_docker(self):
        with tempfile.TemporaryDirectory() as folder:
            output = Path(folder) / 'must-not-exist'
            result = subprocess.run([sys.executable, str(SCRIPT), '--output', str(output),
                                     '--keystone-url', 'http://127.0.0.1:1/v3',
                                     '--validator-file', 'not-a-file', '--admin-token-file', 'not-a-file'],
                                    capture_output=True, text=True)
            self.assertEqual(result.returncode, 2)
            self.assertFalse(output.exists())
            self.assertIn('explicit mutation opt-in', result.stderr)

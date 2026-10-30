import importlib.util
import json
import os
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import patch

SCRIPTS = Path(__file__).resolve().parents[2] / 'scripts/integration'


def load(name):
    spec = importlib.util.spec_from_file_location(name, SCRIPTS / (name + '.py'))
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class PlacementIntegrationGuards(unittest.TestCase):
    def test_mutation_opt_in_precedes_sdk_files_and_privileged_commands(self):
        for name in ['lab_identity', 'placement_runtime', 'placement_switch', 'placement_boot']:
            with self.subTest(name=name):
                module = load(name)
                with self.assertRaisesRegex(ValueError, 'opt-in'):
                    module.run(SimpleNamespace(allow_mutations=False))

    def test_boot_refuses_existing_evidence_before_sdk_or_deletion(self):
        module = load('placement_boot')
        with tempfile.TemporaryDirectory() as folder:
            with self.assertRaises(FileExistsError):
                module.run(SimpleNamespace(allow_mutations=True, output=Path(folder)))

    def test_foreign_site_digest_is_rejected_before_privileged_operations(self):
        module = load('placement_switch')
        with tempfile.TemporaryDirectory() as folder:
            root = Path(folder) / 'tt-placement-unit'
            root.mkdir(mode=0o700)
            (root / 'state.json').write_text(json.dumps({'prepared': True, 'owner_uid': os.getuid(), 'listen_port': 18779}))
            site = Path(folder) / 'placement-api.conf'
            site.write_text('existing foreign configuration\n')
            args = SimpleNamespace(allow_mutations=True, root=root, action='activate', expected_original_sha='0' * 64)
            with patch.object(module, 'SITE_FILE', site), patch.object(module, 'command', side_effect=AssertionError('privileged operation attempted')):
                with self.assertRaisesRegex(ValueError, 'digest'):
                    module.run(args)
            self.assertEqual(site.read_text(), 'existing foreign configuration\n')

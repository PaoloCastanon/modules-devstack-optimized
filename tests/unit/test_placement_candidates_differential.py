import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import unittest

SCRIPTS = Path(__file__).resolve().parents[2] / 'scripts'
sys.path.insert(0, str(SCRIPTS))
SPEC = importlib.util.spec_from_file_location('candidate_runner', SCRIPTS / 'placement_candidates_differential.py')
RUNNER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(RUNNER)


class CandidateRunnerTests(unittest.TestCase):
    def test_existing_catalogue_cannot_be_mutated_or_cleaned(self):
        class Occupied:
            def __init__(self):
                self.methods = []
            def call(self, method, *args, **kwargs):
                self.methods.append(method)
                return {'status': 200, 'headers': {}, 'body_text': '{}'}
        one, two = Occupied(), Occupied()
        with tempfile.TemporaryDirectory() as folder:
            output = Path(folder) / 'new'
            self.assertEqual(RUNNER.run(one, two, output, 'project', 'user'), 1)
            report = json.loads((output / 'report.json').read_text())
            self.assertEqual(report['cases'], [])
            self.assertEqual(report['cleanup'], [])
            with self.assertRaises(FileExistsError):
                RUNNER.run(one, two, output, 'project', 'user')
        self.assertEqual(set(one.methods + two.methods), {'GET'})

    def test_normalization_never_deduplicates_or_changes_capacity(self):
        class Target:
            origin = ('http', 'fixture')
        def response(values):
            return {'status': 200, 'headers': {}, 'body_text': json.dumps(values)}
        item = {'allocations': {'rp': {'resources': {'VCPU': 1}}}}
        one = RUNNER.canonical(response({'allocation_requests': [item]}), Target())
        two = RUNNER.canonical(response({'allocation_requests': [item, item]}), Target())
        self.assertTrue(RUNNER.base.differences(one, two))
        self.assertTrue(RUNNER.base.differences({'capacity': 4}, {'capacity': 5}))

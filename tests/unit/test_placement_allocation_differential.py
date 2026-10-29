import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import unittest

SCRIPTS = Path(__file__).resolve().parents[2] / 'scripts'
sys.path.insert(0, str(SCRIPTS))
SPEC = importlib.util.spec_from_file_location('placement_allocation_differential', SCRIPTS / 'placement_allocation_differential.py')
RUNNER = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(RUNNER)


class AllocationRunnerTests(unittest.TestCase):
    def test_existing_provider_prevents_mutations_and_cleanup(self):
        class Occupied:
            def __init__(self):
                self.methods = []
            def call(self, method, *args, **kwargs):
                self.methods.append(method)
                return {'status': 200, 'headers': {}, 'body_text': '{}'}
        one, two = Occupied(), Occupied()
        with tempfile.TemporaryDirectory() as folder:
            output = Path(folder) / 'fresh'
            self.assertEqual(RUNNER.run(one, two, output, 'existing-project', 'existing-user'), 1)
            report = json.loads((output / 'report.json').read_text())
            self.assertEqual(report['cases'], [])
            self.assertEqual(report['cleanup'], [])
            with self.assertRaises(FileExistsError):
                RUNNER.run(one, two, output, 'existing-project', 'existing-user')
        self.assertEqual(set(one.methods + two.methods), {'GET'})

    def test_generation_errors_and_amounts_remain_strict(self):
        for left, right in [({'generation': 2}, {'generation': 3}),
                            ({'used': 2}, {'used': '2'}),
                            ({'detail': 'capacity'}, {'detail': 'constraints'}),
                            ({'allocations': {}}, {'allocations': []})]:
            self.assertTrue(RUNNER.differences(left, right))


if __name__ == '__main__':
    unittest.main()

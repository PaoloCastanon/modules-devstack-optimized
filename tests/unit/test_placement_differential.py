import importlib.util
from pathlib import Path
import tempfile
import unittest

SPEC = importlib.util.spec_from_file_location('placement_differential', Path(__file__).resolve().parents[2] / 'scripts/placement_differential.py')
DIFF = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(DIFF)


class PlacementDifferentialTests(unittest.TestCase):
    def test_no_type_generation_or_error_coercion(self):
        for a, b in [(1, True), (0, None), ([], {}), ('1', 1)]:
            self.assertTrue(DIFF.differences(a, b))
        self.assertEqual(DIFF.differences({'generation': 1}, {'generation': 2})[0]['path'], '$.generation')
        self.assertTrue(DIFF.differences({'detail': 'different'}, {'detail': 'error'}))
        self.assertTrue(DIFF.differences(1.234567, 1.234568))

    def test_normalization_is_narrow(self):
        original = {'status': 404, 'headers': {'location': 'http://python/placement/p', 'vary': 'OpenStack-API-Version'}, 'body_text': '{"errors":[{"request_id":"req-1","status":404,"detail":"x"}]}'}
        other = {'status': 404, 'headers': {'location': 'http://go/placement/p', 'vary': 'openstack-api-version'}, 'body_text': '{"errors":[{"request_id":"req-2","status":404,"detail":"x"}]}'}
        self.assertEqual(DIFF.canonical(original, ('http', 'python')), DIFF.canonical(other, ('http', 'go')))
        other['body_text'] = '{"errors":[{"status":404,"detail":"x"}]}'
        self.assertTrue(DIFF.differences(DIFF.canonical(original, ('http', 'python')), DIFF.canonical(other, ('http', 'go'))))
        self.assertIn('req-1', original['body_text'])
        other['headers']['location'] = 'http://third-party/placement/p'
        self.assertEqual(DIFF.canonical(other, ('http', 'go'))['headers']['location'], other['headers']['location'])

    def test_token_file_and_destination_guards(self):
        with tempfile.TemporaryDirectory() as folder:
            token = Path(folder) / 'token'
            token.write_text('fixture-token')
            token.chmod(0o644)
            with self.assertRaises(ValueError):
                DIFF.read_token(token)
            token.chmod(0o600)
            self.assertEqual(DIFF.read_token(token), 'fixture-token')
            with self.assertRaises(FileExistsError):
                DIFF.run(None, None, Path(folder))
        for base in ['file:///tmp/a', 'http://user:secret@host/', 'http://host/?q=x']:
            with self.assertRaises(ValueError):
                DIFF.Target(base, {})

    def test_existing_resource_is_never_mutated_or_cleaned(self):
        class Occupied:
            origin = ('http', 'fixture')
            def __init__(self):
                self.methods = []
            def call(self, method, *args, **kwargs):
                self.methods.append(method)
                return {'status': 200, 'headers': {}, 'body_text': '{}'}
        one, two = Occupied(), Occupied()
        with tempfile.TemporaryDirectory() as folder:
            report = DIFF.run(one, two, Path(folder) / 'new')
        self.assertTrue(one.methods)
        self.assertEqual(set(one.methods + two.methods), {'GET'})
        self.assertEqual(report['summary']['strict_match'], 0)


if __name__ == '__main__':
    unittest.main()

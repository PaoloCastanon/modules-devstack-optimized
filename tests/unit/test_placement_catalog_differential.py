import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import unittest

SCRIPTS = Path(__file__).resolve().parents[2] / 'scripts'
sys.path.insert(0, str(SCRIPTS))
SPEC = importlib.util.spec_from_file_location('placement_catalog_differential', SCRIPTS / 'placement_catalog_differential.py')
CATALOG = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(CATALOG)


class CatalogComparisonTests(unittest.TestCase):
    class Target:
        origin = ('http', 'fixture')

    def normalized(self, body, global_traits=False):
        return CATALOG.normalized({'status': 200, 'headers': {}, 'body_text': json.dumps(body)},
                                  self.Target(), {'CUSTOM_OWNED'}, global_traits)['body']

    def test_order_only_keeps_duplicates_and_generations(self):
        self.assertEqual(self.normalized({'traits': ['OWNER_NOVA', 'CUSTOM_OWNED'], 'resource_provider_generation': 3}),
                         self.normalized({'traits': ['CUSTOM_OWNED', 'OWNER_NOVA'], 'resource_provider_generation': 3}))
        self.assertNotEqual(self.normalized({'traits': ['CUSTOM_OWNED']}),
                            self.normalized({'traits': ['CUSTOM_OWNED', 'CUSTOM_OWNED']}))
        self.assertNotEqual(self.normalized({'resource_provider_generation': 3}),
                            self.normalized({'resource_provider_generation': 4}))
        self.assertNotEqual(self.normalized({'traits': []}), self.normalized({'traits': ['CUSTOM_UNEXPECTED']}))
        self.assertNotEqual(self.normalized({'errors': [{'detail': 'a'}]}), self.normalized({'errors': [{'detail': 'b'}]}))

    def test_catalogue_projection_is_explicit(self):
        body = {'traits': ['OWNER_NOVA', 'CUSTOM_OWNED', 'CUSTOM_OTHER']}
        self.assertIn('CUSTOM_OTHER', self.normalized(body)['traits'])
        self.assertEqual(self.normalized(body, True)['traits'], ['CUSTOM_OWNED', 'OWNER_NOVA'])
        body = {'resource_classes': [{'name': 'CUSTOM_OTHER'}, {'name': 'CUSTOM_OWNED'}, {'name': 'VCPU'}]}
        self.assertEqual(self.normalized(body)['resource_classes'], [{'name': 'CUSTOM_OWNED'}, {'name': 'VCPU'}])

    def test_existing_resource_never_mutates(self):
        class Occupied(self.Target):
            def __init__(self):
                self.methods = []
            def call(self, method, *args, **kwargs):
                self.methods.append(method)
                return {'status': 200, 'headers': {}, 'body_text': '{}'}
        one, two = Occupied(), Occupied()
        with tempfile.TemporaryDirectory() as folder:
            output = Path(folder) / 'fresh'
            self.assertEqual(CATALOG.run(one, two, output), 1)
            report = json.loads((output / 'report.json').read_text())
            self.assertEqual(report['cases'], [])
            self.assertEqual(report['cleanup'], [])
            with self.assertRaises(FileExistsError):
                CATALOG.run(one, two, output)
        self.assertEqual(set(one.methods + two.methods), {'GET'})


if __name__ == '__main__':
    unittest.main()

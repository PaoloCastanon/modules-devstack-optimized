import importlib.util
import json
from pathlib import Path
import sys
import unittest

SCRIPTS = Path(__file__).resolve().parents[2] / 'scripts'
sys.path[:0] = [str(SCRIPTS / 'integration'), str(SCRIPTS / 'benchmarks')]
spec = importlib.util.spec_from_file_location('glance_devstack_suite', SCRIPTS / 'integration' / 'glance_devstack_suite.py')
suite = importlib.util.module_from_spec(spec)
spec.loader.exec_module(suite)


class DevStackSuiteHelpers(unittest.TestCase):
    def test_unit_listen_and_apache_backend(self):
        env = 'GLANCE_GO_CONFIG_FILE=/etc/glance/glance-api.conf GLANCE_GO_LISTEN_ADDR=127.0.0.1:19292 GLANCE_GO_STORE_SCOPE=x'
        self.assertEqual(suite.unit_listen(env), '127.0.0.1:19292')
        with self.assertRaises(AssertionError):
            suite.unit_listen('GLANCE_GO_CONFIG_FILE=/x')
        site = 'KeepAlive Off\nSetEnv proxy-sendchunked 1\nProxyPass "/image" "http://127.0.0.1:19292" retry=0 acquire=1 \n'
        self.assertEqual(suite.proxied_backend(site), 'http://127.0.0.1:19292')
        with self.assertRaises(AssertionError):
            suite.proxied_backend('ProxyPass "/compute" "http://127.0.0.1:1"\n')

    def test_baseline_is_rendered_for_this_installation(self):
        baseline = json.loads(suite.BASELINE.read_text())['messages']
        self.assertEqual([m['event_type'] for m in baseline],
                         ['image.create', 'image.update', 'image.update', 'image.prepare', 'image.upload',
                          'image.activate', 'image.update', 'image.send', 'image.delete'])
        rendered = suite.render_baseline(baseline, {'{project}': 'p', '{user}': 'u', '{tag}': 't', '{publisher}': 'image.h'})
        self.assertEqual(rendered[0]['payload']['owner'], 'p')
        self.assertEqual(rendered[0]['payload']['tags'], ['t'])
        self.assertEqual(rendered[7]['payload']['receiver_user_id'], 'u')
        self.assertTrue(all(m['publisher_id'] == 'image.h' for m in rendered))
        self.assertNotIn('{project}', json.dumps(rendered))
        # The same messages compare equal with the suite's comparison.
        self.assertEqual(suite.compare_notifications(rendered, rendered), [])


if __name__ == '__main__':
    unittest.main()

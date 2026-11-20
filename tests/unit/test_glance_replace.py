"""Permanent replacement of the original Glance by Glance Go: offline helpers."""
import importlib.util
from pathlib import Path
import sys
import unittest

SCRIPTS = Path(__file__).resolve().parents[2] / 'scripts/integration'
sys.path.insert(0, str(SCRIPTS))
spec = importlib.util.spec_from_file_location('glance_replace', SCRIPTS / 'glance_replace.py')
replace = importlib.util.module_from_spec(spec)
spec.loader.exec_module(replace)

SITE = 'SetEnv proxy-sendchunked 1\nProxyPass "/image" "http://127.0.0.1:60999" retry=0 acquire=1 \n'


class GlanceReplaceTests(unittest.TestCase):
    def test_database_comes_from_the_original_connection(self):
        conf = '[DEFAULT]\ndebug = True\n[database]\nconnection = mysql+pymysql://root:p%40ss@127.0.0.1/glance?charset=utf8\n'
        self.assertEqual(replace.database_from_glance_conf(conf), {'host': '127.0.0.1', 'port': 3306, 'user': 'root', 'password': 'p@ss'})
        with self.assertRaises(ValueError):
            replace.database_from_glance_conf('[database]\nconnection = sqlite:////tmp/x\n')

    def test_site_keeps_devstack_options_and_only_moves_the_port(self):
        go = replace.go_site(SITE, 19292)
        self.assertIn('SetEnv proxy-sendchunked 1', go)
        self.assertIn('ProxyPass "/image" "http://127.0.0.1:19292" retry=0 acquire=1', go)
        self.assertIn('ProxyPassReverse "/image" "http://127.0.0.1:19292"', go)
        self.assertEqual((replace.backend_port(SITE), replace.backend_port(go)), (60999, 19292))
        with self.assertRaises(ValueError):
            replace.go_site('ProxyPass "/compute" "http://127.0.0.1:1"\n', 19292)

    def test_serve_env_and_unit(self):
        root = Path('/opt/stack/data/glance-go')
        env = replace.serve_env(root, 'scope-uuid', {'driver': 'messagingv2', 'topics': ['notifications'], 'publisher_id': 'image.localhost', 'disabled': []})
        self.assertIn('GLANCE_GO_LISTEN_ADDR=127.0.0.1:19292\n', env)
        self.assertIn('GLANCE_GO_STORE_SCOPE=scope-uuid\n', env)
        self.assertIn('GLANCE_GO_NOTIFICATION_DRIVER=messagingv2\n', env)
        self.assertNotIn('rabbit://', env)  # the broker URL only lives in transport.url
        self.assertNotIn('NOTIFICATION', replace.serve_env(root, 's', {'driver': 'noop'}))
        unit = replace.unit_text(root, 'stack')
        self.assertIn('ExecStart=/opt/stack/data/glance-go/bin/glance serve', unit)
        self.assertIn('WantedBy=multi-user.target', unit)

    def test_copied_image_keeps_id_metadata_and_drops_reserved_fields(self):
        meta = {'id': 'i', 'name': 'cirros', 'disk_format': 'qcow2', 'container_format': 'bare', 'visibility': 'public',
                'min_disk': 0, 'min_ram': 0, 'protected': False, 'tags': ['t'], 'checksum': 'x', 'size': 1, 'owner': 'o',
                'os_hash_value': 'h', 'os_glance_import_task': 'z', 'hw_rng_model': 'virtio', 'owner_specified.openstack.md5': '',
                'stores': 'file', 'self': '/v2/images/i', 'odd': 3}
        body, skipped = replace.copied_image(meta)
        self.assertEqual(body, {'id': 'i', 'name': 'cirros', 'disk_format': 'qcow2', 'container_format': 'bare', 'visibility': 'public',
                                'min_disk': 0, 'min_ram': 0, 'protected': False, 'tags': ['t'], 'hw_rng_model': 'virtio',
                                'owner_specified.openstack.md5': ''})
        self.assertEqual(skipped, ['odd'])


if __name__ == '__main__':
    unittest.main()

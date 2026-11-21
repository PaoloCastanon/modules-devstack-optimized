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

    def test_shared_e2e_uses_only_what_exists(self):
        # glance_shared_e2e.py imports these from glance_replace.
        for name in ('admin_token', 'use_go', 'use_original', 'PUBLIC_URL'):
            self.assertTrue(hasattr(replace, name), name)

    def test_unit(self):
        root = Path('/opt/stack/data/glance-go')
        unit = replace.unit_text(root, 'stack')
        self.assertIn('ExecStart=/opt/stack/data/glance-go/bin/glance serve', unit)
        self.assertIn('WantedBy=multi-user.target', unit)

    def test_official_catalog_settings(self):
        conf = '[glance_store]\nfilesystem_store_datadir = /opt/stack/data/glance/images/\n'
        self.assertEqual(replace.official_store_dir(conf), '/opt/stack/data/glance/images')
        with self.assertRaises(ValueError):
            replace.official_store_dir('[glance_store]\n')
        with self.assertRaises(ValueError):
            replace.official_store_dir('filesystem_store_datadir = images/\n')

    def test_original_config_env_has_only_what_glance_api_conf_does_not_say(self):
        env = replace.serve_env_original(Path('/opt/stack/data/glance-go'), 'scope-uuid')
        self.assertIn('GLANCE_GO_CONFIG_FILE=/etc/glance/glance-api.conf\n', env)
        self.assertIn('GLANCE_GO_LISTEN_ADDR=127.0.0.1:19292\n', env)
        for absent in ('DSN', 'KEYSTONE', 'NOTIFICATION', 'TRANSPORT', 'OFFICIAL'):
            self.assertNotIn(absent, env)



if __name__ == '__main__':
    unittest.main()

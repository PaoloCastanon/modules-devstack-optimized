#!/usr/bin/env python3
"""Opt-in real Glance baseline, own MySQL/store, route and byte preflight."""
import argparse
import contextlib
import hashlib
import io
import json
import logging
from pathlib import Path
import re
import shutil
import subprocess
import sys
from types import SimpleNamespace
import uuid

HERE = Path(__file__).resolve().parent
sys.path[:0] = [str(HERE), str(HERE.parent / 'integration'), str(HERE.parent)]
import glance_switch
from glance_http import Client, metadata_matches, payload
import lab_identity
import placement_preflight
import placement_runtime
from placement_suite import exclusive_run, require, save_json
from placement_differential import read_token


class Session:
    def __init__(self, args):
        self.args, self.root = args, args.work_root
        self.auth = self.root / 'tt-placement-auth'
        self.runtime = self.root / 'tt-placement-runtime'
        self.pending = {}
        self.token = None
        self.original = None
        self.cases = []
        self.python_ids = []

    def action(self, label, module, **values):
        stream = io.StringIO()
        with contextlib.redirect_stdout(stream):
            result = module.run(SimpleNamespace(allow_mutations=True, **values))
        (self.args.output / (label + '.log')).write_text(stream.getvalue())
        require(result == 0, label)

    def controls(self, action):
        result = subprocess.run(['sudo', sys.executable, str(HERE / 'lab_controls.py'), action,
                                 '--state', str(self.root / 'maintenance-controls.json')], capture_output=True, text=True)
        require(result.returncode == 0, 'temporary maintenance controls ' + action)
        return json.loads(result.stdout)

    def request(self, url, method, path, expected, **kwargs):
        client = Client(url, self.token)
        try:
            sample, value = client.request(method, path, **kwargs)
        finally:
            client.close()
        self.cases.append({'method': method, 'path': path, 'status': sample['status'], 'expected': expected,
                           'bytes': sample['response_bytes'], 'error_kind': sample['error_kind']})
        require(sample['status'] == expected and sample['error_kind'] is None, 'Glance preflight request/state')
        return value

    def snapshot(self):
        value = self.request(self.args.url, 'GET', '/v2/images?limit=1000', 200)
        require(not value.get('next'), 'bounded complete original image catalog')
        fields = ['id', 'name', 'owner', 'status', 'size', 'checksum', 'os_hash_algo', 'os_hash_value', 'tags', 'visibility', 'disk_format', 'container_format']
        images = sorted([{k: image.get(k) for k in fields} for image in value['images']], key=lambda i: i['id'])
        return {'images': images, 'apache_site_sha256': hashlib.sha256(glance_switch.SITE_FILE.read_bytes()).hexdigest(),
                'placement_site_sha256': hashlib.sha256(Path('/etc/apache2/sites-enabled/placement-api.conf').read_bytes()).hexdigest()}

    def prepare(self):
        save_json(self.args.output / 'controls-before.json', self.controls('enter'))
        self.action('identity-prepare', lab_identity, action='prepare', private_root=self.auth, application_lifetime_hours=24)
        self.token = read_token(self.auth / 'admin.token')
        metadata = json.loads((self.auth / 'metadata.json').read_text())
        self.owner, self.tag = metadata['project'], 'tt-glance-preflight-' + uuid.uuid4().hex
        self.original = self.snapshot()
        save_json(self.args.output / 'original-before.json', self.original)
        self.action('placement-before', placement_preflight, url='http://192.168.122.10/placement',
                    token_file=self.auth / 'service.token', output=self.args.output / 'placement-before')
        self.action('glance-runtime', placement_runtime, action='prepare', domain='glance', root=self.runtime,
                    binary=self.args.binary, auth_root=self.auth, mysql_port=13317, listen_port=19293,
                    public_url=self.args.url, keystone_url='http://192.168.122.10/identity/v3')
        save_json(self.args.output / 'runtime.json', json.loads((self.runtime / 'public-report.json').read_text()))

    def cycle(self, label, url, size):
        ident, name = str(uuid.uuid4()), 'tt-glance-preflight-' + label
        base = '/v2/images/' + ident
        self.request(url, 'GET', base, 404)
        self.pending[(label, ident)] = {'url': url, 'name': name}
        if label.startswith('python'):
            require(not (Path('/opt/stack/data/glance/images') / ident).exists(), 'fresh original store key')
            self.python_ids.append(ident)
        image = {'id': ident, 'name': name, 'tags': [self.tag], 'disk_format': 'raw',
                 'container_format': 'bare', 'visibility': 'private'}
        created = self.request(url, 'POST', '/v2/images', 201, body=image)
        require(metadata_matches(created, {**image, 'owner': self.owner, 'status': 'queued'}), 'created image metadata')
        data = self.root / ('payload-' + str(size) + '.bin')
        if data.exists():
            expected = json.loads((self.root / ('payload-' + str(size) + '.json')).read_text())
        else:
            expected = payload(data, size)
            save_json(self.root / ('payload-' + str(size) + '.json'), expected, private=True)
        self.request(url, 'PUT', base + '/file', 204, upload=data, content_type='application/octet-stream')
        active = self.request(url, 'GET', base, 200)
        require(metadata_matches(active, {**image, 'owner': self.owner, 'status': 'active', 'size': size,
                                         'checksum': expected['md5'], 'os_hash_algo': 'sha512', 'os_hash_value': expected['sha512']}), 'active image hash/length')
        self.request(url, 'GET', base + '/file', 200, expected_payload=expected)
        renamed = name + '-updated'
        changed = self.request(url, 'PATCH', base, 200, body=[{'op': 'replace', 'path': '/name', 'value': renamed}],
                               content_type='application/openstack-images-v2.1-json-patch')
        self.pending[(label, ident)]['name'] = renamed
        require(metadata_matches(changed, {'id': ident, 'name': renamed, 'owner': self.owner, 'tags': [self.tag]}), 'patched image metadata')
        self.request(url, 'DELETE', base, 204)
        self.request(url, 'GET', base, 404)
        self.pending.pop((label, ident))
        save_json(self.args.output / (label + '.json'), {'id': ident, 'size': size, 'payload': expected, 'round_trip': True, 'deleted': True})

    def restore_route(self):
        if (self.runtime / 'glance-switch-state.json').exists():
            self.action('route-restore', glance_switch, action='restore', root=self.runtime, public_url=self.args.url)

    def cleanup(self):
        self.restore_route()
        for (label, ident), record in list(self.pending.items()):
            url = 'http://127.0.0.1:19293' if label.startswith('go') else self.args.url
            client = Client(url, self.token)
            try:
                sample, image = client.request('GET', '/v2/images/' + ident)
                if sample['status'] == 404 and sample['error_kind'] is None:
                    self.pending.pop((label, ident)); continue
                require(sample['status'] == 200 and sample['error_kind'] is None and
                        metadata_matches(image, {'id': ident, 'owner': self.owner, 'name': record['name'], 'tags': [self.tag]}), 'pending image ownership')
            finally:
                client.close()
            self.request(url, 'DELETE', '/v2/images/' + ident, 204)
            self.request(url, 'GET', '/v2/images/' + ident, 404)
            self.pending.pop((label, ident))
        if self.original:
            final = self.snapshot()
            save_json(self.args.output / 'original-after.json', final)
            require(final == self.original, 'original Glance catalog and Apache sites unchanged')
            self.action('placement-after', placement_preflight, url='http://192.168.122.10/placement',
                        token_file=self.auth / 'service.token', output=self.args.output / 'placement-after')
            before = json.loads((self.args.output / 'placement-before/report.json').read_text())
            after = json.loads((self.args.output / 'placement-after/report.json').read_text())
            require(before['providers'] == after['providers'] and before['original_mysql'] == after['original_mysql'], 'Placement control unchanged')
            require(all(not (Path('/opt/stack/data/glance/images') / ident).exists() for ident in self.python_ids), 'owned Python payload files removed')
        if (self.runtime / 'store').exists():
            require(not any(p for p in (self.runtime / 'store').rglob('*') if p.suffix in ('.blob', '.part', '.ready')), 'own blob/staging data removed')
        if self.runtime.exists():
            self.action('runtime-cleanup', placement_runtime, action='cleanup', root=self.runtime)
        if self.auth.exists():
            self.action('identity-cleanup', lab_identity, action='cleanup', private_root=self.auth)
        controls = self.controls('restore')
        save_json(self.args.output / 'controls-after.json', controls)
        require(controls['controls_restored'], 'maintenance controls restored')
        units = ['devstack@placement-api.service', 'devstack@g-api.service', 'devstack@n-cpu.service', 'devstack@n-sch.service', 'devstack@keystone.service']
        require(all(glance_switch.active(u) for u in units), 'original services active')
        subprocess.run(['sudo', 'fstrim', '/'], check=True, stdout=subprocess.DEVNULL)
        shutil.rmtree(self.root)
        return {'original_unchanged': self.original is not None, 'private_root_removed': not self.root.exists(),
                'runtime_removed': not self.runtime.exists(), 'identity_removed': not self.auth.exists(),
                'controls_restored': True, 'original_services_active': dict.fromkeys(units, True), 'pending_images': 0}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--work-root', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--binary', type=Path, required=True)
    parser.add_argument('--url', default='http://192.168.122.10/image')
    parser.add_argument('--allow-mutations', action='store_true')
    args = parser.parse_args()
    require(args.allow_mutations and args.work_root.is_absolute() and args.work_root == args.work_root.resolve()
            and re.fullmatch(r'tt-placement-[a-z0-9-]+', args.work_root.name), 'explicit opt-in and canonical new work root')
    logging.disable(logging.CRITICAL)
    args.output.mkdir(parents=True, exist_ok=False)
    report = {'valid': False, 'cleanup': False, 'stage': 'functional-preflight; not benchmark results'}
    with exclusive_run(Path('/run/lock/tt-placement-integration.lock')):
        args.work_root.mkdir(mode=0o700, exist_ok=False)
        session = Session(args)
        try:
            session.prepare()
            session.cycle('python-1m', args.url, 1024**2)
            for size, label in [(1024**2, '1m'), (16 * 1024**2, '16m'), (64 * 1024**2, '64m')]:
                session.cycle('go-direct-' + label, 'http://127.0.0.1:19293', size)
            session.action('route-go', glance_switch, action='activate', root=session.runtime,
                           expected_original_sha=session.original['apache_site_sha256'], public_url=args.url, lease_seconds=600)
            session.cycle('go-public-1m', args.url, 1024**2)
            report['valid'] = True
        except Exception as error:
            report['error'] = type(error).__name__
            report['reason'] = str(error) if isinstance(error, AssertionError) else 'private diagnostic retained'
            diagnostics = args.work_root.with_name(args.work_root.name + '-diagnostics')
            diagnostics.mkdir(mode=0o700, exist_ok=False)
            for log in args.work_root.rglob('*.log'):
                shutil.copyfile(log, diagnostics / (hashlib.sha256(str(log.relative_to(args.work_root)).encode()).hexdigest()[:10] + '-' + log.name))
            report['private_diagnostics'] = str(diagnostics)
        finally:
            try:
                report['cleanup_report'] = session.cleanup()
                report['cleanup'] = True
            except Exception as error:
                report['cleanup_error'] = type(error).__name__
                report['cleanup_reason'] = str(error) if isinstance(error, AssertionError) else 'private state retained for repair'
            report['cases'] = session.cases
            report['valid'] = report['valid'] and report['cleanup']
            save_json(args.output / 'report.json', report)
    print(json.dumps({'valid': report['valid'], 'cleanup': report['cleanup'], 'requests': len(session.cases)}))
    return 0 if report['valid'] else 1


if __name__ == '__main__':
    sys.exit(main())

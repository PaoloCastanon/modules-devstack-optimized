#!/usr/bin/env python3
"""End-to-end check that Glance Go and the original Glance share one catalog.

Run inside the lab VM (openrc admin loaded) after ``glance_replace.py install``.
One round switches /image between both implementations and checks, through
the public endpoint only:

1. an image created and uploaded by Glance Go is listed, shown and downloaded
   byte-identical (MD5/SHA-512) by the original, and vice versa;
2. an image the original uploaded (CirrOS) downloads identical from Glance Go;
3. metadata changed by one (rename, property, tag) is seen by the other;
4. deleting with one gives 404 in the other, and its file is gone;
5. optionally Nova boots a server from the shared image while Go serves.

It leaves /image on Glance Go and deletes everything it created. Exit status
is non-zero on the first failure; ``--rounds`` repeats the whole check.
"""
import argparse
import hashlib
import json
import os
from pathlib import Path
import secrets
import subprocess
import sys
import tempfile
import time
import uuid

HERE = Path(__file__).resolve().parent
sys.path[:0] = [str(HERE), str(HERE.parent / 'benchmarks')]

import glance_replace as replace  # noqa: E402

STORE = Path('/opt/stack/data/glance/images')


def require(condition, message):
    if not condition:
        raise AssertionError(message)


class Round:
    def __init__(self, token, nova):
        from glance_http import Client
        self.token, self.nova = token, nova
        self.client = Client(replace.PUBLIC_URL, token, binary_timeout=600)
        self.created = []

    def request(self, method, path, expected, **kwargs):
        sample, body = self.client.request(method, path, **kwargs)
        require(sample['status'] == expected, '%s %s -> %s, expected %s' % (method, path, sample['status'], expected))
        return body

    def serve(self, implementation):
        status = (replace.use_go if implementation == 'go' else replace.use_original)()
        require(status['image_served_by'] == ('glance-go' if implementation == 'go' else 'original'), 'switch to ' + implementation)
        self.client.close()
        from glance_http import Client
        self.client = Client(replace.PUBLIC_URL, self.token, binary_timeout=600)

    def download(self, image_id):
        with tempfile.NamedTemporaryFile() as out:
            subprocess.run(['curl', '-sf', '-H', 'X-Auth-Token: ' + self.token, '-o', out.name,
                            replace.PUBLIC_URL + '/v2/images/' + image_id + '/file'], check=True)
            data = Path(out.name).read_bytes()
        return hashlib.md5(data).hexdigest(), hashlib.sha512(data).hexdigest(), len(data)

    def create(self, label, payload):
        image_id = str(uuid.uuid4())
        body = {'id': image_id, 'name': 'tt-shared-' + label, 'disk_format': 'raw', 'container_format': 'bare',
                'visibility': 'private', 'tags': ['tt-shared'], 'tt_origin': label}
        self.request('POST', '/v2/images', 201, body=body)
        self.created.append(image_id)
        with tempfile.NamedTemporaryFile() as data:
            data.write(payload)
            data.flush()
            self.request('PUT', '/v2/images/' + image_id + '/file', 204, upload=Path(data.name), content_type='application/octet-stream')
        meta = self.request('GET', '/v2/images/' + image_id, 200)
        require(meta['status'] == 'active' and meta['size'] == len(payload), label + ' upload active')
        require(meta['checksum'] == hashlib.md5(payload).hexdigest() and meta['os_hash_value'] == hashlib.sha512(payload).hexdigest(), label + ' hashes')
        return image_id, meta

    def same_image(self, image_id, expected, who):
        meta = self.request('GET', '/v2/images/' + image_id, 200)
        for key in ('id', 'name', 'status', 'size', 'checksum', 'os_hash_algo', 'os_hash_value', 'owner', 'visibility', 'disk_format', 'container_format'):
            require(meta.get(key) == expected.get(key), '%s sees %s=%r, expected %r' % (who, key, meta.get(key), expected.get(key)))
        require(sorted(meta.get('tags', [])) == sorted(expected.get('tags', [])), who + ' tags')
        md5, sha, size = self.download(image_id)
        require((md5, sha, size) == (expected['checksum'], expected['os_hash_value'], expected['size']), who + ' downloads identical bytes')
        listed = self.request('GET', '/v2/images?limit=1000', 200)
        require(image_id in [i['id'] for i in listed['images']], who + ' lists the image')
        return meta

    def gone(self, image_id, who):
        self.request('GET', '/v2/images/' + image_id, 404)
        require(not (STORE / image_id).exists(), who + ' deletion removed the file')

    def run(self):
        steps = {}
        self.serve('go')
        cirros = [i for i in self.request('GET', '/v2/images?limit=1000', 200)['images'] if i['name'].startswith('cirros')]
        require(len(cirros) == 1, 'one original CirrOS image')
        cirros = cirros[0]
        md5, sha, size = self.download(cirros['id'])
        require((md5, sha, size) == (cirros['checksum'], cirros['os_hash_value'], cirros['size']), 'Go serves the original upload')
        steps['go_downloads_original_upload'] = True

        go_id, go_meta = self.create('go', secrets.token_bytes(1 << 20) + b'go')
        self.request('PATCH', '/v2/images/' + go_id, 200, body=[{'op': 'replace', 'path': '/name', 'value': 'tt-shared-go-renamed'},
                                                                   {'op': 'add', 'path': '/tt_patched', 'value': 'by-go'}],
                     content_type='application/openstack-images-v2.1-json-patch')
        go_meta = self.request('GET', '/v2/images/' + go_id, 200)
        self.serve('original')
        seen = self.same_image(go_id, go_meta, 'original')
        require(seen.get('tt_patched') == 'by-go' and seen.get('tt_origin') == 'go', 'original sees Go properties')
        steps['original_reads_go_image'] = True

        py_id, py_meta = self.create('python', secrets.token_bytes(1 << 20) + b'py')
        self.request('PATCH', '/v2/images/' + py_id, 200, body=[{'op': 'add', 'path': '/tt_patched', 'value': 'by-python'}],
                     content_type='application/openstack-images-v2.1-json-patch')
        py_meta = self.request('GET', '/v2/images/' + py_id, 200)
        self.serve('go')
        seen = self.same_image(py_id, py_meta, 'go')
        require(seen.get('tt_patched') == 'by-python', 'Go sees original properties')
        steps['go_reads_original_image'] = True

        self.request('DELETE', '/v2/images/' + go_id, 204)
        self.request('DELETE', '/v2/images/' + py_id, 204)  # Go deletes what the original uploaded
        self.serve('original')
        self.gone(go_id, 'Go')
        self.gone(py_id, 'Go (original upload)')
        steps['original_sees_go_deletions'] = True

        back_id, back_meta = self.create('python-delete', b'python deletes')
        self.request('DELETE', '/v2/images/' + back_id, 204)
        self.serve('go')
        self.gone(back_id, 'original')
        steps['go_sees_original_deletion'] = True
        self.created = []

        if self.nova:
            steps['nova_boot_from_shared_cirros'] = self.boot(cirros['id'])
        return steps

    def boot(self, image_id):
        name = 'tt-shared-' + secrets.token_hex(4)
        out = subprocess.run(['openstack', 'server', 'create', '--image', image_id, '--flavor', self.nova['flavor'],
                              '--network', self.nova['network'], '--wait', '-f', 'value', '-c', 'status', name],
                             capture_output=True, text=True)
        status = out.stdout.strip()
        subprocess.run(['openstack', 'server', 'delete', '--wait', name], capture_output=True)
        require(status == 'ACTIVE', 'Nova booted from the shared image: ' + (status or out.stderr[-300:]))
        return True

    def cleanup(self):
        for image_id in self.created:
            for implementation in ('go', 'original'):
                try:
                    self.serve(implementation)
                    self.client.request('DELETE', '/v2/images/' + image_id)
                except Exception:
                    pass
        self.serve('go')
        self.client.close()


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument('--rounds', type=int, default=1)
    parser.add_argument('--flavor', default='42')
    parser.add_argument('--network', help='boot a Nova server each round on this network')
    parser.add_argument('--allow-mutations', action='store_true')
    args = parser.parse_args()
    if not args.allow_mutations:
        raise SystemExit('explicit --allow-mutations required')
    _, token = replace.admin_token()
    nova = {'flavor': args.flavor, 'network': args.network} if args.network else None
    results = []
    for n in range(1, args.rounds + 1):
        started = time.monotonic()
        r = Round(token, nova)
        try:
            steps = r.run()
        except Exception as error:
            r.cleanup()
            print(json.dumps({'round': n, 'passed': False, 'error': str(error)}), flush=True)
            return 1
        r.cleanup()
        results.append({'round': n, 'passed': True, 'seconds': round(time.monotonic() - started, 1), **steps})
        print(json.dumps(results[-1]), flush=True)
    print(json.dumps({'rounds': len(results), 'passed': all(r['passed'] for r in results)}))
    return 0


if __name__ == '__main__':
    os.umask(0o077)
    sys.exit(main())

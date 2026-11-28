#!/usr/bin/env python3
"""Joint Placement x Glance campaign (2x2): original Nova boots CirrOS servers
with Placement Python/Go and Glance Python/Go (protocols joint-compact-v3/v4).

Runs inside the lab VM with admin openrc and DevStack's venv Python. Reuses
the B26 suite (glance_suite.Suite): lab identity, maintenance controls,
service validator, both Go runtimes, reversible Apache routes with rollback
leases and the full cleanup/restoration. Per block it uploads one new image
per boot through the active Glance (cold profile), boots N servers behind a
common barrier, polls each at 1 s until ACTIVE, verifies allocations, cold
transfer and service traces, then deletes everything it created. The first
invalid block stops the campaign; cleanup always runs.

Usage: docs/benchmarks/joint-campaign.md.
"""
import argparse
import datetime
import hashlib
import json
import os
from pathlib import Path
import random
import re
import subprocess
import sys
import threading
import time
from urllib.error import HTTPError
from urllib.request import Request, urlopen
import uuid

HERE = Path(__file__).resolve().parent
sys.path[:0] = [str(HERE), str(HERE.parent / 'benchmarks'), str(HERE.parent)]
import glance_flow  # noqa: E402
import glance_suite  # noqa: E402
from glance_suite import (PLACEMENT_URL, Suite, boot_fault, cirros_image, file_digest,  # noqa: E402
                          server_image_id, wait_schedulable)
import glance_switch  # noqa: E402
from placement_differential import read_token  # noqa: E402
import placement_switch  # noqa: E402
from placement_suite import exclusive_run, require, save_json, sdk_connection  # noqa: E402

BASE_CACHE = Path('/opt/stack/data/nova/instances/_base')
SHARED_UNITS = {'nova-compute': 'devstack@n-cpu.service', 'nova-scheduler': 'devstack@n-sch.service',
                'nova-api': 'devstack@n-api.service', 'nova-conductor-cell1': 'devstack@n-cond-cell1.service',
                'nova-super-conductor': 'devstack@n-super-cond.service', 'keystone': 'devstack@keystone.service',
                'mysql': 'mysql.service', 'apache': 'apache2.service', 'rabbitmq': 'rabbitmq-server.service'}
UWSGI = re.compile(r'\] (GET|PUT|POST|DELETE|PATCH|HEAD) (\S+) => generated (\d+) bytes in (\d+) msecs \(HTTP/[\d.]+ (\d{3})\)')
UUID_RE = re.compile(r'[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}')
NAMESPACE = uuid.UUID('6f1c7b52-3c1e-4d0e-9a57-7a3c4b0a2711')


# -- pure helpers (unit tested) ---------------------------------------------
def block_plan(protocol, pilot=False):
    """Seeded order: per repetition the variants shuffled, and inside each
    variant the concurrencies shuffled. Returns a list of block dicts."""
    reps = protocol['pilot']['repetitions'] if pilot else protocol['repetitions']
    variants = [c['id'] for c in protocol['configurations']]
    plan = []
    for rep in range(1, reps + 1):
        rng = random.Random('%s/%s/%d' % (protocol['order_seed'], 'pilot' if pilot else 'official', rep))
        order = list(variants)
        rng.shuffle(order)
        for variant in order:
            sizes = list(protocol['concurrent_boots'])
            rng.shuffle(sizes)
            for n in sizes:
                plan.append({'index': len(plan) + 1, 'repetition': rep, 'variant': variant, 'concurrency': n})
    return plan


def variant_services(protocol, variant):
    found = [c for c in protocol['configurations'] if c['id'] == variant]
    require(len(found) == 1, 'known variant')
    return found[0]['placement'], found[0]['glance']


def base_cache_name(image_id):
    """Nova's libvirt image cache file for an image (imagecache.get_cache_fname)."""
    return hashlib.sha1(image_id.encode()).hexdigest()


def template(path):
    path = path.split('?', 1)[0]
    for prefix in ('/placement', '/image'):
        if path.startswith(prefix + '/') or path == prefix:
            path = path[len(prefix):] or '/'
    return re.sub(r'\{[a-z_]+\}', '{id}', UUID_RE.sub('{id}', path))


def uwsgi_events(journal_lines, since_us, until_us):
    """Request lines of an original (uWSGI) unit inside the window."""
    events = []
    for line in journal_lines:
        try:
            entry = json.loads(line)
            message = entry['MESSAGE']
            ts = int(entry['__REALTIME_TIMESTAMP'])
        except (ValueError, KeyError, TypeError):
            continue
        found = UWSGI.search(message) if isinstance(message, str) else None
        if found and since_us <= ts <= until_us:
            events.append({'ts_us': ts, 'method': found.group(1), 'route': template(found.group(2)),
                           'path_ids': UUID_RE.findall(found.group(2)), 'status': int(found.group(5)),
                           'duration_ms': int(found.group(4)), 'response_bytes': int(found.group(3))})
    return events


def go_events(journal_lines, since_us, until_us):
    """'request completed' records of a Go unit inside the window."""
    events = []
    for line in journal_lines:
        try:
            entry = json.loads(line)
            record = json.loads(entry['MESSAGE'])
            ts = int(entry['__REALTIME_TIMESTAMP'])
        except (ValueError, KeyError, TypeError):
            continue
        if record.get('msg') == 'request completed' and since_us <= ts <= until_us:
            path = str(record.get('path') or '')
            events.append({'ts_us': ts, 'method': record.get('method'), 'route': template(str(record.get('route') or path)),
                           'path_ids': UUID_RE.findall(path), 'status': record.get('status'),
                           'duration_ms': record.get('duration_ms'), 'response_bytes': record.get('response_bytes'),
                           'user_id': record.get('user_id')})
    return events


def trace_summary(events):
    counts = {}
    for e in events:
        key = '%s %s %s' % (e['method'], e['route'], e['status'])
        counts[key] = counts.get(key, 0) + 1
    durations = [e['duration_ms'] for e in events if isinstance(e.get('duration_ms'), (int, float))]
    route_ms = {}
    for e in events:
        if isinstance(e.get('duration_ms'), (int, float)):
            key = '%s %s' % (e['method'], e['route'])
            route_ms[key] = round(route_ms.get(key, 0) + e['duration_ms'], 1)
    return {'requests': len(events), 'by_route': dict(sorted(counts.items())),
            'server_ms_total': round(sum(durations), 1), 'by_route_ms': dict(sorted(route_ms.items()))}


def placement_trace_ok(summary, servers):
    by = summary['by_route']
    candidates = sum(v for k, v in by.items() if k.startswith('GET /allocation_candidates 200'))
    claims = sum(v for k, v in by.items() if k.startswith('PUT /allocations/{id} 204'))
    return candidates >= 1 and claims >= servers


def glance_trace_ok(events, image_ids, size):
    """One complete download per image. Go logs only the templated route, so
    it counts full downloads; uWSGI lines also carry the ID, checked too."""
    full = [e for e in events if e['method'] == 'GET' and e['route'] == '/v2/images/{id}/file'
            and e['status'] == 200 and (e.get('response_bytes') or 0) >= size]
    served = {i for e in full for i in e.get('path_ids', [])}
    return len(full) >= len(image_ids) and (not served or set(image_ids) <= served)


NOVA_TIME = re.compile(r'(\d{4}-\d\d-\d\dT\d\d:\d\d:\d\d(?:\.\d+)?)')
TOOK = re.compile(r'\[instance: ([0-9a-f-]{36})\] Took ([\d.]+) seconds to (spawn the instance on the hypervisor|build instance)')
LOCK_HELD = re.compile(r'Lock "([0-9a-f]{40})" "released" by "([^"]+)" :: held ([\d.]+)s')


def nova_time(text):
    """Nova API timestamps are UTC without zone (microseconds optional)."""
    found = NOVA_TIME.search(text or '')
    if not found:
        return None
    value = found.group(1)
    fmt = '%Y-%m-%dT%H:%M:%S.%f' if '.' in value else '%Y-%m-%dT%H:%M:%S'
    return datetime.datetime.strptime(value, fmt).replace(tzinfo=datetime.timezone.utc).timestamp()


def action_phases(action):
    """Split a create action (GET os-instance-actions/{req}) at the compute
    build event: before it API, conductor, scheduler (Placement claims) and
    RPC; inside it the compute build (Glance download, network, libvirt)."""
    start = nova_time(action.get('start_time'))
    events = {e.get('event'): e for e in action.get('events') or []}
    build = events.get('compute__do_build_and_run_instance') or {}
    b0, b1 = nova_time(build.get('start_time')), nova_time(build.get('finish_time'))
    out = {'events': sorted(events), 'build_result': build.get('result')}
    if start is not None and b0 is not None and b1 is not None:
        out.update(request_to_build_s=round(b0 - start, 3), build_s=round(b1 - b0, 3), request_to_built_s=round(b1 - start, 3),
                   start_epoch=start, built_epoch=b1)
    for name, e in events.items():  # e.g. conductor_schedule_and_build_instances when present
        e0, e1 = nova_time(e.get('start_time')), nova_time(e.get('finish_time'))
        if name != 'compute__do_build_and_run_instance' and e0 is not None and e1 is not None:
            out.setdefault('other_events_s', {})[name] = round(e1 - e0, 3)
    return out


def compute_log_phases(journal_lines, server_ids, image_ids):
    """nova-compute's own timings: 'Took X seconds to spawn/build' per
    instance and the hold time of the base-image lock (the cold fetch from
    Glance and conversion into _base) per image."""
    spawn, build, fetch = {}, {}, {}
    bases = {base_cache_name(i): i for i in image_ids}
    for line in journal_lines:
        try:
            message = json.loads(line)['MESSAGE']
        except (ValueError, KeyError, TypeError):
            continue
        if not isinstance(message, str):
            continue
        took = TOOK.search(message)
        if took and took.group(1) in server_ids:
            (spawn if took.group(3).startswith('spawn') else build)[took.group(1)] = float(took.group(2))
        held = LOCK_HELD.search(message)
        if held and held.group(1) in bases and 'fetch' in held.group(2):
            image = bases[held.group(1)]
            fetch[image] = max(fetch.get(image, 0.0), float(held.group(3)))
    return {'spawn_s': spawn, 'build_s': build, 'image_fetch_s': fetch}


def window_resources(samples, since_us, until_us):
    """CPU seconds (counter delta between the last sample before the window
    and the first after it) and peak PSS inside, per group; VM busy share."""
    before = [s for s in samples if s['ts_us'] <= since_us]
    after = [s for s in samples if s['ts_us'] >= until_us]
    inside = [s for s in samples if since_us <= s['ts_us'] <= until_us]
    if not before or not after:
        return None
    a, b = before[-1], after[0]
    groups = {}
    for name in a['groups']:
        x, y = a['groups'].get(name), b['groups'].get(name)
        peaks = [s['groups'][name]['pss_kib'] for s in inside + [a, b] if s['groups'].get(name)]
        mean = [s['groups'][name]['pss_kib'] for s in inside if s['groups'].get(name)] or peaks
        groups[name] = {'cpu_s': round((y['cpu_usec'] - x['cpu_usec']) / 1e6, 3) if x and y else None,
                        'pss_max_mib': round(max(peaks) / 1024, 1) if peaks else None,
                        'pss_mean_mib': round(sum(mean) / len(mean) / 1024, 1) if mean else None}
    busy = b['vm']['busy_ticks'] - a['vm']['busy_ticks']
    total = b['vm']['total_ticks'] - a['vm']['total_ticks']
    return {'span_s': round((b['ts_us'] - a['ts_us']) / 1e6, 2), 'samples_inside': len(inside), 'groups': groups,
            'vm_busy_fraction': round(busy / total, 4) if total else None}


def read_samples(path):
    samples = []
    if path.exists():
        for line in path.read_text().splitlines():
            try:
                samples.append(json.loads(line))
            except ValueError:  # last line may be mid-write
                continue
    return samples


# -- campaign ---------------------------------------------------------------
class Campaign(Suite):
    def __init__(self, args):
        args.with_placement_go = True
        args.quick_compare = args.audit = False
        super().__init__(args)
        self.protocol = json.loads(args.protocol.read_text())
        self.plan = block_plan(self.protocol, args.pilot)
        self.report.update(suite='joint-campaign', run_id='tt-joint-' + uuid.uuid4().hex[:12],
                           protocol=self.protocol['protocol'], pilot=args.pilot,
                           protocol_sha256=hashlib.sha256(args.protocol.read_bytes()).hexdigest(),
                           blocks_required=len(self.plan), blocks_valid=0, required_cases=[])
        self.state = {'placement': 'python', 'glance': 'python'}
        self.sampler = None
        self.samples_path = args.output / 'samples.jsonl'
        self.blocks_path = args.output / 'blocks.jsonl'
        self.owned_servers, self.owned_images = {}, {}

    # -- setup ---------------------------------------------------------------
    def manifest(self):
        cirros = cirros_image(self.args.cirros_image)
        self.cirros, self.cirros_digest = cirros, file_digest(cirros)
        binaries = {name: file_digest(path)['sha256'] for name, path in
                    (('glance', self.args.binary), ('placement', self.args.placement_binary))}
        value = {'run_id': self.report['run_id'], 'protocol': self.protocol['protocol'],
                 'protocol_sha256': self.report['protocol_sha256'], 'pilot': self.args.pilot, 'plan': self.plan,
                 'binaries_sha256': binaries, 'cirros': {'name': cirros.name, **self.cirros_digest},
                 'flavor_id': self.args.flavor_id, 'network_id': self.args.network_id,
                 'kernel': os.uname().release, 'cpus': os.cpu_count(),
                 'mem_total_kib': int(Path('/proc/meminfo').read_text().split()[1]),
                 'started_utc': self.report['started_utc']}
        save_json(self.args.output / 'manifest.json', value)
        self.report['manifest_sha256'] = hashlib.sha256((self.args.output / 'manifest.json').read_bytes()).hexdigest()

    def start_sampler(self):
        units = dict(SHARED_UNITS)
        units['placement-python'] = placement_switch.ORIGINAL_UNIT
        units['glance-python'] = glance_switch.ORIGINAL_UNIT
        pl = json.loads((self.pl_runtime / 'state.json').read_text())
        gl = json.loads((self.runtime / 'state.json').read_text())
        units['placement-go'] = pl['go_unit'] + '.service'
        units['placement-go-mysql'] = pl['db_unit'] + '.service'
        units['glance-go'] = gl['go_unit'] + '.service'
        self.units = units
        groups = []
        for name, unit in sorted(units.items()):
            group = subprocess.run(['systemctl', 'show', '-p', 'ControlGroup', '--value', unit],
                                   check=True, capture_output=True, text=True).stdout.strip()
            require(group.startswith('/') and '..' not in group, 'cgroup of ' + name)
            groups += ['--group', name + '=' + group]
        save_json(self.args.output / 'sampled-units.json', units)
        self.sampler = subprocess.Popen(['sudo', '-n', sys.executable, str(HERE.parent / 'benchmarks' / 'cgroup_sampler.py'),
                                         *groups, '--interval', str(self.protocol['sampling_interval_s']),
                                         '--output', str(self.samples_path)])
        time.sleep(3)
        require(self.sampler.poll() is None and self.samples_path.exists(), 'resource sampler running')

    def stop_sampler(self):
        if self.sampler and self.sampler.poll() is None:
            subprocess.run(['sudo', '-n', 'kill', '-TERM', str(self.sampler.pid)], check=False)
            try:
                self.sampler.wait(timeout=10)
            except subprocess.TimeoutExpired:
                subprocess.run(['sudo', '-n', 'kill', '-KILL', str(self.sampler.pid)], check=False)

    # -- variant switching ---------------------------------------------------
    def set_glance(self, want):
        if self.state['glance'] == want:
            return
        if want == 'go':
            self.action('glance-route-go', glance_switch, action='activate', root=self.runtime,
                        expected_original_sha=self.original['apache_site_sha256'], public_url=self.args.url,
                        lease_seconds=self.protocol['route_lease_s'])
            require(not glance_switch.active(glance_switch.ORIGINAL_UNIT), 'original Glance stopped')
        else:
            self.action('glance-route-restore', glance_switch, action='restore', root=self.runtime, public_url=self.args.url)
            require(glance_switch.active(glance_switch.ORIGINAL_UNIT), 'original Glance active')
        self.state['glance'] = want

    def set_placement(self, want):
        if self.state['placement'] == want:
            return
        if want == 'go':
            site = json.loads((self.args.output / 'placement-before/report.json').read_text())['apache_site_sha256']
            self.action('placement-route-go', placement_switch, action='activate', root=self.pl_runtime,
                        expected_original_sha=site, public_url=PLACEMENT_URL, lease_seconds=self.protocol['route_lease_s'])
            require(not placement_switch.active(placement_switch.ORIGINAL_UNIT), 'original Placement stopped')
        else:
            self.action('placement-route-restore', placement_switch, action='restore', root=self.pl_runtime,
                        public_url=PLACEMENT_URL)
            placement_switch.wait_version(PLACEMENT_URL, '1.39')
        self.state['placement'] = want

    def activate(self, variant):
        placement, glance = variant_services(self.protocol, variant)
        started = time.monotonic()
        # Routes changed while a lease is pending would restore early: renew
        # both on every variant by switching back to Python first.
        self.set_glance('python')
        self.set_placement('python')
        self.set_glance(glance)
        self.set_placement(placement)
        restart_epoch = time.time()
        self.restart_nova()
        connection, _, _ = sdk_connection()
        try:
            def services():
                return [{'binary': s.binary, 'status': s.status, 'state': s.state, 'updated_at': s.updated_at}
                        for s in connection.compute.services()]
            wait_schedulable(services, restart_epoch, 240)
        finally:
            connection.close()
        inventory = None
        while time.monotonic() - started < 240:
            status, body = self.pl_public('/resource_providers/' + self.provider + '/inventories', '1.26')
            if status == 200 and {'VCPU', 'MEMORY_MB', 'DISK_GB'} <= set(body['inventories']):
                inventory = {k: v['total'] for k, v in body['inventories'].items()}
                break
            time.sleep(2)
        require(inventory is not None, 'compute provider inventory visible in the active Placement')
        status, usages = self.pl_public('/resource_providers/' + self.provider + '/usages', '1.0')
        require(status == 200 and not any(usages['usages'].values()), 'compute provider idle before the variant')
        time.sleep(self.protocol['settle_s'])
        return {'variant': variant, 'switch_s': round(time.monotonic() - started, 1), 'inventory': inventory}

    def pl_public(self, path, version):
        request = Request(PLACEMENT_URL + path, headers={
            'X-Auth-Token': read_token(self.auth / 'service.token'),
            'OpenStack-API-Version': 'placement ' + version, 'Accept': 'application/json'})
        try:
            with urlopen(request, timeout=15) as response:
                return response.status, json.loads(response.read(4 * 1024 * 1024))
        except HTTPError as error:
            try:
                return error.code, None
            finally:
                error.close()

    def expect_routes(self, placement, glance):
        require(placement_switch.active(placement_switch.ORIGINAL_UNIT) == (placement == 'python'),
                'original Placement unit matches the variant')
        require(glance_switch.active(glance_switch.ORIGINAL_UNIT) == (glance == 'python'),
                'original Glance unit matches the variant')
        gsite = glance_switch.SITE_FILE.read_text()
        psite = placement_switch.SITE_FILE.read_text()
        require(('127.0.0.1:19293' in gsite) == (glance == 'go'), 'Apache /image route matches the variant')
        require(('127.0.0.1:18779' in psite) == (placement == 'go'), 'Apache /placement route matches the variant')

    # -- one block -----------------------------------------------------------
    def upload_image(self, block, i):
        ident = str(uuid.uuid5(NAMESPACE, '%s/%d/%d' % (self.report['run_id'], block['index'], i)))
        name = 'tt-joint-b%02d-%d' % (block['index'], i)
        base = '/v2/images/' + ident
        self.request(self.args.url, 'GET', base, 404)
        self.pending[('joint', ident)] = {'url': self.args.url, 'name': name}
        self.owned_images[ident] = name
        self.request(self.args.url, 'POST', '/v2/images', 201, body={
            'id': ident, 'name': name, 'tags': [self.tag], 'disk_format': 'qcow2', 'container_format': 'bare',
            'visibility': 'private'})
        self.request(self.args.url, 'PUT', base + '/file', 204, upload=self.cirros, content_type='application/octet-stream')
        active = self.request(self.args.url, 'GET', base, 200)
        require(active.get('status') == 'active' and active.get('size') == self.cirros_digest['size']
                and active.get('os_hash_value') == self.cirros_digest['sha512'], 'image active with matching hashes')
        require(not (BASE_CACHE / base_cache_name(ident)).exists(), 'cold: no Nova base cache entry before the boot')
        return ident

    def boot_all(self, block, images):
        n = len(images)
        connections = [sdk_connection()[0] for _ in range(n)]
        barrier = threading.Barrier(n + 1)
        results = [None] * n
        poll = self.protocol['boot_poll_interval_s']
        timeout = self.protocol['boot_timeout_s']

        def boot(i):
            conn, name = connections[i], 'tt-joint-b%02d-s%d' % (block['index'], i)
            record = {'name': name, 'image_id': images[i], 'states': []}
            results[i] = record
            barrier.wait()
            t0 = time.monotonic()
            record['post_epoch'] = time.time()
            try:
                server = conn.compute.create_server(name=name, image_id=images[i], flavor_id=self.args.flavor_id,
                                                    networks=[{'uuid': self.args.network_id}])
                record['id'] = server.id
                self.owned_servers[server.id] = name
                record['post_s'] = round(time.monotonic() - t0, 3)
                while time.monotonic() - t0 < timeout:
                    current = conn.compute.get_server(server.id)
                    if not record['states'] or record['states'][-1]['status'] != current.status:
                        record['states'].append({'t_s': round(time.monotonic() - t0, 3), 'status': current.status})
                    if current.status in ('ACTIVE', 'ERROR'):
                        break
                    time.sleep(poll)
                record['final'] = current.status
                record['active_s'] = record['states'][-1]['t_s'] if current.status == 'ACTIVE' else None
                record['image_ok'] = server_image_id(current) == images[i]
                if current.status == 'ERROR':
                    record['fault'] = boot_fault(current)
            except Exception as error:  # noqa: BLE001 - recorded, block becomes invalid
                record['error'] = type(error).__name__
            record['end_s'] = round(time.monotonic() - t0, 3)

        threads = [threading.Thread(target=boot, args=(i,), daemon=True) for i in range(n)]
        for t in threads:
            t.start()
        since = time.time_ns() // 1000
        barrier.wait()
        released = time.monotonic()
        for t in threads:
            t.join(timeout + 60)
        makespan = time.monotonic() - released
        for conn in connections:
            conn.close()
        time.sleep(3)  # journald/Apache flush the last records of the window
        return results, since, time.time_ns() // 1000, round(makespan, 3)

    def traces(self, block, placement, glance, servers, images, since, until):
        pl_state = json.loads((self.pl_runtime / 'state.json').read_text())
        gl_state = json.loads((self.runtime / 'state.json').read_text())
        units = {'placement-go': pl_state['go_unit'], 'glance-go': gl_state['go_unit'],
                 'placement-python': placement_switch.ORIGINAL_UNIT, 'glance-python': glance_switch.ORIGINAL_UNIT}
        events = {}
        for name, unit in units.items():
            lines = glance_flow.journal([unit], since, until)
            events[name] = go_events(lines, since, until) if name.endswith('-go') else uwsgi_events(lines, since, until)
        active_pl, idle_pl = 'placement-' + placement, 'placement-' + ('python' if placement == 'go' else 'go')
        active_gl, idle_gl = 'glance-' + glance, 'glance-' + ('python' if glance == 'go' else 'go')
        summary = {name: trace_summary(ev) for name, ev in events.items()}
        checks = {'placement_active_served': placement_trace_ok(summary[active_pl], len(servers)),
                  'glance_active_served': glance_trace_ok(events[active_gl], images, self.cirros_digest['size']),
                  'placement_idle_silent': summary[idle_pl]['requests'] == 0,
                  'glance_idle_silent': summary[idle_gl]['requests'] == 0}
        save_json(self.args.output / 'traces' / ('block-%02d.json' % block['index']),
                  {name: [{k: v for k, v in e.items() if k != 'user_id'} for e in ev] for name, ev in events.items()})
        return summary, checks

    def phases(self, servers, images, since, until):
        """Per server: Nova's create action events and nova-compute's own
        timings. Read before the servers are deleted."""
        lines = glance_flow.journal(['devstack@n-cpu.service'], since, until)
        logs = compute_log_phases(lines, {s['id'] for s in servers}, images)
        ok = True
        for s in servers:
            actions = self.connection.compute.get('/servers/%s/os-instance-actions' % s['id'],
                                                  microversion='2.84').json().get('instanceActions') or []
            create = [a for a in actions if a.get('action') == 'create']
            phase = {}
            if len(create) == 1:
                detail = self.connection.compute.get('/servers/%s/os-instance-actions/%s' % (s['id'], create[0]['request_id']),
                                                     microversion='2.84').json().get('instanceAction') or {}
                phase = action_phases(detail)
            phase.update(spawn_log_s=logs['spawn_s'].get(s['id']), build_log_s=logs['build_s'].get(s['id']),
                         image_fetch_s=logs['image_fetch_s'].get(s['image_id']))
            if phase.get('built_epoch') is not None and s.get('post_epoch') is not None:
                # host and Nova share this VM's clock: last ACTIVE poll minus build end
                phase['active_poll_lag_s'] = round(s['post_epoch'] + s['active_s'] - phase['built_epoch'], 3)
            s['phases'] = phase
            ok = ok and all(phase.get(k) is not None for k in ('request_to_build_s', 'build_s', 'spawn_log_s', 'image_fetch_s'))
        return ok

    def verify_allocations(self, servers):
        flavor = self.connection.compute.get_flavor(self.args.flavor_id)
        expected = {'VCPU': int(flavor.vcpus), 'MEMORY_MB': int(flavor.ram), 'DISK_GB': int(flavor.disk)}
        ok = True
        for s in servers:
            status, body = self.pl_public('/allocations/' + s['id'], '1.28')
            ok = ok and status == 200 and set(body['allocations']) == {self.provider} \
                and body['allocations'][self.provider]['resources'] == expected
        return ok, expected

    def remove_block(self, servers, images):
        done = {}
        deleting = []
        for s in servers:
            if s.get('id') and s['id'] in self.owned_servers:
                actual = self.connection.compute.find_server(s['id'], ignore_missing=True)
                if actual is None:
                    self.owned_servers.pop(s['id'])
                    continue
                require(actual.name == self.owned_servers[s['id']], 'server ownership')
                self.connection.compute.delete_server(actual, ignore_missing=True)
                deleting.append(actual)
        for actual in deleting:  # wait_for_delete treats NotFound as done
            self.connection.compute.wait_for_delete(actual, interval=1, wait=self.protocol['cleanup_timeout_s'])
            self.owned_servers.pop(actual.id)
        started = time.monotonic()
        released = False
        while time.monotonic() - started < 60:
            states = [self.pl_public('/allocations/' + s['id'], '1.28') for s in servers if s.get('id')]
            if all(st == 200 and body['allocations'] == {} for st, body in states):
                released = True
                break
            time.sleep(1)
        done['allocations_released_s'] = round(time.monotonic() - started, 1) if released else None
        status, usages = self.pl_public('/resource_providers/' + self.provider + '/usages', '1.0')
        done['provider_idle'] = status == 200 and not any(usages['usages'].values())
        for ident in images:
            self.request(self.args.url, 'DELETE', '/v2/images/' + ident, 204)
            self.request(self.args.url, 'GET', '/v2/images/' + ident, 404)
            self.pending.pop(('joint', ident), None)
            self.owned_images.pop(ident, None)
        cache = [BASE_CACHE / base_cache_name(i) for i in images]
        done['base_cache_created'] = all(p.exists() for p in cache)
        for p in cache:  # only the entries of this block's own image IDs
            subprocess.run(['sudo', '-n', 'rm', '-f', '--', str(p)], check=True)
        done['base_cache_removed'] = not any(p.exists() for p in cache)
        done['images_removed'] = True
        return done

    def run_block(self, block):
        placement, glance = variant_services(self.protocol, block['variant'])
        self.expect_routes(placement, glance)
        free = os.statvfs('/')
        require(free.f_bavail * free.f_frsize >= self.protocol['vm_free_minimum_bytes'], 'VM free disk above minimum')
        idle = None
        if self.protocol.get('idle_s'):  # both services loaded but quiet: idle CPU and PSS before the block
            idle_since = time.time_ns() // 1000
            time.sleep(self.protocol['idle_s'])
            idle = [idle_since, time.time_ns() // 1000]
        images = [self.upload_image(block, i) for i in range(block['concurrency'])]
        servers, since, until, makespan = self.boot_all(block, images)
        record = dict(block, placement=placement, glance=glance, window_us=[since, until], makespan_s=makespan,
                      servers=servers, idle_window_us=idle, valid=False)
        try:
            active = all(s.get('final') == 'ACTIVE' and s.get('image_ok') for s in servers)
            record['all_active'] = active
            record['allocations_ok'], record['flavor'] = self.verify_allocations(servers) if active else (False, None)
            record['traces'], record['trace_checks'] = self.traces(block, placement, glance, servers, images, since, until)
            if self.protocol.get('phases'):
                record['phases_ok'] = self.phases(servers, images, since, until) if active else False
        finally:
            try:
                record['cleanup'] = self.remove_block(servers, images)
            except Exception as error:
                record['cleanup'] = {'error': type(error).__name__}
                record['checks'] = {'cleanup': False}
                with self.blocks_path.open('a') as out:
                    out.write(json.dumps(record, separators=(',', ':')) + '\n')
                raise
        samples = read_samples(self.samples_path)
        record['resources'] = window_resources(samples, since, until)
        if idle:
            record['idle_resources'] = window_resources(samples, *idle)
        checks = {'all_active': record['all_active'], 'allocations_ok': record['allocations_ok'],
                  **record['trace_checks'], 'cold_transfer': record['cleanup']['base_cache_created'],
                  'released': record['cleanup']['allocations_released_s'] is not None,
                  'provider_idle': record['cleanup']['provider_idle'],
                  'base_cache_removed': record['cleanup']['base_cache_removed'],
                  'resources_sampled': record['resources'] is not None}
        if idle:
            checks['idle_sampled'] = record['idle_resources'] is not None
        if self.protocol.get('phases'):
            checks['phases_ok'] = record['phases_ok']
        record['checks'] = checks
        record['valid'] = all(checks.values())
        with self.blocks_path.open('a') as out:
            out.write(json.dumps(record, separators=(',', ':')) + '\n')
        return record

    # -- driver --------------------------------------------------------------
    def execute(self):
        (self.args.output / 'traces').mkdir()
        try:
            self.manifest()
            self.report['nova_ready_wait_s'] = self.wait_for_nova()
            self.prepare()
            self.connection, _, _ = sdk_connection()
            self.start_sampler()
            switches, current = [], None
            for block in self.plan:
                if block['variant'] != current:
                    switches.append(self.activate(block['variant']))
                    current = block['variant']
                    self.report['switches'] = switches
                record = self.run_block(block)
                print(json.dumps({'block': block['index'], 'of': len(self.plan), 'variant': block['variant'],
                                  'n': block['concurrency'], 'valid': record['valid'],
                                  'active_s': [s.get('active_s') for s in record['servers']]}), flush=True)
                if not record['valid']:
                    raise AssertionError('block %d invalid: %s' % (block['index'],
                                         ', '.join(k for k, v in record['checks'].items() if not v)))
                self.report['blocks_valid'] += 1
                self.checkpoint()
        except Exception as error:
            self.report['execution_error'] = type(error).__name__
            self.report['execution_reason'] = str(error) if isinstance(error, AssertionError) else None
            self.keep_diagnostics()
        finally:
            try:
                self.emergency_remove()
            except Exception as error:  # noqa: BLE001
                self.report['emergency_error'] = type(error).__name__
            self.stop_sampler()
            self.server = None
            try:
                self.report['cleanup'] = self.cleanup()
            except Exception as error:
                self.report['cleanup'] = {'error': type(error).__name__}
            finally:
                if self.connection is not None:
                    self.connection.close()
            cleanup = self.report['cleanup']
            self.report['passed'] = (self.report['blocks_valid'] == len(self.plan) and not self.report['execution_error']
                                     and all(cleanup.get(k) is True for k in glance_suite.CLEANUP + glance_suite.JOINT_CLEANUP))
            self.report['request_cases'] = len(self.cases)
            self.report['finished_utc'] = datetime.datetime.now(datetime.timezone.utc).isoformat()
            self.checkpoint()
        print(json.dumps({'run_id': self.report['run_id'], 'passed': self.report['passed'],
                          'blocks_valid': self.report['blocks_valid'], 'blocks_required': len(self.plan)}), flush=True)
        return 0 if self.report['passed'] else 1

    def emergency_remove(self):
        """Servers/images left by an aborted block (normal path empties these)."""
        if self.connection is None:
            return
        for ident, name in list(self.owned_servers.items()):
            actual = self.connection.compute.find_server(ident, ignore_missing=True)
            if actual is not None and actual.name == name:
                self.connection.compute.delete_server(ident, ignore_missing=True)
                self.connection.compute.wait_for_delete(actual, interval=2, wait=self.protocol['cleanup_timeout_s'])
            self.owned_servers.pop(ident)
        for ident in list(self.owned_images):
            subprocess.run(['sudo', '-n', 'rm', '-f', '--', str(BASE_CACHE / base_cache_name(ident))], check=False)
            self.owned_images.pop(ident)
        # Images still listed in self.pending are removed by Session.cleanup.


def run(args):
    if not args.allow_mutations:
        raise ValueError('explicit mutation opt-in required')
    if (not args.work_root.is_absolute() or args.work_root != args.work_root.resolve()
            or not re.fullmatch(r'tt-placement-[a-z0-9-]+', args.work_root.name)):
        raise ValueError('canonical owned private work root (tt-placement-*) required')
    for path in (args.binary, args.placement_binary, args.protocol):
        if not path.is_file():
            raise ValueError('binaries and protocol required')
    args.output.mkdir(parents=True, exist_ok=False)
    args.wait_seconds, args.lease_seconds, args.nova_ready_seconds, args.payload_mib = 300, 1200, 240, [1]
    with exclusive_run(Path('/run/lock/tt-placement-integration.lock')):
        args.work_root.mkdir(mode=0o700, exist_ok=False)
        import logging
        logging.disable(logging.CRITICAL)
        return Campaign(args).execute()


def main():
    parser = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    parser.add_argument('--protocol', type=Path, required=True)
    parser.add_argument('--pilot', action='store_true')
    parser.add_argument('--flavor-id', required=True)
    parser.add_argument('--network-id', required=True)
    parser.add_argument('--binary', type=Path, required=True, help='Glance Go binary')
    parser.add_argument('--placement-binary', type=Path, required=True)
    parser.add_argument('--work-root', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--url', default='http://192.168.122.10/image')
    parser.add_argument('--cirros-image', type=Path)
    parser.add_argument('--allow-mutations', action='store_true')
    args = parser.parse_args()
    try:
        return run(args)
    except Exception as error:
        print('joint campaign failed: ' + type(error).__name__ + (': ' + str(error) if isinstance(error, ValueError) else ''),
              file=sys.stderr)
        return 2


if __name__ == '__main__':
    sys.exit(main())

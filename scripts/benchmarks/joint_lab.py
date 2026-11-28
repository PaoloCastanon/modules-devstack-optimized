#!/usr/bin/env python3
"""Exclusive VM RPC for the joint Placement x Glance API campaign.

Composes the two reviewed labs instead of re-implementing them:
placement_lab.Lab (16 owned providers, own Go runtime + MySQL, route switch,
compute tracker published in both) and glance_lab.Lab (owned seeds on the
original catalog, Glance Go runtime, route switch, noop notifications). Each
keeps its own private root, identity and maintenance-control ledger. One
collector samples every cgroup of both services and the shared controls, so
one window yields Placement and Glance CPU/PSS on a common clock.

stdout is a private RPC channel (the token action returns credentials).
"""
import argparse
import json
import logging
from pathlib import Path
import re
import shutil
import subprocess
import sys
import time
from types import SimpleNamespace

HERE = Path(__file__).resolve().parent
sys.path[:0] = [str(HERE), str(HERE.parent / 'integration'), str(HERE.parent)]
import glance_lab  # noqa: E402
import placement_lab  # noqa: E402
from placement_suite import exclusive_run, require, save_json  # noqa: E402

SHARED_UNITS = [('mysql', 'mysql.service'), ('apache', 'apache2.service'), ('keystone', 'devstack@keystone.service'),
                ('memcached', 'memcached.service'), ('nova-compute', 'devstack@n-cpu.service'),
                ('nova-scheduler', 'devstack@n-sch.service'), ('rabbitmq', 'rabbitmq-server.service')]


def collector_units(pl_state, gl_state):
    """Every cgroup sampled in each window: both implementations of both
    services (the inactive ones stay as controls) and the shared stack."""
    units = [('placement-python', 'devstack@placement-api.service'), ('placement-go', pl_state['go_unit']),
             ('placement-go-mysql', pl_state['db_unit']), ('glance-python', 'devstack@g-api.service'),
             ('glance-go', gl_state['go_unit'])] + SHARED_UNITS
    labels = [label for label, _ in units]
    require(len(labels) == len(set(labels)) and all(unit for _, unit in units), 'distinct non-empty units')
    return units


class JointLab:
    def __init__(self, args):
        self.args = args
        self.closed = False
        self.collector = None
        self.current = None
        pl_root, gl_root = args.work_root / 'tt-placement-pl', args.work_root / 'tt-placement-gl'
        for root in (pl_root, gl_root):
            root.mkdir(mode=0o700)
        (args.output / 'placement').mkdir(mode=0o700)
        (args.output / 'glance').mkdir(mode=0o700)
        self.pl = placement_lab.Lab(SimpleNamespace(work_root=pl_root, output=args.output / 'placement',
                                                    binary=args.placement_binary, url=args.placement_url,
                                                    keystone_url=args.keystone_url))
        self.gl = glance_lab.Lab(SimpleNamespace(work_root=gl_root, output=args.output / 'glance', binary=args.glance_binary,
                                                 url=args.glance_url, go_route='public', go_catalog='official',
                                                 notification_policy='noop', go_notifications='noop'))

    # -- lifecycle -------------------------------------------------------------
    def prepare(self):
        # Glance first: its Session snapshots the original Placement as an
        # untouched control before Placement's tracker publication switches routes.
        glance = self.gl.prepare()
        placement = self.pl.prepare()
        self.pl_state = json.loads((self.pl.runtime / 'state.json').read_text())
        self.gl_state = json.loads((self.gl.runtime / 'state.json').read_text())
        return {'placement': placement, 'glance': glance,
                'collector_units': dict(collector_units(self.pl_state, self.gl_state))}

    def token(self):
        p, g = self.pl.refresh(), self.gl.refresh()
        return {'placement_token': p['token'], 'glance_token': g['token'],
                'metadata': {'placement': p['metadata'], 'glance': g['metadata']}}

    def seed(self, values):
        require(self.current is None, 'finish the previous window first')
        require(re.fullmatch(r'[a-z0-9-]+', values['run_id']), 'controlled run id')
        self.current = values['run_id']
        placement = self.pl.seed(values['placement'])
        glance = self.gl.seed(values['glance'])
        return {'placement': placement, 'glance': glance, 'routes': self.routes()}

    def routes(self):
        import glance_switch
        import placement_switch
        return {'placement_original_active': placement_switch.active(placement_switch.ORIGINAL_UNIT),
                'glance_original_active': glance_switch.active(glance_switch.ORIGINAL_UNIT),
                'placement_site_go': '127.0.0.1:18779' in placement_switch.SITE_FILE.read_text(),
                'glance_site_go': '127.0.0.1:19293' in glance_switch.SITE_FILE.read_text()}

    def finish(self, generations=None):
        placement = self.pl.cleanup_seed(generations)
        glance = self.gl.finish()
        self.current = None
        return {'removed': placement['removed'] and glance['removed'], 'placement': placement, 'glance': glance}

    # -- resources -------------------------------------------------------------
    def start_collector(self, run_id):
        require(self.collector is None and run_id == self.current, 'collector/run identity')
        groups = {}
        for label, unit in collector_units(self.pl_state, self.gl_state):
            group = subprocess.check_output(['systemctl', 'show', '--value', '-p', 'ControlGroup', unit], text=True).strip()
            if group:  # a stopped original has no cgroup; it is recorded as absent
                groups[label] = group
        path = self.args.output / (run_id + '-groups.json')
        save_json(path, groups)
        output, stop = self.args.output / (run_id + '-resources'), self.args.work_root / (run_id + '.stop')
        log = (self.args.output / (run_id + '-collector.log')).open('x')
        process = subprocess.Popen(['sudo', sys.executable, str(HERE / 'placement_resources.py'), '--groups', str(path),
                                    '--output', str(output), '--stop-file', str(stop), '--maximum-seconds', '900'],
                                   stdout=log, stderr=log)
        self.collector = process, log, stop
        time.sleep(5)  # idle baseline before the warm-up of both generators
        require(process.poll() is None, 'collector running')
        return {'groups': groups, 'idle_s': 5}

    def stop_collector(self):
        if self.collector is None:
            return {'stopped': True}
        process, log, stop = self.collector
        time.sleep(2)  # post-load bracket outside both timers
        stop.touch(mode=0o600)
        process.wait(timeout=15)
        log.close()
        self.collector = None
        require(process.returncode == 0, 'collector exit')
        return {'stopped': True}

    def cleanup(self):
        if self.closed:
            return {'already_closed': True}
        self.stop_collector()
        result = {}
        # Placement back to Python and its seeds gone before Glance's Session
        # compares the original Placement with its own initial snapshot.
        try:
            self.pl.cleanup_seed()
        finally:
            if self.pl.original and (self.pl.runtime / 'switch-state.json').exists():
                self.pl.route('python')
        # Reverse order of prepare: Glance's control ledger was entered last.
        result['glance'] = self.gl.cleanup()
        result['placement'] = self.pl.cleanup()
        shutil.rmtree(self.args.work_root)
        self.closed = True
        result['private_root_removed'] = not self.args.work_root.exists()
        save_json(self.args.output / 'cleanup.json', result)
        return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for name in ['work-root', 'output', 'placement-binary', 'glance-binary']:
        parser.add_argument('--' + name, type=Path, required=True)
    parser.add_argument('--placement-url', default='http://192.168.122.10/placement')
    parser.add_argument('--glance-url', default='http://192.168.122.10/image')
    parser.add_argument('--keystone-url', default='http://192.168.122.10/identity/v3')
    parser.add_argument('--allow-mutations', action='store_true')
    args = parser.parse_args()
    require(args.allow_mutations and args.work_root.is_absolute() and args.work_root == args.work_root.resolve()
            and re.fullmatch(r'tt-placement-[a-z0-9-]+', args.work_root.name), 'owned canonical private root and opt-in')
    logging.disable(logging.CRITICAL)
    args.output.mkdir(mode=0o700, parents=True, exist_ok=False)
    with exclusive_run(Path('/run/lock/tt-placement-integration.lock')):
        args.work_root.mkdir(mode=0o700, exist_ok=False)
        lab = JointLab(args)
        try:
            for line in sys.stdin:
                try:
                    request = json.loads(line)
                    action = request['action']
                    if action == 'ping':
                        value = {'utc_ns': time.time_ns()}
                    elif action == 'prepare':
                        value = lab.prepare()
                    elif action == 'token':
                        value = lab.token()
                    elif action == 'seed':
                        value = lab.seed(request['values'])
                    elif action == 'finish':
                        value = lab.finish(request.get('generations'))
                    elif action == 'start-collector':
                        value = lab.start_collector(request['run_id'])
                    elif action == 'stop-collector':
                        value = lab.stop_collector()
                    elif action == 'cleanup':
                        value = lab.cleanup()
                    else:
                        raise ValueError('unknown action')
                    print(json.dumps({'ok': True, 'value': value}), flush=True)
                except Exception as error:
                    print(json.dumps({'ok': False, 'error': type(error).__name__,
                                      'reason': str(error) if isinstance(error, AssertionError) else None}), flush=True)
                if lab.closed:
                    break
        finally:
            try:
                lab.cleanup()
            except Exception as error:
                save_json(args.output / 'cleanup-error.json', {'error': type(error).__name__, 'private_state_retained': True})


if __name__ == '__main__':
    main()

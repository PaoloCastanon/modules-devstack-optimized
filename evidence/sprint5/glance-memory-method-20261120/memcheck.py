"""Compare, at the same moment and under load, the collector's sources with
standard tools: PSS (smaps_rollup) vs smem, cgroup anon vs sum of Pss_Anon."""
import json, subprocess, sys, threading, time, http.client
sys.path[:0] = ['/home/stack/tt-b26-src/scripts/integration', '/home/stack/tt-b26-src/scripts/benchmarks']
import glance_replace as r, glance_memory
_, token = r.admin_token()
units = {'python': 'devstack@g-api.service', 'go': 'glance-go.service', 'memcached': 'memcached.service'}
subprocess.run(['sudo', 'systemctl', 'start', 'glance-go'], check=True); r.wait_up('http://127.0.0.1:19292')
ports = {'python': 60999, 'go': 19292}
stop = time.monotonic() + 40
def load(port):
    c = http.client.HTTPConnection('127.0.0.1', port, timeout=30)
    while time.monotonic() < stop:
        c.request('GET', '/v2/images?limit=50', headers={'X-Auth-Token': token}); c.getresponse().read()
threads = [threading.Thread(target=load, args=(p,)) for p in ports.values() for _ in range(8)]
for t in threads: t.start()
time.sleep(15)
out = {}
for label, unit in units.items():
    cg = subprocess.check_output(['systemctl', 'show', '--value', '-p', 'ControlGroup', unit], text=True).strip()
    pids = [int(x) for x in open('/sys/fs/cgroup' + cg + '/cgroup.procs').read().split()]
    pss = pss_anon = rss = 0
    for pid in pids:
        roll = dict(l.split(':', 1) for l in subprocess.check_output(['sudo', 'cat', '/proc/%d/smaps_rollup' % pid], text=True).splitlines()[1:])
        kb = lambda k: int(roll[k].split()[0]) * 1024
        pss += kb('Pss'); pss_anon += kb('Pss_Anon'); rss += kb('Rss')
    smem = subprocess.run(['sudo', 'smem', '-c', 'pid pss', '-H', '-P', '.'], capture_output=True, text=True).stdout
    smem_pss = sum(int(l.split()[1]) * 1024 for l in smem.splitlines() if l.split() and int(l.split()[0]) in pids)
    cgm = glance_memory.read_group(cg)
    out[label] = {'pids': len(pids), 'pss_collector_mib': round(pss / 2**20, 1), 'pss_smem_mib': round(smem_pss / 2**20, 1),
                  'rss_sum_mib': round(rss / 2**20, 1), 'pss_anon_sum_mib': round(pss_anon / 2**20, 1),
                  'cgroup_anon_mib': round(cgm['anon'] / 2**20, 1), 'cgroup_file_mib': round(cgm['file'] / 2**20, 1)}
for t in threads: t.join()
subprocess.run(['sudo', 'systemctl', 'stop', 'glance-go'], check=True)
print(json.dumps(out, indent=1))

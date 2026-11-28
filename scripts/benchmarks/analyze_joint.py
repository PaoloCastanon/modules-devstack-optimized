#!/usr/bin/env python3
"""Summarize a joint Placement x Glance campaign (blocks.jsonl of joint_campaign.py).

Per cell (variant/concurrency): every boot time, median and range, block
makespan, request counts and server time of the active Placement and Glance,
CPU seconds and peak PSS of each. Ratios against python-python are paired by
repetition (same repetition, same concurrency). Invalid blocks are counted
and excluded, never silently dropped. No tail percentiles: three repetitions
do not support them.
"""
import argparse
import json
from pathlib import Path
import statistics
import sys

REFERENCE = 'python-python'
VARIANT_ORDER = ('python-python', 'go-python', 'python-go', 'go-go')


def spread(values):
    values = [v for v in values if v is not None]
    if not values:
        return None
    return {'median': round(statistics.median(values), 4), 'min': round(min(values), 4), 'max': round(max(values), 4)}


def load(path):
    return [json.loads(line) for line in Path(path).read_text().splitlines() if line.strip()]


PHASES = ('request_to_build_s', 'build_s', 'image_fetch_s', 'spawn_log_s', 'active_poll_lag_s')
SERVICE_FIELDS = ('requests', 'server_ms', 'cpu_s', 'cpu_s_per_boot', 'pss_max_mib', 'pss_mean_mib', 'idle_cpu_s', 'idle_pss_mib')


def service_view(block, service):
    impl = block[service]
    name = service + '-' + impl
    trace = (block.get('traces') or {}).get(name) or {}
    res = ((block.get('resources') or {}).get('groups') or {}).get(name) or {}
    idle = ((block.get('idle_resources') or {}).get('groups') or {}).get(name) or {}
    cpu = res.get('cpu_s')
    return {'requests': trace.get('requests'), 'server_ms': trace.get('server_ms_total'),
            'cpu_s': cpu, 'cpu_s_per_boot': cpu / block['concurrency'] if cpu is not None else None,
            'pss_max_mib': res.get('pss_max_mib'), 'pss_mean_mib': res.get('pss_mean_mib'),
            'idle_cpu_s': idle.get('cpu_s'), 'idle_pss_mib': idle.get('pss_mean_mib')}


def stack_view(block):
    """Placement + Glance (active implementations) together."""
    views = [service_view(block, service) for service in ('placement', 'glance')]
    out = {}
    for field in ('cpu_s_per_boot', 'pss_mean_mib', 'idle_pss_mib'):
        values = [v[field] for v in views]
        out[field] = sum(values) if all(v is not None for v in values) else None
    return out


def analyze(blocks):
    valid = [b for b in blocks if b.get('valid')]
    cells, ratios = {}, {}
    for b in valid:
        cells.setdefault('%s/%d' % (b['variant'], b['concurrency']), []).append(b)
    out = {}
    for key, group in sorted(cells.items(), key=lambda kv: (VARIANT_ORDER.index(kv[0].split('/')[0]), kv[0])):
        boots = [s['active_s'] for b in group for s in b['servers']]
        row = {'blocks': len(group), 'boots': len(boots), 'boot_s_all': boots, 'boot_s': spread(boots),
               'makespan_s': spread([b['makespan_s'] for b in group]),
               'boots_per_min': spread([60 * b['concurrency'] / b['makespan_s'] for b in group if b['makespan_s']]),
               'vm_busy_fraction': spread([(b.get('resources') or {}).get('vm_busy_fraction') for b in group])}
        for service in ('placement', 'glance'):
            views = [service_view(b, service) for b in group]
            row[service] = {field: spread([v[field] for v in views]) for field in SERVICE_FIELDS}
        stacks = [stack_view(b) for b in group]
        row['stack'] = {field: spread([v[field] for v in stacks]) for field in ('cpu_s_per_boot', 'pss_mean_mib', 'idle_pss_mib')}
        row['phases'] = {name: spread([(s.get('phases') or {}).get(name) for b in group for s in b['servers']]) for name in PHASES}
        out[key] = row
    by_rep = {(b['variant'], b['concurrency'], b['repetition']): b for b in valid}
    for (variant, n, rep), b in sorted(by_rep.items()):
        if variant == REFERENCE or (REFERENCE, n, rep) not in by_rep:
            continue
        ref = by_rep[(REFERENCE, n, rep)]
        med = statistics.median([s['active_s'] for s in b['servers']])
        ref_med = statistics.median([s['active_s'] for s in ref['servers']])
        entry = ratios.setdefault('%s/%d' % (variant, n), {'boot': [], 'makespan': [], 'stack_cpu_per_boot': [], 'stack_pss_mean': []})
        entry['boot'].append(med / ref_med)
        entry['makespan'].append(b['makespan_s'] / ref['makespan_s'])
        for field, name in (('cpu_s_per_boot', 'stack_cpu_per_boot'), ('pss_mean_mib', 'stack_pss_mean')):
            x, y = stack_view(b)[field], stack_view(ref)[field]
            if x is not None and y:
                entry[name].append(x / y)
    ratio_out = {key: {'pairs': len(v['boot']), 'boot_median_ratio': spread(v['boot']),
                       'makespan_ratio': spread(v['makespan']), 'stack_cpu_per_boot_ratio': spread(v['stack_cpu_per_boot']),
                       'stack_pss_mean_ratio': spread(v['stack_pss_mean'])} for key, v in sorted(ratios.items())}
    return {'reference': REFERENCE, 'valid_blocks': len(valid), 'invalid_blocks': len(blocks) - len(valid),
            'cells': out, 'ratios': ratio_out}


def fmt(value, digits=1):
    if not value:
        return '—'
    return '%.*f [%.*f–%.*f]' % (digits, value['median'], digits, value['min'], digits, value['max'])


def markdown(result):
    lines = ['Bloques válidos: %d; inválidos: %d. Mediana [mín–máx] por celda.' % (result['valid_blocks'], result['invalid_blocks']),
             '', '| Variante | N | Boots | POST→ACTIVE (s) | Makespan (s) | Boots/min | Placement req | Glance req |',
             '|---|---:|---:|---|---|---|---|---|']
    for key, row in result['cells'].items():
        variant, n = key.split('/')
        lines.append('| %s | %s | %d | %s | %s | %s | %s | %s |' % (
            variant, n, row['boots'], fmt(row['boot_s']), fmt(row['makespan_s']), fmt(row['boots_per_min']),
            fmt(row['placement']['requests'], 0), fmt(row['glance']['requests'], 0)))
    lines += ['', '| Variante | N | Placement CPU (s) | Placement PSS (MiB) | Glance CPU (s) | Glance PSS (MiB) |',
              '|---|---:|---|---|---|---|']
    for key, row in result['cells'].items():
        variant, n = key.split('/')
        lines.append('| %s | %s | %s | %s | %s | %s |' % (
            variant, n, fmt(row['placement']['cpu_s'], 2), fmt(row['placement']['pss_max_mib']),
            fmt(row['glance']['cpu_s'], 2), fmt(row['glance']['pss_max_mib'])))
    if any(row['phases'][p] for row in result['cells'].values() for p in PHASES):
        lines += ['', 'Fases por boot (s): petición→build = API, conductor, scheduler y Placement; build = cómputo '
                  '(descarga de Glance, red, libvirt); descarga = lock del archivo base mientras se trae de Glance.', '',
                  '| Variante | N | Petición→build | Build | Descarga imagen | Spawn (log) | Retardo del polling |',
                  '|---|---:|---|---|---|---|---|']
        for key, row in result['cells'].items():
            variant, n = key.split('/')
            ph = row['phases']
            lines.append('| %s | %s | %s | %s | %s | %s | %s |' % (
                variant, n, fmt(ph['request_to_build_s'], 2), fmt(ph['build_s'], 2), fmt(ph['image_fetch_s'], 2),
                fmt(ph['spawn_log_s'], 2), fmt(ph['active_poll_lag_s'], 2)))
    if any(row['placement']['cpu_s_per_boot'] for row in result['cells'].values()):
        lines += ['', 'CPU por boot (s), PSS medio en la ventana y PSS en reposo antes del bloque (MiB):', '',
                  '| Variante | N | Placement CPU/boot | Glance CPU/boot | Placement PSS medio | Glance PSS medio '
                  '| Placement PSS reposo | Glance PSS reposo | Pila PSS medio |',
                  '|---|---:|---|---|---|---|---|---|---|']
        for key, row in result['cells'].items():
            variant, n = key.split('/')
            pl, gl = row['placement'], row['glance']
            lines.append('| %s | %s | %s | %s | %s | %s | %s | %s | %s |' % (
                variant, n, fmt(pl['cpu_s_per_boot'], 3), fmt(gl['cpu_s_per_boot'], 3), fmt(pl['pss_mean_mib']),
                fmt(gl['pss_mean_mib']), fmt(pl['idle_pss_mib']), fmt(gl['idle_pss_mib']), fmt(row['stack']['pss_mean_mib'])))
    lines += ['', 'Cociente frente a %s, pareado por repetición (<1 = más rápido o menos recursos; '
              'pila = Placement + Glance activos):' % result['reference'], '',
              '| Celda | Pares | Boot (mediana) | Makespan | Pila CPU/boot | Pila PSS medio |', '|---|---:|---|---|---|---|']
    for key, row in result['ratios'].items():
        lines.append('| %s | %d | %s | %s | %s | %s |' % (key, row['pairs'], fmt(row['boot_median_ratio'], 3),
                                                       fmt(row['makespan_ratio'], 3), fmt(row['stack_cpu_per_boot_ratio'], 3),
                                                       fmt(row['stack_pss_mean_ratio'], 3)))
    return '\n'.join(lines) + '\n'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('blocks', type=Path)
    parser.add_argument('--json', type=Path)
    parser.add_argument('--markdown', type=Path)
    args = parser.parse_args()
    result = analyze(load(args.blocks))
    if args.json:
        args.json.write_text(json.dumps(result, indent=2) + '\n')
    text = markdown(result)
    if args.markdown:
        args.markdown.write_text(text)
    print(text)
    return 0


if __name__ == '__main__':
    sys.exit(main())

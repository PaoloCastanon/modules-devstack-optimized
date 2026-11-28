#!/usr/bin/env python3
"""Summarize a joint Placement x Glance API campaign (joint_api_campaign.py output).

Per window and service (Placement and Glance, measured at the same time):
throughput, per-run and per-operation percentiles from the raw samples,
CPU s per correct request, PSS mean/peak and idle PSS from the aligned 1 Hz
cgroup samples (placement_resources.interval_summary, the B18 method), and
the shared controls and VM CPU. Cells are (pair, concurrency, variant):
median and observed range of the repetitions; ratios against python-python
are paired by repetition. Invalid windows are listed, never dropped silently.
"""
import argparse
import datetime
import gzip
import json
from pathlib import Path
import statistics
import sys

HERE = Path(__file__).resolve().parent
sys.path.insert(0, str(HERE))
from placement_http import percentile  # noqa: E402
from placement_resources import interval_summary  # noqa: E402

REFERENCE = 'python-python'
VARIANTS = ('python-python', 'go-python', 'python-go', 'go-go')
SERVICE_METRICS = ('correct_ops_s', 'p50_ms', 'p95_ms', 'p99_ms', 'complete_cycles_s', 'cpu_ms_per_correct_request',
                   'cpu_fraction_of_four_cores', 'pss_mean_mib', 'pss_peak_mib', 'idle_pss_mib', 'rss_peak_mib')
RATIO_METRICS = ('correct_ops_s', 'p50_ms', 'p99_ms', 'cpu_ms_per_correct_request', 'pss_mean_mib')
MIB = 1024 ** 2


def nanos(value):
    return round(datetime.datetime.fromisoformat(value).timestamp() * 1e9)


def spread(values):
    values = [v for v in values if v is not None]
    if not values:
        return None
    return {'n': len(values), 'median': statistics.median(values), 'min': min(values), 'max': max(values)}


def read_jsonl(path):
    opener = gzip.open if str(path).endswith('.gz') else open
    with opener(path, 'rt') as stream:
        return [json.loads(line) for line in stream if line.strip()]


def service_run(kind, report, samples, records, ticks, label, offset):
    """Metrics of one service in one window. kind: 'placement' | 'glance'."""
    m = report['measurement']
    start, end = nanos(m['start_utc']) + offset, nanos(m['end_utc']) + offset
    valid = [s for s in samples if s['valid']]
    if len(samples) != m['requests'] or len(valid) != m['correct']:
        raise ValueError(kind + ' samples/report count mismatch')
    latency = m['correct_latency_ms'] if kind == 'placement' else m['latency_ms']
    cycles = sum(m['cycles_per_worker']) if kind == 'placement' else m['cycles']
    out = {'correct_ops_s': m['correct_ops_s'], 'requests': m['requests'], 'errors': m['errors'],
           'p50_ms': latency['p50'], 'p95_ms': latency['p95'], 'p99_ms': latency['p99'],
           'p99_tail_supported': m['p99_tail_supported'], 'complete_cycles_s': cycles / m['elapsed_s']}
    if kind == 'glance':
        out['validated_payload_mib_s'] = m['validated_payload_mib_s']
        ttfb = [s['ttfb_ns'] / 1e6 for s in valid if s.get('ttfb_ns') is not None]
        out['ttfb_p50_ms'] = percentile(ttfb, .5) if ttfb else None
    per_op = {}
    for s in valid:
        per_op.setdefault(s['operation'], []).append(s['elapsed_ns'] / 1e6)
    out['per_operation_ms'] = {op: {'n': len(v), 'p50': percentile(v, .5), 'p95': percentile(v, .95), 'p99': percentile(v, .99)}
                               for op, v in sorted(per_op.items())}
    completions = [start + s['offset_ns'] + s['elapsed_ns'] for s in valid]
    res = interval_summary(records, label, start, end, completions, cores=4, ticks=ticks)
    cpu_op = res['cpu_s_per_correct_request']
    out.update(cpu_ms_per_correct_request=cpu_op * 1000 if cpu_op is not None else None,
               cpu_fraction_of_four_cores=res['cpu_fraction_of_four_cores'], cpu_seconds=res['cpu_seconds'],
               pss_mean_mib=res['pss_mean_bytes'] / MIB, pss_peak_mib=res['pss_peak_observed_bytes'] / MIB,
               rss_peak_mib=res['rss_peak_observed_bytes'] / MIB, resource_samples=res['samples'],
               window_ns=(start, end), vm_control=res.get('vm_control'))
    warm = report.get('warmup')
    warm_start = nanos(warm['start_utc']) + offset if warm else start
    idle = [sum(p['pss_bytes'] for p in r['groups'][label]['processes']) for r in records
            if r['read_finished_utc_ns'] < warm_start and r['groups'].get(label, {}).get('processes')
            and all(p['pss_bytes'] is not None for p in r['groups'][label]['processes'])]
    out['idle_pss_mib'] = statistics.mean(idle) / MIB if len(idle) >= 3 else None
    return out


def vm_busy(control):
    if not control:
        return None
    d = control['cpu_delta_ticks']
    total = sum(d[:8])
    return (total - d[3] - d[4]) / total if total else None


def analyze_window(root, entry):
    ident = entry['run_id']
    report = json.loads((root / ident / 'report.json').read_text())
    pl, gl = entry['variant'].split('-')
    clocks = report['clocks']
    offset = (clocks['before']['best']['remote_minus_host_ns'] + clocks['after']['best']['remote_minus_host_ns']) // 2
    res_dir = root / 'lab' / (ident + '-resources')
    records = read_jsonl(res_dir / 'resources.jsonl')
    ticks = json.loads((res_dir / 'collector.json').read_text())['clock_ticks_s']
    preport = json.loads((root / ident / 'placement' / 'report.json').read_text())
    greport = json.loads((root / ident / 'glance' / 'report.json').read_text())
    psamples = read_jsonl(root / ident / 'placement' / 'samples.jsonl.gz')
    gsamples = read_jsonl(root / ident / 'glance' / 'measurement-samples.jsonl.gz')
    row = {'run_id': ident, 'pair': entry['pair'], 'concurrency': entry['concurrency'], 'repetition': entry['repetition'],
           'variant': entry['variant'], 'overlap_s': report['overlap_s'],
           'placement': service_run('placement', preport, psamples, records, ticks, 'placement-' + pl, offset),
           'glance': service_run('glance', greport, gsamples, records, ticks, 'glance-' + gl, offset)}
    # Controls over the union of both measurement windows (shared stack cost).
    start = min(row['placement']['window_ns'][0], row['glance']['window_ns'][0])
    end = max(row['placement']['window_ns'][1], row['glance']['window_ns'][1])
    controls = {}
    for label in records[0]['groups']:
        if label in ('placement-' + pl, 'glance-' + gl):
            continue
        try:
            c = interval_summary(records, label, start, end, [], cores=4, ticks=ticks)
            controls[label] = {'cpu_seconds': c['cpu_seconds'], 'pss_mean_mib': c['pss_mean_bytes'] / MIB}
        except (ValueError, KeyError):
            controls[label] = {'unavailable': True}  # e.g. a stopped original behind a Go route
    row['controls'] = controls
    row['vm_busy_fraction'] = vm_busy(row['placement'].get('vm_control'))
    row['stack_pss_mib'] = row['placement']['pss_mean_mib'] + row['glance']['pss_mean_mib']
    row['stack_cpu_s'] = row['placement']['cpu_seconds'] + row['glance']['cpu_seconds']
    for service in ('placement', 'glance'):
        row[service].pop('window_ns')
        row[service].pop('vm_control', None)
    return row


def aggregate(rows):
    cells = {}
    for r in rows:
        cells.setdefault((r['pair'], r['concurrency'], r['variant']), []).append(r)
    out = []
    for (pair, c, variant), group in sorted(cells.items(), key=lambda kv: (kv[0][0], kv[0][1], VARIANTS.index(kv[0][2]))):
        cell = {'pair': pair, 'concurrency': c, 'variant': variant, 'repetitions': len(group),
                'vm_busy_fraction': spread([g['vm_busy_fraction'] for g in group]),
                'stack_pss_mib': spread([g['stack_pss_mib'] for g in group]),
                'stack_cpu_s': spread([g['stack_cpu_s'] for g in group])}
        for service in ('placement', 'glance'):
            keys = SERVICE_METRICS + (('validated_payload_mib_s', 'ttfb_p50_ms') if service == 'glance' else ())
            cell[service] = {k: spread([g[service].get(k) for g in group]) for k in keys}
        out.append(cell)
    ratios = []
    index = {(r['pair'], r['concurrency'], r['repetition'], r['variant']): r for r in rows}
    for (pair, c, rep, variant), r in sorted(index.items()):
        ref = index.get((pair, c, rep, REFERENCE))
        if variant == REFERENCE or ref is None:
            continue
        ratios.append({'pair': pair, 'concurrency': c, 'repetition': rep, 'variant': variant,
                       **{service + ':' + k: (r[service][k] / ref[service][k]) if r[service].get(k) and ref[service].get(k) else None
                          for service in ('placement', 'glance') for k in RATIO_METRICS}})
    ratio_cells = {}
    for x in ratios:
        ratio_cells.setdefault((x['pair'], x['concurrency'], x['variant']), []).append(x)
    ratio_out = [{'pair': p, 'concurrency': c, 'variant': v,
                  **{k: spread([g[k] for g in group]) for k in group[0] if ':' in k}}
                 for (p, c, v), group in sorted(ratio_cells.items(), key=lambda kv: (kv[0][0], kv[0][1], VARIANTS.index(kv[0][2])))]
    return out, ratio_out


def analyze(root):
    manifest = json.loads((root / 'manifest.json').read_text())
    rows, errors = [], []
    for entry in manifest['sequence']:
        done = next((r for r in manifest['runs'] if r['run_id'] == entry['run_id']), None)
        if not done:
            continue
        if not done['valid']:
            errors.append({'run_id': entry['run_id'], 'error': 'invalid window'})
            continue
        try:
            rows.append(analyze_window(root, entry))
        except (OSError, ValueError, KeyError, TypeError) as error:
            errors.append({'run_id': entry['run_id'], 'error': str(error) if isinstance(error, ValueError) else type(error).__name__})
    cells, ratios = aggregate(rows)
    required = len(manifest['sequence'])
    return {'schema': 1, 'campaign_id': manifest['campaign_id'], 'stage': manifest['stage'],
            'accepted': manifest['valid'] and manifest['cleanup'] and not errors and len(rows) == required,
            'windows_required': required, 'windows_analyzed': len(rows), 'errors': errors,
            'runs': rows, 'cells': cells, 'ratios_vs_python_python': ratios,
            'method': {'percentiles': 'per-run nearest rank from raw samples; cell median of per-run values, never pooled',
                       'cpu': 'service cgroup CPU over the common sampled interval / correct requests completed inside it (B18 method)',
                       'pss': '1 Hz sum of the service processes; mean and observed peak; idle = samples before warm-up'},
            'limitations': ['3 repetitions per cell: median and observed range, no significance',
                            'both generators run on the host; closed loop',
                            'deployed workers, caches and SQL pools differ between implementations; no language-only attribution',
                            'PSS sampled at 1 Hz; peak is observed, not absolute']}


def fmt(cell, key, scale=1, digits=1):
    value = cell.get(key) if cell else None
    if not value:
        return '—'
    return '%.*f [%.*f–%.*f]' % (digits, value['median'] * scale, digits, value['min'] * scale, digits, value['max'] * scale)


def markdown(result):
    lines = ['Campaña %s: %d/%d ventanas analizadas%s. Mediana [mín–máx] de las repeticiones.' % (
        result['stage'], result['windows_analyzed'], result['windows_required'], ', aceptada' if result['accepted'] else '')]
    for service, title in (('placement', 'Placement (con carga de Glance)'), ('glance', 'Glance (con carga de Placement)')):
        lines += ['', '### ' + title, '',
                  '| Par | C | Variante | ops/s | p50 ms | p99 ms | CPU ms/op | PSS medio MiB | PSS reposo MiB |',
                  '|---|---:|---|---|---|---|---|---|---|']
        for cell in result['cells']:
            s = cell[service]
            lines.append('| %s | %d | %s | %s | %s | %s | %s | %s | %s |' % (
                cell['pair'], cell['concurrency'], cell['variant'], fmt(s, 'correct_ops_s'), fmt(s, 'p50_ms', digits=2),
                fmt(s, 'p99_ms', digits=2), fmt(s, 'cpu_ms_per_correct_request', digits=3), fmt(s, 'pss_mean_mib'), fmt(s, 'idle_pss_mib')))
    lines += ['', '### Pila (los dos servicios activos) y VM', '',
              '| Par | C | Variante | PSS Placement+Glance MiB | CPU Placement+Glance s | CPU VM ocupada |', '|---|---:|---|---|---|---|']
    for cell in result['cells']:
        lines.append('| %s | %d | %s | %s | %s | %s |' % (cell['pair'], cell['concurrency'], cell['variant'],
                     fmt(cell, 'stack_pss_mib'), fmt(cell, 'stack_cpu_s', digits=2), fmt(cell, 'vm_busy_fraction', 100, 0) + ' %'))
    lines += ['', '### Cocientes frente a python-python (pareados por repetición)', '',
              '| Par | C | Variante | Placement ops/s | Placement CPU/op | Placement PSS | Glance ops/s | Glance CPU/op | Glance PSS |',
              '|---|---:|---|---|---|---|---|---|---|']
    for r in result['ratios_vs_python_python']:
        lines.append('| %s | %d | %s | %s | %s | %s | %s | %s | %s |' % (
            r['pair'], r['concurrency'], r['variant'], fmt(r, 'placement:correct_ops_s', digits=2),
            fmt(r, 'placement:cpu_ms_per_correct_request', digits=2), fmt(r, 'placement:pss_mean_mib', digits=2),
            fmt(r, 'glance:correct_ops_s', digits=2), fmt(r, 'glance:cpu_ms_per_correct_request', digits=2),
            fmt(r, 'glance:pss_mean_mib', digits=2)))
    if result['errors']:
        lines += ['', 'Ventanas con error: ' + ', '.join(e['run_id'] + ' (' + e['error'] + ')' for e in result['errors'])]
    return '\n'.join(lines) + '\n'


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('root', type=Path)
    parser.add_argument('--output', type=Path, required=True, help='summary JSON (new file)')
    parser.add_argument('--markdown', type=Path)
    args = parser.parse_args()
    result = analyze(args.root)
    with args.output.open('x') as stream:
        json.dump(result, stream, indent=2)
        stream.write('\n')
    text = markdown(result)
    if args.markdown:
        args.markdown.write_text(text)
    print(text)
    return 0 if result['accepted'] else 1


if __name__ == '__main__':
    sys.exit(main())

#!/usr/bin/env python3
"""Reproduce B17/B18 summaries from request samples and aligned /proc samples."""
import argparse
import datetime
import gzip
import json
from pathlib import Path
import random
import statistics

from placement_http import percentile
from placement_resources import interval_summary
from campaign_protocol import accounting


def nanos(value):
    return round(datetime.datetime.fromisoformat(value).timestamp() * 1e9)


def bootstrap(values, seed=20261029, count=10000):
    if not values:
        raise ValueError('no independent repetitions')
    result = {'n': len(values), 'median': statistics.median(values), 'min': min(values), 'max': max(values)}
    if len(values) < 5:
        return {**result, 'interval_kind': 'observed_range; insufficient independent repetitions for confidence claim'}
    rng = random.Random(seed)
    draws = [statistics.median(rng.choices(values, k=len(values))) for _ in range(count)]
    return {'n': len(values), 'median': statistics.median(values), 'min': min(values), 'max': max(values),
            'ci95_bootstrap_median': [percentile(draws, .025), percentile(draws, .975)],
            'resamples': count, 'seed': seed}


def analyze(root):
    manifest = json.loads((root / 'manifest.json').read_text())
    protocol = manifest['protocol']
    dimensions = accounting(protocol)
    runs, errors = {}, []
    for entry in manifest['runs']:
        ident = entry['run_id']
        report = json.loads((root / ident / 'report.json').read_text())
        measurement = report['measurement']
        if not measurement or not report['valid']:
            errors.append({'run_id': ident, 'error': 'invalid HTTP/state run'})
            continue
        run = {'b17_valid': True, 'b18_valid': False, 'scenario': report['scenario'], 'concurrency': report['concurrency'],
               'implementation': report['implementation'], 'correct_ops_s': measurement['correct_ops_s'],
               'completed_requests_s': measurement['completed_requests_s'], 'error_rate': measurement['errors'] / measurement['requests'],
               'p99_tail_supported': measurement['p99_tail_supported'],
               **{key + '_ms': value for key, value in measurement['correct_latency_ms'].items()},
               'complete_cycles_s': sum(measurement['cycles_per_worker']) / measurement['elapsed_s']}
        try:
            resource_file = root / 'lab' / (ident + '-resources') / 'resources.jsonl'
            opener = resource_file.open if resource_file.exists() else lambda mode: gzip.open(str(resource_file) + '.gz', mode)
            with opener('rt') as stream:
                records = [json.loads(line) for line in stream]
            clocks = report['clocks']
            offset = (clocks['before']['best']['remote_minus_host_ns'] + clocks['after']['best']['remote_minus_host_ns']) // 2
            start = nanos(measurement['start_utc']) + offset
            end = nanos(measurement['end_utc']) + offset
            completions = []
            per_operation = {}
            with gzip.open(root / ident / 'samples.jsonl.gz', 'rt') as samples:
                count, correct = 0, 0
                for line in samples:
                    item = json.loads(line)
                    count += 1
                    correct += bool(item['valid'])
                    if item['valid']:
                        completions.append(start + item['offset_ns'] + item['elapsed_ns'])
                        per_operation.setdefault(item['operation'], []).append(item['elapsed_ns'] / 1e6)
            if count != measurement['requests'] or correct != measurement['correct']:
                raise ValueError('request samples/report count mismatch')
            run['samples_verified'] = True
            run['per_operation_latency_ms'] = {op: {'n': len(values), 'p50': percentile(values, .5), 'p95': percentile(values, .95),
                                                   'p99': percentile(values, .99), 'p99_tail_supported': len(values) >= 1000} for op, values in per_operation.items()}
            run['resources'] = interval_summary(records, report['implementation'], start, end, completions,
                                                  cores=4, ticks=json.loads((root / 'lab' / (ident + '-resources') / 'collector.json').read_text())['clock_ticks_s'])
            run['resources']['clock_uncertainty_ns'] = max(c['best']['rtt_ns'] // 2 for c in clocks.values()) + report['clock_drift_ns']
            # Samples before warmup start are the explicitly measured idle baseline.
            warmup_start = nanos(report['warmup']['start_utc']) + offset if report.get('warmup') else start
            idle = [sum(p['pss_bytes'] for p in row['groups'][report['implementation']]['processes']) for row in records
                    if row['read_finished_utc_ns'] < warmup_start and row['groups'][report['implementation']]['processes']
                    and all(p['pss_bytes'] is not None for p in row['groups'][report['implementation']]['processes'])]
            if len(idle) < 3:
                raise ValueError('insufficient idle baseline')
            run['resources']['idle_pss_mean_bytes'] = statistics.mean(idle)
            run['resources']['idle_samples'] = len(idle)
            control = {}
            for label in records[0]['groups']:
                if label == report['implementation']:
                    continue
                try:
                    control[label] = interval_summary(records, label, start, end, [], ticks=json.loads((root / 'lab' / (ident + '-resources') / 'collector.json').read_text())['clock_ticks_s'])
                except ValueError:
                    control[label] = {'static_group_unavailable': True, 'raw_membership_preserved': True}
            run['controls'] = control
            run['b18_valid'] = True
        except (OSError, ValueError, KeyError, TypeError) as error:
            errors.append({'run_id': ident, 'error': str(error) if isinstance(error, ValueError) else type(error).__name__})
        run['b17_valid'] = run['b17_valid'] and run.get('samples_verified', False)
        runs[ident] = run
    groups = []
    for scenario in protocol['scenarios']:
        for concurrency in protocol['concurrency']:
            matching = {ident: row for ident, row in runs.items() if row['scenario'] == scenario and row['concurrency'] == concurrency}
            if not matching:
                continue
            group = {'scenario': scenario, 'concurrency': concurrency, 'implementations': {}, 'paired_go_over_python': {}}
            for implementation in ['python', 'go']:
                rows = [row for row in matching.values() if row['implementation'] == implementation]
                metrics = {key: bootstrap([r[key] for r in rows], count=protocol['bootstrap_resamples'])
                           for key in ['correct_ops_s', 'completed_requests_s', 'p50_ms', 'p95_ms', 'p99_ms', 'complete_cycles_s'] if rows}
                metrics['runs'] = len(rows)
                metrics['p99_runs_supported'] = sum(r['p99_tail_supported'] for r in rows)
                measured = [r['resources'] for r in rows if r['b18_valid']]
                for key in ['cpu_seconds', 'cpu_fraction_of_four_cores', 'cpu_s_per_correct_request', 'pss_mean_bytes', 'pss_peak_observed_bytes', 'idle_pss_mean_bytes', 'collector_core_fraction']:
                    if measured:
                        metrics[key] = bootstrap([r[key] for r in measured], count=protocol['bootstrap_resamples'])
                group['implementations'][implementation] = metrics
            ratios = {}
            for pair in manifest['sequence']:
                if pair['scenario'] != scenario or pair['concurrency'] != concurrency:
                    continue
                python, go = [runs.get(pair['pair_id'] + '-' + impl) for impl in ['python', 'go']]
                if not python or not go:
                    continue
                for key in ['correct_ops_s', 'p50_ms', 'p95_ms', 'p99_ms']:
                    ratios.setdefault(key, []).append(go[key] / python[key])
                if python['b18_valid'] and go['b18_valid']:
                    for key in ['cpu_s_per_correct_request', 'pss_mean_bytes', 'pss_peak_observed_bytes']:
                        if python['resources'][key] > 0:
                            ratios.setdefault(key, []).append(go['resources'][key] / python['resources'][key])
            group['paired_go_over_python'] = {k: bootstrap(v, count=protocol['bootstrap_resamples']) for k, v in ratios.items()}
            groups.append(group)
    required = dimensions['runs']
    expected_ids = {f'{scenario}-c{concurrency:02}-r{repetition:02}-{impl}'
                    for scenario in protocol['scenarios'] for concurrency in protocol['concurrency']
                    for repetition in range(1, protocol['repetitions'] + 1) for impl in ['python', 'go']}
    observed_ids = [entry['run_id'] for entry in manifest['runs']]
    official = (manifest['stage'] == 'measurement' and manifest['valid'] and manifest['cleanup']
                and len(observed_ids) == required and set(observed_ids) == expected_ids and set(runs) == expected_ids)
    return {'schema': 1, 'campaign_id': manifest['campaign_id'], 'stage': manifest['stage'],
            'b17_accepted': official and all(r['b17_valid'] for r in runs.values()),
            'b18_accepted': official and not errors and all(r['b18_valid'] for r in runs.values()),
            'protocol': protocol, 'runs_required': required, 'runs_observed': len(runs), 'errors': errors, 'runs': runs, 'groups': groups,
            'statistical_unit': 'independent repetition; paired ratio for implementation comparisons',
            'percentiles': 'per-run nearest rank; group median of per-run percentiles, never a pooled percentile',
            'limitations': [f"n={protocol['repetitions']} independent pairs per cell; small samples limit inference",
                            'closed-loop load; does not infer open-arrival performance',
                            'deployed auth caches, workers, SQL pools and database settings differ; no language-only attribution',
                            'PSS sampled at 1Hz, maximum is observed, not absolute peak',
                            'Go heap/GC counters unavailable in this production binary; total process PSS measured',
                            'Nova boot timing is separate historical B14/B15 evidence, not HTTP Placement latency']}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('root', type=Path)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    result = analyze(args.root)
    with args.output.open('x') as stream:
        json.dump(result, stream, indent=2)
        stream.write('\n')
    print(json.dumps({k: result[k] for k in ['b17_accepted', 'b18_accepted', 'runs_required', 'runs_observed', 'errors']}))


if __name__ == '__main__':
    main()

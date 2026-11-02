#!/usr/bin/env python3
"""Render reproducible result tables/figures after both strict acceptance gates."""
import argparse
import json
from pathlib import Path


def render(summary, output):
    if not summary['b17_accepted'] or not summary['b18_accepted']:
        raise ValueError('both full campaign acceptance gates required')
    output.mkdir(parents=True, exist_ok=False)
    metrics = [('B17 latency and throughput', [('correct_ops_s', 'Correct HTTP requests/s'), ('p50_ms', 'p50 latency (ms)'), ('p95_ms', 'p95 latency (ms)'), ('p99_ms', 'p99 latency (ms)')]),
               ('B18 CPU and PSS', [('cpu_s_per_correct_request', 'CPU seconds / correct request'), ('pss_mean_bytes', 'Mean PSS (MiB)')])]
    for title, columns in metrics:
        rows = [f'# {title}', '', 'Computed from all 240 official runs, five paired repetitions per cell.', '',
                'Values are medians of independent run metrics. Ratios and 95% bootstrap intervals use paired repetitions.',
                'A median of run percentiles is not a pooled request percentile. n=5 limits inference.', '',
                '| Load | Concurrency | Metric | Python median | Go median | Go/Python paired median [95% CI] |',
                '|---|---:|---|---:|---:|---:|']
        for group in summary['groups']:
            for metric, label in columns:
                scale = 1024 * 1024 if metric == 'pss_mean_bytes' else 1
                python = group['implementations']['python'][metric]['median'] / scale
                go = group['implementations']['go'][metric]['median'] / scale
                ratio = group['paired_go_over_python'][metric]
                ci = ratio['ci95_bootstrap_median']
                if metric == 'p99_ms' and any(group['implementations'][impl]['p99_runs_supported'] < 5 for impl in ['python', 'go']):
                    label += ' (some runs have insufficient tail support; see summary)'
                rows.append(f"| {group['scenario']} | {group['concurrency']} | {label} | {python:.6g} | {go:.6g} | {ratio['median']:.4g} [{ci[0]:.4g}, {ci[1]:.4g}] |")
        rows.extend(['', 'Limits: same VM and Apache entry, but deployed workers, authentication token caching, SQL pools and MySQL configuration differ.',
                     'This compares these deployments and does not isolate programming language. Go is not required to be faster.',
                     'CPU excludes database, Keystone, Apache and generator; their raw control activity is retained. PSS peak is sampled, not absolute.',
                     'Lower service CPU does not imply lower whole-stack CPU; inspect the separate controls, especially authentication work in Keystone.',
                     'Go heap/GC counters are not exposed by this binary. RSS can double count shared pages and is secondary.',
                     'Nova boot duration includes other services and is covered separately in B14/B15; it is not Placement HTTP latency.', ''])
        (output / ('b17-results.md' if title.startswith('B17') else 'b18-results.md')).write_text('\n'.join(rows))
    # Plotting is deliberately after every load window has ended.
    import matplotlib
    matplotlib.use('Agg')
    import matplotlib.pyplot as plt
    scenarios = sorted({g['scenario'] for g in summary['groups']})
    for _, columns in metrics:
        for metric, label in columns:
            if metric in ('p50_ms', 'p99_ms'):
                continue
            figure, axes = plt.subplots(2, 3, figsize=(13, 7), constrained_layout=True)
            for axis, scenario in zip(axes.flat, scenarios):
                groups = sorted((g for g in summary['groups'] if g['scenario'] == scenario), key=lambda g: g['concurrency'])
                for implementation, color in [('python', '#b45f06'), ('go', '#007f9c')]:
                    scale = 1024 * 1024 if metric == 'pss_mean_bytes' else 1
                    values = [g['implementations'][implementation][metric] for g in groups]
                    mid = [v['median'] / scale for v in values]
                    low = [(v['median'] - v['ci95_bootstrap_median'][0]) / scale for v in values]
                    high = [(v['ci95_bootstrap_median'][1] - v['median']) / scale for v in values]
                    axis.errorbar([g['concurrency'] for g in groups], mid, yerr=[low, high], marker='o', label=implementation, color=color)
                axis.set_title(scenario)
                axis.set_xticks([1, 4, 8, 16])
                axis.set_xlabel('Concurrent workers')
                axis.set_ylabel(label)
                axis.grid(alpha=.2)
                axis.legend()
            figure.suptitle('Placement: five repetitions per point; 95% bootstrap median intervals')
            figure.savefig(output / (metric + '.png'), dpi=160)
            plt.close(figure)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--summary', type=Path, required=True)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    render(json.loads(args.summary.read_text()), args.output)


if __name__ == '__main__':
    main()

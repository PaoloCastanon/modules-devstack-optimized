# B18 CPU and PSS

Protocol placement-compact-v2: 72 valid official windows, 3 independent pairs per cell.

Values are medians of independent run metrics; paired ratios show observed range.
A median of run percentiles is not a pooled request percentile. Limited coverage and repetitions do not establish statistical significance.

| Load | Concurrency | Metric | Python median | Go median | Go/Python paired median [observed range] |
|---|---:|---|---:|---:|---:|
| provider | 1 | CPU seconds / correct request | 0.00272067 | 0.000527375 | 0.1918 [0.1749, 0.1956] |
| provider | 1 | Mean PSS (MiB) | 183.106 | 14.8107 | 0.08098 [0.07988, 0.08106] |
| provider | 16 | CPU seconds / correct request | 0.00255344 | 0.000461467 | 0.1859 [0.1786, 0.1867] |
| provider | 16 | Mean PSS (MiB) | 183.074 | 16.3184 | 0.08919 [0.08885, 0.08966] |
| inventories | 1 | CPU seconds / correct request | 0.00429018 | 0.000652314 | 0.1541 [0.1462, 0.1561] |
| inventories | 1 | Mean PSS (MiB) | 183 | 15.3196 | 0.08375 [0.08212, 0.08398] |
| inventories | 16 | CPU seconds / correct request | 0.0039811 | 0.000575762 | 0.1446 [0.1353, 0.1452] |
| inventories | 16 | Mean PSS (MiB) | 184.222 | 16.4338 | 0.08921 [0.08755, 0.08953] |
| allocations-read | 1 | CPU seconds / correct request | 0.00282234 | 0.000505357 | 0.18 [0.1791, 0.1819] |
| allocations-read | 1 | Mean PSS (MiB) | 184.374 | 15.4391 | 0.08349 [0.08261, 0.08392] |
| allocations-read | 16 | CPU seconds / correct request | 0.00260005 | 0.000435633 | 0.1682 [0.167, 0.1721] |
| allocations-read | 16 | Mean PSS (MiB) | 184.821 | 16.329 | 0.08835 [0.08722, 0.08837] |
| candidates | 1 | CPU seconds / correct request | 0.020127 | 0.00164797 | 0.08188 [0.08119, 0.08257] |
| candidates | 1 | Mean PSS (MiB) | 193.18 | 15.1715 | 0.07854 [0.07749, 0.07867] |
| candidates | 16 | CPU seconds / correct request | 0.0194784 | 0.0021309 | 0.1094 [0.1073, 0.112] |
| candidates | 16 | Mean PSS (MiB) | 193.489 | 16.9016 | 0.08702 [0.08625, 0.08735] |
| claim-cycle | 1 | CPU seconds / correct request | 0.00861436 | 0.0011601 | 0.134 [0.1214, 0.1349] |
| claim-cycle | 1 | Mean PSS (MiB) | 182.966 | 15.6912 | 0.08592 [0.08545, 0.08632] |
| claim-cycle | 16 | CPU seconds / correct request | 0.0083063 | 0.000919018 | 0.1108 [0.1054, 0.1142] |
| claim-cycle | 16 | Mean PSS (MiB) | 183.536 | 16.8284 | 0.09174 [0.09131, 0.09181] |
| nova-mix | 1 | CPU seconds / correct request | 0.0107986 | 0.00113525 | 0.1047 [0.1014, 0.1067] |
| nova-mix | 1 | Mean PSS (MiB) | 191.194 | 15.4821 | 0.08069 [0.07993, 0.08099] |
| nova-mix | 16 | CPU seconds / correct request | 0.0102215 | 0.00126771 | 0.123 [0.1224, 0.1254] |
| nova-mix | 16 | Mean PSS (MiB) | 192.493 | 16.7383 | 0.08688 [0.08599, 0.08696] |

Limits: same VM and Apache entry, but deployed workers, authentication token caching, SQL pools and MySQL configuration differ.
This compares these deployments and does not isolate programming language. Go is not required to be faster.
CPU excludes database, Keystone, Apache and generator; their raw control activity is retained. PSS peak is sampled, not absolute.
Lower service CPU does not imply lower whole-stack CPU; inspect the separate controls, especially authentication work in Keystone.
Go heap/GC counters are not exposed by this binary. RSS can double count shared pages and is secondary.
Nova boot duration includes other services and is covered separately in B14/B15; it is not Placement HTTP latency.

Evidence: [aligned CPU/PSS and controls](../../evidence/sprint5/benchmarks/placement-compact-v2-20261031/summary.json), [restoration and VM](../../evidence/sprint5/benchmarks/placement-compact-v2-20261031/vm-final-state.json).

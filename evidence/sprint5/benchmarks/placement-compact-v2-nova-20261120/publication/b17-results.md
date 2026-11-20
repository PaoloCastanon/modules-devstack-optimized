# B17 latency and throughput

Protocol placement-compact-v2-nova: 36 valid official windows, 3 independent pairs per cell.

Values are medians of independent run metrics; paired ratios show observed range.
A median of run percentiles is not a pooled request percentile. Limited coverage and repetitions do not establish statistical significance.

| Load | Concurrency | Metric | Python median | Go median | Go/Python paired median [observed range] |
|---|---:|---|---:|---:|---:|
| candidates | 1 | Correct HTTP requests/s | 40.843 | 388.955 | 9.403 [8.6, 9.601] |
| candidates | 1 | p50 latency (ms) | 22.5078 | 1.78016 | 0.07984 [0.07909, 0.08948] |
| candidates | 1 | p95 latency (ms) | 29.4405 | 2.64357 | 0.09233 [0.08979, 0.1047] |
| candidates | 1 | p99 latency (ms) | 43.7246 | 3.60821 | 0.09054 [0.07814, 0.1044] |
| candidates | 16 | Correct HTTP requests/s | 68.3523 | 646.596 | 9.471 [9.348, 9.534] |
| candidates | 16 | p50 latency (ms) | 227.859 | 22.1635 | 0.09762 [0.09689, 0.1015] |
| candidates | 16 | p95 latency (ms) | 293.342 | 39.4524 | 0.1349 [0.1339, 0.1407] |
| candidates | 16 | p99 latency (ms) | 323.356 | 48.6963 | 0.1527 [0.1355, 0.1559] |
| claim-cycle | 1 | Correct HTTP requests/s | 38.5622 | 121.41 | 2.982 [2.93, 3.204] |
| claim-cycle | 1 | p50 latency (ms) | 25.3697 | 5.92106 | 0.2334 [0.2207, 0.2859] |
| claim-cycle | 1 | p95 latency (ms) | 55.0676 | 20.9424 | 0.387 [0.3663, 0.413] |
| claim-cycle | 1 | p99 latency (ms) | 81.1569 | 25.6397 | 0.3159 [0.2719, 0.371] |
| claim-cycle | 16 | Correct HTTP requests/s | 60.0219 | 848.835 | 14.14 [13.94, 15.19] |
| claim-cycle | 16 | p50 latency (ms) | 235.631 | 19.4772 | 0.08141 [0.0734, 0.08493] |
| claim-cycle | 16 | p95 latency (ms) | 508.122 | 40.2997 | 0.07884 [0.07872, 0.08527] |
| claim-cycle | 16 | p99 latency (ms) | 844.916 | 58.4335 | 0.06798 [0.0676, 0.09844] |
| nova-mix | 1 | Correct HTTP requests/s | 54.0119 | 293.762 | 5.439 [5.34, 5.582] |
| nova-mix | 1 | p50 latency (ms) | 14.8444 | 1.949 | 0.1313 [0.1261, 0.1319] |
| nova-mix | 1 | p95 latency (ms) | 38.6654 | 9.08614 | 0.2382 [0.2261, 0.2444] |
| nova-mix | 1 | p99 latency (ms) | 56.3764 | 19.8564 | 0.3355 [0.3075, 0.3896] |
| nova-mix | 16 | Correct HTTP requests/s | 98.5405 | 1069.89 | 10.86 [10.27, 11.26] |
| nova-mix | 16 | p50 latency (ms) | 170.006 | 11.6603 | 0.06859 [0.06857, 0.0742] |
| nova-mix | 16 | p95 latency (ms) | 290.642 | 34.2944 | 0.1192 [0.1069, 0.1214] |
| nova-mix | 16 | p99 latency (ms) | 392.236 | 44.6756 | 0.1194 [0.098, 0.1196] |

Limits: same VM and Apache entry, but deployed workers, authentication token caching, SQL pools and MySQL configuration differ.
This compares these deployments and does not isolate programming language. Go is not required to be faster.
CPU excludes database, Keystone, Apache and generator; their raw control activity is retained. PSS peak is sampled, not absolute.
Lower service CPU does not imply lower whole-stack CPU; inspect the separate controls, especially authentication work in Keystone.
Go heap/GC counters are not exposed by this binary. RSS can double count shared pages and is secondary.
Nova boot duration includes other services and is covered separately in B14/B15; it is not Placement HTTP latency.

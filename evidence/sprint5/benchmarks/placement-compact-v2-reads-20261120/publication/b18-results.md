# B18 CPU and PSS

Protocol placement-compact-v2-reads: 36 valid official windows, 3 independent pairs per cell.

Values are medians of independent run metrics; paired ratios show observed range.
A median of run percentiles is not a pooled request percentile. Limited coverage and repetitions do not establish statistical significance.

| Load | Concurrency | Metric | Python median | Go median | Go/Python paired median [observed range] |
|---|---:|---|---:|---:|---:|
| provider | 1 | CPU seconds / correct request | 0.00258455 | 0.000159062 | 0.06156 [0.06054, 0.06176] |
| provider | 1 | Mean PSS (MiB) | 183.656 | 15.0634 | 0.08172 [0.08151, 0.08245] |
| provider | 16 | CPU seconds / correct request | 0.00249762 | 0.000164181 | 0.06526 [0.06513, 0.06616] |
| provider | 16 | Mean PSS (MiB) | 182.951 | 16.1771 | 0.08843 [0.08835, 0.08863] |
| inventories | 1 | CPU seconds / correct request | 0.00406132 | 0.000254259 | 0.06287 [0.06194, 0.06338] |
| inventories | 1 | Mean PSS (MiB) | 182.424 | 15.469 | 0.08472 [0.0844, 0.0848] |
| inventories | 16 | CPU seconds / correct request | 0.00392191 | 0.000240298 | 0.06046 [0.04948, 0.06554] |
| inventories | 16 | Mean PSS (MiB) | 184.361 | 16.4069 | 0.08915 [0.08745, 0.08942] |
| allocations-read | 1 | CPU seconds / correct request | 0.00372546 | 0.00022301 | 0.05986 [0.05408, 0.06658] |
| allocations-read | 1 | Mean PSS (MiB) | 184.126 | 15.6723 | 0.08521 [0.08466, 0.08555] |
| allocations-read | 16 | CPU seconds / correct request | 0.00335376 | 0.000206401 | 0.06208 [0.05995, 0.06243] |
| allocations-read | 16 | Mean PSS (MiB) | 184.774 | 16.2044 | 0.08765 [0.08662, 0.08781] |

Limits: same VM and Apache entry, but deployed workers, authentication token caching, SQL pools and MySQL configuration differ.
This compares these deployments and does not isolate programming language. Go is not required to be faster.
CPU excludes database, Keystone, Apache and generator; their raw control activity is retained. PSS peak is sampled, not absolute.
Lower service CPU does not imply lower whole-stack CPU; inspect the separate controls, especially authentication work in Keystone.
Go heap/GC counters are not exposed by this binary. RSS can double count shared pages and is secondary.
Nova boot duration includes other services and is covered separately in B14/B15; it is not Placement HTTP latency.

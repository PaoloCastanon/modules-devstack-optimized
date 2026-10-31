# B17 latency and throughput

Protocol placement-compact-v2: 72 valid official windows, 3 independent pairs per cell.

Values are medians of independent run metrics; paired ratios show observed range.
A median of run percentiles is not a pooled request percentile. Limited coverage and repetitions do not establish statistical significance.

| Load | Concurrency | Metric | Python median | Go median | Go/Python paired median [observed range] |
|---|---:|---|---:|---:|---:|
| provider | 1 | Correct HTTP requests/s | 279.66 | 171.255 | 0.6124 [0.6052, 0.6977] |
| provider | 1 | p50 latency (ms) | 3.25971 | 5.52149 | 1.683 [1.437, 1.704] |
| provider | 1 | p95 latency (ms) | 4.84781 | 7.94529 | 1.639 [1.471, 1.642] |
| provider | 1 | p99 latency (ms) | 5.64722 | 9.18866 | 1.627 [1.465, 1.63] |
| provider | 16 | Correct HTTP requests/s | 698.985 | 448.816 | 0.648 [0.6194, 0.6509] |
| provider | 16 | p50 latency (ms) | 22.0228 | 34.249 | 1.584 [1.541, 1.604] |
| provider | 16 | p95 latency (ms) | 26.4499 | 41.0828 | 1.541 [1.478, 1.629] |
| provider | 16 | p99 latency (ms) | 36.1918 | 45.4132 | 1.231 [1.174, 1.389] |
| inventories | 1 | Correct HTTP requests/s | 176.627 | 160.414 | 0.9048 [0.9013, 0.9462] |
| inventories | 1 | p50 latency (ms) | 5.29499 | 5.89791 | 1.114 [1.079, 1.119] |
| inventories | 1 | p95 latency (ms) | 7.54136 | 8.36712 | 1.093 [1.041, 1.12] |
| inventories | 1 | p99 latency (ms) | 8.77927 | 9.60152 | 1.089 [1.025, 1.094] |
| inventories | 16 | Correct HTTP requests/s | 430.431 | 446.074 | 1.036 [1.032, 1.09] |
| inventories | 16 | p50 latency (ms) | 36.4575 | 34.7116 | 0.9528 [0.9127, 0.9615] |
| inventories | 16 | p95 latency (ms) | 42.1948 | 41.3068 | 0.9851 [0.9332, 1.003] |
| inventories | 16 | p99 latency (ms) | 52.4209 | 45.1189 | 0.8607 [0.796, 0.9421] |
| allocations-read | 1 | Correct HTTP requests/s | 272.065 | 170.237 | 0.6276 [0.6208, 0.6305] |
| allocations-read | 1 | p50 latency (ms) | 3.42006 | 5.55662 | 1.622 [1.617, 1.644] |
| allocations-read | 1 | p95 latency (ms) | 4.98107 | 7.88395 | 1.579 [1.57, 1.61] |
| allocations-read | 1 | p99 latency (ms) | 5.77799 | 9.06104 | 1.568 [1.53, 1.571] |
| allocations-read | 16 | Correct HTTP requests/s | 680.814 | 457.055 | 0.6729 [0.6709, 0.6754] |
| allocations-read | 16 | p50 latency (ms) | 22.8164 | 34.1643 | 1.486 [1.473, 1.5] |
| allocations-read | 16 | p95 latency (ms) | 27.3187 | 40.5467 | 1.47 [1.462, 1.494] |
| allocations-read | 16 | p99 latency (ms) | 36.2993 | 45.5401 | 1.283 [1.251, 1.327] |
| candidates | 1 | Correct HTTP requests/s | 40.3316 | 120.198 | 2.98 [2.941, 3.001] |
| candidates | 1 | p50 latency (ms) | 22.8232 | 7.23508 | 0.3164 [0.3118, 0.3219] |
| candidates | 1 | p95 latency (ms) | 30.5113 | 10.0565 | 0.3296 [0.3276, 0.3319] |
| candidates | 1 | p99 latency (ms) | 34.6801 | 11.0918 | 0.3204 [0.3067, 0.3224] |
| candidates | 16 | Correct HTTP requests/s | 89.1871 | 403.554 | 4.525 [4.507, 4.639] |
| candidates | 16 | p50 latency (ms) | 176.1 | 37.647 | 0.2138 [0.2101, 0.2156] |
| candidates | 16 | p95 latency (ms) | 199.233 | 49.0088 | 0.2462 [0.238, 0.2476] |
| candidates | 16 | p99 latency (ms) | 214.53 | 58.8203 | 0.2736 [0.254, 0.2742] |
| claim-cycle | 1 | Correct HTTP requests/s | 55.3832 | 97.0566 | 1.726 [1.654, 2.067] |
| claim-cycle | 1 | p50 latency (ms) | 14.5028 | 10.134 | 0.6918 [0.5303, 0.701] |
| claim-cycle | 1 | p95 latency (ms) | 37.002 | 15.9245 | 0.4535 [0.3415, 0.4732] |
| claim-cycle | 1 | p99 latency (ms) | 68.1532 | 24.6891 | 0.3729 [0.3623, 0.3828] |
| claim-cycle | 16 | Correct HTTP requests/s | 71.5701 | 320.498 | 4.478 [4.317, 4.713] |
| claim-cycle | 16 | p50 latency (ms) | 194.184 | 43.2672 | 0.2221 [0.2188, 0.237] |
| claim-cycle | 16 | p95 latency (ms) | 430.252 | 98.6128 | 0.2292 [0.2208, 0.2344] |
| claim-cycle | 16 | p99 latency (ms) | 573.182 | 157.236 | 0.2775 [0.2743, 0.2857] |
| nova-mix | 1 | Correct HTTP requests/s | 66.5216 | 107.528 | 1.609 [1.602, 1.686] |
| nova-mix | 1 | p50 latency (ms) | 11.835 | 7.12802 | 0.5896 [0.5849, 0.6153] |
| nova-mix | 1 | p95 latency (ms) | 32.1368 | 25.8252 | 0.773 [0.7158, 0.8036] |
| nova-mix | 1 | p99 latency (ms) | 48.9964 | 34.0213 | 0.6676 [0.6466, 0.7707] |
| nova-mix | 16 | Correct HTTP requests/s | 143.415 | 401.259 | 2.798 [2.728, 2.835] |
| nova-mix | 16 | p50 latency (ms) | 105.28 | 38.0793 | 0.3617 [0.3584, 0.3641] |
| nova-mix | 16 | p95 latency (ms) | 201.017 | 50.4364 | 0.2509 [0.2373, 0.2652] |
| nova-mix | 16 | p99 latency (ms) | 279.175 | 61.6177 | 0.2207 [0.194, 0.2807] |

Limits: same VM and Apache entry, but deployed workers, authentication token caching, SQL pools and MySQL configuration differ.
This compares these deployments and does not isolate programming language. Go is not required to be faster.
CPU excludes database, Keystone, Apache and generator; their raw control activity is retained. PSS peak is sampled, not absolute.
Lower service CPU does not imply lower whole-stack CPU; inspect the separate controls, especially authentication work in Keystone.
Go heap/GC counters are not exposed by this binary. RSS can double count shared pages and is secondary.
Nova boot duration includes other services and is covered separately in B14/B15; it is not Placement HTTP latency.

# B17 latency and throughput

Protocol placement-compact-v2-reads: 36 valid official windows, 3 independent pairs per cell.

Values are medians of independent run metrics; paired ratios show observed range.
A median of run percentiles is not a pooled request percentile. Limited coverage and repetitions do not establish statistical significance.

| Load | Concurrency | Metric | Python median | Go median | Go/Python paired median [observed range] |
|---|---:|---|---:|---:|---:|
| provider | 1 | Correct HTTP requests/s | 301.614 | 2063.65 | 6.829 [6.749, 6.842] |
| provider | 1 | p50 latency (ms) | 3.09881 | 0.429497 | 0.1393 [0.1386, 0.1407] |
| provider | 1 | p95 latency (ms) | 4.15329 | 0.577685 | 0.1383 [0.1379, 0.1403] |
| provider | 1 | p99 latency (ms) | 4.9684 | 0.729362 | 0.1448 [0.1419, 0.1477] |
| provider | 16 | Correct HTTP requests/s | 720.843 | 3516.03 | 4.878 [4.872, 4.881] |
| provider | 16 | p50 latency (ms) | 21.3624 | 4.16122 | 0.1949 [0.1929, 0.1969] |
| provider | 16 | p95 latency (ms) | 26.8612 | 7.11029 | 0.2647 [0.2617, 0.2763] |
| provider | 16 | p99 latency (ms) | 36.6277 | 8.59954 | 0.2337 [0.2305, 0.249] |
| inventories | 1 | Correct HTTP requests/s | 191.042 | 1403.09 | 7.276 [7.237, 7.472] |
| inventories | 1 | p50 latency (ms) | 4.99485 | 0.640882 | 0.1305 [0.1278, 0.1306] |
| inventories | 1 | p95 latency (ms) | 6.04972 | 0.850113 | 0.1394 [0.1304, 0.141] |
| inventories | 1 | p99 latency (ms) | 7.71443 | 1.10054 | 0.1355 [0.1338, 0.1477] |
| inventories | 16 | Correct HTTP requests/s | 439.736 | 3284.99 | 7.47 [7.136, 9.517] |
| inventories | 16 | p50 latency (ms) | 35.2521 | 4.53976 | 0.1288 [0.0945, 0.1329] |
| inventories | 16 | p95 latency (ms) | 43.0951 | 7.64707 | 0.1774 [0.1184, 0.1885] |
| inventories | 16 | p99 latency (ms) | 52.6036 | 9.20689 | 0.175 [0.1215, 0.1911] |
| allocations-read | 1 | Correct HTTP requests/s | 200.625 | 1338.83 | 6.673 [6.158, 7.021] |
| allocations-read | 1 | p50 latency (ms) | 4.57519 | 0.621624 | 0.1387 [0.1359, 0.1435] |
| allocations-read | 1 | p95 latency (ms) | 6.76773 | 1.26798 | 0.1874 [0.1325, 0.2124] |
| allocations-read | 1 | p99 latency (ms) | 7.97026 | 1.77614 | 0.2228 [0.1385, 0.2349] |
| allocations-read | 16 | Correct HTTP requests/s | 518.767 | 3289.36 | 6.261 [6.233, 6.454] |
| allocations-read | 16 | p50 latency (ms) | 29.2485 | 4.56759 | 0.1557 [0.1539, 0.1578] |
| allocations-read | 16 | p95 latency (ms) | 40.0272 | 7.74741 | 0.1974 [0.1916, 0.2014] |
| allocations-read | 16 | p99 latency (ms) | 48.4961 | 9.45069 | 0.1931 [0.177, 0.2033] |

Limits: same VM and Apache entry, but deployed workers, authentication token caching, SQL pools and MySQL configuration differ.
This compares these deployments and does not isolate programming language. Go is not required to be faster.
CPU excludes database, Keystone, Apache and generator; their raw control activity is retained. PSS peak is sampled, not absolute.
Lower service CPU does not imply lower whole-stack CPU; inspect the separate controls, especially authentication work in Keystone.
Go heap/GC counters are not exposed by this binary. RSS can double count shared pages and is secondary.
Nova boot duration includes other services and is covered separately in B14/B15; it is not Placement HTTP latency.

# B18 CPU and PSS

Protocol placement-compact-v2-nova: 36 valid official windows, 3 independent pairs per cell.

Values are medians of independent run metrics; paired ratios show observed range.
A median of run percentiles is not a pooled request percentile. Limited coverage and repetitions do not establish statistical significance.

| Load | Concurrency | Metric | Python median | Go median | Go/Python paired median [observed range] |
|---|---:|---|---:|---:|---:|
| candidates | 1 | CPU seconds / correct request | 0.0198918 | 0.0012511 | 0.0633 [0.06178, 0.06637] |
| candidates | 1 | Mean PSS (MiB) | 193.577 | 15.1473 | 0.07767 [0.07735, 0.07888] |
| candidates | 16 | CPU seconds / correct request | 0.0246493 | 0.00206377 | 0.08341 [0.08185, 0.08379] |
| candidates | 16 | Mean PSS (MiB) | 193.195 | 17.1446 | 0.08875 [0.08869, 0.08956] |
| claim-cycle | 1 | CPU seconds / correct request | 0.0108826 | 0.0011509 | 0.1058 [0.1041, 0.1105] |
| claim-cycle | 1 | Mean PSS (MiB) | 181.332 | 16.0756 | 0.08865 [0.08484, 0.08875] |
| claim-cycle | 16 | CPU seconds / correct request | 0.0108333 | 0.000648682 | 0.0603 [0.05932, 0.06174] |
| claim-cycle | 16 | Mean PSS (MiB) | 182.808 | 16.8903 | 0.09243 [0.08978, 0.09257] |
| nova-mix | 1 | CPU seconds / correct request | 0.0133142 | 0.00105337 | 0.07912 [0.07872, 0.08232] |
| nova-mix | 1 | Mean PSS (MiB) | 191.114 | 15.6869 | 0.08227 [0.08197, 0.08228] |
| nova-mix | 16 | CPU seconds / correct request | 0.0131875 | 0.00111675 | 0.08249 [0.08247, 0.08472] |
| nova-mix | 16 | Mean PSS (MiB) | 192.375 | 17.2051 | 0.08946 [0.08804, 0.08977] |

Limits: same VM and Apache entry, but deployed workers, authentication token caching, SQL pools and MySQL configuration differ.
This compares these deployments and does not isolate programming language. Go is not required to be faster.
CPU excludes database, Keystone, Apache and generator; their raw control activity is retained. PSS peak is sampled, not absolute.
Lower service CPU does not imply lower whole-stack CPU; inspect the separate controls, especially authentication work in Keystone.
Go heap/GC counters are not exposed by this binary. RSS can double count shared pages and is secondary.
Nova boot duration includes other services and is covered separately in B14/B15; it is not Placement HTTP latency.

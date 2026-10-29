# Casos observados

| Caso | Python | Go | Estado antes/después | Comparación estricta |
|---|---:|---:|---|---|
| create | 201 | 201 | igual | igual |
| show-service | 200 | 200 | igual | igual |
| list-uuid | 200 | 200 | igual | igual |
| list-name | 200 | 200 | igual | igual |
| rename | 200 | 200 | igual | igual |
| inventories-empty | 200 | 200 | igual | igual |
| inventories-replace | 200 | 200 | igual | igual |
| inventory-show | 200 | 200 | igual | igual |
| inventories-clear | 200 | 200 | igual | igual |
| delete | 204 | 204 | igual | igual |
| duplicate-uuid | 409 | 409 | igual | igual |
| duplicate-name | 409 | 409 | igual | igual |
| stale-generation | 409 | 409 | igual | igual |
| inventory-missing | 404 | 404 | igual | igual |
| provider-missing | 404 | 404 | igual | igual |
| missing-name | 400 | 400 | igual | difiere |
| null-name | 400 | 400 | igual | difiere |
| unknown-field | 400 | 400 | igual | difiere |
| rename-generation-field | 400 | 400 | igual | difiere |
| reserved-total | 400 | 400 | igual | difiere |
| zero-total | 400 | 400 | igual | difiere |
| null-total | 400 | 400 | igual | difiere |
| unknown-class | 400 | 400 | igual | igual |
| mixed-invalid | 400 | 400 | igual | difiere |
| integer-as-decimal | 200 | 200 | igual | igual |
| ratio-precision | 200 | 200 | igual | igual |
| name-case-filter | 200 | 200 | igual | igual |
| name-trailing-space | 200 | 200 | igual | igual |
| duplicate-query | 200 | 200 | igual | igual |
| uuid-case-lookup | 200 | 200 | igual | igual |
| uuid-compact-query | 400 | 400 | igual | difiere |
| duplicate-name-case | 409 | 409 | igual | igual |
| duplicate-name-space | 409 | 409 | igual | igual |
| reader-forbidden | 403 | 403 | igual | igual |
| missing-token | 401 | 401 | igual | igual |
| invalid-token | 401 | 401 | igual | igual |
| unsupported-media | 415 | 415 | igual | difiere |
| unacceptable | 406 | 406 | igual | difiere |

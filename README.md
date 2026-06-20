# bbolt debugging playground

A collection of small examples for exploring and debugging [bbolt](https://github.com/etcd-io/bbolt) internals — B+tree page structure, MVCC, freelist rebalancing, and large transactions.

## Prerequisites

- Go 1.21+
- [Delve](https://github.com/go-delve/delve) debugger:
  ```
  go install github.com/go-delve/delve/cmd/dlv@latest
  ```

## Examples

Examples live in `toy/` and are toggled by uncommenting the relevant call in `toy/main.go`:

| Function | File | What it shows |
|---|---|---|
| `basicExample()` | `basic.go` | Basic bucket and key/value operations |
| `mvccExample()` | `mvcc.go` | MVCC — concurrent read and write transactions |
| `branchPagesExample()` | `branch_pages.go` | B+tree growth from leaf → branch pages across multiple depths |
| `largeTxExample()` | `large_tx.go` | Large transaction behaviour (single vs batched writes) |
| `rebalanceExample()` | `rebalance.go` | Freelist rebalancing after deletions |
| `inlineBucketExample()` | `inline_bucket.go` | Inline vs regular buckets — when a child bucket lives inside the parent's leaf page vs owning its own page |
| `pageElementsExample()` | `page_elements.go` | Raw on-disk layout of branch and leaf pages read directly with `encoding/binary` |

## Debugging

Start the headless Delve server from the repo root:

```
dlv debug ./toy --headless --listen=:2345 --api-version=2 --accept-multiclient
```

Then connect with any Delve-compatible client (VS Code, GoLand, `dlv connect :2345`, etc.).

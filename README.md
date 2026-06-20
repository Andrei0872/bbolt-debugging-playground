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
| `leafRootExample()` | `leaf_root.go` | Leaf root bucket — has its own page but no branch pages yet; shows both ways to leave inline mode and the transition to a branch root |
| `deleteKVExample()` | `delete_kv.go` | `bucket.Delete()` — existing key, missing key (no-op), and deleting a bucket key (ErrIncompatibleValue) |
| `deleteBucketExample()` | `delete_bucket.go` | `bucket.DeleteBucket()` — inline bucket, regular bucket, nested recursive delete, ErrBucketNotFound, ErrIncompatibleValue |
| `deleteRebalanceNoMergeExample()` | `delete_rebalance_no_merge.go` | `node.rebalance()` early return — node stays above threshold, no structural change |
| `deleteRebalanceCollapseExample()` | `delete_rebalance_collapse.go` | `node.rebalance()` root branch collapse — branch with one child pulls that child up as new root, depth drops |
| `deleteRebalanceMergeExample()` | `delete_rebalance_merge.go` | `node.rebalance()` merge paths — empty node removed from parent; merge with right sibling; merge with left sibling |
| `deleteRebalanceBranchMergeExample()` | `delete_rebalance_branch_merge.go` | `node.rebalance()` branch-level merge — depth-3 tree where two intermediate branch nodes merge and materialized children are reparented |

## Debugging

Start the headless Delve server from the repo root:

```
dlv debug ./toy --headless --listen=:2345 --api-version=2 --accept-multiclient
```

Then connect with any Delve-compatible client (VS Code, GoLand, `dlv connect :2345`, etc.).

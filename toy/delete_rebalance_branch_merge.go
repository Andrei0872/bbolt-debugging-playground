package main

import (
	"fmt"
	"log"
	"os"

	bolt "go.etcd.io/bbolt"
)

// deleteRebalanceReparentExample triggers the reparenting block in
// node.rebalance() (node.go, lines 431-437):
//
//	for _, inode := range rightNode.inodes {
//	    if child, ok := n.bucket.nodes[inode.Pgid()]; ok {
//	        child.parent.removeChild(child)
//	        child.parent = leftNode
//	        child.parent.children = append(child.parent.children, child)
//	    }
//	}
//
// This block only fires during a BRANCH node merge. Leaf inodes carry key/value
// data and have pgid=0, so n.bucket.nodes[0] is almost never set. Branch inodes
// carry child page pgids — if those pages were materialized into nodes during the
// same transaction (present in bucket.nodes), their parent pointer must be updated
// from the right branch (being discarded) to the left branch (surviving).
// Without this, spill() would follow a stale parent pointer into a freed node.
//
// Depth >= 3 is required so that intermediate branch nodes exist and can merge.
//
// Tree structure with 3500 entries (pageSize=4096, key=12B, value=64B):
//
//	entry size: leafElementSize(16) + key(12) + value(64) = 92 B
//	leaf splits at 44 entries, first page gets 22  → ~159 leaf pages
//	branch element: branchElementSize(16) + key(12) = 28 B
//	branch splits at ~146 children, first gets 72  → 2 intermediate branches
//
//	root branch
//	├── branch A  (leaves  0–71,  keys     0–1583)   72 children
//	└── branch B  (leaves 72–158, keys 1584–3499)    87 children
//
// In one write transaction:
//
//	Step 1 — delete all entries from leaves 0–36 (keys 0–813, 814 deletions).
//	  Each Delete cursor seek materializes branch A and the target leaf into
//	  bucket.nodes. The leaves become empty → removed from branch A during
//	  rebalance. Branch A: 72−37 = 35 children, size = 16+35×28 = 996 B ≤ 1024 B
//	  threshold → undersized → merge with branch B.
//
//	Step 2 — delete one key from branch B's first leaf (leaf 72, "key-00001584").
//	  Cursor materializes branch B and leaf 72 → leaf 72 enters bucket.nodes.
//
//	At commit:
//	  Branch A (undersized) merges with branch B (next sibling).
//	  rightNode = branch B. Iterating branch B's 87 inodes, leaf 72's pgid is found
//	  in bucket.nodes → reparenting fires: leaf 72's parent changes from branch B
//	  to the merged node.
func deleteRebalanceBranchMergeExample() {
	const dbPath = "delete_rebalance_branch_merge.db"
	os.Remove(dbPath)

	db, err := bolt.Open(dbPath, 0600, &bolt.Options{PageSize: 4096})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	bkt := []byte("reparent")
	value := make([]byte, 64)

	// Insert 3500 entries in one tx — single spill builds the full depth-3 tree.
	if err := db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket(bkt)
		if err != nil {
			return err
		}
		for i := 0; i < 3500; i++ {
			if err := b.Put([]byte(fmt.Sprintf("key-%08d", i)), value); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	printStats := func(label string) {
		_ = db.View(func(tx *bolt.Tx) error {
			s := tx.Bucket(bkt).Stats()
			fmt.Printf("  %-48s depth=%-2d leaf=%-2d branch=%-2d keys=%d\n",
				label, s.Depth, s.LeafPageN, s.BranchPageN, s.KeyN)
			return nil
		})
	}

	fmt.Println("=== deleteRebalanceReparentExample ===")
	printStats("after insert (expect depth=3, branch=3)")

	if err := db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bkt)

		// Step 1: empty leaves 0–36 by deleting all their entries (keys 0–813).
		// Cursor for each Delete traverses root → branch A → leaf K, materializing
		// branch A and leaf K into bucket.nodes on first visit.
		// After commit: 37 empty leaves removed from branch A → branch A undersized.
		for i := 0; i < 814; i++ {
			if err := b.Delete([]byte(fmt.Sprintf("key-%08d", i))); err != nil {
				return err
			}
		}

		// Step 2: delete one key from branch B's first leaf (leaf 72).
		// Cursor traverses root → branch B → leaf 72, adding both to bucket.nodes.
		// When branch A later merges with branch B, rightNode.inodes is iterated
		// and leaf 72's pgid is found in bucket.nodes → reparenting fires.
		return b.Delete([]byte("key-00001584"))
	}); err != nil {
		log.Fatal(err)
	}

	printStats("after delete")
	fmt.Println("  → branch A merged with branch B; leaf 72 reparented during merge")
}

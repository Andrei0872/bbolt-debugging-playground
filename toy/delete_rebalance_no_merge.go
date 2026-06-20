package main

import (
	"fmt"
	"log"
	"os"

	bolt "go.etcd.io/bbolt"
)

// deleteRebalanceNoMergeExample shows the early-return path in node.rebalance()
// (node.go, lines 376-378):
//
//	if n.size() > threshold && len(n.inodes) > n.minKeys() {
//	    return
//	}
//
// With pageSize=4096 and fillPercent=0.5:
//
//	threshold = pageSize * fillPercent / 2 = 4096 * 0.5 / 2 = 1024 B
//
// Entry size: leafElementSize(16) + key(12) + value(64) = 92 B.
// A leaf node holding ≥ 11 entries has size 16 + 11*92 = 1028 B > 1024 B,
// so deleting one key from such a leaf leaves it above the threshold —
// rebalance exits immediately and the tree structure is unchanged.
//
// Setup: 50 entries → 2 leaf pages.
//
//	Leaf A: keys 0–21 (22 entries, size = 16 + 22*92 = 2040 B)
//	Leaf B: keys 22–49 (28 entries)
//	Root: branch page with 2 children
//
// Delete one key from leaf A → 21 entries remain (size = 1948 B > 1024 B).
// Rebalance is called but returns at the threshold check.
func deleteRebalanceNoMergeExample() {
	const dbPath = "delete_rebalance_no_merge.db"
	os.Remove(dbPath)

	db, err := bolt.Open(dbPath, 0600, &bolt.Options{PageSize: 4096})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	bkt := []byte("no-merge")
	value := make([]byte, 64)

	if err := db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket(bkt)
		if err != nil {
			return err
		}
		for i := 0; i < 50; i++ {
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
			fmt.Printf("  %-30s depth=%-2d leaf=%-2d branch=%-2d keys=%d\n",
				label, s.Depth, s.LeafPageN, s.BranchPageN, s.KeyN)
			return nil
		})
	}

	fmt.Println("=== deleteRebalanceNoMergeExample ===")
	printStats("before delete")

	// Delete one key from leaf A. The leaf retains 21 entries (size=1948 B > threshold=1024 B)
	// so rebalance returns early — no merge, no structural change.
	if err := db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bkt).Delete([]byte("key-00000000"))
	}); err != nil {
		log.Fatal(err)
	}

	printStats("after delete")
	fmt.Println("  → tree depth and page counts unchanged: node stayed above threshold")
}

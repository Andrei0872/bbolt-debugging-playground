package main

import (
	"fmt"
	"log"
	"os"

	bolt "go.etcd.io/bbolt"
)

// deleteRebalanceCollapseExample triggers the root-branch collapse path in
// node.rebalance() (node.go, lines 382-403):
//
//	if n.parent == nil {
//	    if !n.isLeaf && len(n.inodes) == 1 {
//	        child := n.bucket.node(n.inodes[0].Pgid(), n)
//	        n.isLeaf = child.isLeaf
//	        n.inodes = child.inodes[:]
//	        ...
//	    }
//	    return
//	}
//
// Setup: 50 entries → 2 leaf pages under a branch root (depth 2).
//
//	Leaf A: keys 0–21  (22 entries, size = 16 + 22*92 = 2040 B)
//	Leaf B: keys 22–49 (28 entries)
//	Root:   branch page with 2 children
//
// Deleting all 28 keys from leaf B empties it.  During rebalance:
//  1. Leaf B: numChildren()==0 → removed from parent (lines 407-414), parent.rebalance() called.
//  2. Root branch: len(inodes)==1 → child (leaf A) is pulled up to replace root.
//
// Result: depth drops from 2 to 1, branch page disappears.
func deleteRebalanceCollapseExample() {
	const dbPath = "delete_rebalance_collapse.db"
	os.Remove(dbPath)

	db, err := bolt.Open(dbPath, 0600, &bolt.Options{PageSize: 4096})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	bkt := []byte("collapse")
	value := make([]byte, 64)

	// 50 entries → split into 2 leaves at commit.
	// Leaf A: keys 0–21, Leaf B: keys 22–49.
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
			fmt.Printf("  %-35s depth=%-2d leaf=%-2d branch=%-2d keys=%d\n",
				label, s.Depth, s.LeafPageN, s.BranchPageN, s.KeyN)
			return nil
		})
	}

	fmt.Println("=== deleteRebalanceCollapseExample ===")
	printStats("before delete")

	// Delete all keys on leaf B (keys 22-49). Leaf B becomes empty →
	// removed from parent → root branch left with 1 child → collapses.
	if err := db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bkt)
		for i := 22; i < 50; i++ {
			if err := b.Delete([]byte(fmt.Sprintf("key-%08d", i))); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	printStats("after delete")
	fmt.Println("  → branch page gone: root branch with 1 child was replaced by that leaf")
}

package main

import (
	"fmt"
	"log"
	"os"

	bolt "go.etcd.io/bbolt"
)

// deleteRebalanceMergeExample covers the three remaining paths in node.rebalance()
// that involve a non-root node with a live parent (node.go, lines 406-447).
//
// Setup for all three sub-cases: 70 entries → 3 leaf pages (depth 2).
//
//	Leaf A: keys  0–21  (22 entries, size = 16 + 22*92 = 2040 B)
//	Leaf B: keys 22–43  (22 entries)
//	Leaf C: keys 44–69  (26 entries)
//	Root: branch page with 3 children
//
// Rebalance threshold = pageSize * fillPercent / 2 = 4096 * 0.5 / 2 = 1024 B.
// A leaf with 10 entries has size 16 + 10*92 = 936 B ≤ 1024 B → undersized.
//
// Sub-case 1 — empty node removed (lines 407-414):
//
//	Delete all 22 keys from leaf A → numChildren()==0.
//	→ parent.del(A.key), parent.removeChild(A), A freed, parent.rebalance().
//
// Sub-case 2 — merge with right sibling (lines 418-447, useNextSibling=true):
//
//	Leaf A is the leftmost child (childIndex==0).
//	Delete 12 keys (0–11) → 10 remain → undersized → merges with leaf B.
//
// Sub-case 3 — merge with left sibling (lines 418-447, useNextSibling=false):
//
//	Leaf B is not the leftmost child (childIndex==1).
//	Delete 12 keys (22–33) → 10 remain → undersized → merges with leaf A.
func deleteRebalanceMergeExample() {
	fmt.Println("=== deleteRebalanceMergeExample ===")
	mergeEmptyNode()
	mergWithRightSibling()
	mergeWithLeftSibling()
}

func setup70(dbPath string) *bolt.DB {
	os.Remove(dbPath)
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{PageSize: 4096})
	if err != nil {
		log.Fatal(err)
	}
	bkt := []byte("merge")
	value := make([]byte, 64)
	if err := db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket(bkt)
		if err != nil {
			return err
		}
		for i := 0; i < 70; i++ {
			if err := b.Put([]byte(fmt.Sprintf("key-%08d", i)), value); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		log.Fatal(err)
	}
	return db
}

func printMergeStats(db *bolt.DB, label string) {
	_ = db.View(func(tx *bolt.Tx) error {
		s := tx.Bucket([]byte("merge")).Stats()
		fmt.Printf("    %-38s depth=%-2d leaf=%-2d branch=%-2d keys=%d\n",
			label, s.Depth, s.LeafPageN, s.BranchPageN, s.KeyN)
		return nil
	})
}

// mergeEmptyNode: leaf A gets all its keys deleted → numChildren()==0 →
// removed from parent via parent.del + removeChild + free.
func mergeEmptyNode() {
	db := setup70("delete_rebalance_merge_empty.db")
	defer db.Close()

	fmt.Println("  sub-case 1: empty node removed")
	printMergeStats(db, "before")

	if err := db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("merge"))
		for i := 0; i < 22; i++ { // wipe all of leaf A (keys 0–21)
			if err := b.Delete([]byte(fmt.Sprintf("key-%08d", i))); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	printMergeStats(db, "after (leaf A emptied)")
	fmt.Println("    → leaf A removed from parent; parent rebalanced")
}

// mergWithRightSibling: leaf A (childIndex==0) becomes undersized →
// useNextSibling=true → A.inodes + B.inodes merged into A, B removed.
func mergWithRightSibling() {
	db := setup70("delete_rebalance_merge_right.db")
	defer db.Close()

	fmt.Println("  sub-case 2: merge with right sibling (childIndex==0)")
	printMergeStats(db, "before")

	if err := db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("merge"))
		// Delete 12 keys from leaf A (keys 0–11) → 10 remain → size=936 B ≤ 1024 B.
		for i := 0; i < 12; i++ {
			if err := b.Delete([]byte(fmt.Sprintf("key-%08d", i))); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	printMergeStats(db, "after (leaf A merged into leaf B)")
	fmt.Println("    → leaf A (leftmost, undersized) merged with its right sibling B")
}

// mergeWithLeftSibling: leaf B (childIndex==1) becomes undersized →
// useNextSibling=false → A.inodes + B.inodes merged into A, B removed.
func mergeWithLeftSibling() {
	db := setup70("delete_rebalance_merge_left.db")
	defer db.Close()

	fmt.Println("  sub-case 3: merge with left sibling (childIndex==1)")
	printMergeStats(db, "before")

	if err := db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("merge"))
		// Delete 12 keys from leaf B (keys 22–33) → 10 remain → size=936 B ≤ 1024 B.
		for i := 22; i < 34; i++ {
			if err := b.Delete([]byte(fmt.Sprintf("key-%08d", i))); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	printMergeStats(db, "after (leaf B merged into leaf A)")
	fmt.Println("    → leaf B (non-leftmost, undersized) merged with its left sibling A")
}

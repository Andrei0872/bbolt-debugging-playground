package main

import (
	"fmt"
	"log"
	"os"

	bolt "go.etcd.io/bbolt"
)

// rebalanceExample demonstrates case 2: deletions that cause leaf pages to
// merge via node.rebalance().
//
// Every b.Delete() call marks the affected node as "unbalanced". Before spill,
// tx.Commit() calls rebalance() on each unbalanced node (tx.go, just before
// spill). rebalance() checks:
//
//	threshold = pageSize * fillPercent / 2 = 4096 * 0.5 / 2 = 1024 bytes
//
//	if n.size() > threshold && len(n.inodes) > n.minKeys() {
//	    return   // healthy, do nothing
//	}
//
// A leaf is healthy when it has more than 1024 bytes of data. With
// key=12B and value=64B (92 bytes per entry), that means ≥ 11 entries
// (16 + 11×92 = 1028 bytes). Drop to ≤ 10 entries and the node underflows.
//
// On underflow, rebalance() merges the node with a sibling:
//   - leftmost child (index 0) → merge with its right sibling
//   - any other child         → merge with its left sibling
//
// After the merge, the parent loses one child pointer and rebalance() is
// called recursively on the parent. If the merged node now exceeds pageSize,
// spill() will re-split it; otherwise it stays as one smaller page.
func rebalanceExample() {
	const dbPath = "rebalance.db"
	os.Remove(dbPath)

	db, err := bolt.Open(dbPath, 0600, &bolt.Options{PageSize: 4096})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	bucket := []byte("rebalance-demo")
	value := make([]byte, 64)

	printStats := func(label string) {
		if err := db.View(func(tx *bolt.Tx) error {
			s := tx.Bucket(bucket).Stats()
			fmt.Printf("\n  --- %s ---\n", label)
			fmt.Printf("  leaf pages   : %d\n", s.LeafPageN)
			fmt.Printf("  branch pages : %d\n", s.BranchPageN)
			fmt.Printf("  tree depth   : %d\n", s.Depth)
			return nil
		}); err != nil {
			log.Fatal(err)
		}
	}

	// Insert 200 entries in 20-entry batches.
	// Insertions are in sorted key order, so only the rightmost leaf
	// accumulates new entries each batch; earlier leaf pages are never touched
	// again after they split off. After 10 batches the tree has 9 leaf pages.
	// The leftmost page holds key-00000000..key-00000021 (22 entries).
	for start := range 10 {
		lo, hi := start*20, start*20+20
		if err = db.Update(func(tx *bolt.Tx) error {
			b, err := tx.CreateBucketIfNotExists(bucket)
			if err != nil {
				return err
			}
			for i := lo; i < hi; i++ {
				if err := b.Put([]byte(fmt.Sprintf("key-%08d", i)), value); err != nil {
					return err
				}
			}
			return nil
		}); err != nil {
			log.Fatal(err)
		}
	}

	fmt.Println("\n=== Rebalance example ===")
	printStats("after 200 insertions")

	// Delete 12 entries from the leftmost leaf page (key-00000000..key-00000011).
	//
	// That page drops from 22 to 10 entries:
	//   size = 16 + 10×92 = 936 bytes ≤ 1024 → underflows → rebalance() fires
	//
	// It is the leftmost child (index 0), so rebalance() merges it with
	// its right sibling (key-00000022..key-00000043, 22 entries):
	//
	//   merged node: 10 + 22 = 32 entries
	//   size = 16 + 32×92 = 2960 bytes < 4096 → fits, no re-split
	//
	// The parent loses one child pointer; leaf count drops from 9 to 8.
	if err = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		for i := range 12 {
			if err := b.Delete([]byte(fmt.Sprintf("key-%08d", i))); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	printStats("after deleting key-00000000..key-00000011 (one page underflows → merges with right sibling)")

	// Now delete most of the remaining entries, keeping only the last 10
	// (key-00000190..key-00000199). Every leaf page underflows in sequence;
	// each merge triggers rebalance() on the parent, cascading up the tree.
	//
	// Once only 10 entries remain, the total serialised size of the bucket is
	// 16 + 10×92 = 936 bytes, which falls below maxInlineBucketSize() = pageSize/4 = 1024.
	// bbolt inlines the entire bucket into the parent (root) bucket's leaf page
	// via bucket.inlineable() → bucket.write(). An inline bucket has RootPage() == 0;
	// Stats() routes it to InlineBucketInuse rather than LeafPageN, which is why
	// both LeafPageN and BranchPageN show 0 even though Depth == 1.
	if err = db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		for i := 12; i < 190; i++ {
			if err := b.Delete([]byte(fmt.Sprintf("key-%08d", i))); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	printStats("after mass deletion (936 B < 1024 B threshold → bucket inlined, LeafPageN=0)")
}

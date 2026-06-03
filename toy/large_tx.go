package main

import (
	"fmt"
	"log"
	"os"

	bolt "go.etcd.io/bbolt"
)

// largeTxExample demonstrates case 1: a single transaction that inserts many
// entries, forcing the loop inside node.split() to run many times on commit.
//
// Normally, a transaction adds a handful of entries to a leaf page that is
// near its post-split minimum (~22 entries). At spill, that leaf node has at
// most ~44 inodes — just enough to split once. splitTwo() fires, returns
// a right piece that fits in one page, and the loop exits after two calls.
//
// When all entries arrive in one transaction the picture changes:
//   - b.Put() calls accumulate every new entry into the single in-memory node
//     (there is no spill between puts inside one tx).
//   - At commit, that node can have hundreds of inodes.
//   - split() must loop, peeling a 22-entry left page on each iteration,
//     until the remainder finally fits.
//
//   node: 500 inodes
//     → iteration  1: page[k0–k21]   + 478 remaining (still > pageSize)
//     → iteration  2: page[k22–k43]  + 456 remaining
//     → …
//     → iteration 23: page[k484–k499]  + 0 remaining (fits, b = nil, exit)
//
// The resulting B-tree is structurally equivalent to inserting the same keys
// across 50 small transactions — the loop is purely an implementation concern.
func largeTxExample() {
	const total = 500
	value := make([]byte, 64)
	bucket := []byte("data")

	opts := &bolt.Options{PageSize: 4096}

	printStats := func(db *bolt.DB, label string) {
		if err := db.View(func(tx *bolt.Tx) error {
			s := tx.Bucket(bucket).Stats()
			fmt.Printf("  %-52s  leaf=%d  branch=%d  depth=%d\n",
				label, s.LeafPageN, s.BranchPageN, s.Depth)
			return nil
		}); err != nil {
			log.Fatal(err)
		}
	}

	// --- approach 1: every entry in one transaction ---
	// At spill the single leaf root holds all 500 inodes; split() loops ~23×.
	const path1 = "large_tx_one.db"
	os.Remove(path1)
	db1, err := bolt.Open(path1, 0600, opts)
	if err != nil {
		log.Fatal(err)
	}
	defer db1.Close()

	if err = db1.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(bucket)
		if err != nil {
			return err
		}
		for i := range total {
			if err := b.Put([]byte(fmt.Sprintf("key-%08d", i)), value); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		log.Fatal(err)
	}

	// --- approach 2: same keys spread across 50 transactions of 10 ---
	// Each commit touches at most two leaf nodes; split() exits on the first
	// call (right piece fits immediately, so the loop body runs ≤ twice).
	const path2 = "large_tx_batched.db"
	os.Remove(path2)
	db2, err := bolt.Open(path2, 0600, opts)
	if err != nil {
		log.Fatal(err)
	}
	defer db2.Close()

	for start := range total / 10 {
		lo, hi := start*10, start*10+10
		if err = db2.Update(func(tx *bolt.Tx) error {
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

	fmt.Println("\n=== Large tx example: 500 entries, key=12B value=64B, pageSize=4096 ===")
	printStats(db1, "1 transaction   (split loop runs ~23× at commit)")
	printStats(db2, "50 transactions of 10  (split loop exits on 1st call each time)")
	fmt.Println("  → same depth; the loop is an internal mechanism, not a user-visible difference")
}

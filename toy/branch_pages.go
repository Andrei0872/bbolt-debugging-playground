package main

import (
	"fmt"
	"log"
	"os"

	bolt "go.etcd.io/bbolt"
)

// branchPagesExample demonstrates that bbolt's B-tree grows branch pages at
// multiple levels as keys accumulate.
//
// The page size is forced to 4096 bytes so the math works out the same way on
// macOS, where os.Getpagesize() returns 16 384 bytes and would require ~53 000
// entries to reach depth 3 instead of ~5 000.
//
// With 4096-byte pages and fillPercent=0.5:
//
//	leafPageElement  = 16 B header + key + value → node splits at 44 entries, first page gets 22
//	branchPageElement = 16 B header + key        → node splits at 145 children, first page gets 72
//
// Tree depth milestones (key=12 B, value=64 B):
//
//	depth 1 (~0–44 entries)    : single root leaf, no branch pages
//	depth 2 (~45–3 200 entries): root branch  →  leaf pages
//	  (branch node splits when size ≥ 4096; that happens at ~146 leaf pages)
//	depth 3 (~3 200+ entries)  : root branch  →  branch pages  →  leaf pages
//
// inode.go's WriteInodeToPage branch-page else-branch executes every time a
// branch page is written during spill() — which happens on every commit once
// depth >= 2.
func branchPagesExample() {
	const dbPath = "branch_pages.db"
	os.Remove(dbPath)

	db, err := bolt.Open(dbPath, 0600, &bolt.Options{
		PageSize: 4096,
		// Pre-allocate mmap space to avoid the mmaplock write-lock that would
		// deadlock when a write tx commits while a read tx is open.
		// InitialMmapSize: 64 * 1024 * 1024,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	bucket := []byte("branch-demo")
	value := make([]byte, 64) // fixed-size payload keeps the per-entry footprint predictable

	rounds := []struct {
		label string
		total int
	}{
		{"depth 1: single root leaf, no branch pages", 20},
		{"depth 2: root branch page → leaf pages", 50},
		{"depth 2 (growing): more leaf pages, still one branch level", 1000},
		{"depth 3: root branch → intermediate branches → leaf pages", 5000},
	}

	inserted := 0
	for _, r := range rounds {
		err = db.Update(func(tx *bolt.Tx) error {
			b, err := tx.CreateBucketIfNotExists(bucket)
			if err != nil {
				return err
			}
			for inserted < r.total {
				key := fmt.Sprintf("key-%08d", inserted)
				if err := b.Put([]byte(key), value); err != nil {
					return err
				}
				inserted++
			}
			return nil
		})
		if err != nil {
			log.Fatal(err)
		}

		err = db.View(func(tx *bolt.Tx) error {
			s := tx.Bucket(bucket).Stats()
			fmt.Printf("\n=== %s ===\n", r.label)
			fmt.Printf("  entries      : %d\n", inserted)
			fmt.Printf("  leaf pages   : %d\n", s.LeafPageN)
			fmt.Printf("  branch pages : %d\n", s.BranchPageN)
			fmt.Printf("  tree depth   : %d\n", s.Depth)
			return nil
		})
		if err != nil {
			log.Fatal(err)
		}
	}

	// Visualisation of the B+Tree.
	err = db.View(func(tx *bolt.Tx) error {
		key := "key-00001584"

		res := tx.Bucket(bucket).Get([]byte(key))
		fmt.Printf("res: = %v", res)

		return nil
	})
}

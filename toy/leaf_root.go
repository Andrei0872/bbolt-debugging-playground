package main

import (
	"fmt"
	"log"
	"os"

	bolt "go.etcd.io/bbolt"
)

// leafRootExample demonstrates a bucket whose root node is a leaf page — it has
// its own page on disk (RootPage() != 0) but has not grown past a single leaf,
// so depth=1 and BranchPageN=0.
//
// This is distinct from an inline bucket (RootPage()==0, data embedded in parent
// page value) and from a branch-rooted bucket (depth>=2).
//
// A bucket becomes a leaf root — rather than staying inline — in two ways:
//
//  1. It contains a sub-bucket. inlineable() returns false when any inode has
//     BucketLeafFlag set (bucket.go:820), so the bucket is spilled to its own page.
//
//  2. Its total serialised size exceeds pageSize/4. With 4 KB pages the inline
//     threshold is 1024 B. Each entry costs leafElementSize(16) + key + value bytes.
//     Using key=12 B and value=64 B: 11 entries → 16 + 11*92 = 1028 B > 1024 B.
//
// The example then shows the leaf root → branch root transition: adding enough
// entries causes a split at commit and depth jumps from 1 to 2.
func leafRootExample() {
	const dbPath = "leaf_root.db"
	os.Remove(dbPath)

	db, err := bolt.Open(dbPath, 0600, &bolt.Options{PageSize: 4096})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	printStats := func(label string, bkt []byte) {
		_ = db.View(func(tx *bolt.Tx) error {
			s := tx.Bucket(bkt).Stats()
			fmt.Printf("  %-45s depth=%-2d leaf=%-2d branch=%-2d keys=%d\n",
				label, s.Depth, s.LeafPageN, s.BranchPageN, s.KeyN)
			return nil
		})
	}

	fmt.Println("=== leafRootExample ===")

	// --- Way 1: sub-bucket forces non-inline ---
	//
	// A bucket with even a single sub-bucket cannot be inlined. inlineable()
	// checks each inode's flags and returns false on BucketLeafFlag (bucket.go:820).
	// The bucket gets its own leaf page even though it holds almost no data.
	if err := db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket([]byte("has-subbucket"))
		if err != nil {
			return err
		}
		if err := b.Put([]byte("key-00000000"), make([]byte, 64)); err != nil {
			return err
		}
		_, err = b.CreateBucket([]byte("child"))
		return err
	}); err != nil {
		log.Fatal(err)
	}
	printStats("way 1: sub-bucket → leaf root", []byte("has-subbucket"))

	// --- Way 2: size exceeds pageSize/4 ---
	//
	// 11 entries: 16 (page header) + 11 * (16+12+64) = 1028 B > 1024 B threshold.
	// inlineable() returns false due to size check (bucket.go:822), bucket spills
	// to its own page.
	if err := db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket([]byte("size-exceeded"))
		if err != nil {
			return err
		}
		val := make([]byte, 64)
		for i := 0; i < 11; i++ {
			if err := b.Put([]byte(fmt.Sprintf("key-%08d", i)), val); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		log.Fatal(err)
	}
	printStats("way 2: size > pageSize/4 → leaf root", []byte("size-exceeded"))

	// --- Delete within leaf root ---
	//
	// Deleting entries that keep the bucket above the inline threshold leaves it
	// as a leaf root: still one leaf page, still no branch page.
	if err := db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("size-exceeded"))
		return b.Delete([]byte("key-00000000"))
	}); err != nil {
		log.Fatal(err)
	}
	printStats("after deleting 1 key (still leaf root)", []byte("size-exceeded"))

	// --- Leaf root → branch root ---
	//
	// With key=12 B and value=64 B, a node fills a 4096 B page at ~45 entries.
	// Adding enough entries forces a split at the next commit: the single leaf
	// becomes two leaves under a new branch root (depth 1 → 2).
	if err := db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte("size-exceeded"))
		val := make([]byte, 64)
		for i := 11; i < 50; i++ {
			if err := b.Put([]byte(fmt.Sprintf("key-%08d", i)), val); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		log.Fatal(err)
	}
	printStats("after adding 39 more keys (branch root)", []byte("size-exceeded"))
	fmt.Println("  → leaf root promoted to branch root once split threshold crossed")
}

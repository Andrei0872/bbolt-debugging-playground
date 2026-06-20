package main

import (
	"fmt"
	"log"
	"os"

	bolt "go.etcd.io/bbolt"
)

// inlineBucketExample demonstrates the difference between inline and regular buckets.
//
// A child bucket is inlined when, at commit time (spill), ALL of these hold:
//   - its root node is a single leaf (no splits yet)
//   - it contains no nested sub-buckets
//   - its total serialised size <= pageSize/4 (1024 B for 4 KB pages)
//
// When inlined the bucket has no page of its own. Its header + leaf data are
// serialised into a []byte and stored as the *value* of the parent bucket's
// key-value entry for that child bucket name. It lives inside the parent's leaf
// page, not on a separate page.
//
// A regular (non-inline) bucket gets its own page(s) and is referenced by a
// page ID stored in the InBucket header.
//
// With 4 KB pages and pageSize/4 = 1024 B threshold:
//
//	Page header:        16 B
//	Per entry:          leafPageElement (16 B) + key + value
//
// A child with a few tiny entries stays well under 1024 B → inline.
// A child whose entries exceed 1024 B total → regular, owns a page.
func inlineBucketExample() {
	const dbPath = "inline_bucket.db"
	os.Remove(dbPath)

	db, err := bolt.Open(dbPath, 0600, &bolt.Options{PageSize: 4096})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	parent := []byte("parent")
	inlineChild := []byte("inline-child")
	regularChild := []byte("regular-child")

	err = db.Update(func(tx *bolt.Tx) error {
		pb, err := tx.CreateBucket(parent)
		if err != nil {
			return err
		}

		// inline-child: 3 small entries → well under 1024 B threshold.
		ic, err := pb.CreateBucket(inlineChild)
		if err != nil {
			return err
		}
		ic.Put([]byte("a"), []byte("1"))
		ic.Put([]byte("b"), []byte("2"))
		ic.Put([]byte("c"), []byte("3"))

		// regular-child: enough entries to exceed pageSize/4 = 1024 B.
		// Each entry: 16 B element header + ~8 B key + ~20 B value = ~44 B.
		// 1024 / 44 ≈ 23 entries to cross the threshold; use 30 to be safe.
		rc, err := pb.CreateBucket(regularChild)
		if err != nil {
			return err
		}
		for i := 0; i < 30; i++ {
			rc.Put([]byte(fmt.Sprintf("key-%04d", i)), []byte(fmt.Sprintf("value-%04d-padding", i)))
		}

		return nil
	})
	if err != nil {
		log.Fatal(err)
	}

	db.View(func(tx *bolt.Tx) error {
		pb := tx.Bucket(parent)

		is := pb.Bucket(inlineChild).Stats()
		fmt.Println("=== inline-child ===")
		fmt.Printf("  leaf pages   : %d  (0 = stored inside parent's page)\n", is.LeafPageN)
		fmt.Printf("  branch pages : %d\n", is.BranchPageN)
		fmt.Printf("  inline       : %v\n", is.LeafPageN == 0)

		rs := pb.Bucket(regularChild).Stats()
		fmt.Println("=== regular-child ===")
		fmt.Printf("  leaf pages   : %d  (has its own page(s))\n", rs.LeafPageN)
		fmt.Printf("  branch pages : %d\n", rs.BranchPageN)
		fmt.Printf("  inline       : %v\n", rs.LeafPageN == 0)

		return nil
	})
}

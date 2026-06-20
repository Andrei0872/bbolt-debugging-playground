package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	bolt "go.etcd.io/bbolt"
)

// deleteBucketExample covers the five code paths in bucket.DeleteBucket() (bucket.go):
//
//  1. Inline bucket — child.free() is a no-op (RootPage==0); entry removed from
//     parent leaf.                                               (line 320-322)
//  2. Regular bucket — child.free() walks pages and releases them to freelist.
//                                                               (line 320-322)
//  3. Nested buckets — ForEachBucket + recursive DeleteBucket clears children
//     before removing the outer bucket.                         (line 306-314)
//  4. Bucket not found → ErrBucketNotFound.                    (line 298-299)
//  5. Key is a plain value, not a bucket → ErrIncompatibleValue.(line 300-302)
func deleteBucketExample() {
	const dbPath = "delete_bucket.db"
	os.Remove(dbPath)

	db, err := bolt.Open(dbPath, 0600, &bolt.Options{PageSize: 4096})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	parent := []byte("parent")

	if err := db.Update(func(tx *bolt.Tx) error {
		p, err := tx.CreateBucket(parent)
		if err != nil {
			return err
		}

		// Inline bucket: a handful of tiny entries stays well below
		// pageSize/4 = 1024 B → stored inside the parent's leaf page.
		inline, err := p.CreateBucket([]byte("inline"))
		if err != nil {
			return err
		}
		if err := inline.Put([]byte("k1"), []byte("v1")); err != nil {
			return err
		}

		// Regular bucket: 50 entries exceed pageSize/4 at commit → spills to
		// its own page(s), referenced by a pgid in the parent leaf.
		regular, err := p.CreateBucket([]byte("regular"))
		if err != nil {
			return err
		}
		val := make([]byte, 64)
		for i := 0; i < 50; i++ {
			if err := regular.Put([]byte(fmt.Sprintf("key-%08d", i)), val); err != nil {
				return err
			}
		}

		// Nested: outer contains inner — DeleteBucket must recurse.
		outer, err := p.CreateBucket([]byte("outer"))
		if err != nil {
			return err
		}
		inner, err := outer.CreateBucket([]byte("inner"))
		if err != nil {
			return err
		}
		if err := inner.Put([]byte("x"), []byte("y")); err != nil {
			return err
		}

		// Plain value — not a bucket.
		return p.Put([]byte("plainvalue"), []byte("data"))
	}); err != nil {
		log.Fatal(err)
	}

	printStats := func(label string) {
		_ = db.View(func(tx *bolt.Tx) error {
			s := tx.Bucket(parent).Stats()
			fmt.Printf("  %-42s buckets=%-2d leaf=%-2d branch=%d\n",
				label, s.BucketN, s.LeafPageN, s.BranchPageN)
			return nil
		})
	}

	fmt.Println("=== deleteBucketExample ===")
	printStats("initial")

	// Case 1: inline bucket — child.free() returns immediately (RootPage==0).
	if err := db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(parent).DeleteBucket([]byte("inline"))
	}); err != nil {
		log.Fatalf("case 1: %v", err)
	}
	printStats("after DeleteBucket(inline)")

	// Case 2: regular bucket — child.free() releases owned pages to freelist.
	if err := db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(parent).DeleteBucket([]byte("regular"))
	}); err != nil {
		log.Fatalf("case 2: %v", err)
	}
	printStats("after DeleteBucket(regular)")

	// Case 3: nested bucket — inner is deleted first via ForEachBucket recursion,
	// then outer is removed.
	if err := db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(parent).DeleteBucket([]byte("outer"))
	}); err != nil {
		log.Fatalf("case 3: %v", err)
	}
	printStats("after DeleteBucket(outer→inner)")

	// Case 4: bucket not found.
	err = db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(parent).DeleteBucket([]byte("missing"))
	})
	if !errors.Is(err, bolt.ErrBucketNotFound) {
		log.Fatalf("case 4: expected ErrBucketNotFound, got: %v", err)
	}
	fmt.Println("  case 4: ErrBucketNotFound ✓")

	// Case 5: key exists but is a plain value, not a bucket.
	err = db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(parent).DeleteBucket([]byte("plainvalue"))
	})
	if !errors.Is(err, bolt.ErrIncompatibleValue) {
		log.Fatalf("case 5: expected ErrIncompatibleValue, got: %v", err)
	}
	fmt.Println("  case 5: ErrIncompatibleValue ✓")
}

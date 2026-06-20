package main

import (
	"errors"
	"fmt"
	"log"
	"os"

	bolt "go.etcd.io/bbolt"
)

// deleteKVExample covers the three code paths in bucket.Delete() (bucket.go):
//
//  1. Key exists → inode removed, node marked unbalanced.        (line 532)
//  2. Key does not exist → seek falls through, nil returned.     (line 522-524)
//  3. Key refers to a sub-bucket → ErrIncompatibleValue.         (line 527-529)
func deleteKVExample() {
	const dbPath = "delete_kv.db"
	os.Remove(dbPath)

	db, err := bolt.Open(dbPath, 0600, &bolt.Options{PageSize: 4096})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	bkt := []byte("kv")

	if err := db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket(bkt)
		if err != nil {
			return err
		}
		if err := b.Put([]byte("exists"), []byte("value")); err != nil {
			return err
		}
		_, err = b.CreateBucket([]byte("subbucket"))
		return err
	}); err != nil {
		log.Fatal(err)
	}

	// Case 1: key exists — inode is removed, node flagged unbalanced.
	if err := db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bkt).Delete([]byte("exists"))
	}); err != nil {
		log.Fatalf("case 1: %v", err)
	}
	fmt.Println("case 1: deleted existing key — ok")

	// Case 2: key does not exist — cursor seek finds no match, returns nil.
	if err := db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bkt).Delete([]byte("missing"))
	}); err != nil {
		log.Fatalf("case 2: expected nil, got: %v", err)
	}
	fmt.Println("case 2: deleted missing key — nil returned (no-op)")

	// Case 3: key refers to a sub-bucket — BucketLeafFlag is set on the inode,
	// Delete() refuses and returns ErrIncompatibleValue.
	err = db.Update(func(tx *bolt.Tx) error {
		return tx.Bucket(bkt).Delete([]byte("subbucket"))
	})
	if !errors.Is(err, bolt.ErrIncompatibleValue) {
		log.Fatalf("case 3: expected ErrIncompatibleValue, got: %v", err)
	}
	fmt.Println("case 3: Delete() on a bucket key → ErrIncompatibleValue")
}

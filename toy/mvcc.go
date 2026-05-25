package main

import (
	"fmt"
	"log"

	bolt "go.etcd.io/bbolt"
)

// mvccExample demonstrates that a read-only transaction sees a consistent
// snapshot of the database at the moment it was opened, regardless of
// concurrent writes that commit afterward.
//
// bbolt implements MVCC by keeping old pages alive as long as any read
// transaction still references them. The write transaction allocates new
// pages and updates the meta page atomically on commit; the read transaction
// keeps a pointer to the old meta and therefore never sees the new data.
func mvccExample() {
	// InitialMmapSize pre-allocates enough mmap space so that writes committed
	// while a read transaction is open never need to remap — remapping requires
	// a write lock on mmaplock, which would deadlock against the read lock held
	// by the open read transaction in the same goroutine.
	db, err := bolt.Open("mvcc.db", 0600, &bolt.Options{InitialMmapSize: 10 * 1024 * 1024})
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	bucket := []byte("mvcc-bucket")
	key := []byte("counter")

	// Seed the database with an initial value.
	mustUpdate(db, bucket, key, []byte("v1"))

	// Open a long-lived read transaction — this pins the current DB snapshot.
	readTx, err := db.Begin(false)
	if err != nil {
		log.Fatal(err)
	}
	defer readTx.Rollback()

	fmt.Println("=== MVCC example ===")
	fmt.Printf("read-tx opened  → sees: %s\n", readTx.Bucket(bucket).Get(key))

	// Commit a write while the read transaction is still open.
	mustUpdate(db, bucket, key, []byte("v2"))
	fmt.Println("write committed → value is now v2 in the DB")

	// The read transaction still returns the snapshot it was opened with.
	fmt.Printf("read-tx still   → sees: %s  (snapshot isolation)\n", readTx.Bucket(bucket).Get(key))

	// A new read transaction started after the write sees the updated value.
	err = db.View(func(tx *bolt.Tx) error {
		fmt.Printf("new read-tx     → sees: %s  (latest committed)\n", tx.Bucket(bucket).Get(key))
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}

func mustUpdate(db *bolt.DB, bucket, key, value []byte) {
	err := db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(bucket)
		if err != nil {
			return err
		}
		return b.Put(key, value)
	})
	if err != nil {
		log.Fatal(err)
	}
}

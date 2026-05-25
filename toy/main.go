package main

import (
	"fmt"
	"log"

	bolt "go.etcd.io/bbolt"
)

func main() {
	db, err := bolt.Open("my.db", 0600, nil)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()

	bucket := []byte("widgets")
	key := []byte("foo")
	value := []byte("bar")

	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists(bucket)
		if err != nil {
			return err
		}
		return b.Put(key, value)
	})
	if err != nil {
		log.Fatal(err)
	}

	err = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket(bucket)
		v := b.Get(key)
		fmt.Printf("key=%s value=%s\n", key, v)
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
}

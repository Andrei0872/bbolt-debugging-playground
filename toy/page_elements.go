package main

import (
	"encoding/binary"
	"fmt"
	"log"
	"os"

	bolt "go.etcd.io/bbolt"
)

// pageElementsExample shows the raw on-disk layout of branch and leaf pages by
// reading the .db file directly with encoding/binary — no internal packages needed.
//
// Struct layouts (little-endian, from bbolt/internal/common/page.go & meta.go):
//
//	Page header (16 B):
//	  id       uint64  offset  0
//	  flags    uint16  offset  8   (0x01=branch, 0x02=leaf, 0x04=meta, 0x10=freelist)
//	  count    uint16  offset 10   number of elements on this page
//	  overflow uint32  offset 12
//
//	Meta (immediately after page header, so absolute offset 16 from page start):
//	  magic    uint32  offset  0
//	  version  uint32  offset  4
//	  pageSize uint32  offset  8
//	  flags    uint32  offset 12
//	  root.pgid uint64 offset 16  ← root bucket's first page
//	  root.seq  uint64 offset 24
//	  freelist uint64  offset 32
//	  pgid     uint64  offset 40  high-water mark
//	  txid     uint64  offset 48
//	  checksum uint64  offset 56
//
//	branchPageElement (16 B, packed after page header):
//	  pos   uint32  offset  0  byte offset from this element's address to its key
//	  ksize uint32  offset  4
//	  pgid  uint64  offset  8  child page id
//	  key bytes: elem[i*16 + pos .. +ksize]
//
//	leafPageElement (16 B, packed after page header):
//	  flags uint32  offset  0  (0x01 = sub-bucket entry, 0 = plain kv)
//	  pos   uint32  offset  4  byte offset from this element's address to its key
//	  ksize uint32  offset  8
//	  vsize uint32  offset 12
//	  key   bytes: elem[i*16 + pos .. +ksize]
//	  value bytes: elem[i*16 + pos+ksize .. +vsize]
func pageElementsExample() {
	const dbPath = "page_elements.db"
	os.Remove(dbPath)

	// Insert enough entries to force a branch page (depth 2 starts at ~45 entries
	// with 4 KB pages; use 60 to be safe).
	db, err := bolt.Open(dbPath, 0600, &bolt.Options{PageSize: 4096})
	if err != nil {
		log.Fatal(err)
	}
	// key-%08d = 12 B, value = 64 B → 16+12+64 = 92 B per entry.
	// A 4 KB page holds ~44 entries before splitting; 60 is safely above that.
	bucket := []byte("data")
	value := make([]byte, 64)
	err = db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucket(bucket)
		if err != nil {
			return err
		}
		for i := 0; i < 60; i++ {
			if err := b.Put([]byte(fmt.Sprintf("key-%08d", i)), value); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		log.Fatal(err)
	}
	db.Close()

	// --- raw file walk ---

	f, err := os.Open(dbPath)
	if err != nil {
		log.Fatal(err)
	}
	defer f.Close()

	readPage := func(pgid uint64, pageSize uint64) []byte {
		buf := make([]byte, pageSize)
		if _, err := f.ReadAt(buf, int64(pgid*pageSize)); err != nil {
			log.Fatalf("ReadAt pgid=%d: %v", pgid, err)
		}
		return buf
	}

	le := binary.LittleEndian

	// Step 1: read meta page 0 to learn pageSize and the root namespace's first page.
	meta0 := readPage(0, 4096)
	pageSize := uint64(le.Uint32(meta0[16+8:]))  // Meta.pageSize
	rootPgid := le.Uint64(meta0[16+16:])         // Meta.root.pgid

	fmt.Printf("page size : %d B\n", pageSize)
	fmt.Printf("root pgid : %d\n\n", rootPgid)

	// Step 2: root namespace page — one leaf entry per top-level bucket.
	// The "data" bucket's entry has flags=0x01 (BucketLeafFlag) and its value
	// is an InBucket: root_pgid(8) + sequence(8).
	rootPage := readPage(rootPgid, pageSize)
	rootFlags := le.Uint16(rootPage[8:])
	rootCount := le.Uint16(rootPage[10:])
	fmt.Printf("=== root namespace page (pgid %d, flags 0x%02x, count %d) ===\n",
		rootPgid, rootFlags, rootCount)

	var bucketRootPgid uint64
	for i := 0; i < int(rootCount); i++ {
		elemOff := 16 + i*16 // page header + i * leafPageElementSize
		elemFlags := le.Uint32(rootPage[elemOff:])
		pos := le.Uint32(rootPage[elemOff+4:])
		ksize := le.Uint32(rootPage[elemOff+8:])
		vsize := le.Uint32(rootPage[elemOff+12:])
		keyStart := elemOff + int(pos)
		key := rootPage[keyStart : keyStart+int(ksize)]
		valStart := keyStart + int(ksize)
		val := rootPage[valStart : valStart+int(vsize)]
		fmt.Printf("  [%d] flags=0x%02x key=%q ksize=%d vsize=%d\n",
			i, elemFlags, key, ksize, vsize)
		if elemFlags&0x01 != 0 { // BucketLeafFlag
			bucketRootPgid = le.Uint64(val) // InBucket.root
			fmt.Printf("       └─ sub-bucket root pgid: %d\n", bucketRootPgid)
		}
	}

	// Step 3: branch page — the root of the "data" bucket.
	branchPage := readPage(bucketRootPgid, pageSize)
	branchFlags := le.Uint16(branchPage[8:])
	branchCount := le.Uint16(branchPage[10:])
	fmt.Printf("\n=== bucket root page (pgid %d, flags 0x%02x = %s, count %d) ===\n",
		bucketRootPgid, branchFlags, pageTypeName(branchFlags), branchCount)
	if branchFlags != 0x01 {
		fmt.Println("  (not a branch page — insert more entries to force a split)")
		return
	}

	var firstLeafPgid uint64
	for i := 0; i < int(branchCount); i++ {
		elemOff := 16 + i*16 // page header + i * branchPageElementSize
		pos := le.Uint32(branchPage[elemOff:])
		ksize := le.Uint32(branchPage[elemOff+4:])
		childPgid := le.Uint64(branchPage[elemOff+8:])
		keyStart := elemOff + int(pos)
		key := branchPage[keyStart : keyStart+int(ksize)]
		fmt.Printf("  [%d] separator_key=%-14q  →  child pgid %d\n", i, key, childPgid)
		if i == 0 {
			firstLeafPgid = childPgid
		}
	}

	// Step 4: first leaf child — actual key/value entries.
	leafPage := readPage(firstLeafPgid, pageSize)
	leafFlags := le.Uint16(leafPage[8:])
	leafCount := le.Uint16(leafPage[10:])
	fmt.Printf("\n=== first child page (pgid %d, flags 0x%02x = %s, count %d) ===\n",
		firstLeafPgid, leafFlags, pageTypeName(leafFlags), leafCount)

	limit := int(leafCount)
	if limit > 5 {
		limit = 5
	}
	for i := 0; i < limit; i++ {
		elemOff := 16 + i*16
		pos := le.Uint32(leafPage[elemOff+4:])
		ksize := le.Uint32(leafPage[elemOff+8:])
		vsize := le.Uint32(leafPage[elemOff+12:])
		keyStart := elemOff + int(pos)
		key := leafPage[keyStart : keyStart+int(ksize)]
		valStart := keyStart + int(ksize)
		val := leafPage[valStart : valStart+int(vsize)]
		fmt.Printf("  [%d] key=%-12q value=%q\n", i, key, val)
	}
	if int(leafCount) > limit {
		fmt.Printf("  ... (%d more entries)\n", int(leafCount)-limit)
	}
}

func pageTypeName(flags uint16) string {
	switch flags {
	case 0x01:
		return "branch"
	case 0x02:
		return "leaf"
	case 0x04:
		return "meta"
	case 0x10:
		return "freelist"
	default:
		return fmt.Sprintf("unknown(0x%02x)", flags)
	}
}

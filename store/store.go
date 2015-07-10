// wrapper for Bolt database
package store

import (
	"errors"
	"github.com/boltdb/bolt"
	"github.com/tv42/topic"
	"log"
	"sync/atomic"
)

var ErrNotFound = errors.New("Item not found")

// Wrapper for a single Bolt database
type DB struct {
	NotifyQuit *topic.Topic
	debug      *uint64
	*bolt.DB
}

// open a bolt database file
func Open(file string) (b *DB, err error) {
	b = &DB{}
	b.DB, err = bolt.Open(file, 0600, nil)
	if err == nil {
		b.NotifyQuit = topic.New()
	}
	return
}

// enable a debugger for bolt database transactions
func (b *DB) EnableTransactionDebug() {
	ctr := uint64(0)
	b.debug = &ctr
}

// helper: Debug a transaction if debug mode is enabled
func (b *DB) debugTx(ftx func(func(tx *bolt.Tx) error) error, f func(tx *bolt.Tx) error, name string) (err error) {
	if b.debug != nil {
		id := atomic.AddUint64(b.debug, 1)
		log.Printf("start tx %s %d", name, id)
		err = ftx(func(tx *bolt.Tx) error {
			log.Printf("in tx %s %d", name, id)
			return f(tx)
		})
		log.Printf("end tx %s %d, err=%v", name, id, err)
	} else {
		err = ftx(f)
	}
	return
}

// wrapper that can be used for debugging purposes
func (b *DB) View(f func(tx *bolt.Tx) error) error {
	return b.debugTx(b.DB.View, f, "View")
}

// wrapper that can be used for debugging purposes
func (b *DB) Update(f func(tx *bolt.Tx) error) error {
	return b.debugTx(b.DB.Update, f, "Update")
}

// wrapper that can be used for debugging purposes
func (b *DB) Batch(f func(tx *bolt.Tx) error) error {
	return b.debugTx(b.DB.Batch, f, "Batch")
}

// test for the existence of an item, identified by key
// The item parameter must be a pointer to an item of the kind
// to be tested for.
func (b *DB) Exists(tx *bolt.Tx, key []byte, item Item) bool {
	bucket := tx.Bucket(item.StoreID())
	if bucket == nil {
		return false
	}
	if bucket.Get(key) != nil {
		return true
	}
	return false
}

// get an item, identified by key.
// The item parameter must be a pointer to the value that will get set.
func (b *DB) Get(tx *bolt.Tx, key []byte, item Item) error {
	bucket := tx.Bucket(item.StoreID())
	if bucket == nil {
		return ErrNotFound
	}
	v := bucket.Get(key)
	if v == nil {
		return ErrNotFound
	}
	err := item.DeserializeFrom(v)
	if err == nil {
		item.SetKey(key)
	}
	return err
}

// put an item to the corresponding store
func (b *DB) Put(tx *bolt.Tx, item Item) error {
	bucket, err := tx.CreateBucketIfNotExists(item.StoreID())
	if err == nil {
		bytes, err := item.Bytes()
		if err == nil {
			bucket.Put(item.Key(), bytes)
		}
	}
	return err
}

// update a record that is encapsuled in a Meta struct
func (b *DB) UpdateMeta(tx *bolt.Tx, olditem *Meta, newitem *Meta) error {
	err := b.Get(tx, newitem.Key(), olditem)
	if err == nil {
		// got an old entry
		if newitem.Created.Equal(Never) || olditem.Created.Before(newitem.Created) {
			// set created datestamp to that of old entry - if the new item does
			// not actually have an earlier datestamp.
			newitem.Created = olditem.Created
		}
	}
	return b.Put(tx, newitem)
}

// iterate through the items in a bucket
// Calls a callback function handler, which will get the key and the cursor
// as parameters so it can do deleten. The actual item is stored to the
// item parameter - must be a pointer.
// When the callback handler returns true, that will trigger a reset of the
// loop, starting anew. Do this after a delete.
func (b *DB) ForEach(tx *bolt.Tx, item Item, handler func(cursor *bolt.Cursor) (bool, error)) error {
	return b.iterate(tx, item, handler,
		func(c *bolt.Cursor) ([]byte, []byte) { return c.First() },
		func(c *bolt.Cursor) ([]byte, []byte) { return c.Next() })
}

// like ForEach, but running from last item to first one
func (b *DB) ForEachReverse(tx *bolt.Tx, item Item, handler func(cursor *bolt.Cursor) (bool, error)) error {
	return b.iterate(tx, item, handler,
		func(c *bolt.Cursor) ([]byte, []byte) { return c.Last() },
		func(c *bolt.Cursor) ([]byte, []byte) { return c.Prev() })
}

// actual implementation of iteration through items in bucket
func (b *DB) iterate(tx *bolt.Tx, item Item,
	handler func(cursor *bolt.Cursor) (bool, error),
	start func(cursor *bolt.Cursor) ([]byte, []byte),
	cont func(cursor *bolt.Cursor) ([]byte, []byte)) error {

	bucket := tx.Bucket(item.StoreID())
	if bucket == nil {
		// no error if there's nothing to loop over
		return nil
	}
	c := bucket.Cursor()
restart:
	for k, v := start(c); k != nil; k, v = cont(c) {
		err := item.DeserializeFrom(v)
		if err != nil {
			// skip over invalid data
			continue
		}
		item.SetKey(k)
		restart, err := handler(c)
		if err != nil {
			return err
		}
		if restart {
			goto restart
		}
	}
	return nil
}

// Shut down database, end all registered tasks.
func (b *DB) Close() {
	b.NotifyQuit.Broadcast <- struct{}{}
	close(b.NotifyQuit.Broadcast)
}

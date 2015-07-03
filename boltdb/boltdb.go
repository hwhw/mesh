// wrapper for Bolt database
package boltdb

import (
    "github.com/boltdb/bolt"
    "sync"
    "errors"
    "time"
    "runtime"
    "log"
)

var ErrUnknownItem = errors.New("Unknown item")
var ErrNotFound = errors.New("Item not found")
var ErrNoStore = errors.New("No store found for item")

type StoreID byte

type Notification interface {}
type QuitNotification struct{}
type UpdateNotification struct {
    StoreID StoreID
    Key []byte
}
type PurgeNotification struct {
    StoreID StoreID
    Key []byte
}

type subscriptions struct {
    list []chan Notification
    sync.Mutex
}

type StoreMatch func(Item) bool

// Wrapper for a single Bolt database
type BoltDB struct {
    storeFor func(Item) StoreID
    subscriptions subscriptions
    debug chan int
    *bolt.DB
}

const NoStore = 255

// open a bolt database file
func Open(file string) (b *BoltDB, err error) {
    b = &BoltDB{
        subscriptions: subscriptions{list: make([]chan Notification, 0, 5)},
        storeFor: func(i Item) StoreID { return NoStore },
    }
    b.DB, err = bolt.Open(file, 0600, nil)
    return
}

// enable a debugger for bolt database transactions
func (b *BoltDB) EnableTransactionDebug() {
    b.debug = make(chan int)
    go func() {
        notifications := b.Subscribe()
        defer b.Unsubscribe(notifications)
        i := 0
        for {
            select {
            case n := <-notifications:
                if _, q := n.(QuitNotification); q {
                    return
                }
            case b.debug <- i:
                i++
            }
        }
    }()
}

// wrapper that can be used for debugging purposes
func (b *BoltDB) View(f func(tx *bolt.Tx) error) error {
    if b.debug != nil {
        id := <- b.debug
        log.Printf("start tx view %d", id)
        err := b.DB.View(func(tx *bolt.Tx) error {
            log.Printf("in tx view %d", id)
            return f(tx)
        })
        log.Printf("end tx view %d, err=%v", id, err)
        return err
    }
    return b.DB.View(f)
}

// wrapper that can be used for debugging purposes
func (b *BoltDB) Update(f func(tx *bolt.Tx) error) error {
    if b.debug != nil {
        id := <- b.debug
        stack := make([]byte, 500)
        runtime.Stack(stack, false)
        log.Printf("start tx update %d: %v", id, string(stack))
        err := b.DB.Update(func(tx *bolt.Tx) error {
            log.Printf("in tx update %d", id)
            return f(tx)
        })
        log.Printf("end tx update %d, err=%v", id, err)
        return err
    }
    return b.DB.Update(f)
}

// a simple wrapper
func Storekey(id StoreID) []byte {
    return []byte{byte(id)}
}

// register a Store within a Bolt database
func (b *BoltDB) RegisterStore(storeid StoreID, match StoreMatch, purgeInterval *time.Duration) (err error) {
    err = b.Update(func(tx *bolt.Tx) error {
        _, err = tx.CreateBucketIfNotExists(Storekey(storeid))
        return err
    })
    if err == nil && match != nil {
        oldstoreFor := b.storeFor
        b.storeFor = func(i Item) StoreID {
            if match(i) {
                return storeid
            } else {
                return oldstoreFor(i)
            }
        }
    }
    if err == nil && purgeInterval != nil {
        // start a background task that will regularly purge
        // database entries that have become invalid
        go func() {
            notifications := b.Subscribe()
            defer b.Unsubscribe(notifications)
            for {
                countdown := time.After(*purgeInterval)
                for {
                    select {
                    case n := <-notifications:
                        if _, q := n.(QuitNotification); q {
                            return
                        }
                    case <-countdown:
                        b.Update(func(tx *bolt.Tx) error {
                            b.purge(tx, storeid)
                            return nil
                        })
                        break
                    }
                }
            }
        }()
    }
    return
}

// Subscribe to notifications about events
// Returns a channel that will receive notifications
// Remember to unsubscribe when stopping to read from channel:
// it will block otherwise.
func (b *BoltDB) Subscribe() chan Notification {
    // all channels can buffer one notification so the subscribers
    // won't block if sending a notification themselves
    s := make(chan Notification, 1)
    b.subscriptions.Lock()
    b.subscriptions.list = append(b.subscriptions.list, s)
    b.subscriptions.Unlock()
    return s
}

// Unsubscribe from notifications
func (b *BoltDB) Unsubscribe(sub chan Notification) {
    b.subscriptions.Lock()
    for id, s := range b.subscriptions.list {
        if s == sub {
            b.subscriptions.list = append(
                b.subscriptions.list[:id],
                b.subscriptions.list[id+1:]...)
            break
        }
    }
    b.subscriptions.Unlock()
}

// Send out notification
func (b *BoltDB) Notify(n Notification) {
    b.subscriptions.Lock()
    for _, s := range b.subscriptions.list {
        s <- n
    }
    b.subscriptions.Unlock()
}

// Shut down database, end all running tasks.
func (b *BoltDB) Close() {
    b.Notify(QuitNotification{})
}

// get an item, identified by key.
// The item parameter must be a pointer to the value that will get set.
func (b *BoltDB) Get(tx *bolt.Tx, item Item, key []byte) error {
    store := b.storeFor(item)
    if store == NoStore {
        return ErrNoStore
    }
    bundle, err := b.GetBundle(tx, store, key)
    if err != nil {
        return err
    }
    err = item.DeserializeFrom(bundle.Data)
    return err
}

// iterator over all items in a store
// The item parameter must be a pointer to the value that will get set.
func (b *BoltDB) ForEach(tx *bolt.Tx, item Item, handler func(key []byte) error) error {
    store := b.storeFor(item)
    if store == NoStore {
        return ErrNoStore
    }
    return b.ForEachBundle(tx, store, func(bundle *ItemBundle, key []byte) error {
        err := item.DeserializeFrom(bundle.Data)
        if err != nil {
            return err
        }
        return handler(key)
    })
}

// Wrapper for Update() that will work with types that fulfill the interface Item.
// Will automatically serialize contained data.
func (b *BoltDB) UpdateItem(tx *bolt.Tx, item Item, lifetime time.Duration, presetMeta *ItemMeta) (*ItemBundle, error) {
    store := b.storeFor(item)
    if store == NoStore {
        return nil, ErrNoStore
    }
    data, err := item.Bytes()
    if err != nil {
        return nil, err
    }
    oldbundle, err := b.UpdateData(tx, store, item.Key(), lifetime, data, presetMeta)
    return oldbundle, err
}

// get an ItemBundle, identified by "key", from store "store"
func (b *BoltDB) GetBundle(tx *bolt.Tx, storeid StoreID, key []byte) (data *ItemBundle, err error) {
    v := tx.Bucket(Storekey(storeid)).Get(key)
    if v == nil {
        return nil, ErrNotFound
    }
    return DecodeItemBundle(v)
}

// iterator over all bundles in a store
func (b *BoltDB) ForEachBundle(tx *bolt.Tx, storeid StoreID, handler func(*ItemBundle, []byte) error) error {
    bk := tx.Bucket(Storekey(storeid))
    c := bk.Cursor()
    for k, v := c.First(); k != nil; k, v = c.Next() {
        bundle, err := DecodeItemBundle(v)
        if err == nil {
            err = handler(bundle, k)
        }
        if err != nil {
            return err
        }
    }
    return nil
}

// Update data in store, identified by key.
// Must be called from a transaction.
// Bundle metadata will get set accordingly.
// Lifetime of the data must be given.
// Returns a pointer to an ItemBundle set to the old value when an update takes place.
// When there was no old value, the pointer is nil.
func (b *BoltDB) UpdateData(tx *bolt.Tx, storeid StoreID, key []byte, lifetime time.Duration, data []byte, presetMeta *ItemMeta) (old *ItemBundle, err error) {
    bundle := ItemBundle{Data: data}
    now := time.Now()

    bk := tx.Bucket(Storekey(storeid))
    v := bk.Get(key)
    if v != nil {
        old, _ = DecodeItemBundle(v)
    }

    if presetMeta == nil {
        bundle.Meta = &ItemMeta{Updated: now, Created: now}
        if old != nil {
            // keep creation date
            bundle.Meta.Created = old.Meta.Created
        }
    } else {
        // use preset ItemMeta data
        bundle.Meta = presetMeta
    }
    // always set invalidation date
    bundle.Meta.Invalid = now.Add(lifetime)
    err = bk.Put(key, bundle.Encode())
    if err == nil {
        go b.Notify(UpdateNotification{StoreID: storeid, Key: append(make([]byte,0), key...)})
    }
    return
}

// helper: purge a store
// Must be called from a transaction.
func (b *BoltDB) purge(tx *bolt.Tx, storeid StoreID) error {
    now := time.Now()
    bk := tx.Bucket(Storekey(storeid))
    c := bk.Cursor()
    startover:
    for k, v := c.First(); k != nil; k, v = c.Next() {
        bundle, err := DecodeItemBundle(v)
        if err == nil {
            if bundle.Meta.Invalid.Before(now) {
                go b.Notify(PurgeNotification{StoreID: storeid, Key: append(make([]byte,0), k...)})
                c.Delete()
                goto startover
            }
        }
    }
    return nil
}

// For convenient storage, special methods are provided that apply to
// data which fulfills the Item interface
type Item interface {
    // return a key for the data item
    Key() ([]byte)
    // serialize data item
    Bytes() ([]byte, error)
    // unserialize data item
    DeserializeFrom([]byte) error
}

// Database entry metadata
type ItemMeta struct {
    // first creation of an item with this key in this store
    Created time.Time
    // last update of the item with this key in this store
    Updated time.Time
    // date when this item becomes invalid and may be purged
    Invalid time.Time
}

// these are needed for binary size calculation for ItemMeta
var timeMarshaled, _ = time.Time{}.MarshalBinary()
var timeSize = len(timeMarshaled)
var itemMetaSize = 3*timeSize

// fast ItemMeta binary encoder
func (i *ItemMeta) Encode() []byte {
    e := make([]byte, itemMetaSize)
    t, _ := i.Created.MarshalBinary()
    copy(e, t)
    t, _ = i.Updated.MarshalBinary()
    copy(e[timeSize:], t)
    t, _ = i.Invalid.MarshalBinary()
    copy(e[timeSize*2:], t)
    return e
}

// fast ItemMeta binary decoder
func DecodeItemMeta(d []byte) (*ItemMeta, error) {
    i := ItemMeta{}
    err := i.Created.UnmarshalBinary(d[:timeSize])
    if err == nil {
        err = i.Updated.UnmarshalBinary(d[timeSize:2*timeSize])
    }
    if err == nil {
        err = i.Invalid.UnmarshalBinary(d[timeSize:2*timeSize])
    }
    if err != nil {
        return nil, err
    }
    return &i, nil
}

// the storage entity is generally an ItemBundle
type ItemBundle struct {
    Meta *ItemMeta
    Data []byte
}

// encode ItemBundle type for storing to bolt db
func (i *ItemBundle) Encode() []byte {
    return append(i.Meta.Encode(), i.Data...)
}

// decode ItemBundle type from byte slice (e.g. from bolt db)
func DecodeItemBundle(d []byte) (bundle *ItemBundle, err error) {
    i := ItemBundle{Data: d[itemMetaSize:]}
    i.Meta, err = DecodeItemMeta(d[:itemMetaSize])
    return &i, err
}

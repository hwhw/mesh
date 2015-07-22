package store

import (
	"github.com/boltdb/bolt"
	"github.com/tv42/topic"
	"time"
)

type NotifyPurge struct {
	StoreID []byte
	Key     []byte
}

// Run a background task to purge invalid items of a given type in given intervals.
// Pass a *topic.Topic to be able to receive NotifyPurge messages for each purged
// item.
func (b *DB) Purger(itemtype Item, interval time.Duration, notifyQuit, notifyPurge *topic.Topic) {
	quit := make(chan interface{})
	notifyQuit.Register(quit)
	defer notifyQuit.Unregister(quit)
    actionloop:
	for {
		select {
		case <-quit:
			break actionloop
		case <-time.After(interval):
			meta := NewMeta(itemtype)
			b.Update(func(tx *bolt.Tx) error {
				return b.ForEach(tx, meta, func(cursor *bolt.Cursor) (bool, error) {
					if !meta.IsValid() {
						if notifyPurge != nil {
							notifyPurge.Broadcast <- NotifyPurge{StoreID: itemtype.StoreID(), Key: meta.Key()}
						}
						cursor.Delete()
						return true, nil
					}
					return false, nil
				})
			})
		}
	}
	if notifyPurge != nil {
		close(notifyPurge.Broadcast)
	}
}

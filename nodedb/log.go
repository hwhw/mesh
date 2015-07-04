package nodedb

import (
	"github.com/boltdb/bolt"
	"github.com/hwhw/mesh/store"
    "time"
    "errors"
    "log"
)

const (
    // log this client count for offline nodes
	NODE_OFFLINE = -1
    // log this if the client statistics data cannot be parsed
    NODE_DATAERROR = -2
)

// Will wait for update notifications and upon receiving them,
// run a count of the data items to be logged.
// Only one count will be run at any time. When notifications
// arrive during a count is running, another count will be run
// as soon the current one is done.
func (db *NodeDB) LogCounts(offlineAfter time.Duration) {
	quit := make(chan interface{})
	db.NotifyQuitLogger.Register(quit)
	defer db.NotifyQuitLogger.Unregister(quit)

	update := make(chan interface{})
	db.NotifyUpdateStatistics.Register(update)
	defer db.NotifyUpdateStatistics.Unregister(update)

    done := make(chan interface{})
    running := false // a count run is currently executing
    waiting := false // do a new count run as soon as the current is finished

    for {
        select {
        case <-quit:
            return
        case <-update:
            if !running {
                waiting = false
                go db.count(offlineAfter, done)
            } else {
                waiting = true
            }
        case <-done:
            if waiting {
                waiting = false
                running = true // just for clarity, should be set already
                go db.count(offlineAfter, done)
            } else {
                running = false
            }
        }
    }
}

// we use this to jump off the iteration
var errBreak = errors.New("break")

// write to the log
func (db *NodeDB) logCount(c Counter) error {
    timestamp := c.GetTimestamp()
    // save data because it will get overwritten when iterating through
    // existing data points
    count := c.GetCount()
    logit := true
    err := db.Logs.Batch(func(tx *bolt.Tx) error {
        db.Logs.ForEachReverse(tx, c, func(cursor *bolt.Cursor) (bool, error) {
            if c.GetTimestamp().Before(timestamp) && c.GetCount() == count {
                // not a new data point (no change)
                logit = false
                return false, errBreak
            }
            return false, nil
        })
        // we ignore errors, since often it will just be that we've never seen
        // the count's context before
        c.SetCount(count)
        c.SetTimestamp(timestamp)
        db.Logs.Put(tx, c)
        return nil
    })
    return err
}

// count data items
func (db *NodeDB) count(offlineAfter time.Duration, done chan<- interface{}) error {
    s := &Statistics{}
    m := store.NewMeta(s)
    clients := 0
    nodes := 0
    now := time.Now()
    deadline := now.Add(-offlineAfter)
    err := db.Main.View(func(tx *bolt.Tx) error {
        return db.Main.ForEach(tx, m, func(cursor *bolt.Cursor) (bool, error) {
            if m.Updated.Before(deadline) {
                // node is offline
                l := NewCountNodeClients(m.Key(), now, NODE_OFFLINE)
                db.logCount(l)
                return false, nil
            }
            if m.GetItem(s) == nil {
                if s.Data.Clients != nil {
                    //TODO: log Total or just Wifi? For now: Wifi.
                    l := NewCountNodeClients(m.Key(), m.Updated, s.Data.Clients.Wifi)
                    db.logCount(l)
                    clients += s.Data.Clients.Wifi
                } else {
                    l := NewCountNodeClients(m.Key(), now, 0)
                    db.logCount(l)
                }
            } else {
                l := NewCountNodeClients(m.Key(), now, NODE_DATAERROR)
                db.logCount(l)
            }
            nodes += 1
            return false, nil
        })
    })
    lc := &CountMeshClients{Count{timestamp: now, count:clients}}
    db.logCount(lc)
    ln := &CountMeshNodes{Count{timestamp:now, count:nodes}}
    db.logCount(ln)
    log.Printf("Log: %d nodes with %d clients", nodes, clients)
    done <- struct{}{}
    return err
}

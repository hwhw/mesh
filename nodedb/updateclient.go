package nodedb

import (
    "github.com/boltdb/bolt"
    "github.com/hwhw/mesh/boltdb"
    "github.com/hwhw/mesh/alfred"
    "github.com/hwhw/mesh/gluon"
    "github.com/hwhw/mesh/batadvvis"
    "time"
    "log"
    "math/rand"
)

type UpdateVisDataNotification struct{}
type UpdateNodeInfoNotification struct{}
type UpdateStatisticsNotification struct{}

// helper: wait for a certain time or quit notification
func wait(db *boltdb.BoltDB, t time.Duration) bool {
    notifications := db.Subscribe()
    defer db.Unsubscribe(notifications)
    waiter := time.After(t)
    for {
        select {
        case n := <-notifications:
            switch n.(type) {
            case boltdb.QuitNotification:
                return false
            }
        case <-waiter:
            return true
        }
    }
}

// Create a new update client.
// Give the network (unix, tcp) in network, the address to connect to in address,
// the time to wait between updates in updatewait and the time to wait after failure
// before retrying in retrywait.
// The updatewait duration is also the timeout duration for the actual
// network connections.
func (db *NodeDB) AlfredUpdate(
    network string, address string,
    updatewait time.Duration, retrywait time.Duration,
    packettype uint8,
    reader func(alfred.Data) (interface{}, error),
    success boltdb.Notification) {

    client := alfred.NewClient(network, address, updatewait)

    // write a random 0..updatewait at startup
    if !wait(db.store, time.Duration(rand.Float64() * float64(updatewait))) {
        return
    }
    for {
        log.Printf("UpdateClient: Updating data from alfred server for type %d", packettype)
        data, err := client.Request(packettype)
        if err == nil {
            err = db.store.Update(func(tx *bolt.Tx) error {
                for _, d := range data {
                    parsed, err := reader(d)
                    if err != nil {
                        log.Printf("UpdateClient: type %d, parse error: %+v (data: %+v)", packettype, err, d)
                    } else {
                        err = db.UpdateMeshData(tx, parsed, false, nil)
                        if err != nil {
                            log.Printf("UpdateClient: type %d, problem while updating data: %v", packettype, err)
                        }
                    }
                }
                return nil
            })
            if err == nil {
                log.Printf("UpdateClient: type %d, success.", packettype)
                go db.store.Notify(success)
                if !wait(db.store, updatewait) {
                    break
                } else {
                    continue
                }
            }
        } else {
            log.Printf("UpdateClient: type %d, error fetching data", packettype)
        }
        if !wait(db.store, retrywait) {
            break
        }
    }
}

func (db *NodeDB) NewUpdateClient(network string, address string, updatewait time.Duration, retrywait time.Duration) {
    go db.AlfredUpdate(network, address, updatewait, retrywait,
        batadvvis.PACKETTYPE,
        func(a alfred.Data) (interface{}, error) { return batadvvis.Read(a) },
        UpdateVisDataNotification{})
    go db.AlfredUpdate(network, address, updatewait, retrywait,
        gluon.NODEINFO_PACKETTYPE,
        func(a alfred.Data) (interface{}, error) { return gluon.ReadNodeInfo(a) },
        UpdateNodeInfoNotification{})
    go db.AlfredUpdate(network, address, updatewait, retrywait,
        gluon.STATISTICS_PACKETTYPE,
        func(a alfred.Data) (interface{}, error) { return gluon.ReadStatistics(a) },
        UpdateStatisticsNotification{})
}

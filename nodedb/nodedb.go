// this file contains data model specific functions

package nodedb

import (
    "github.com/boltdb/bolt"
    "github.com/hwhw/mesh/boltdb"
    "github.com/hwhw/mesh/gluon"
    "github.com/hwhw/mesh/batadvvis"
    "time"
    "errors"
)

var LongTime = time.Hour * 24 * 365 * 100 // 100 years should be enough for everyone (famous last words)
var noTime = time.Time{}

// bolt db stores
const (
    NodeInfo = iota
    Statistics
    Gateways
    VisData
    Aliases
    Clients
    ClientsSum
    NodesSum
)

type NodeDB struct {
    store *boltdb.BoltDB
    logstore *boltdb.BoltDB
    settings Settings
}

var ErrNoStoreForThisType = errors.New("no store for this data type")

// Settings for the database
type Settings struct {
    NodeOfflineDuration time.Duration
    GluonPurge time.Duration
    VisPurge time.Duration
}

// create a new database instance
func New(nodeofflineduration, gluonpurge, gluonpurgeint, vispurge, vispurgeint time.Duration, storefile string, logfile string) (*NodeDB, error) {
    store, err := boltdb.Open(storefile)
    if err != nil {
        return nil, err
    }
    logstore, err := boltdb.Open(logfile)
    if err != nil {
        return nil, err
    }

    // register stores
    store.RegisterStore(NodeInfo,
        func(i boltdb.Item) bool {
            _, ok := i.(*gluon.NodeInfo)
            return ok
        },
        &gluonpurgeint)
    store.RegisterStore(Statistics,
        func(i boltdb.Item) bool {
            _, ok := i.(*gluon.Statistics)
            return ok
        },
        &gluonpurgeint)
    store.RegisterStore(Gateways, nil, &gluonpurgeint)
    store.RegisterStore(VisData,
        func(i boltdb.Item) bool {
            _, ok := i.(*batadvvis.VisV1)
            return ok
        },
        &vispurgeint)
    store.RegisterStore(Aliases, nil, &vispurgeint)

    logstore.RegisterStore(Clients, nil, nil)
    logstore.RegisterStore(ClientsSum, nil, nil)

    db := NodeDB{
        settings: Settings{
            NodeOfflineDuration: nodeofflineduration,
            GluonPurge: gluonpurge,
            VisPurge: vispurge,
        },
        store: store,
        logstore: logstore,
    }

    go func() {
        // handle certain purge notifications
        notifications := db.store.Subscribe()
        defer db.store.Unsubscribe(notifications)
        for {
            n := <-notifications
            switch n := n.(type) {
            case boltdb.QuitNotification:
                return
            case boltdb.PurgeNotification:
                if n.StoreID == VisData {
                    // purged vis data means node is offline
                    db.LogClientsTotal(n.Key, time.Now(), -1)
                }
            }
        }
    }()

    return &db, nil
}

// Generic updater function
func (db *NodeDB) UpdateMeshData(tx *bolt.Tx, data interface{}, persistent bool, presetMeta *boltdb.ItemMeta) (err error) {
    dur := LongTime // default: keep data for a looooooong time (persistent)
    // switch depending on data type
    switch data := data.(type) {
    case *gluon.NodeInfo:
        if !persistent {
            dur = db.settings.GluonPurge
        }
        _, err = db.store.UpdateItem(tx, data, dur, presetMeta)
    case *gluon.Statistics:
        if !persistent {
            dur = db.settings.GluonPurge
        }
        _, err := db.store.UpdateItem(tx, data, dur, presetMeta)
        if err == nil {
            if data.Data.Gateway != nil {
                _, err = db.store.UpdateData(tx, Gateways, []byte(*data.Data.Gateway), dur, []byte{1}, nil)
            }
        }
        // Update Log
        if err == nil && data.Data.Clients != nil {
            err = db.LogClientsTotal(data.Key(), data.TimeStamp, data.Data.Clients.Total)
        }
    case *batadvvis.VisV1:
        if !persistent {
            dur = db.settings.VisPurge
        }
        _, err = db.store.UpdateItem(tx, data, dur, nil)
        if err == nil {
            _, err = db.store.UpdateData(tx, Aliases, []byte(data.Mac), dur, data.Key(), nil)
        }
        if err == nil {
            for _, mac := range data.Ifaces {
                _, err = db.store.UpdateData(tx, Aliases, []byte(mac.Mac), dur, data.Key(), nil)
                if err != nil {
                    break
                }
            }
        }
    default:
        return ErrNoStoreForThisType
    }
    return
}

// look up main ID for an alias
func (db *NodeDB) resolveAlias(tx *bolt.Tx, alias []byte) ([]byte, bool) {
    bundle, err := db.store.GetBundle(tx, Aliases, alias)
    if err != nil {
        return alias, false
    }
    return bundle.Data, true
}

// check if a given node is a Gateway
func (db *NodeDB) isGateway(tx *bolt.Tx, addr gluon.HardwareAddr) bool {
    _, err := db.store.GetBundle(tx, Gateways, addr)
    if err != nil {
        return false
    } else {
        return true
    }
}


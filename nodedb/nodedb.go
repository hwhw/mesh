// Package nodedb provides a database that collects information
// about a mesh network.
// It is oriented at batman-adv mesh networks using "Gluon" based
// node metadata.
// It can update from A.L.F.R.E.D. servers and provide data in
// formats suitable for mesh visualization, which is its main aim.
package nodedb

import (
    "github.com/hwhw/mesh/gluon"
    "github.com/hwhw/mesh/batadvvis"
    "sync"
    "errors"
    "time"
)

var ErrUnknownItem = errors.New("Unknown item")
var ErrNotFound = errors.New("Item not found")

var noTime = time.Time{}

// Database
type NodeDB struct {
    nodeinfo Store
    statistics Store
    visdata Store
    aliases Store
    gateways Store
    settings Settings
    tasks []chan struct{}
    updatesubscriber []chan Update
    sync.RWMutex
}

// Settings for the database
type Settings struct {
    NodeOfflineDuration time.Duration
    GluonPurge time.Duration
    GluonPurgeInt time.Duration
    BatAdvVisPurge time.Duration
    BatAdvVisPurgeInt time.Duration
    LogStoreFile string
}

// An item store, indexed by strings
type Store map[string]Item

// Item in a Store
type Item struct {
    item interface{}
    created time.Time
    updated time.Time
}

type Update struct {
    Store *Store
    Key string
    From interface{}
    To interface{}
}

// create a new database instance
func New(nodeofflineduration, gluonpurge, gluonpurgeint, vispurge, vispurgeint time.Duration, logstorefile string) NodeDB {
    db := NodeDB{
        nodeinfo: make(Store),
        statistics: make(Store),
        visdata: make(Store),
        aliases: make(Store),
        gateways: make(Store),
        settings: Settings{
            NodeOfflineDuration: nodeofflineduration,
            GluonPurge: gluonpurge,
            GluonPurgeInt: gluonpurgeint,
            BatAdvVisPurge: vispurge,
            BatAdvVisPurgeInt: vispurgeint,
            LogStoreFile: logstorefile,
        },
        tasks: make([]chan struct{}, 0, 5),
    }

    // database cleanup
    db.purgeTask(&db.nodeinfo, gluonpurge, gluonpurgeint)
    db.purgeTask(&db.statistics, gluonpurge, gluonpurgeint)
    db.purgeTask(&db.gateways, gluonpurge, gluonpurgeint)
    db.purgeTask(&db.visdata, vispurge, vispurgeint)
    db.purgeTask(&db.aliases, vispurge, vispurgeint)

    return db
}

// Wrapper for updating database entries in a Store.
// Assumes that the caller has already write-locked the
// database!
func (db *NodeDB) update(store *Store, id string, data interface{}) {
    u := Update{Store: store, Key: id, From: struct{}, To: data}
    db.Lock()
    item, exists := (*store)[id]
    if !exists {
        item = Item{created: time.Now()}
    } else {
        u.From = item.item
    }
    item.item = data
    item.updated = time.Now()
    (*store)[id] = item
    db.Unlock()
    for _, c := range db.updatesubscriber {
        c <- u
    }
}

// Purge entries older than "age" from a Store.
func (db *NodeDB) purge(store *Store, age time.Duration) {
    purgelist := make([]string, 0, 100)
    deadline := time.Now().Add(-age)
    db.Lock()
    for key, item := range *store {
        if item.created != noTime && item.updated != noTime && item.updated.Before(deadline) {
            purgelist = append(purgelist, key)
        }
    }
    for _, key := range purgelist {
        for _, c := range db.updatesubscriber {
            c <- Update{Store: store, Key: key, From: store[key], To: struct{}}
        }
        delete(*store, key)
    }
    db.Unlock()
}

// Create background task that regularly purges a store
func (db *NodeDB) purgeTask(store *Store, age time.Duration, interval time.Duration) {
    quit := db.task()
    go func() {
        for {
            select {
            case <-quit:
                return
            case <-time.After(interval):
                db.purge(store, age)
            }
        }
    }()
}

// Update gluon.NodeInfo entry
func (db *NodeDB) UpdateNodeInfo(data gluon.NodeInfo) {
    db.update(&db.nodeinfo, data.Source.String(), data)
}

// Update gluon.Statistics entry
func (db *NodeDB) UpdateStatistics(data gluon.Statistics) {
    db.update(&db.statistics, data.Source.String(), data)
    db.update(&db.gateways, data.Data.Gateway.String(), struct{}{})
}

// look up main ID for an alias
// Assumes that the database is already locked.
func (db *NodeDB) resolveAlias(alias string) (string, bool) {
    if resolve, ok := db.aliases[alias]; ok {
        return resolve.item.(string), true
    }
    return alias, false
}

// Update batadvvis.VisV1 data entry and update the alias Store, too.
func (db *NodeDB) UpdateVis(data batadvvis.VisV1) {
    db.update(&db.visdata, data.Mac.String(), data)
    mainaddr := data.Ifaces[0].Mac.String()
    db.update(&db.aliases, data.Mac.String(), mainaddr)
    for _, mac := range data.Ifaces {
        db.update(&db.aliases, mac.Mac.String(), mainaddr)
    }
}

func (db *NodeDB) task() chan struct{} {
    t := make(chan struct{})
    db.Lock()
    db.tasks = append(db.tasks, t)
    db.Unlock()
    return t
}

func (db *NodeDB) subscribeUpdates(s chan Update) {
    db.Lock()
    db.tasks = append(db.updatesubscriber, s)
    db.Unlock()
}

// Shut down database, end all running tasks.
func (db *NodeDB) Close() {
    db.Lock()
    for _, t := range db.tasks {
        t <- struct{}{}
        close(t)
    }
    db.Unlock()
}

package nodedb

import (
    "github.com/hwhw/mesh/gluon"
    "github.com/hwhw/mesh/batadvvis"
    "github.com/boltdb/bolt"
    "log"
    "time"
)

func (db *NodeDB) runLogger(file string) {
    bolt, err := bolt.Open(db.settings.LogStoreFile)
    if err != nil {
        log.Printf("Can not open data log store file %v: %v", db.settings.LogStoreFile, err)
        return
    }

    updates := make(chan Updates)
    quit := db.task
    db.subscribe(updates)
    go logger(bolt, updates, quit)
}

func logClientsTotal(bolt *bolt.DB, key string, timestamp time.Time, clients int) {
    log.Printf("at %v, we got new client count %d for %v", timestamp, clients, key)

}

func logger(bolt *bolt.DB, updates chan Updates, quit chan struct{}) {
    defer bolt.Close()
    for {
        select {
        case up := <-updates:
            switch to := up.To.(type) {
            case gluon.Statistics:
                from, isUpdate := up.From.(gluon.Statistics)
                if !isUpdate || from.Data.Clients.Total != to.Data.Clients.Total {
                    logClientsTotal(bolt, up.Key, to.TimeStamp, to.Data.Clients.Total)
                }
            case struct{}:
                switch from := up.From.(type) {
                case gluon.Statistics:
                    logClientsTotal(bolt, up.Key, time.Now(), -1)
                }
            }
        case <-quit:
            return
        }
    }
}

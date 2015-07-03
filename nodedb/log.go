package nodedb

import (
    "github.com/hwhw/mesh/boltdb"
    "github.com/boltdb/bolt"
    "log"
    "io"
    "time"
    "strconv"
    "errors"
    "encoding/json"
)

var ErrGotNoData = errors.New("no data logged")
var errUnchanged = errors.New("unchanged")

const (
    NODE_OFFLINE = -1
)

type ClientsTotal struct {
    Time    time.Time
    Clients int
}

type ClientsTotalList struct {
    ClientsTotal map[string][]ClientsTotal
    From time.Time
    Until time.Time
}

func (db *NodeDB) LogClientsTotal(key []byte, timestamp time.Time, clients int) error {
    // first, we check if this is a change to what we already logged
    return db.logstore.Update(func(tx *bolt.Tx) error {
        b, err := tx.Bucket(boltdb.Storekey(Clients)).CreateBucketIfNotExists(key)
        if err != nil {
            return err
        }
        c := b.Cursor()
        t := time.Time{}
        var k, v []byte
        for k, v = c.Last(); k != nil; k, v = c.Prev() {
            err := t.UnmarshalBinary(k)
            if err != nil {
                return err
            }
            if t.Before(timestamp) {
                break
            }
        }
        old, err := strconv.Atoi(string(v))
        if err == nil && clients == old {
            return nil
        }
        log.Printf("Logger: at %v, we got new client count %d for %v", timestamp, clients, key)
        time, err := timestamp.MarshalBinary()
        if err != nil {
            return err
        }
        b.Put(time, []byte(strconv.Itoa(clients)))
        return nil
    })
}

func (db *NodeDB) GetClientsTotal(key []byte, until time.Time, interval time.Duration) ([]ClientsTotal, error) {
    ret := make([]ClientsTotal, 0, 100)
    err := db.logstore.View(func(tx *bolt.Tx) error {
        bkey := tx.Bucket(boltdb.Storekey(Clients)).Bucket(key)
        if bkey == nil {
            return ErrGotNoData
        }
        cursor := bkey.Cursor()
        from := until.Add(-interval)
        for k, v := cursor.Last(); k != nil && v != nil; k, v = cursor.Prev() {
            t := time.Time{}
            err := t.UnmarshalBinary(k)
            if err != nil {
                return err
            }
            if !t.Before(until) {
                continue
            }
            if t.Before(from) {
                return nil
            }
            c, err := strconv.Atoi(string(v))
            if err != nil {
                return err
            }
            ret = append(ret, ClientsTotal{Time: t, Clients: c})
        }
        return nil
    })
    return ret, err
}

func (db *NodeDB) JSONClientsTotal(w io.Writer, key string, until time.Time, interval time.Duration) error {
    keys := make([][]byte, 0, 10)
    cdoc := ClientsTotalList{
        ClientsTotal: make(map[string][]ClientsTotal),
        From: until.Add(-interval),
        Until: until,
    }
    if key == "" {
        db.logstore.View(func(tx *bolt.Tx) error {
            tx.Bucket(boltdb.Storekey(Clients)).ForEach(func(k, v []byte) error {
                if v == nil {
                    keys = append(keys, k)
                }
                return nil
            })
            return nil
        })
    } else {
        keys = append(keys, []byte(key))
    }

    for _, k := range keys {
        clients, err := db.GetClientsTotal(k, until, interval)
        if err != nil {
            log.Printf("Logger: error when fetching clients_total:%v, continuing", key)
        }
        cdoc.ClientsTotal[string(k)] = clients
    }
    enc := json.NewEncoder(w)
    err := enc.Encode(cdoc)
    return err
}
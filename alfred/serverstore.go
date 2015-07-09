package alfred

// this is the implementation of the data storage part of an
// A.L.F.R.E.D. server

import (
    "github.com/tv42/topic"
    "time"
)

// for requests, this means to return data of all packettypes
const PACKETTYPE_ALL = 0

// a single entity in our store
type storeEntity struct {
    Invalid time.Time
    Local bool
    *Data
}

// the state of the data storage used by an A.L.F.R.E.D. server
type Store struct {
    req chan interface{}
    db map[string]map[uint8]*storeEntity
    purgeInterval time.Duration
    purgeAfter time.Duration
    NotifyUpdates *topic.Topic
}

// send this to the store to put data into it (or update data)
type ReqPut struct {
    // flag that tells if the data origin is the local server
    // instance (i.e. not propagated by another master server)
    IsLocal bool
    // actual data item
    Data
}

// send this to fetch data from the store
type ReqGetAll struct {
    // if PACKETTYPE_ALL, return data of all types, otherwise
    // specify type to return
    TypeFilter uint8
    // if set to true, return only data that has its origin
    // locally, i.e. was not propagated by another master server
    LocalOnly bool
    // data will be sent back via this channel, it will get
    // closed as soon as all data has been sent
    Return chan<- Data
}

// used internally for purging runs
type reqPurge struct{}

// return a new store instance
func NewStore(purgeAfter time.Duration, purgeInterval time.Duration) *Store {
    s := &Store{
        req: make(chan interface{}),
        db: make(map[string]map[uint8]*storeEntity),
        purgeAfter: purgeAfter,
        purgeInterval: purgeInterval,
        NotifyUpdates: topic.New(),
    }
    go s.dispatch()
    return s
}

// background task that will regularly trigger purge runs
func (s *Store) purge(quit <-chan struct{}) {
    select {
    case <-quit:
        return
    case <-time.After(s.purgeInterval):
        s.req <- reqPurge{}
    }
}

// background task that does the actual data storage
// and is fully synchronized by using channels
func (s *Store) dispatch() {
    quitPurge := make(chan struct{})
    go s.purge(quitPurge)
    for r := range s.req {
        switch r := r.(type) {
        case ReqPut:
            source := r.Data.Source.String()
            _, exists := s.db[source]
            if !exists {
                s.db[source] = make(map[uint8]*storeEntity)
            }
            t := r.Data.Header.Type
            i, exists := s.db[source][t]
            if !exists {
                i = &storeEntity{}
                s.db[source][t] = i
            }
            // can be set from false to true, but not the other
            // way around:
            if !i.Local {
                i.Local = r.IsLocal
            }
            i.Invalid = time.Now().Add(s.purgeAfter)
            if i.Data == nil || !i.Data.Equals(&r.Data) {
                s.NotifyUpdates.Broadcast <- r.Data
            }
            i.Data = &r.Data
        case ReqGetAll:
            for _, types := range s.db {
                if r.TypeFilter == PACKETTYPE_ALL {
                    for _, entity := range types {
                        if !r.LocalOnly || entity.Local {
                            r.Return <- *entity.Data
                        }
                    }
                } else {
                    entity, exists := types[r.TypeFilter]
                    if exists {
                        if !r.LocalOnly || entity.Local {
                            r.Return <- *entity.Data
                        }
                    }
                }
            }
            close(r.Return)
        case reqPurge:
            now := time.Now()
            restart:
            for source, types := range s.db {
                for t, entity := range types {
                    if entity.Invalid.Before(now) {
                        delete(s.db[source], t)
                        if len(s.db[source]) == 0 {
                            delete(s.db, source)
                        }
                        goto restart
                    }
                }
            }
        }
    }
    quitPurge <- struct{}{}
}

// shutdown data store and spawned tasks
func (s *Store) Shutdown() {
    close(s.req)
}

// send a request to the store
func (s *Store) Request(req interface{}) {
    s.req <- req
}

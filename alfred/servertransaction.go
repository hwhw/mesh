package alfred

import (
    "time"
    "log"
)

// A.L.F.R.E.D. server: transaction abstraction

// we keep track of open transactions. Every transaction's state data
// is stored in a struct like this
type transaction struct {
    start time.Time
    feed chan<- *PushDataV0
    abort chan<- interface{}
    complete chan<- uint16 // the final sequence number
}

// start a transaction with a given ID
// a signal channel to be triggered when the transaction is finished
// can be specified (nil if not needed)
// the islocal flag specifies if the data's origin is local (i.e. not
// propagated by another master server)
//
// the actual transaction is handled in a spawned sub-task that will
// collect data until signaled that the final packet has arrived.
// then it will check if the transaction was fully received and if so,
// it will store the data to the database.
func (s *Server) startTransaction(id uint16, done chan<- struct{}, islocal bool) *transaction {
    feed := make(chan *PushDataV0)
    abort := make(chan interface{})
    complete := make(chan uint16)
    reallycomplete := make(chan uint16)
    t := &transaction{
        start: time.Now(),
        feed: feed,
        abort: abort,
        complete: complete,
    }
    s.wg.Add(1)
    go func() {
        defer s.wg.Done()
        // keep track of the individual sequence parts
        seqmap := make(map[uint16]*PushDataV0)
        for {
            select {
            case <-abort:
                goto stop
            case finalseq := <-complete:
                // first, check if we got all packets
                for i := uint16(0); i < finalseq; i++ {
                    _, exists := seqmap[i]
                    if !exists {
                        goto wait
                    }
                }
                // spawn a sub-task to avoid blocking here
                s.wg.Add(1)
                go func() {
                    defer s.wg.Done()
                    reallycomplete <- finalseq
                }()
                break
            wait:
                // wait some time even after being marked as complete
                // to really finish things up -- we might get the
                // packets back in mixed order, which holds true for
                // the final packet, too.
                s.wg.Add(1)
                go func() {
                    defer s.wg.Done()
                    <-time.After(s.TransactionWaitComplete)
                    reallycomplete <- finalseq
                }()
            case finalseq := <-reallycomplete:
                // now, everything should have been arrived.
                if(s.DropIncompleteTransactions) {
                    for i := uint16(0); i < finalseq; i++ {
                        _, exists := seqmap[i]
                        if !exists {
                            log.Printf("alfred/server: dropping incomplete transaction %v", id)
                            goto stop
                        }
                    }
                }
                // put data into store
                for _, pd := range seqmap {
                    for _, d := range pd.Data {
                        s.store.Request(ReqPut{
                            IsLocal: islocal,
                            Data: d,
                        })
                    }
                }
                if done != nil {
                    // send signal that transaction is done
                    done <- struct{}{}
                }
                goto stop
            case pd := <-feed:
                // cache data for further handling as soon completion
                // is signaled
                seqmap[pd.Tx.SeqNo] = pd
            }
        }
    stop:
        s.Lock()
        delete(s.transactions, id)
        s.Unlock()
        log.Printf("alfred/server: finished transaction %v", id)
    }()
    return t
}

// scan the list of known/running transactions for a given transaction
// id and return a matching transaction if found. If not found,
// start a new transaction for that id and return it
func (s *Server) getTransaction(id uint16, islocal bool) transaction {
    s.Lock()
    defer s.Unlock()
    t, exists := s.transactions[id]
    if !exists {
        t = s.startTransaction(id, nil, islocal)
        s.transactions[id] = t
    }
    return *t
}

// create a new transaction that is locally initiated (before sending
// a request to another server). Ensure that we get a non-colliding
// transaction ID.
func (s *Server) initiateTransaction(done chan<- struct{}) uint16 {
    s.Lock()
    defer s.Unlock()
    var id uint16
    for {
        id = getRandomId()
        _, exists := s.transactions[id]
        if !exists {
            break
        }
    }
    t := s.startTransaction(id, done, false)
    s.transactions[id] = t
    return id
}

// background task for cleaning up old and stale transactions
func (s *Server) purgeTransactionTask(age time.Duration, interval time.Duration) {
    quit := make(chan interface{})
    s.notifyQuit.Register(quit)
    s.wg.Add(1)
    go func() {
        defer s.wg.Done()
        for {
            select {
            case <-quit:
                s.Lock()
                s.transactions = make(map[uint16]*transaction)
                s.Unlock()
                return
            case <-time.After(interval):
                s.Lock()
                maxage := time.Now().Add(-age)
            restart:
                for k, t := range s.transactions {
                    if t.start.Before(maxage) {
                        log.Printf("alfred/server: transaction %v timed out unfinished.", k)
                        delete(s.transactions, k)
                        goto restart
                    }
                }
                s.Unlock()
            }
        }
    }()
}

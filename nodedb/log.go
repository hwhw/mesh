package nodedb

import (
	"github.com/boltdb/bolt"
	"github.com/hwhw/mesh/store"
	"github.com/hwhw/mesh/alfred"
    "time"
    "errors"
    "log"
    "io"
    "encoding/json"
    "math"
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
            if c.GetTimestamp().Before(timestamp) {
                if c.GetCount() == count {
                    // not a new data point (no change)
                    logit = false
                }
                return false, errBreak
            }
            return false, nil
        })
        // we ignore errors, since often it will just be that we've never seen
        // the count's context before
        if logit {
            c.SetCount(count)
            c.SetTimestamp(timestamp)
            db.Logs.Put(tx, c)
        }
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
    lc := &CountMeshClients{Count{Timestamp: now, Count:clients}}
    db.logCount(lc)
    ln := &CountMeshNodes{Count{Timestamp:now, Count:nodes}}
    db.logCount(ln)
    log.Printf("Log: %d nodes with %d clients", nodes, clients)
    done <- struct{}{}
    return err
}

// generate a JSON list of nodes for which there is log data available
func (db *NodeDB) GenerateLogList(w io.Writer) error {
    enc := json.NewEncoder(w)
    list := make([]alfred.HardwareAddr, 0, 100)
    db.Logs.View(func(tx *bolt.Tx) error {
        return tx.ForEach(func(name []byte, b *bolt.Bucket) error {
            list = append(list, alfred.HardwareAddr(name))
            return nil
        })
    })
    enc.Encode(list)
    return nil
}

// fetch raw log data
//
// handler function might return "true" to abort reading further
func (db *NodeDB) ForEachLogEntry(logitem Counter, handler func() (bool, error)) error {
    return db.Logs.View(func(tx *bolt.Tx) error {
        return db.Logs.ForEachReverse(tx, logitem, func(cursor *bolt.Cursor) (bool, error) {
            return handler()
        })
    })
}

// Wrapper to generate JSON output for raw log data
func (db *NodeDB) GenerateLogJSON(w io.Writer, logitem Counter) error {
    enc := json.NewEncoder(w)
    first := true
    w.Write([]byte{'['})
    db.ForEachLogEntry(logitem, func() (bool, error) {
        if first {
            first = false
        } else {
            w.Write([]byte{','})
        }
        enc.Encode(logitem)
        return false, nil
    })
    w.Write([]byte{']'})
    return nil
}

// a sample of log data
type LogSample struct {
    // the start of the sample is the point in time from where we are going
    // backwards, i.e. into the past
    Start time.Time
    // part of the sample that the logged instance is considered offline
    Offline float64
    // part of the sample that the logged instance gave us invalid data
    Errorneous float64
    // a weighted (by the duration of it being valid) average of the log count
    WeightedAverage float64
    // minimum count within sample time
    Min float64
    // maximum count within sample time
    Max float64
}

// sample log data into a certain amount of samples, starting at
// a fixed point in time and going backwards from there, covering
// a given overall duration to take samples of
//
// For the reader of the following code: take your time, and read
// the documentation within the code carefully.
func (db *NodeDB) GetLogSamples(logitem Counter, start time.Time, over time.Duration, samples int, handler func(sample LogSample)) error {
    step := time.Duration(int64(over) / int64(samples))
    l := LogSample{
        Start: start,
        Min: math.Inf(1),
        Max: math.Inf(-1),
        WeightedAverage: 0.0,
    }
    last := time.Now()
    n := samples
    db.ForEachLogEntry(logitem, func() (bool, error) {
        t := logitem.GetTimestamp()
        for n > 0 {
            if l.Start.Before(t) {
                // newer than the point where we start looking back into history
                //                          LogItem ->
                // .........................|---------------
                // .....|----------------|..................
                // Start-Step          Start
                return false, nil
            }
            // log item's timestamp is earlier than start point.
            // Variants: (t------>now)
            //
            //                          LogItem ->
            //    ......................|-------------|.
            //                          t       last--^
            //    .............|----------------|.......
            //            Start-Step          Start
            // AND/OR:
            //                     LogItem
            //    .................|----|...............
            //                     t    ^--last
            //    .............|----------------|.......
            //            Start-Step          Start
            //
            // (1) this log item's count contributes to
            //     the current sample, but does not finish
            //     it
            //
            //              LogItem ->
            //    ..........|-------------|.............
            //              t             ^--last
            //    .............|----------------|.......
            //            Start-Step          Start
            // AND/OR
            //       LogItem ->
            //    ...|--------------------|.............
            //       t                    ^--last
            //    .....|----------------|...............
            //    Start-Step          Start
            //
            // (2) this log item's count contributes to
            //     the current sample *and* finishes it.

            weightstart := last
            if l.Start.Before(last) {
                // cut off logitem at sample's start time
                weightstart = l.Start
            }
            weightstop := t
            if t.Before(l.Start.Add(-step)) {
                // cut off logitem at sample's end time
                weightstop = l.Start.Add(-step)
            }
            weight := float64(weightstart.Sub(weightstop)) / float64(step)
            count := logitem.GetCount()
            switch count {
            case NODE_OFFLINE:
                count = 0
                l.Offline += weight
            case NODE_DATAERROR:
                count = 0
                l.Errorneous += weight
            }
            weightedcount := weight * float64(count)
            l.WeightedAverage += weightedcount
            l.Min = math.Min(l.Min, float64(count))
            l.Max = math.Max(l.Max, float64(count))
            if l.Start.Add(-step).Before(t) {
                // cases (1) above
                // go to next log entry
                last = t
                return false, nil
            } else {
                // cases (2) above
                // push out sample
                handler(l)
                n--
                l.Start = l.Start.Add(-step)
                l.Offline = 0.0
                l.Errorneous = 0.0
                l.WeightedAverage = 0.0
                l.Min = math.Inf(1)
                l.Max = math.Inf(-1)
                continue
            }
        }
        // when reaching this point, enough samples have been gathered.
        return false, errBreak
    })
    // time before we have log data is considered as being time where the
    // loggint instance has been offline
    for n > 0 {
        handler(l)
        n--
        l.Offline = 1.0
        l.Errorneous = 0.0
        l.WeightedAverage = 0.0
        l.Min = 0.0
        l.Max = 0.0
        l.Start = l.Start.Add(-step)
    }
    return nil
}

// generate a JSON list of samples
func (db *NodeDB) GenerateLogitemSamplesJSON(w io.Writer, logitem Counter, start time.Time, over time.Duration, samples int) error {
    enc := json.NewEncoder(w)
    samplelist := make([]LogSample, samples)
    db.GetLogSamples(logitem, start, over, samples, func(sample LogSample) {
        samplelist = append(samplelist, sample)
    })
    return enc.Encode(samplelist)
}

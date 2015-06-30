package nodedb

import (
    "github.com/hwhw/mesh/alfred"
    "github.com/hwhw/mesh/gluon"
    "github.com/hwhw/mesh/batadvvis"
    "time"
    "log"
)

// Create a new update client.
// Give the network (unix, tcp) in network, the address to connect to in address,
// the time to wait between updates in updatewait and the time to wait after failure
// before retrying in retrywait.
// The updatewait duration is also the timeout duration for the actual
// network connections.
func (db *NodeDB) NewUpdateClient(network string, address string, updatewait time.Duration, retrywait time.Duration) {
    quit := db.task()
    go func() {
        client := alfred.NewClient(network, address, updatewait)
        for {
            log.Printf("UpdateClient: Updating data from alfred server")
            retrybatadvvis:
            data, err := client.Request(batadvvis.PACKETTYPE)
            if err == nil {
                for _, d := range data {
                    vis, err := batadvvis.Read(d)
                    if err != nil {
                        log.Printf("UpdateClient: batadv-vis parse error: %+v (data: %+v)", err, d)
                    } else {
                        db.UpdateVis(vis)
                    }
                }
            } else {
                log.Printf("UpdateClient: Error fetching batadv-vis data, retrying in %s", retrywait)
                if !(wait(retrywait, quit)) { return }
                goto retrybatadvvis
            }

            retrynodeinfo:
            data, err = client.Request(gluon.NODEINFO_PACKETTYPE)
            if err == nil {
                for _, d := range data {
                    ni, err := gluon.ReadNodeInfo(d)
                    if err != nil {
                        log.Printf("UpdateClient: Gluon nodeinfo parse error: %+v (data: %+v)", err, d)
                    } else {
                        db.UpdateNodeInfo(ni)
                    }
                }
            } else {
                log.Printf("UpdateClient: Error fetching nodeinfo data, retrying in %s", retrywait)
                if !(wait(retrywait, quit)) { return }
                goto retrynodeinfo
            }

            retrystatistics:
            data, err = client.Request(gluon.STATISTICS_PACKETTYPE)
            if err == nil {
                for _, d := range data {
                    s, err := gluon.ReadStatistics(d)
                    if err != nil {
                        log.Printf("UpdateClient: Gluon statistics parse error: %+v (data: %+v)", err, d)
                    } else {
                        db.UpdateStatistics(s)
                    }
                }
            } else {
                log.Printf("UpdateClient: Error fetching statistics data, retrying in %s", retrywait)
                if !(wait(retrywait, quit)) { return }
                goto retrystatistics
            }

            log.Printf("UpdateClient: Done updating from alfred server, sleeping %s", updatewait)
            if updated != nil {
                (*updated)()
            }
            if !(wait(updatewait, quit)) { return }
        }
    }()
}

func wait(t time.Duration, quit chan struct{}) bool {
    select {
    case <-quit:
        return false
    case <-time.After(t):
        return true
    }
}

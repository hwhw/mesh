package alfred

// A.L.F.R.E.D. server: keep track of known masters

import (
    "log"
    "net"
    "time"
)

// keep the state information for a known master server
type master struct {
    address *net.UDPAddr
    listenerudp *listenerUDP
    lastseen time.Time
    firstseen time.Time
}

// purge masters that have not been seen for a certain amount of time
func (s *Server) purgeMasterTask(age time.Duration, interval time.Duration) {
    quit := make(chan interface{})
    s.notifyQuit.Register(quit)
    s.wg.Add(1)
    go func() {
        defer s.wg.Done()
        for {
            select {
            case <-quit:
                s.Lock()
                s.masters = make(map[string]*master)
                s.Unlock()
                return
            case <-time.After(interval):
                s.Lock()
                maxage := time.Now().Add(-age)
            restart:
                for k, m := range s.masters {
                    if m.lastseen.Before(maxage) {
                        log.Printf("alfred/server: master %v seems to be gone, removing.", m.address)
                        delete(s.masters, k)
                        goto restart
                    }
                }
                s.Unlock()
            }
        }
    }()
}

// add a master to our list
func (s *Server) addMaster(listener *listenerUDP, masteraddr *net.UDPAddr) {
    s.Lock()
    defer s.Unlock()
    _, exists := s.masters[masteraddr.String()]
    s.masters[masteraddr.String()] = &master{
        address: masteraddr,
        listenerudp: listener,
        lastseen: time.Now(),
    }
    if !exists {
        log.Printf("alfred/server: new master: %v", masteraddr)
    }
}

// return one master from the list
// for now, we return the one that we know the longest
func (s *Server) getPreferredMaster() *master {
    s.Lock()
    defer s.Unlock()
    m := master{}
    for _, master := range s.masters {
        // most currently seen one wins
        if m.address == nil || m.lastseen.Before(master.lastseen) {
            // make a copy
            m = *master
        }
    }
    return &m
}

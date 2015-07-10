package alfred

// an implementation of an A.L.F.R.E.D. server

import (
	"bufio"
	"github.com/tv42/topic"
	"log"
	"net"
	"sync"
	"time"
)

const (
	// slave mode: regularly forward local data
	SERVER_MODE_SLAVE = iota
	// master mode: receive and cache data from slaves, local data, and sync it all to all other known masters
	// announce presence via broadcasts
	SERVER_MODE_MASTER
	// special mode not present in reference implementation:
	// announce presence via unicast to other known masters
	// receive sync data from them, but only propagate local
	// data to one of them (like a slave server)
	SERVER_MODE_STEALTH_MASTER
)

// state information for an A.L.F.R.E.D. server
type Server struct {
	// this will typically be auto-calculated
	MaxPayload int
	// interval between sending master announcements in master modes
	AnnouncementInterval time.Duration
	// keep information about known masters for this duration after having last
	// heard of them
	MasterMaxAge time.Duration
	// interval between checks
	MasterPurgeInterval time.Duration
	// wait for outstanding packets for this time after
	// receiving final packet (which might arrive out of band)
	TransactionWaitComplete time.Duration
	// after this time, a started transaction is removed
	// from the list, regardless in what state it is
	TransactionMaxAge time.Duration
	// interval between checks
	TransactionPurgeInterval time.Duration
	// wait this duration for a reply of a master server
	// after forwarding a (client) request to it
	WaitForMasterReply time.Duration
	// interval between data synchronization pushes to other
	// master servers
	SyncInterval time.Duration
	// should we drop transactions that are not complete?
	// defaults to true, and should better be left that way
	// except your data/network policy mandates differently
	DropIncompleteTransactions bool
	// operation mode
	Mode int
	// data store
	store *Store
	// stores the first handled packet interface's hardware address
	// which is then used as the source identifier for local packets
	firstInterface HardwareAddr
	// spawned tasks can register here to get notified upon shutdown
	notifyQuit *topic.Topic
	// spawned tasks should register here to be waited for upon shutdown
	wg sync.WaitGroup
	// bookkeeping of transactions, masters, listeners
	transactions    map[uint16]*transaction
	masters         map[string]*master
	listenersudp    map[*listenerUDP]struct{}
	listenersstream map[*listenerStream]struct{}
	sync.Mutex
}

var maxDatagramSize = 0xFFFF

// run a new server instance
// Select an operation mode in "mode"
func NewServer(mode int) *Server {
	server := &Server{
		store:                    NewStore(time.Minute*10, time.Second*20),
		transactions:             make(map[uint16]*transaction),
		masters:                  make(map[string]*master),
		listenersudp:             make(map[*listenerUDP]struct{}),
		listenersstream:          make(map[*listenerStream]struct{}),
		MaxPayload:               maxDatagramSize - 8,
		AnnouncementInterval:     time.Second * 10,
		MasterMaxAge:             time.Minute * 2,
		MasterPurgeInterval:      time.Second * 30,
		TransactionWaitComplete:  time.Second * 5,
		TransactionMaxAge:        time.Second * 20,
		TransactionPurgeInterval: time.Second * 3,
		WaitForMasterReply:       time.Second * 10,
		SyncInterval:             time.Second * 10,
		Mode:                     mode,
		DropIncompleteTransactions: true,
		notifyQuit:                 topic.New(),
	}
	// start background tasks:
	server.purgeMasterTask(server.MasterMaxAge, server.MasterPurgeInterval)
	server.purgeTransactionTask(server.TransactionMaxAge, server.TransactionPurgeInterval)
	server.announceMaster(server.AnnouncementInterval)
	server.syncToMasters(server.SyncInterval)
	return server
}

// shutdown a server
func (s *Server) Shutdown() {
	log.Printf("alfred/server: shutting down")
	// shut down all socket listeners
	s.Lock()
	for l, _ := range s.listenersudp {
		l.quit <- struct{}{}
	}
	for l, _ := range s.listenersstream {
		l.quit <- struct{}{}
	}
	s.Unlock()
	// shutdown all background tasks
	log.Printf("alfred/server: waiting for tasks to finish")
	s.notifyQuit.Broadcast <- struct{}{}
	// shutdown the data store
	s.store.Shutdown()
	// wait for everything to finish
	s.wg.Wait()
}

// generic data sender task
// this will send data items arriving on a channel over a given connection,
// setting the transaction ID to a given value.
// it will operate in "single data packet" mode for communicating with
// stream clients and will collect data for sending large packets
// over packet sockets (UDP).
func (s *Server) sendData(c net.Conn, txid uint16, data <-chan Data, single bool) {
	defer c.Close()
	cbuf := bufio.NewWriter(c)
	tm := &TransactionMgmt{Id: txid, SeqNo: 0}
	pd := NewPushDataV0(tm, make([]Data, 0))
	for d := range data {
		if (single && len(pd.Data) > 0) || (pd.Size()+d.Size() > s.MaxPayload) {
			// adding this packet would lead to a too large packet, so
			// first send what we have collected for now
			err := pd.Write(cbuf)
			if err == nil {
				err = cbuf.Flush()
			}
			if err != nil {
				log.Printf("alfred/server: error sending data to %v: %v", c, err)
				return
			}
			// reset data store
			pd.Data = make([]Data, 0)
			// increment sequence number
			pd.Tx.SeqNo++
		}
		// add new data packet to our collection
		pd.Data = append(pd.Data, d)
		if !single && pd.Size() > s.MaxPayload {
			log.Printf("alfred/server: got a too large data packet, will not propagate it.")
			pd.Data = make([]Data, 0)
		}
	}
	// check if there's unsent data cached
	if len(pd.Data) > 0 {
		// yes, so send it
		err := pd.Write(cbuf)
		if err == nil {
			err = cbuf.Flush()
		}
		if err != nil {
			log.Printf("alfred/server: error sending data to %v: %v", c, err)
			return
		}
		tm.SeqNo++
	}
	if !single && tm.SeqNo > 0 {
		// when in packet transport mode (UDP), send a final
		// packet that signals completion (and states the number of
		// sequential packets that were sent)
		stat := NewStatusV0(ALFRED_STATUS_TXEND, tm)
		err := stat.Write(cbuf)
		if err == nil {
			err = cbuf.Flush()
		}
		if err != nil {
			log.Printf("alfred/server: error sending data to %v: %v", c, err)
			return
		}
	}
	return
}

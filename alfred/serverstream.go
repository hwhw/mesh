package alfred

// A.L.F.R.E.D. server: stream (TCP, Unix) connection specifics

import (
	"bufio"
	"log"
	"net"
	"sync"
	"time"
)

// this is for stream (TCP, Unix) sockets' state
type listenerStream struct {
	listener net.Listener
	wg       sync.WaitGroup
	quit     chan struct{}
}

// passes a connection to the data sender, returns
// the data channel it got from the data sender
func (s *Server) dataSenderStream(c net.Conn, txid uint16) (chan<- Data, error) {
	data := make(chan Data, 100)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.sendData(c, txid, data, true)
	}()
	return data, nil
}

// listen on a socket and spawn handling tasks for each
// incoming connection
func (s *Server) listenStream(l *listenerStream) {
	for {
		conn, err := l.listener.Accept()
		if err != nil {
			log.Printf("alfred/server: error accepting connection on %v: %v, stop listening", l.listener, err)
			return
		}
		l.wg.Add(1)
		go func() {
			defer l.wg.Done()
			s.readStream(conn)
		}()
	}
}

// handle a single connection
func (s *Server) readStream(conn net.Conn) {
	pkg, err, _ := Read(conn)
	if err != nil {
		log.Printf("alfred/server: cannot parse data just received: %v (%+v)", err, pkg)
		return
	}
	switch pkg := pkg.(type) {
	case *RequestV0:
		log.Printf("alfred/server: got a request!")
		if s.Mode == SERVER_MODE_SLAVE {
			if m := s.getPreferredMaster(); m.address != nil {
				// forward request to master
				done := make(chan struct{})
				id := s.initiateTransaction(done)
				request(m.address, NewRequestV0(pkg.RequestedType, id))
				select {
				case <-time.After(s.WaitForMasterReply):
					log.Printf("alfred/server: timeout when waiting for reply to forwarded request")
					goto reporterror
				case <-done:
					goto senddata
				}
			} else {
				log.Printf("alfred/server: got a request, but knows of no master. reporting an error.")
				goto reporterror
			}
		reporterror:
			stat := NewStatusV0(ALFRED_STATUS_ERROR, &TransactionMgmt{Id: pkg.TxId, SeqNo: 0})
			cbuf := bufio.NewWriter(conn)
			stat.Write(cbuf)
			cbuf.Flush()
			conn.Close()
			return
		}
	senddata:
		c, err := s.dataSenderStream(conn, pkg.TxId)
		if err == nil {
			s.store.Request(ReqGetAll{TypeFilter: pkg.RequestedType, LocalOnly: false, Return: c})
		}
		// connection will be closed by data send routine
	case *PushDataV0:
		defer conn.Close()
		log.Printf("alfred/server: got data")
		for i, _ := range pkg.Data {
			if pkg.Data[i].Source.IsUnset() {
				if s.firstInterface == nil {
					log.Printf("alfred/server: want to store data but don't have an interface address, skipping.")
					return
				} else {
					pkg.Data[i].Source = s.firstInterface
				}
			}
		}
		t := s.getTransaction(pkg.Tx.Id, true)
		t.feed <- pkg
		t.complete <- pkg.Tx.SeqNo + 1
	case *ModeSwitchV0:
		defer conn.Close()
		switch pkg.Mode {
		case ALFRED_MODESWITCH_SLAVE:
			log.Printf("alfred/server: switching mode to slave mode")
			s.Mode = SERVER_MODE_SLAVE
		case ALFRED_MODESWITCH_MASTER:
			log.Printf("alfred/server: switching mode to master mode")
			s.Mode = SERVER_MODE_MASTER
		case SERVER_MODE_STEALTH_MASTER:
			log.Printf("alfred/server: switching mode to stealth master mode")
			s.Mode = SERVER_MODE_STEALTH_MASTER
		}
	case *ChangeInterfaceV0:
		defer conn.Close()
		log.Printf("alfred/server: got a request to change interfaces - unimplemented!")
	}
}

// start a task for listening on a TCP or Unix socket
func (s *Server) NewListenerStream(network string, address string) error {
	listener, err := net.Listen(network, address)
	if err != nil {
		return err
	}
	l := &listenerStream{
		listener: listener,
		quit:     make(chan struct{}),
	}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		l.wg.Add(1)
		go func() {
			defer l.wg.Done()
			s.listenStream(l)
		}()
		<-l.quit
		l.listener.Close()
		l.wg.Wait()
		s.Lock()
		defer s.Unlock()
		delete(s.listenersstream, l)
	}()
	s.Lock()
	defer s.Unlock()
	s.listenersstream[l] = struct{}{}
	return nil
}

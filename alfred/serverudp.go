package alfred

import (
	"bufio"
	"bytes"
	"log"
	"net"
	"time"
)

// A.L.F.R.E.D. server: UDP connection specifics

// keep track of UDP sockets to listen on in these structs
type listenerUDP struct {
	iface  *net.Interface
	ifname string
	addr   *net.UDPAddr
	listen *net.UDPConn
	quit   chan struct{}
}

// small function to send announcements
func announce(dst *net.UDPAddr) {
	c, err := net.DialUDP("udp", nil, dst)
	if err != nil {
		log.Printf("alfred/server: cannot send announcements to %v: %v", dst, err)
	} else {
		cbuf := bufio.NewWriter(c)
		log.Printf("alfred/server: announce to %v", dst)
		a := NewAnnounceMasterV0()
		a.Write(cbuf)
		cbuf.Flush()
		c.Close()
	}
}

// task to send Master announcements regularly
func (s *Server) announceMaster(interval time.Duration) {
	quit := make(chan interface{})
	s.notifyQuit.Register(quit)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-quit:
				return
			case <-time.After(interval):
				switch s.Mode {
				case SERVER_MODE_STEALTH_MASTER:
					m := s.getPreferredMaster()
					if m.address != nil {
						announce(m.address)
					}
				case SERVER_MODE_MASTER:
					s.Lock()
					for l, _ := range s.listenersudp {
						announce(l.addr)
					}
					s.Unlock()
				}
			}
		}
	}()
}

// small function to send requests
func request(dst *net.UDPAddr, req *RequestV0) {
	c, err := net.DialUDP("udp", nil, dst)
	if err != nil {
		log.Printf("alfred/server: cannot send request to %v: %v", dst, err)
	} else {
		cbuf := bufio.NewWriter(c)
		log.Printf("alfred/server: send request to %v", dst)
		req.Write(cbuf)
		cbuf.Flush()
		c.Close()
	}
}

// task for pushing data to other masters regularly
func (s *Server) syncToMasters(interval time.Duration) {
	quit := make(chan interface{})
	s.notifyQuit.Register(quit)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		for {
			select {
			case <-quit:
				return
			case <-time.After(interval):
				switch s.Mode {
				case SERVER_MODE_SLAVE, SERVER_MODE_STEALTH_MASTER:
					m := s.getPreferredMaster()
					if m.address != nil {
						log.Printf("alfred/server: syncing to %+v", m.address)
						c, err := s.dataSenderUDP(m.address, getRandomId())
						if err == nil {
							// only push local data
							s.store.Request(ReqGetAll{TypeFilter: PACKETTYPE_ALL, LocalOnly: true, Return: c})
						}
					}
				case SERVER_MODE_MASTER:
					s.Lock() // because we loop on listener list
					for l, _ := range s.listenersudp {
						c, err := s.dataSenderUDP(l.addr, getRandomId())
						if err == nil {
							// push all known data
							s.store.Request(ReqGetAll{TypeFilter: PACKETTYPE_ALL, LocalOnly: false, Return: c})
						}
					}
					s.Unlock()
				}
			}
		}
	}()
}

// opens a connection and passes it to the data sender, returns
// the data channel it got from the data sender
func (s *Server) dataSenderUDP(dst net.Addr, txid uint16) (chan<- Data, error) {
	addr, _ := dst.(*net.UDPAddr)
	c, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		log.Printf("alfred/server: cannot send to %v: %v", dst, err)
		return nil, err
	}
	data := make(chan Data, 100)
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.sendData(c, txid, data, false)
	}()
	return data, nil
}

// read and handle a UDP packet
func (s *Server) readUDP(l *listenerUDP) error {
	for {
		back := make([]byte, maxDatagramSize)
		_, src, err := l.listen.ReadFromUDP(back)
		if err != nil {
			log.Printf("alfred/server: error: %v, UDP Reader shutting down", err)
			return err
		}
		if src.Zone != l.ifname {
			continue
		}
		buf := bytes.NewBuffer(back)
		pkg, err, _ := Read(buf)
		if err != nil {
			log.Printf("alfred/server: cannot parse data just received.")
			return err
		}
		switch pkg := pkg.(type) {
		case *AnnounceMasterV0:
			log.Printf("alfred/server: got master announcement from %+v on %+v", src, l)
			s.addMaster(l, src)
		case *PushDataV0:
			log.Printf("alfred/server: got push data, transaction %+v", pkg.Tx)
			t := s.getTransaction(pkg.Tx.Id, false)
			t.feed <- pkg
		case *StatusV0:
			log.Printf("alfred/server: got status %+v %+v", pkg.Header, pkg.Tx)
			t := s.getTransaction(pkg.Tx.Id, false)
			switch pkg.Header.Type {
			case ALFRED_STATUS_TXEND:
				t.complete <- pkg.Tx.SeqNo
			case ALFRED_STATUS_ERROR:
				t.abort <- struct{}{}
			}
		case *RequestV0:
			log.Printf("alfred/server: got request %+v", pkg)
			c, err := s.dataSenderUDP(src, pkg.TxId)
			if err == nil {
				s.store.Request(ReqGetAll{TypeFilter: pkg.RequestedType, LocalOnly: false, Return: c})
			}
		default:
			log.Printf("alfred/server: got packet: %+v", pkg)
		}
	}
}

// spawn a task that listens of incoming UDP packets
func (s *Server) NewListenerUDP(address string, ifname string) error {
	iface, err := net.InterfaceByName(ifname)
	if err != nil {
		return err
	}
	if s.firstInterface == nil {
		s.firstInterface = HardwareAddr(iface.HardwareAddr)
	}
	addr, err := net.ResolveUDPAddr("udp", address)
	if err != nil {
		return err
	}
	listen, err := net.ListenMulticastUDP("udp", iface, addr)
	if err != nil {
		return err
	}
	listen.SetReadBuffer(maxDatagramSize)
	l := &listenerUDP{iface: iface, ifname: ifname, addr: addr, listen: listen, quit: make(chan struct{})}
	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.readUDP(l)
		}()
		<-l.quit
		l.listen.Close()
		s.Lock()
		delete(s.listenersudp, l)
		s.Unlock()
	}()
	s.Lock()
	s.listenersudp[l] = struct{}{}
	s.Unlock()
	return nil
}

// Package batadvvis provides types for "vis" packets
// containing "batman-adv" protocol related metadata
// distributed via the A.L.F.R.E.D. daemon
package batadvvis

import (
	"errors"
	"github.com/hwhw/mesh/alfred"
)

const (
	// A.L.F.R.E.D. packet type ID for vis data
	PACKETTYPE = 1
	// A.L.F.R.E.D. packet version for vis data
	PACKETVERSION = 1
)

var ErrParse = errors.New("parse error")

// vis data item
type VisV1 struct {
	Mac       alfred.HardwareAddr
	Iface_n   uint8
	Entries_n uint8
	Ifaces    []Iface
	Entries   []Entry
}

type Iface struct {
	Mac alfred.HardwareAddr
}

type Entry struct {
	Mac     alfred.HardwareAddr
	IfIndex uint8
	Qual    uint8
}

// read structured information from A.L.F.R.E.D. packet
func (vis *VisV1) ReadAlfred(data alfred.Data) error {
	if data.Header.Type != PACKETTYPE {
		return ErrParse
	}
	if data.Header.Version != PACKETVERSION {
		return ErrParse
	}
	payload := data.Data[:]
	if len(payload) < 8 {
		return ErrParse
	}

	/* disabled:
	   vis.Mac = payload[:6]
	   /* MAC is stored here instead: */
	vis.Mac = data.Source
	vis.Iface_n = payload[6]
	vis.Entries_n = payload[7]
	vis.Ifaces = make([]Iface, vis.Iface_n)
	vis.Entries = make([]Entry, vis.Entries_n)

	if vis.Iface_n < 1 {
		return ErrParse
	}
	payload = payload[8:]
	for i := 0; i < (int)(vis.Iface_n); i++ {
		if len(payload) < 6 {
			return ErrParse
		}
		vis.Ifaces[i] = Iface{payload[:6]}
		payload = payload[6:]
	}
	for i := 0; i < (int)(vis.Entries_n); i++ {
		if len(payload) < 8 {
			return ErrParse
		}
		vis.Entries[i] = Entry{payload[:6], payload[6], payload[7]}
		payload = payload[8:]
	}
	return nil
}

func (vis *VisV1) GetPacketType() uint8 {
	return PACKETTYPE
}

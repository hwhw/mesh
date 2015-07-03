// Package batadvvis provides types for "vis" packets
// containing "batman-adv" protocol related metadata
// distributed via the A.L.F.R.E.D. daemon
package batadvvis

import (
    "github.com/hwhw/mesh/alfred"
    "net"
    "errors"
    "encoding/gob"
    "bytes"
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
    Mac         net.HardwareAddr
    Iface_n     uint8
    Entries_n   uint8
    Ifaces      []Iface
    Entries     []Entry
}

type Iface struct {
    Mac         net.HardwareAddr
}

type Entry struct {
    Mac         net.HardwareAddr
    IfIndex     uint8
    Qual        uint8
}

// read structured information from A.L.F.R.E.D. packet
func Read(data alfred.Data) (*VisV1, error) {
    vis := VisV1{}
    if data.Header.Type != PACKETTYPE {
        return &vis, ErrParse
    }
    if data.Header.Version != PACKETVERSION {
        return &vis, ErrParse
    }
    payload := data.Data[:]
    if len(payload) < 8 {
        return &vis, ErrParse
    }
    /* disabled:
    vis.Mac = payload[:6]
    /* MAC is stored here instead: */
    vis.Mac = data.Source
    vis.Iface_n = payload[6]
    vis.Entries_n = payload[7]
    if(vis.Iface_n < 1) {
        return &vis, ErrParse
    }
    payload = payload[8:]
    for i := 0; i < (int)(vis.Iface_n); i++ {
        if len(payload) < 6 {
            return &vis, ErrParse
        }
        vis.Ifaces = append(vis.Ifaces, Iface{payload[:6]})
        payload = payload[6:]
    }
    for i := 0; i < (int)(vis.Entries_n); i++ {
        if len(payload) < 8 {
            return &vis, ErrParse
        }
        vis.Entries = append(vis.Entries, Entry{payload[:6], payload[6], payload[7]})
        payload = payload[8:]
    }
    return &vis, nil
}

func (v *VisV1) Bytes() ([]byte, error) {
    itembuf := new(bytes.Buffer)
    enc := gob.NewEncoder(itembuf)
    err := enc.Encode(v)
    if err != nil {
        return nil, err
    }
    return itembuf.Bytes(), nil
}

func (v *VisV1) Key() ([]byte) {
    return []byte(v.Ifaces[0].Mac)
}

func (v *VisV1) DeserializeFrom(b []byte) error {
    buf := bytes.NewBuffer(b)
    dec := gob.NewDecoder(buf)
    // we make an explicit copy since the gob decoder will make weird modification to existing data
    newvis := VisV1{}
    err := dec.Decode(&newvis)
    v.Mac = newvis.Mac
    v.Iface_n = newvis.Iface_n
    v.Entries_n = newvis.Entries_n
    v.Ifaces = newvis.Ifaces
    v.Entries = newvis.Entries
    return err
}

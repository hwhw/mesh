// Package gluon provides types for Gluon-firmware style
// mesh node metadata and means to (un)serialize it.
package gluon

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"github.com/hwhw/mesh/alfred"
	"net"
)

var ErrParse = errors.New("parse error")
var ErrParseMAC = errors.New("error parsing MAC address")

// Wrapper for the MAC addresses found as main identifier.
type HardwareAddr net.HardwareAddr

// wrap printing from net.HardwareAddr
func (i HardwareAddr) String() string {
	return net.HardwareAddr(i).String()
}

// JSON encoder for MAC addresses
func (i HardwareAddr) MarshalJSON() ([]byte, error) {
	buf := new(bytes.Buffer)
	buf.WriteByte('"')
	buf.WriteString(net.HardwareAddr(i).String())
	buf.WriteByte('"')
	return buf.Bytes(), nil
}

// very forgiving parser for MAC addresses
func (i *HardwareAddr) UnmarshalJSON(data []byte) error {
	addr := make([]byte, 6)
	n := 0
	v := byte(0)
	for _, c := range data {
		switch {
		case c >= 0x30 && c <= 0x39:
			v = c - 0x30
		case c >= 0x41 && c <= 0x46:
			v = c - 0x41 + 0xA
		case c >= 0x61 && c <= 0x66:
			v = c - 0x61 + 0xA
		default:
			continue
		}
		if n >= 12 {
			// more than 12 hex chars
			return ErrParseMAC
		}
		if 0 == n%2 {
			addr[n>>1] = v << 4
		} else {
			addr[n>>1] += v
		}
		n++
	}
	if n == 12 {
		*i = HardwareAddr(addr)
		return nil
	}
	return ErrParseMAC
}

// convenience function that will compare packet type and version
// and also care for proper unzipping and deserializing JSON data
func readJSON(data alfred.Data, packetType uint8, packetVersion uint8, v interface{}) error {
	if data.Header.Type != packetType {
		return ErrParse
	}
	if data.Header.Version != packetVersion {
		return ErrParse
	}
	unzip, err := gzip.NewReader(bytes.NewReader(data.Data))
	if err != nil {
		return err
	}
	dec := json.NewDecoder(unzip)
	err = dec.Decode(v)
	if err != nil {
		return err
	}
	return nil
}

// Package gluon provides types for Gluon-firmware style
// mesh node metadata and means to (un)serialize it.
package gluon

import (
    "github.com/hwhw/mesh/alfred"
    "errors"
    "bytes"
    "compress/gzip"
    "encoding/json"
    "net"
)

var ErrParse = errors.New("parse error")

// Wrapper for the MAC addresses found as main identifier.
type HardwareAddr net.HardwareAddr

func (i HardwareAddr)String() string {
    return net.HardwareAddr(i).String()
}

func (i HardwareAddr)MarshalJSON() ([]byte, error) {
    buf := new(bytes.Buffer)
    buf.WriteByte('"')
    buf.WriteString(net.HardwareAddr(i).String())
    buf.WriteByte('"')
    return buf.Bytes(), nil
}

func hextoi(b byte) (byte, error) {
    switch {
    case b >= 0x30 && b <= 0x39:
        return b - 0x30, nil
    case b >= 0x41 && b <= 0x46:
        return 10 + b - 0x41, nil
    case b >= 0x61 && b <= 0x66:
        return 10 + b - 0x61, nil
    default:
        return 0, ErrParse
    }
}

func (i *HardwareAddr)UnmarshalJSON(val []byte) (error) {
    if val[0] == '"' {
        val = val[1:len(val)-1]
    }
    mac, err := net.ParseMAC(string(val))
    if err == nil {
        *i = HardwareAddr(mac)
        return nil
    }
    if len(val) == 12 {
        mac = make(net.HardwareAddr, 6)
        for i := 0; i < 6; i ++ {
            v, err := hextoi(val[i*2])
            if err != nil {
                return err
            }
            mac[i] = v * 0x10
            v, err = hextoi(val[i*2+1])
            if err != nil {
                return err
            }
            mac[i] += v
        }
        *i = HardwareAddr(mac)
        return nil
    }
    return ErrParse
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

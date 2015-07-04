// Package gluon provides types for Gluon-firmware style
// mesh node metadata and means to (un)serialize it.
package gluon

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"github.com/hwhw/mesh/alfred"
)

var ErrParse = errors.New("parse error")

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

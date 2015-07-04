package nodedb

import (
	"bytes"
	"encoding/gob"
	"errors"
	"github.com/hwhw/mesh/alfred"
	"github.com/hwhw/mesh/batadvvis"
	"github.com/hwhw/mesh/gluon"
	"github.com/hwhw/mesh/store"
	"time"
)

var ErrInvalid = errors.New("invalid data")

type NodeInfo struct {
	store.ContainedKey
	gluon.NodeInfo
}

func (n *NodeInfo) Bytes() ([]byte, error) {
	itembuf := new(bytes.Buffer)
	enc := gob.NewEncoder(itembuf)
	err := enc.Encode(n.NodeInfo)
	return itembuf.Bytes(), err
}
func (n *NodeInfo) Key() []byte {
	return []byte(n.NodeInfo.Data.NodeID)
}

var nodeInfoStoreID = []byte("NodeInfo")

func (n *NodeInfo) StoreID() []byte {
	return nodeInfoStoreID
}
func (n *NodeInfo) DeserializeFrom(b []byte) error {
	buf := bytes.NewBuffer(b)
	dec := gob.NewDecoder(buf)
	n.NodeInfo = gluon.NodeInfo{}
	err := dec.Decode(&n.NodeInfo)
	return err
}

type Statistics struct {
	store.ContainedKey
	gluon.Statistics
}

func (s *Statistics) Bytes() ([]byte, error) {
	itembuf := new(bytes.Buffer)
	enc := gob.NewEncoder(itembuf)
	err := enc.Encode(s.Statistics)
	return itembuf.Bytes(), err
}
func (s *Statistics) Key() []byte {
	return []byte(s.Data.NodeID)
}

var statisticsStoreID = []byte("Statistics")

func (n *Statistics) StoreID() []byte {
	return statisticsStoreID
}
func (s *Statistics) DeserializeFrom(b []byte) error {
	buf := bytes.NewBuffer(b)
	dec := gob.NewDecoder(buf)
	s.Statistics = gluon.Statistics{}
	err := dec.Decode(&s.Statistics)
	return err
}

type VisData struct {
	store.ContainedKey
	batadvvis.VisV1
}

func (v *VisData) Bytes() ([]byte, error) {
	itembuf := new(bytes.Buffer)
	enc := gob.NewEncoder(itembuf)
	err := enc.Encode(v.VisV1)
	return itembuf.Bytes(), err
}
func (v *VisData) Key() []byte {
	return []byte(v.VisV1.Ifaces[0].Mac)
}

var visdataStoreID = []byte("VisData")

func (n *VisData) StoreID() []byte {
	return visdataStoreID
}
func (v *VisData) DeserializeFrom(b []byte) error {
	buf := bytes.NewBuffer(b)
	dec := gob.NewDecoder(buf)
	v.VisV1 = batadvvis.VisV1{}
	err := dec.Decode(&v.VisV1)
	return err
}

type Alias struct{ store.Byte }

var aliasStoreID = []byte("Aliases")

func NewAlias(alias alfred.HardwareAddr, main alfred.HardwareAddr) *Alias {
	a := &Alias{}
	a.SetKey(alias)
	a.Set(main)
	return a
}
func (a *Alias) StoreID() []byte {
	return aliasStoreID
}

type Gateway struct{ store.Flag }

var gatewayStoreID = []byte("Gateways")

func (g *Gateway) StoreID() []byte {
	return gatewayStoreID
}

type Counter interface {
    store.Item
    GetTimestamp() time.Time
    SetTimestamp(time.Time)
    GetCount() int
    SetCount(int)
}

type Count struct {
	timestamp time.Time
	count   int
}

func (n *Count) Key() []byte {
	m, err := n.timestamp.MarshalBinary()
	if err != nil {
		panic("can not marshal timestamp")
	}
	return m
}
func (n *Count) SetKey(k []byte) {
	err := n.timestamp.UnmarshalBinary(k)
	if err != nil {
		panic("can not marshal timestamp")
	}
}
func (n *Count) GetTimestamp() time.Time {
    return n.timestamp
}
func (n *Count) SetTimestamp(timestamp time.Time) {
    n.timestamp = timestamp
}
func (n *Count) GetCount() int {
    return n.count
}
func (n *Count) SetCount(count int) {
    n.count = count
}
func (n *Count) Bytes() ([]byte, error) {
	c := []byte{
		byte(n.count & 0xFF),
		byte(n.count & 0xFF00 >> 8),
		byte(n.count & 0xFF0000 >> 16),
		byte(n.count & 0x7F000000 >> 24)}
	return c, nil
}
func (n *Count) DeserializeFrom(d []byte) error {
	if len(d) != 4 {
		return ErrInvalid
	}
	n.count = int(d[0]) + int(d[1])<<8 + int(d[2])<<16 + int(d[3])<<24
	return nil
}

type CountNodeClients struct {
	Count
	node alfred.HardwareAddr
}
func NewCountNodeClients(node alfred.HardwareAddr, timestamp time.Time, count int) *CountNodeClients {
	n := &CountNodeClients{node: node, Count: Count{timestamp: timestamp, count: count}}
	return n
}
func (c *CountNodeClients) StoreID() []byte {
	return c.node
}

type CountMeshClients struct{ Count }
var countmeshclientsStoreID = []byte("MeshClients")
func (c *CountMeshClients) StoreID() []byte {
	return countmeshclientsStoreID
}

type CountMeshNodes struct{ Count }
var countmeshnodesStoreID = []byte("MeshNodes")
func (c *CountMeshNodes) StoreID() []byte {
	return countmeshnodesStoreID
}

package nodedb

import (
	"bytes"
	"encoding/binary"
	"encoding/gob"
	"errors"
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
	return []byte(n.NodeInfo.Source)
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
	return []byte(s.Source)
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
	return []byte(v.VisV1.Mac)
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

type NodeID struct{ store.Byte }

var nodeIDStoreID = []byte("NodeID")

func (n *NodeID) StoreID() []byte {
	return nodeIDStoreID
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
	Timestamp time.Time
	Count     int
}

func (n *Count) Key() []byte {
	m, err := n.Timestamp.MarshalBinary()
	if err != nil {
		panic("can not marshal timestamp")
	}
	return m
}
func (n *Count) SetKey(k []byte) {
	err := n.Timestamp.UnmarshalBinary(k)
	if err != nil {
		panic("can not marshal timestamp")
	}
}
func (n *Count) GetTimestamp() time.Time {
	return n.Timestamp
}
func (n *Count) SetTimestamp(timestamp time.Time) {
	n.Timestamp = timestamp
}
func (n *Count) GetCount() int {
	return n.Count
}
func (n *Count) SetCount(count int) {
	n.Count = count
}
func (n *Count) Bytes() ([]byte, error) {
	c := make([]byte, 10)
	i := binary.PutVarint(c, int64(n.Count))
	return c[0:i], nil
}
func (n *Count) DeserializeFrom(d []byte) error {
	val, i := binary.Varint(d)
	if i <= 0 {
		return ErrInvalid
	}
	n.Count = int(val)
	return nil
}

type CountNodeClients struct {
	Count
	Node string
}

func NewCountNodeClients(node string, timestamp time.Time, count int) *CountNodeClients {
	n := &CountNodeClients{Node: node, Count: Count{Timestamp: timestamp, Count: count}}
	return n
}
func (c *CountNodeClients) StoreID() []byte {
	return []byte(c.Node)
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

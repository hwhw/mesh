package gluon

import (
	"bytes"
	"encoding/gob"
	"github.com/hwhw/mesh/alfred"
	"time"
)

const (
	// A.L.F.R.E.D. packet type ID for Gluon Statistics data
	STATISTICS_PACKETTYPE = 159
	// A.L.F.R.E.D. packet version for Gluon Statistics data
	STATISTICS_PACKETVERSION = 0
)

// wrapper type for storing the statistics data and its origin
type Statistics struct {
	Source    HardwareAddr
	Data      StatisticsData
	TimeStamp time.Time
}

// mesh node statistics
type StatisticsData struct {
	NodeID      *HardwareAddr `json:"node_id,omitempty"`
	Clients     *Clients      `json:"clients,omitempty"`
	RootFSUsage float64       `json:"rootfs_usage,omitempty"`
	LoadAvg     float64       `json:"loadavg,omitempty"`
	Uptime      float64       `json:"uptime,omitempty"`
	IdleTime    float64       `json:"idletime,omitempty"`
	Gateway     *HardwareAddr `json:"gateway,omitempty"`
	Processes   *Processes    `json:"processes,omitempty"`
	Traffic     *Traffic      `json:"traffic,omitempty"`
	Memory      *Memory       `json:"memory,omitempty"`
}

type Clients struct {
	Wifi  int `json:"wifi,omitempty"`
	Total int `json:"total,omitempty"`
}

type Memory struct {
	Cached  int `json:"cached,omitempty"`
	Total   int `json:"total,omitempty"`
	Buffers int `json:"buffers,omitempty"`
	Free    int `json:"free,omitempty"`
}

type Processes struct {
	Total   int `json:"total,omitempty"`
	Running int `json:"running,omitempty"`
}

type Traffic struct {
	Tx      *TrafficCounter `json:"tx,omitempty"`
	Rx      *TrafficCounter `json:"rx,omitempty"`
	MgmtTx  *TrafficCounter `json:"mgmt_tx,omitempty"`
	MgmtRx  *TrafficCounter `json:"mgmt_rx,omitempty"`
	Forward *TrafficCounter `json:"forward,omitempty"`
}

type TrafficCounter struct {
	Bytes   int `json:"bytes,omitempty"`
	Packets int `json:"packets,omitempty"`
}

// read structured information from A.L.F.R.E.D. packet
func ReadStatistics(data alfred.Data) (*Statistics, error) {
	stat := Statistics{Source: HardwareAddr(data.Source), TimeStamp: time.Now()}
	err := readJSON(data, STATISTICS_PACKETTYPE, STATISTICS_PACKETVERSION, &stat.Data)
	return &stat, err
}

func (s *Statistics) Bytes() ([]byte, error) {
	itembuf := new(bytes.Buffer)
	enc := gob.NewEncoder(itembuf)
	err := enc.Encode(s)
	if err != nil {
		return nil, err
	}
	return itembuf.Bytes(), nil
}

func (s *Statistics) Key() []byte {
	return []byte(s.Source)
}

func (s *Statistics) DeserializeFrom(b []byte) error {
	buf := bytes.NewBuffer(b)
	dec := gob.NewDecoder(buf)
	news := Statistics{}
	err := dec.Decode(&news)
	s.Source = news.Source
	s.Data = news.Data
	s.TimeStamp = news.TimeStamp
	return err
}

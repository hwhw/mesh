package gluon

import (
	"github.com/hwhw/mesh/alfred"
)

const (
	// A.L.F.R.E.D. packet type ID for Gluon Statistics data
	STATISTICS_PACKETTYPE = 159
	// A.L.F.R.E.D. packet version for Gluon Statistics data
	STATISTICS_PACKETVERSION = 0
)

// wrapper type for storing the statistics data and its origin
type Statistics struct {
	Source alfred.HardwareAddr
	Data   *StatisticsData
}

// mesh node statistics
type StatisticsData struct {
	NodeID      string               `json:"node_id"`
	Clients     *Clients             `json:"clients,omitempty"`
	RootFSUsage float64              `json:"rootfs_usage,omitempty"`
	LoadAvg     float64              `json:"loadavg,omitempty"`
	Uptime      float64              `json:"uptime,omitempty"`
	IdleTime    float64              `json:"idletime,omitempty"`
	Gateway     *alfred.HardwareAddr `json:"gateway,omitempty"`
	Processes   *Processes           `json:"processes,omitempty"`
	Traffic     *Traffic             `json:"traffic,omitempty"`
	Memory      *Memory              `json:"memory,omitempty"`
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
func (stat *Statistics) ReadAlfred(data alfred.Data) error {
	stat.Data = &StatisticsData{}
	stat.Source = alfred.HardwareAddr(data.Source)
	return readJSON(data, STATISTICS_PACKETTYPE, STATISTICS_PACKETVERSION, &stat.Data)
}

func (stat *Statistics) GetPacketType() uint8 {
	return STATISTICS_PACKETTYPE
}

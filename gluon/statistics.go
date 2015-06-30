package gluon

import (
    "github.com/hwhw/mesh/alfred"
    "net"
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
    Source  net.HardwareAddr
    Data    StatisticsData
    TimeStamp time.Time
}

// mesh node statistics
type StatisticsData struct {
    NodeID *HardwareAddr `json:"node_id,omitempty"`
    Clients *Clients `json:"clients,omitempty"`
    RootFSUsage float64 `json:"rootfs_usage,omitempty"`
    LoadAvg float64 `json:"loadavg,omitempty"`
    Uptime float64 `json:"uptime,omitempty"`
    IdleTime float64 `json:"idletime,omitempty"`
    Gateway *HardwareAddr `json:"gateway,omitempty"`
    Processes *Processes `json:"processes,omitempty"`
    Traffic *Traffic `json:"traffic,omitempty"`
    Memory *Memory `json:"memory,omitempty"`
}

type Clients struct {
    Wifi int `json:"wifi,omitempty"`
    Total int `json:"total,omitempty"`
}

type Memory struct {
    Cached int `json:"cached,omitempty"`
    Total int `json:"total,omitempty"`
    Buffers int `json:"buffers,omitempty"`
    Free int `json:"free,omitempty"`
}

type Processes struct {
    Total int `json:"total,omitempty"`
    Running int `json:"running,omitempty"`
}

type Traffic struct {
    Tx *TrafficCounter `json:"tx,omitempty"`
    Rx *TrafficCounter `json:"rx,omitempty"`
    MgmtTx *TrafficCounter `json:"mgmt_tx,omitempty"`
    MgmtRx *TrafficCounter `json:"mgmt_rx,omitempty"`
    Forward *TrafficCounter `json:"forward,omitempty"`
}

type TrafficCounter struct {
    Bytes int `json:"bytes,omitempty"`
    Packets int `json:"packets,omitempty"`
}

// read structured information from A.L.F.R.E.D. packet
func ReadStatistics(data alfred.Data) (Statistics, error) {
    stat := Statistics{Source: data.Source, TimeStamp: time.Now()}
    err := readJSON(data, STATISTICS_PACKETTYPE, STATISTICS_PACKETVERSION, &stat.Data)
    return stat, err
}

package gluon

import (
    "github.com/hwhw/mesh/alfred"
    "net"
)

const (
    // A.L.F.R.E.D. packet type ID for Gluon NodeInfo data
    NODEINFO_PACKETTYPE = 158
    // A.L.F.R.E.D. packet version for Gluon NodeInfo data
    NODEINFO_PACKETVERSION = 0
)

// wrapper type for storing the metadata and its origin
type NodeInfo struct {
    Source  net.HardwareAddr
    Data    NodeInfoData
}

// mesh node metadata
type NodeInfoData struct {
    NodeID HardwareAddr `json:"node_id,omitempty"`
    Network *Network `json:"network,omitempty"`
    Hostname string `json:"hostname,omitempty"`
    Location *Location `json:"location,omitempty"`
    Software *Software `json:"software,omitempty"`
    Hardware *Hardware `json:"hardware,omitempty"`
    Owner *Owner `json:"owner,omitempty"`
}

type Network struct {
    Mac string `json:"mac,omitempty"`
    Addresses []net.IP `json:"addresses,omitempty"`
    MeshInterfaces []HardwareAddr `json:"mesh_interfaces,omitempty"`
}

type Location struct {
    Longitude float64 `json:"longitude,omitempty"`
    Latitude float64 `json:"latitude,omitempty"`
}

type Software struct {
    FastD *FastD `json:"fastd,omitempty"`
    AutoUpdater *AutoUpdater `json:"autoupdater,omitempty"`
    BatmanAdv *BatmanAdv `json:"batman-adv,omitempty"`
    Firmware *Firmware `json:"firmware,omitempty"`
}

type FastD struct {
    Enabled bool `json:"enabled,omitempty"`
    Version string `json:"version,omitempty"`
}

type AutoUpdater struct {
    Enabled bool `json:"enabled,omitempty"`
    Branch string `json:"branch,omitempty"`
}

type BatmanAdv struct {
    Compat int `json:"compat,omitempty"`
    Version string `json:"version,omitempty"`
}

type Firmware struct {
    Base string `json:"base,omitempty"`
    Release string `json:"release,omitempty"`
}

type Hardware struct {
    Model string `json:"model,omitempty"`
}

type Owner struct {
    Contact string `json:"contact,omitempty"`
}

// read structured information from A.L.F.R.E.D. packet
func ReadNodeInfo(data alfred.Data) (NodeInfo, error) {
    ni := NodeInfo{Source: data.Source}
    err := readJSON(data, NODEINFO_PACKETTYPE, NODEINFO_PACKETVERSION, &ni.Data)
    return ni, err
}

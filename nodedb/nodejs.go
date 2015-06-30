package nodedb

import (
    "github.com/hwhw/mesh/gluon"
    "time"
    "io"
    "encoding/json"
    "errors"
)

var ErrUnknownVersion = errors.New("Unknown data format version")

// Wrapped time.Time type to provide JSON Marshaling as we like it to be
type NodesJSONTime time.Time

const (
    // time.Time format string for what we like the JSON Marshaled
    // variant to look like
    NodesJSONTimeStamp = `"2006-01-02T15:04:05"`
)

// Base type for the nodes.json document
type NodesJSON struct {
    Timestamp   NodesJSONTime              `json:"timestamp"`
    Nodes       map[string]NodesJSONData   `json:"nodes"`
    Version     int                       `json:"version,omitempty"`
}

// Type for the data of a single Mesh node
type NodesJSONData struct {
    // wrap gluon.NodeInfoData
    NodeInfo    gluon.NodeInfoData      `json:"nodeinfo,omitempty"`
    Flags       NodesJSONFlags             `json:"flags,omitempty"`
    FirstSeen   NodesJSONTime              `json:"firstseen,omitempty"`
    LastSeen    NodesJSONTime              `json:"lastseen,omitempty"`
    Statistics  NodesJSONStatistics        `json:"statistics,omitempty"`
}

type NodesJSONFlags struct {
    Online      bool                    `json:"online"`
    Gateway     bool                    `json:"gateway,omitempty"`
}

type NodesJSONStatistics struct {
    Clients     int                     `json:"clients,omitempty"`
    Gateway     *gluon.HardwareAddr     `json:"gateway,omitempty"`
    Uptime      float64                 `json:"uptime,omitempty"`
    LoadAvg     float64                 `json:"loadavg,omitempty"`
    MemoryUsage float64                 `json:"memory_usage,omitempty"`
    RootFSUsage float64                 `json:"rootfs_usage,omitempty"`
}

// provide interface for JSON serialization
func (t NodesJSONTime)MarshalJSON() ([]byte, error) {
    return []byte(time.Time(t).UTC().Format(NodesJSONTimeStamp)), nil
}

// provide interface for JSON serialization
func (t *NodesJSONTime)UnmarshalJSON(tval []byte) (error) {
    tparsed, err := time.Parse(NodesJSONTimeStamp, string(tval))
    if err != nil {
        return err
    }
    *t = NodesJSONTime(tparsed)
    return nil
}

// Assemble data elements for a mesh node from database.
// This operation assumes the database is already locked by the caller.
func (db *NodeDB) getNodesJSONData(node string) (NodesJSONData, error) {
    data := NodesJSONData{}

    nodeinfo, exists := db.nodeinfo[node]
    if !exists {
        return data, ErrNotFound
    }
    data.NodeInfo = nodeinfo.item.(gluon.NodeInfo).Data

    statistics, exists := db.statistics[node]
    if !exists {
        return data, ErrNotFound
    }

    // earliest datestamp is the "first seen" time,
    // latest datestamp is the "last seen" time
    data.FirstSeen = NodesJSONTime(nodeinfo.created)
    if statistics.created.Before(nodeinfo.created) {
        data.FirstSeen = NodesJSONTime(statistics.created)
    }

    data.LastSeen = NodesJSONTime(nodeinfo.updated)
    if statistics.updated.Before(nodeinfo.updated) {
        data.LastSeen = NodesJSONTime(statistics.updated)
    }

    // online state is determined by the time we have last
    // seen a mesh node
    offline := time.Now().Sub(time.Time(data.LastSeen))
    if offline < db.settings.NodeOfflineDuration {
        data.Flags.Online = true
    } else {
        data.Flags.Online = false
    }

    // set gateway flag when we have the node's address in
    // our list of gateways
    if alias, havealias := db.aliases[node]; havealias {
        _, data.Flags.Gateway = db.gateways[alias.item.(string)]
    }

    statdata := statistics.item.(gluon.Statistics).Data
    if statdata.Memory.Total != 0 {
        // this calculation is a bit stupid, but compatible with ffmap-backend:
        data.Statistics.MemoryUsage = 1.0 - (float64(statdata.Memory.Free) / float64(statdata.Memory.Total))
    } else {
        data.Statistics.MemoryUsage = 1
    }
    data.Statistics.Uptime = statdata.Uptime
    data.Statistics.Clients = statdata.Clients.Total
    data.Statistics.Gateway = statdata.Gateway
    data.Statistics.LoadAvg = statdata.LoadAvg
    data.Statistics.RootFSUsage = statdata.RootFSUsage

    return data, nil
}

// Write a full nodes.json style document based on the current
// database contents.
func (db *NodeDB) GenerateNodesJSON(w io.Writer) error {
    nodejs := NodesJSON{
        Nodes: make(map[string]NodesJSONData),
        Timestamp: NodesJSONTime(time.Now()),
        Version: 1,
    }
    db.RLock()
    for n := range(db.nodeinfo) {
        data, err := db.getNodesJSONData(n)
        if err == nil {
            nodejs.Nodes[n] = data
        }
    }
    db.RUnlock()
    enc := json.NewEncoder(w)
    if err := enc.Encode(&nodejs); err != nil {
        return err
    }
    return nil
}

func readNodesJSON(r io.Reader) (NodesJSON, error) {
    var nodes NodesJSON
    dec := json.NewDecoder(r)
    if err := dec.Decode(&nodes); err != nil {
        return nodes, err
    }
    return nodes, nil
}

// read nodes.json file into database
func (db *NodeDB) ReadNodesJSONtoDB(r io.Reader, persistent bool) error {
    nodes, err := readNodesJSON(r)
    if err != nil {
        return err
    }
    if nodes.Version != 1 {
        return ErrUnknownVersion
    }
    db.Lock()
    defer db.Unlock()
    for id, node := range nodes.Nodes {
        updated := time.Time(node.LastSeen).Local()
        created := time.Time(node.FirstSeen).Local()
        if persistent {
            created = noTime
            updated = time.Now()
        }
        db.nodeinfo[id] = Item{
            updated: updated,
            created: created,
            item: node.NodeInfo,
        }
        if node.Flags.Gateway {
            db.gateways[id] = Item{
                updated: updated,
                created: created,
                item: struct{}{},
            }
        }
        // insert a dummy statistics item for the node
        db.statistics[id] = Item{
            updated: time.Now(),
            created: time.Now(),
            item: gluon.Statistics{Data: gluon.StatisticsData{}},
        }
    }
    return nil
}


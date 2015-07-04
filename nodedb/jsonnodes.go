package nodedb

import (
	"encoding/json"
	"errors"
	"github.com/boltdb/bolt"
	"github.com/hwhw/mesh/gluon"
	"github.com/hwhw/mesh/alfred"
	"github.com/hwhw/mesh/store"
	"io"
	"log"
	"time"
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
	Timestamp NodesJSONTime            `json:"timestamp"`
	Nodes     map[string]*NodesJSONData `json:"nodes"`
	Version   int                      `json:"version,omitempty"`
}

// Type for the data of a single Mesh node
type NodesJSONData struct {
	// wrap gluon.NodeInfoData
	NodeInfo   gluon.NodeInfoData  `json:"nodeinfo,omitempty"`
	Flags      NodesJSONFlags      `json:"flags,omitempty"`
	FirstSeen  NodesJSONTime       `json:"firstseen,omitempty"`
	LastSeen   NodesJSONTime       `json:"lastseen,omitempty"`
	Statistics NodesJSONStatistics `json:"statistics,omitempty"`
}

type NodesJSONFlags struct {
	Online  bool `json:"online"`
	Gateway bool `json:"gateway,omitempty"`
}

type NodesJSONStatistics struct {
	Clients     int                 `json:"clients"`
	Gateway     *alfred.HardwareAddr `json:"gateway,omitempty"`
	Uptime      float64             `json:"uptime"`
	LoadAvg     float64             `json:"loadavg"`
	MemoryUsage float64             `json:"memory_usage"`
	RootFSUsage float64             `json:"rootfs_usage"`
}

// provide interface for JSON serialization
func (t NodesJSONTime) MarshalJSON() ([]byte, error) {
	return []byte(time.Time(t).UTC().Format(NodesJSONTimeStamp)), nil
}

// provide interface for JSON serialization
func (t *NodesJSONTime) UnmarshalJSON(tval []byte) error {
	tparsed, err := time.Parse(NodesJSONTimeStamp, string(tval))
	if err != nil {
		return err
	}
	*t = NodesJSONTime(tparsed)
	return nil
}

// Assemble data elements for a mesh node from database.
// This operation assumes the database is already locked by the caller.
func (db *NodeDB) getNodesJSONData(tx *bolt.Tx, nmeta *store.Meta, offlineDuration time.Duration) (*NodesJSONData, error) {
	data := &NodesJSONData{}

	nodeinfo := &NodeInfo{}
    if err := nmeta.GetItem(nodeinfo); err != nil {
		return data, err
	}

	data.NodeInfo = *nodeinfo.Data // make a copy

	// earliest datestamp is the "first seen" time,
	// latest datestamp is the "last seen" time
	firstseen := nmeta.Created
	lastseen := nmeta.Updated

    statistics := &Statistics{}
    smeta := store.NewMeta(statistics)
	if db.Main.Get(tx, nmeta.Key(), smeta) == nil {
		if smeta.Created.Before(firstseen) {
			firstseen = smeta.Created
		}
		if lastseen.Before(smeta.Updated) {
			lastseen = smeta.Updated
		}

        if smeta.GetItem(statistics) == nil {
			statdata := statistics.Data
			if statdata.Memory != nil {
				if statdata.Memory.Total != 0 {
					// this calculation is a bit stupid, but compatible with ffmap-backend:
					data.Statistics.MemoryUsage = 1.0 - (float64(statdata.Memory.Free) / float64(statdata.Memory.Total))
				} else {
					data.Statistics.MemoryUsage = 1
				}
			}
			data.Statistics.Uptime = statdata.Uptime
			if statdata.Clients != nil {
				data.Statistics.Clients = statdata.Clients.Total
			}
			data.Statistics.Gateway = statdata.Gateway
			data.Statistics.LoadAvg = statdata.LoadAvg
			data.Statistics.RootFSUsage = statdata.RootFSUsage
		}
	}

    vis := &VisData{}
    vmeta := store.NewMeta(vis)
	if db.Main.Get(tx, nmeta.Key(), vmeta) == nil {
		if vmeta.Created.Before(firstseen) {
			firstseen = vmeta.Created
		}
		if lastseen.Before(vmeta.Updated) {
			lastseen = vmeta.Updated
		}
	}

	data.FirstSeen = NodesJSONTime(firstseen)
	data.LastSeen = NodesJSONTime(lastseen)

	// set gateway flag when we have the node's address in
	// our list of gateways
	data.Flags.Gateway = db.Main.Exists(tx, nmeta.Key(), &Gateway{})

	// online state is determined by the time we have last
	// seen a mesh node
	offline := time.Now().Sub(time.Time(data.LastSeen))
	if offline < offlineDuration {
		data.Flags.Online = true
	} else {
		data.Flags.Online = false
	}

	return data, nil
}

// Write a full nodes.json style document based on the current
// database contents.
func (db *NodeDB) GenerateNodesJSON(w io.Writer, offlineDuration time.Duration) error {
	nodejs := NodesJSON{
		Nodes:     make(map[string]*NodesJSONData),
		Timestamp: NodesJSONTime(time.Now()),
		Version:   1,
	}
	err := db.Main.View(func(tx *bolt.Tx) error {
        nodeinfo := &NodeInfo{}
        nmeta := store.NewMeta(nodeinfo)
        return db.Main.ForEach(tx, nmeta, func(cursor *bolt.Cursor) (bool, error) {
			data, err := db.getNodesJSONData(tx, nmeta, offlineDuration)
			if err == nil {
				nodejs.Nodes[alfred.HardwareAddr(nmeta.Key()).String()] = data
			} else {
				log.Printf("NodeDB: can not generate node info JSON for %v: %v", alfred.HardwareAddr(nmeta.Key()), err)
			}
            return false, nil
		})
	})
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	if err := enc.Encode(&nodejs); err != nil {
		return err
	}
	return nil
}

/*
// decode into NodesJSON data structures
func readNodesJSON(r io.Reader) (NodesJSON, error) {
	var nodes NodesJSON
	dec := json.NewDecoder(r)
	if err := dec.Decode(&nodes); err != nil {
		return nodes, err
	}
	return nodes, nil
}

// read nodes.json compatible data into database
func (db *NodeDB) ImportNodes(r io.Reader, persistent bool) error {
	nodes, err := readNodesJSON(r)
	if err != nil {
		return err
	}
	if nodes.Version != 1 {
		return ErrUnknownVersion
	}
	err = db.store.Update(func(tx *bolt.Tx) error {
		for _, node := range nodes.Nodes {
			meta := boltdb.ItemMeta{
				Updated: time.Time(node.LastSeen).Local(),
				Created: time.Time(node.FirstSeen).Local(),
			}
			err := db.UpdateMeshData(tx, &gluon.NodeInfo{Source: node.NodeInfo.NodeID, Data: node.NodeInfo}, persistent, &meta)
			if err == nil {
				// insert empty statistics item
				stat := gluon.Statistics{Source: node.NodeInfo.NodeID, Data: gluon.StatisticsData{}}
				err = db.UpdateMeshData(tx, &stat, persistent, &meta)
			}
			if err != nil {
				log.Printf("Import: error on node %v", node.NodeInfo.NodeID)
			}
		}
		return nil
	})
	return err
}

// read nodes.json file into database
func (db *NodeDB) ImportNodesFile(filename string, persistent bool) error {
	f, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer f.Close()
	err = db.ImportNodes(f, false)
	return err
}
*/

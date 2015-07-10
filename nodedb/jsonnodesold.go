package nodedb

import (
	"encoding/json"
	"github.com/boltdb/bolt"
	"github.com/hwhw/mesh/alfred"
	"github.com/hwhw/mesh/store"
    "fmt"
	"io"
	"log"
	"time"
    "net"
    "bytes"
)

type NodesOldJSONMeta struct {
	Timestamp NodesJSONTime            `json:"timestamp"`
    GluonRelease string                `json:"gluon_release,omitempty"`
}

// Base type for the nodes.json document
type NodesOldJSON struct {
    Meta      NodesOldJSONMeta `json:"meta"`
	Nodes     []*NodesOldJSONData `json:"nodes"`
    Links     []*NodesOldJSONLink `json:"links"`
}

// Type for the data of a single Mesh node
type NodesOldJSONData struct {
    Id              alfred.HardwareAddr `json:"id"`
	Name            *string `json:"name"`
    LastSeen        int64   `json:"lastseen"`
    Uptime          float64 `json:"uptime"`
    Geo             []float64 `json:"geo,omitempty"`
    ClientCount     int     `json:"clientcount"`
    BatmanVersion   *string  `json:"batman_version"`
    BatmanGWMode    *string `json:"batman_gwmode"`
    Group           *string `json:"group"`
	Flags      NodesOldJSONFlags      `json:"flags"`
    AutoUpdaterState bool   `json:"autoupdater_state"`
    AutoUpdaterBranch string `json:"autoupdater_branch"`
    Hardware        string  `json:"hardware,omitempty"`
	Firmware        *string `json:"firmware"`
    GluonBase       string  `json:"gluon_base,omitempty"`
    Gateway         *alfred.HardwareAddr `json:"gateway,omitempty"`
	Addresses      []net.IP              `json:"addresses,omitempty"`
}

type NodesOldJSONFlags struct {
	Online  bool `json:"online"`
	Gateway bool `json:"gateway"`
}

type NodesOldJSONLink struct {
    Id      string  `json:"id"`
	Source  int     `json:"source"`
    Quality string `json:"quality"`
	Target  int     `json:"target"`
	Type    *string `json:"type"`
}

// Assemble data elements for a mesh node from database.
// This operation assumes the database is already locked by the caller.
func (db *NodeDB) getNodesOldJSONData(tx *bolt.Tx, nmeta *store.Meta, mac alfred.HardwareAddr, offlineDuration time.Duration) (*NodesOldJSONData, error) {
	data := &NodesOldJSONData{}

	ninfo := &NodeInfo{}
    if err := nmeta.GetItem(ninfo); err != nil {
		return data, err
	}
    nodeinfo := *ninfo // make a copy

    data.Name = &nodeinfo.Data.Hostname
    data.Id = mac
    if nodeinfo.Data.Location != nil {
        data.Geo = []float64{nodeinfo.Data.Location.Latitude, nodeinfo.Data.Location.Longitude}
    }
    if nodeinfo.Data.Software != nil {
        if nodeinfo.Data.Software.Firmware != nil {
            data.Firmware = &nodeinfo.Data.Software.Firmware.Release
            data.GluonBase = nodeinfo.Data.Software.Firmware.Base
        }
        if nodeinfo.Data.Software.BatmanAdv != nil {
            data.BatmanVersion = &nodeinfo.Data.Software.BatmanAdv.Version
        }
        if nodeinfo.Data.Software.AutoUpdater != nil {
            data.AutoUpdaterState = nodeinfo.Data.Software.AutoUpdater.Enabled
            data.AutoUpdaterBranch = nodeinfo.Data.Software.AutoUpdater.Branch
        }
    }
    if nodeinfo.Data.Hardware != nil {
        data.Hardware = nodeinfo.Data.Hardware.Model
    }
    if nodeinfo.Data.Network != nil {
        data.Addresses = nodeinfo.Data.Network.Addresses
    }

	// latest datestamp is the "last seen" time
	lastseen := nmeta.Updated

    statistics := &Statistics{}
    smeta := store.NewMeta(statistics)
	if db.Main.Get(tx, nmeta.Key(), smeta) == nil {
		if lastseen.Before(smeta.Updated) {
			lastseen = smeta.Updated
		}

        if smeta.GetItem(statistics) == nil {
			statdata := statistics.Data
			data.Uptime = statdata.Uptime
			if statdata.Clients != nil {
				data.ClientCount = statdata.Clients.Total
			}
            if statdata.Gateway != nil {
                g := db.resolveAlias(tx, *statdata.Gateway)
                data.Gateway = &g
            }
		}
	}

    vis := &VisData{}
    vmeta := store.NewMeta(vis)
	if db.Main.Get(tx, nmeta.Key(), vmeta) == nil {
		if lastseen.Before(vmeta.Updated) {
			lastseen = vmeta.Updated
		}
	}

	data.LastSeen = lastseen.Unix()

	// set gateway flag when we have the node's address in
	// our list of gateways
	data.Flags.Gateway = db.Main.Exists(tx, nmeta.Key(), &Gateway{})

	// online state is determined by the time we have last
	// seen a mesh node
	offline := time.Now().Sub(time.Time(lastseen))
	if offline < offlineDuration {
		data.Flags.Online = true
	} else {
		data.Flags.Online = false
	}

	return data, nil
}

// Write a full nodes.json style document based on the current
// database contents.
func (db *NodeDB) GenerateNodesOldJSON(w io.Writer, offlineDuration time.Duration) {
    data := db.cacheExportNodesOld.get(func() []byte {
        nodejs := NodesOldJSON{
            Meta: NodesOldJSONMeta{
                Timestamp: NodesJSONTime(time.Now()),
                GluonRelease: "0.6.3",
            },
            Nodes: make([]*NodesOldJSONData, 0, 500),
            Links: make([]*NodesOldJSONLink, 0, 500),
        }
        nodes := make(map[string]int)
        db.Main.View(func(tx *bolt.Tx) error {
            nodeinfo := &NodeInfo{}
            nmeta := store.NewMeta(nodeinfo)
            err := db.Main.ForEach(tx, nmeta, func(cursor *bolt.Cursor) (bool, error) {
                mac := db.resolveAlias(tx, alfred.HardwareAddr(nmeta.Key()))
                data, err := db.getNodesOldJSONData(tx, nmeta, mac, offlineDuration)
                if err == nil {
                    nodes[mac.String()] = len(nodejs.Nodes)
                    nodejs.Nodes = append(nodejs.Nodes, data)
                } else {
                    log.Printf("NodeDB: can not generate node info JSON for %v: %v", mac, err)
                }
                return false, nil
            })
            if err != nil {
                return err
            }

            links := make(map[int]map[int]*NodesOldJSONLink)

            d := &VisData{}
            m := store.NewMeta(d)
            err = db.Main.ForEach(tx, m, func(cursor *bolt.Cursor) (bool, error) {
                if m.GetItem(d) != nil {
                    // skip unparseable items
                    return false, nil
                }
                // main address is the first element in batadv.VisV1.Ifaces
                mac := db.resolveAlias(tx, d.Ifaces[0].Mac)
                source, ok := nodes[mac.String()]
                if !ok {
                    return false, nil
                }
                _, exists := links[source]
                if !exists {
                    links[source] = make(map[int]*NodesOldJSONLink)
                }
                for _, entry := range d.Entries {
                    if entry.Qual == 0 {
                        // TT entry, we do not cover these
                        continue
                    }
                    emac := db.resolveAlias(tx, []byte(entry.Mac))
                    target, ok := nodes[emac.String()]
                    if !ok {
                        continue
                    }
                    _, exists = links[target]
                    var node *NodesOldJSONLink
                    if exists {
                        node, exists = links[target][source]
                    }
                    if !exists {
                        node, exists = links[source][target]
                    }
                    if exists {
                        node.Quality = fmt.Sprintf("%s, %.03f", node.Quality, 255.0 / float64(entry.Qual))
                    } else {
                        links[source][target] = &NodesOldJSONLink{
                            Id: fmt.Sprintf("%s-%s", mac, emac),
                            Source: source,
                            Target: target,
                            Quality: fmt.Sprintf("%.03f", 255.0 / float64(entry.Qual)),
                            Type: nil,
                        }
                    }
                }
                return false, nil
            })
            if err != nil {
                return err
            }
            for _, s := range links {
                for _, l := range s {
                    nodejs.Links = append(nodejs.Links, l)
                }
            }
            return nil
        })
        buf := new(bytes.Buffer)
        enc := json.NewEncoder(w)
        if err := enc.Encode(&nodejs); err != nil {
            return []byte{}
        }
        return buf.Bytes()
    })
	w.Write(data)
}


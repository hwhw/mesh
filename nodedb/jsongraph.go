package nodedb

import (
	"bytes"
	"encoding/json"
	"github.com/boltdb/bolt"
	"github.com/hwhw/mesh/alfred"
	"github.com/hwhw/mesh/store"
	"io"
)

// Type corresponding to a full graph.json document
type GraphJSON struct {
	Version int             `json:"version,omitempty"`
	BatAdv  GraphJSONBatAdv `json:"batadv"`
}

// batadv subelement in graph.json document
type GraphJSONBatAdv struct {
	Directed bool            `json:"directed"`
	Graph    []struct{}      `json:"graph"`
	Nodes    []GraphJSONNode `json:"nodes"`
	Links    []GraphJSONLink `json:"links"`
}

// Information about the existence of a mesh node
type GraphJSONNode struct {
	NodeID string              `json:"node_id,omitempty"`
	ID     alfred.HardwareAddr `json:"id"`
	Number int                 `json:"number"`
}

// Information about a link.
// Especially stupid design is the numerical indexing based
// on the list index of the corresponding node information.
type GraphJSONLink struct {
	Source   int     `json:"source"`
	Vpn      bool    `json:"vpn"`
	Bidirect bool    `json:"bidirect"`
	Target   int     `json:"target"`
	Tq       float64 `json:"tq"`
}

func NewGraphJSONNode(id alfred.HardwareAddr, nodeid string, number int) GraphJSONNode {
	idcopy := make(alfred.HardwareAddr, len(id))
	copy(idcopy, id)
	return GraphJSONNode{ID: idcopy, NodeID: nodeid, Number: number}
}
func NewGraphJSONNodeIDonly(id alfred.HardwareAddr, number int) GraphJSONNode {
	idcopy := make(alfred.HardwareAddr, len(id))
	copy(idcopy, id)
	return GraphJSONNode{ID: idcopy, Number: number}
}

// Write a full graph.json document based on the contents of
// the database.
func (db *NodeDB) GenerateGraphJSON(w io.Writer) {
	data := db.cacheExportGraph.get(func() []byte {
		// index for the nodes in the node list for later lookup
		nodes := make(map[string]int)
		// actual node list
		nodesjs := make([]GraphJSONNode, 0, 100)

		// index for node links, indexed by their IDs/MACs
		links := make(map[string]map[string]GraphJSONLink)
		// actual link list objects
		linksjs := make([]GraphJSONLink, 0, 100)

		d := &VisData{}
		m := store.NewMeta(d)
		db.Main.View(func(tx *bolt.Tx) error {
			return db.Main.ForEach(tx, m, func(cursor *bolt.Cursor) (bool, error) {
				if m.GetItem(d) != nil {
					// skip unparseable items
					return false, nil
				}
				// main address is the first element in batadv.VisV1.Ifaces
				nodeid, _ := db.ResolveNodeID(tx, d.Ifaces[0].Mac)
				isgateway := db.Main.Exists(tx, []byte(nodeid), &Gateway{})
				if _, seen := nodes[nodeid]; !seen {
					// new node, put into lists
					nodes[nodeid] = len(nodesjs)
					nodesjs = append(nodesjs, NewGraphJSONNode(d.VisV1.Mac, nodeid, len(nodesjs)))
				}

				nodelinks := make(map[string]GraphJSONLink)
				for _, entry := range d.Entries {
					if entry.Qual == 0 {
						// TT entry, we do not cover these
						continue
					}

					enodeid, _ := db.ResolveNodeID(tx, []byte(entry.Mac))
					if _, seen := nodes[enodeid]; !seen {
						// linked node is a new node, also put into lists since it has to exist
						nodes[enodeid] = len(nodesjs)
						nodesjs = append(nodesjs, NewGraphJSONNode(entry.Mac, enodeid, len(nodesjs)))
					}

					// do a cross check: did we already record an entry for the
					// reverse direction? If so, mark it as being birectional
					// and recalculate the link quality value
					if rev, exists := links[enodeid]; exists {
						if rrev, exists := rev[nodeid]; exists {
							if isgateway {
								rrev.Vpn = true
							}
							rrev.Bidirect = true
							// middle value for now - or should we chose bigger (worse) value?
							rrev.Tq = (rrev.Tq + 255.0/float64(entry.Qual)) / 2
							links[enodeid][nodeid] = rrev
							continue
						}
					}

					// new link, record it
					nodelinks[enodeid] = GraphJSONLink{Tq: 255.0 / float64(entry.Qual), Vpn: isgateway}
				}

				links[nodeid] = nodelinks
				return false, nil
			})
		})

		// build link table with numerical references
		for node, nodelinks := range links {
			if iface1, ok := nodes[node]; ok {
				for node2, link := range nodelinks {
					if iface2, ok := nodes[node2]; ok {
						link.Source = iface1
						link.Target = iface2
						linksjs = append(linksjs, link)
					}
				}
			}
		}

		graphjs := GraphJSON{
			BatAdv: GraphJSONBatAdv{
				Directed: false,
				Nodes:    nodesjs,
				Links:    linksjs,
				Graph:    make([]struct{}, 0),
			},
			Version: 1,
		}

		buf := new(bytes.Buffer)
		enc := json.NewEncoder(w)
		if err := enc.Encode(&graphjs); err != nil {
			return []byte{}
		}
		return buf.Bytes()
	})
	w.Write(data)
}

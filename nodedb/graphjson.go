package nodedb

import (
    "github.com/boltdb/bolt"
    "github.com/hwhw/mesh/batadvvis"
    "github.com/hwhw/mesh/gluon"
    "io"
    "encoding/json"
)

// Type corresponding to a full graph.json document
type GraphJSON struct {
    Version     int             `json:"version,omitempty"`
    BatAdv      GraphJSONBatAdv   `json:"batadv"`
}

// batadv subelement in graph.json document
type GraphJSONBatAdv struct {
    Directed    bool            `json:"directed"`
    Graph       []struct{}      `json:"graph"`
    Nodes       []GraphJSONNode   `json:"nodes"`
    Links       []GraphJSONLink   `json:"links"`
}

// Information about the existence of a mesh node
type GraphJSONNode struct {
    NodeID      gluon.HardwareAddr  `json:"node_id,omitempty"`
    ID          gluon.HardwareAddr  `json:"id"`
}

// Information about a link.
// Especially stupid design is the numerical indexing based
// on the list index of the corresponding node information.
type GraphJSONLink struct {
    Source      int             `json:"source"`
    Vpn         bool            `json:"vpn"`
    Bidirect    bool            `json:"bidirect"`
    Target      int             `json:"target"`
    Tq          float64         `json:"tq"`
}

func NewGraphJSONNode(id gluon.HardwareAddr, nodeid gluon.HardwareAddr) GraphJSONNode {
    idcopy := make(gluon.HardwareAddr, len(id))
    copy(idcopy, id)
    nodeidcopy := make(gluon.HardwareAddr, len(nodeid))
    copy(nodeidcopy, nodeid)
    return GraphJSONNode{ID:idcopy, NodeID:nodeidcopy}
}
func NewGraphJSONNodeIDonly(id gluon.HardwareAddr) GraphJSONNode {
    idcopy := make(gluon.HardwareAddr, len(id))
    copy(idcopy, id)
    return GraphJSONNode{ID:idcopy}
}

// Write a full graph.json document based on the contents of
// the database.
func (db *NodeDB) GenerateGraphJSON(w io.Writer) error {
    // index for the nodes in the node list for later lookup
    nodes := make(map[string]int)
    // actual node list
    nodesjs := make([]GraphJSONNode, 0, 100)

    // index for node links, indexed by their IDs/MACs
    links := make(map[string]map[string]GraphJSONLink)
    // actual link list objects
    linksjs := make([]GraphJSONLink, 0, 100)

    d := &batadvvis.VisV1{}
    err := db.store.View(func(tx *bolt.Tx) error {
        return db.store.ForEach(tx, d, func(key []byte) error {
            // main address is the first element in batadv.VisV1.Ifaces
            maca, _ := db.resolveAlias(tx, d.Ifaces[0].Mac)
            isgateway := db.isGateway(tx, maca)
            mac := string(maca)
            nodeid := d.Mac
            if _, seen := nodes[mac]; !seen {
                // new node, put into lists
                nodes[mac] = len(nodesjs)
                nodesjs = append(nodesjs, NewGraphJSONNode(gluon.HardwareAddr(maca), gluon.HardwareAddr(nodeid)))
            } else {
                // record node_id, since we only get that here
                nodesjs[nodes[mac]].NodeID = gluon.HardwareAddr(nodeid)
            }

            nodelinks := make(map[string]GraphJSONLink)
            for _, entry := range d.Entries {
                if entry.Qual == 0 {
                    // TT entry, we do not cover these
                    continue
                }

                emaca, _ := db.resolveAlias(tx, []byte(entry.Mac))
                emac := string(emaca)
                if _, seen := nodes[emac]; !seen {
                    // linked node is a new node, also put into lists since it has to exist
                    nodes[emac] = len(nodesjs)
                    nodesjs = append(nodesjs, NewGraphJSONNodeIDonly(gluon.HardwareAddr(emaca)))
                }

                // do a cross check: did we already record an entry for the
                // reverse direction? If so, mark it as being birectional
                // and recalculate the link quality value
                if rev, exists := links[emac]; exists {
                    if rrev, exists := rev[mac]; exists {
                        if isgateway {
                            rrev.Vpn = true
                        }
                        rrev.Bidirect = true
                        // middle value for now - or should we chose bigger (worse) value?
                        rrev.Tq = (rrev.Tq + 255.0 / float64(entry.Qual)) / 2
                        links[emac][mac] = rrev
                        continue
                    }
                }

                // new link, record it
                nodelinks[emac] = GraphJSONLink{Tq: 255.0 / float64(entry.Qual), Vpn: isgateway}
            }

            links[mac] = nodelinks
            return nil
        })
    })
    if err != nil {
        return err
    }

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
            Nodes: nodesjs,
            Links: linksjs,
            Graph: make([]struct{}, 0),
        },
        Version: 1,
    }

    enc := json.NewEncoder(w)
    if err := enc.Encode(&graphjs); err != nil {
        return err
    }

    return nil
}

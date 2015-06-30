package nodedb

import (
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

    db.RLock()

    for n := range db.visdata {
        d := db.visdata[n].item.(batadvvis.VisV1)
        // main address is the first element in batadv.VisV1.Ifaces
        mac := d.Ifaces[0].Mac.String()
        if _, seen := nodes[mac]; !seen {
            // new node, put into lists
            nodes[mac] = len(nodesjs)
            nodesjs = append(nodesjs, GraphJSONNode{
                ID: gluon.HardwareAddr(d.Ifaces[0].Mac),
                NodeID: gluon.HardwareAddr(d.Mac),
            })
        }

        nodelinks := make(map[string]GraphJSONLink)
        for _, entry := range d.Entries {
            if entry.Qual == 0 {
                // TT entry, we do not cover these
                continue
            }

            emac := entry.Mac.String()
            if _, seen := nodes[emac]; !seen {
                // linked node is a new node, also put into lists since it has to exist
                nodes[emac] = len(nodesjs)
                nodesjs = append(nodesjs, GraphJSONNode{ID: gluon.HardwareAddr(entry.Mac)})
            }

            // do a cross check: did we already record an entry for the
            // reverse direction? If so, mark it as being birectional
            // and recalculate the link quality value
            if rev, exists := links[emac]; exists {
                if rev, exists := rev[mac]; exists {
                    rev.Bidirect = true
                    // middle value for now - or should we chose bigger (worse) value?
                    rev.Tq = (rev.Tq + 255.0 / float64(entry.Qual)) / 2
                    links[emac][mac] = rev
                    continue
                }
            }

            // new link, record it
            nodelinks[emac] = GraphJSONLink{Tq: 255.0 / float64(entry.Qual)}
        }

        links[mac] = nodelinks
    }

    db.RUnlock()

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

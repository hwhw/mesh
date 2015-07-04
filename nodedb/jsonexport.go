package nodedb

import (
	"encoding/json"
	"github.com/boltdb/bolt"
	"github.com/hwhw/mesh/store"
	"io"
)

func (db *NodeDB) jsonexport(w io.Writer, i store.Item) func(tx *bolt.Tx) error {
	return func(tx *bolt.Tx) error {
		enc := json.NewEncoder(w)
		first := true
		m := store.NewMeta(i)
		db.Main.ForEach(tx, m, func(cursor *bolt.Cursor) (bool, error) {
			err := m.GetItem(i)
			if err == nil {
				t := m.GetTransfer()
				if first {
					first = false
				} else {
					w.Write([]byte{','})
				}
				enc.Encode(t)
			}
			return false, nil
		})
		return nil
	}
}

func (db *NodeDB) ExportNodeInfo(w io.Writer) {
    w.Write([]byte{'['})
	db.Main.View(db.jsonexport(w, &NodeInfo{}))
    w.Write([]byte{']'})
}

func (db *NodeDB) ExportStatistics(w io.Writer) {
    w.Write([]byte{'['})
	db.Main.View(db.jsonexport(w, &Statistics{}))
    w.Write([]byte{']'})
}

func (db *NodeDB) ExportVisData(w io.Writer) {
    w.Write([]byte{'['})
	db.Main.View(db.jsonexport(w, &VisData{}))
    w.Write([]byte{']'})
}

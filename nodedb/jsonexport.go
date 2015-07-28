package nodedb

import (
	"bytes"
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
	data := db.cacheExportNodeInfo.get(func() []byte {
		buf := new(bytes.Buffer)
		buf.Write([]byte{'['})
		db.Main.View(db.jsonexport(buf, &NodeInfo{}))
		buf.Write([]byte{']'})
		return buf.Bytes()
	})
	w.Write(data)
}

func (db *NodeDB) ExportStatistics(w io.Writer) {
	data := db.cacheExportStatistics.get(func() []byte {
		buf := new(bytes.Buffer)
		buf.Write([]byte{'['})
		db.Main.View(db.jsonexport(buf, &Statistics{}))
		buf.Write([]byte{']'})
		return buf.Bytes()
	})
	w.Write(data)
}

func (db *NodeDB) ExportVisData(w io.Writer) {
	data := db.cacheExportVisData.get(func() []byte {
		buf := new(bytes.Buffer)
		buf.Write([]byte{'['})
		db.Main.View(db.jsonexport(buf, &VisData{}))
		buf.Write([]byte{']'})
		return buf.Bytes()
	})
	w.Write(data)
}

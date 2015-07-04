package nodedb

import (
	"github.com/boltdb/bolt"
	"github.com/hwhw/mesh/store"
)

func (db *NodeDB) updateNodeInfo(i *NodeInfo, persistent bool) func() error {
	return func() error {
		db.Main.Batch(func(tx *bolt.Tx) error {
			m := store.NewMeta(i)
			if !persistent {
				m.InvalidateIn(db.validTimeGluon)
			}
			return db.Main.UpdateMeta(tx, store.NewMeta(&NodeInfo{}), m)
		})
		return nil
	}
}

func (db *NodeDB) UpdateNodeInfo(i *NodeInfo, persistent bool) error {
    return db.updateNodeInfo(i, persistent)()
}

func (db *NodeDB) updateStatistics(s *Statistics) func() error {
	return func() error {
		db.Main.Batch(func(tx *bolt.Tx) error {
			m := store.NewMeta(s)
			m.InvalidateIn(db.validTimeGluon)
			err := db.Main.UpdateMeta(tx, store.NewMeta(&Statistics{}), m)
			if err == nil && s.Statistics.Data.Gateway != nil {
				// put entry in Gateway table
				g := &Gateway{}
				g.SetKey(*s.Statistics.Data.Gateway)
				m := store.NewMeta(g)
				m.InvalidateIn(db.validTimeVisData)
				err = db.Main.Put(tx, m)
			}
			return err
		})
		return nil
	}
}

func (db *NodeDB) UpdateStatistics(s *Statistics) error {
    return db.updateStatistics(s)()
}

func (db *NodeDB) updateVisData(v *VisData) func() error {
	return func() error {
		db.Main.Batch(func(tx *bolt.Tx) error {
			m := store.NewMeta(v)
			m.InvalidateIn(db.validTimeVisData)
			err := db.Main.Put(tx, m)
			// create entries in aliases table
			a := &Alias{}
			m = store.NewMeta(a)
			if err == nil {
				m.InvalidateIn(db.validTimeVisData)
				a.Set(v.Key())
				a.SetKey(v.VisV1.Mac)
				err = db.Main.Put(tx, m)
			}
			if err == nil {
				for _, mac := range v.VisV1.Ifaces {
					a.SetKey(mac.Mac)
					err = db.Main.Put(tx, m)
					if err != nil {
						break
					}
				}
			}
			return err
		})
		return nil
	}
}

func (db *NodeDB) UpdateVisData(v *VisData) error {
    return db.updateVisData(v)()
}

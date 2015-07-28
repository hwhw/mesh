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
			err := db.Main.UpdateMeta(tx, store.NewMeta(&NodeInfo{}), m)
			if err == nil {
				err = db.NewNodeID(tx, i.NodeInfo.Data.NodeID, i.Key())
			}
			return err
		})
		db.cacheExportNodeInfo.invalidate()
		db.cacheExportNodes.invalidate()
		db.cacheExportNodesOld.invalidate()
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
			if err == nil {
				err = db.NewNodeID(tx, s.Statistics.Data.NodeID, s.Key())
			}
			if err == nil && s.Statistics.Data.Gateway != nil {
				// put entry in Gateway table
				g := &Gateway{}
				gatewayid, _ := db.ResolveNodeID(tx, *s.Statistics.Data.Gateway)
				g.SetKey([]byte(gatewayid))
				m := store.NewMeta(g)
				m.InvalidateIn(db.validTimeVisData)
				err = db.Main.Put(tx, m)
			}
			return err
		})
		db.cacheExportStatistics.invalidate()
		//db.cacheExportNodes.invalidate()
		//db.cacheExportNodesOld.invalidate()
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

			nodeid, _ := db.ResolveNodeID(tx, v.VisV1.Mac)
			err = db.NewNodeID(tx, nodeid, v.VisV1.Mac)
			if err == nil {
				for _, mac := range v.VisV1.Ifaces {
					err = db.NewNodeID(tx, nodeid, mac.Mac)
					if err != nil {
						break
					}
				}
			}
			return err
		})
		db.cacheExportVisData.invalidate()
		db.cacheExportAliases.invalidate()
		db.cacheExportGraph.invalidate()
		return nil
	}
}

func (db *NodeDB) UpdateVisData(v *VisData) error {
	return db.updateVisData(v)()
}

// this file contains data model specific functions

package nodedb

import (
	"github.com/hwhw/mesh/alfred"
	"github.com/hwhw/mesh/store"
	"github.com/tv42/topic"
	"time"
)

type NodeDB struct {
	Main                   *store.DB
	Logs                   *store.DB
	NotifyPurgeVis         *topic.Topic
	NotifyUpdateNodeInfo   *topic.Topic
	NotifyUpdateStatistics *topic.Topic
	NotifyUpdateVis        *topic.Topic
	NotifyQuitUpdater      *topic.Topic
	NotifyQuitPurger       *topic.Topic
	NotifyQuitLogger       *topic.Topic
	validTimeGluon         time.Duration
	validTimeVisData       time.Duration
	cacheExportNodeInfo    Cache
	cacheExportStatistics  Cache
	cacheExportVisData     Cache
	cacheExportAliases     Cache
	cacheExportNodes       Cache
	cacheExportGraph       Cache
	cacheExportNodesOld    Cache
}

var DefaultValidityGluon = time.Hour * 24 * 30
var DefaultValidityVis = time.Minute * 20

// create a new database instance
func New(gluonvalid, visvalid time.Duration, storefile string, logfile string) (*NodeDB, error) {
	main, err := store.Open(storefile)
	if err != nil {
		return nil, err
	}
	logs, err := store.Open(logfile)
	if err != nil {
		return nil, err
	}

	db := NodeDB{
		Main:                   main,
		Logs:                   logs,
		NotifyPurgeVis:         topic.New(),
		NotifyUpdateNodeInfo:   topic.New(),
		NotifyUpdateStatistics: topic.New(),
		NotifyUpdateVis:        topic.New(),
		NotifyQuitUpdater:      topic.New(),
		NotifyQuitPurger:       topic.New(),
		NotifyQuitLogger:       topic.New(),
		validTimeGluon:         gluonvalid,
		validTimeVisData:       visvalid,
	}

	/*
		// run logging handlers
		db.Logger()
	*/

	return &db, nil
}

func (db *NodeDB) StartPurger(gluonpurgeint, vispurgeint time.Duration) {
	go db.Main.Purger(&NodeInfo{}, gluonpurgeint, db.NotifyQuitPurger, nil)
	go db.Main.Purger(&Statistics{}, gluonpurgeint, db.NotifyQuitPurger, nil)
	go db.Main.Purger(&VisData{}, vispurgeint, db.NotifyQuitPurger, db.NotifyPurgeVis)
	go db.Main.Purger(&Gateway{}, vispurgeint, db.NotifyQuitPurger, nil)
	go db.Main.Purger(&Alias{}, vispurgeint, db.NotifyQuitPurger, nil)
}

func (db *NodeDB) StopPurger() {
	db.NotifyQuitPurger.Broadcast <- struct{}{}
}

func (db *NodeDB) StartLogger(offlineAfter time.Duration) {
	go db.LogCounts(offlineAfter)
}

func (db *NodeDB) StopLogger() {
	db.NotifyQuitLogger.Broadcast <- struct{}{}
}

func (db *NodeDB) StartUpdater(client *alfred.Client, updatewait, retrywait time.Duration) {
	i := &NodeInfo{}
	go client.Updater(i, updatewait, retrywait, db.NotifyQuitUpdater, db.NotifyUpdateNodeInfo, db.updateNodeInfo(i, false))
	s := &Statistics{}
	go client.Updater(s, updatewait, retrywait, db.NotifyQuitUpdater, db.NotifyUpdateStatistics, db.updateStatistics(s))
	v := &VisData{}
	go client.Updater(v, updatewait, retrywait, db.NotifyQuitUpdater, db.NotifyUpdateVis, db.updateVisData(v))
}

func (db *NodeDB) StopUpdater() {
	db.NotifyQuitUpdater.Broadcast <- struct{}{}
}

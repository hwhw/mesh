package main

import (
	"github.com/hwhw/mesh/alfred"
	"github.com/hwhw/mesh/nodedb"
	"time"
)

func main() {
	db, err := nodedb.New(
		nodedb.DefaultValidityGluon,
		nodedb.DefaultValidityVis,
		"/tmp/testmain.db",
		"/tmp/testlog.db")
	if err != nil {
		panic(err)
	}
	client := alfred.NewClient("tcp", "10.0.250.2:16962")
	db.StartUpdater(client, time.Minute, time.Second*20)
	db.StartPurger(time.Hour, time.Minute*5)
    db.StartLogger(time.Minute * 5)
	select {}
}

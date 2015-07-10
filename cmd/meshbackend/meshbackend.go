package main

import (
	"flag"
	"github.com/hwhw/mesh/nodedb"
	"github.com/hwhw/mesh/alfred"
	"github.com/hwhw/mesh/webservice"
	"log"
	"time"
)

var db *nodedb.NodeDB
var httpPtr = flag.String(
		"http",
		":8080",
		"listen on this address for HTTP requests, leave empty to disable")
var updateWaitPtr = flag.Duration(
		"updatewait",
		time.Second*60,
		"wait for this duration between update runs")
var retryWaitPtr = flag.Duration(
		"retrywait",
		time.Second*10,
		"wait this for this duration after failed update attempt until retrying")
var clientNetworkPtr = flag.String(
		"clientnetwork",
		"unix",
		"use this type of socket (unix, tcp)")
var clientAddressPtr = flag.String(
		"clientaddress",
		"/var/run/alfred.sock",
		"use this socket address (e.g. unix domain socket, \"host:port\")")
var httpdStaticPtr = flag.String(
		"staticroot",
		"/opt/meshviewer/build",
		"serve static files from this directory")
var nodeOfflineDuration = flag.Duration(
		"offlineafter",
		time.Second*300,
		"consider node offline after not hearing of it for this time")
var gluonPurgePtr = flag.Duration(
		"gluonpurge",
		time.Hour*24*21,
		"purge Gluon node data older than this")
var gluonPurgeIntPtr = flag.Duration(
		"gluonpurgeint",
		time.Hour*1,
		"purge interval for Gluon node data")
var batAdvVisPurgePtr = flag.Duration(
		"batadvvispurge",
		time.Minute*5,
		"purge batman-adv vis data older than this")
var batAdvVisPurgeIntPtr = flag.Duration(
		"batadvvispurgeint",
		time.Minute*1,
		"purge interval for batman-adv vis data")
var storePtr = flag.String(
		"store",
		"/tmp/mesh.db",
		"backing store for mesh database")
var logPtr = flag.String(
		"datalog",
		"/tmp/meshlog.db",
		"backing store for mesh data logging")
var importNodesPtr = flag.String(
		"importnodes",
		"",
		"read nodes from this nodes.json compatible file")
var importNodesPersistentPtr = flag.String(
		"importnodespersistent",
		"",
		"read nodes from this nodes.json compatible file and do not ever drop the records in there")


func main() {
	flag.Parse()

	var err error
	db, err = nodedb.New(
		*gluonPurgePtr,
		*batAdvVisPurgePtr,
		*storePtr,
		*logPtr)

	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}

	if *importNodesPtr != "" {
		if err := db.ImportNodesFile(*importNodesPtr, false); err != nil {
			log.Printf("Error while importing nodes from %v: %v, continuing", *importNodesPtr, err)
		}
	}

	if *importNodesPersistentPtr != "" {
		if err := db.ImportNodesFile(*importNodesPersistentPtr, false); err != nil {
			log.Printf("Error while importing nodes from %v: %v, continuing", *importNodesPersistentPtr, err)
		}
	}

	client := alfred.NewClient(*clientNetworkPtr, *clientAddressPtr, nil)
	db.StartUpdater(client, *updateWaitPtr, *retryWaitPtr)
	db.StartPurger(*gluonPurgeIntPtr, *batAdvVisPurgeIntPtr)
    db.StartLogger(*nodeOfflineDuration)

	if *httpPtr == "" {
        log.Printf("no HTTP server, just updating")
        select {}
	} else {
        log.Printf("starting HTTP server")
        webservice.Run(db, *httpPtr, *httpdStaticPtr, *nodeOfflineDuration)
	}
}

package main

import (
	"compress/gzip"
	"flag"
	"github.com/hwhw/mesh/nodedb"
	"github.com/hwhw/mesh/alfred"
	"io"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

var db *nodedb.NodeDB
var httpPtr = flag.String(
		"http",
		":8080",
		"listen on this address for HTTP requests, leave empty to disable")
var jsonDirPtr = flag.String(
		"jsondir",
		"",
		"write nodes.json and graph.json to this directory")
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
/*
var importNodesPtr = flag.String(
		"importnodes",
		"",
		"read nodes from this nodes.json compatible file")
var importNodesPersistentPtr = flag.String(
		"importnodespersistent",
		"",
		"read nodes from this nodes.json compatible file and do not ever drop the records in there")
*/

func handler_dyn_nodes_json(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")
	if err := db.GenerateNodesJSON(w, *nodeOfflineDuration); err != nil {
		panic(err)
	}
}

func handler_dyn_graph_json(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")
	if err := db.GenerateGraphJSON(w); err != nil {
		panic(err)
	}
}

func handler_export_nodeinfo_json(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")
	db.ExportNodeInfo(w)
}

func handler_export_statistics_json(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")
	db.ExportStatistics(w)
}

func handler_export_visdata_json(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")
	db.ExportVisData(w)
}

func HTTPError(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				http.Error(w, "Internal Server Error", 500)
				log.Printf("Error: %+v\nTrace:\n%s", rec, debug.Stack())
			}
		}()
		handler.ServeHTTP(w, r)
	})
}

func HTTPLog(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		handler.ServeHTTP(w, r)
		end := time.Now()
        log.Printf("HTTP: %s %s %s (%s)", r.RemoteAddr, r.Method, r.URL, end.Sub(start))
	})
}

type gzipResponseWriter struct {
	io.Writer
	http.ResponseWriter
}

func (w gzipResponseWriter) Write(b []byte) (int, error) {
	return w.Writer.Write(b)
}

func compressible(path string) bool {
	if strings.HasSuffix(path, ".js") || strings.HasSuffix(path, ".json") || strings.HasSuffix(path, ".html") || strings.HasSuffix(path, ".css") {
		return true
	}
	return false
}

func HTTPGzip(handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") || !compressible(r.URL.Path) {
			handler.ServeHTTP(w, r)
		} else {
			w.Header().Set("Content-Encoding", "gzip")
			gz := gzip.NewWriter(w)
			defer gz.Close()
			gzw := gzipResponseWriter{Writer: gz, ResponseWriter: w}
			handler.ServeHTTP(gzw, r)
		}
	})
}

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

    /*
	if *importNodesPtr != "" {
		if err := db.ImportNodesFile(*importNodesPtr, false); err != nil {
			log.Printf("Error while reading initial database: %v, continuing", err)
		}
	}
	if *importNodesPersistentPtr != "" {
		if err := db.ImportNodesFile(*importNodesPersistentPtr, false); err != nil {
			log.Printf("Error while reading initial database: %v, continuing", err)
		}
	}
    */

	client := alfred.NewClient(*clientNetworkPtr, *clientAddressPtr)
	db.StartUpdater(client, *updateWaitPtr, *retryWaitPtr)
	db.StartPurger(*gluonPurgeIntPtr, *batAdvVisPurgeIntPtr)
    db.StartLogger(*nodeOfflineDuration)

    if *jsonDirPtr != "" {
		db.StartGenerateJSON(*jsonDirPtr, *nodeOfflineDuration)
    }
	if *httpPtr == "" {
		if *jsonDirPtr == "" {
			log.Printf("no JSON output directory and no HTTP server: this will be a very boring operation")
		}
        select {}
	} else {
		if *jsonDirPtr != "" {
			http.Handle("/json/", http.StripPrefix("/json/", http.FileServer(http.Dir(*jsonDirPtr))))
		} else {
			// generate JSON data on the fly
			http.HandleFunc("/json/nodes.json", handler_dyn_nodes_json)
			http.HandleFunc("/json/graph.json", handler_dyn_graph_json)
		}

        http.HandleFunc("/json/export/nodeinfo.json", handler_export_nodeinfo_json)
        http.HandleFunc("/json/export/statistics.json", handler_export_statistics_json)
        http.HandleFunc("/json/export/visdata.json", handler_export_visdata_json)

		http.Handle("/", http.FileServer(http.Dir(*httpdStaticPtr)))

		log.Printf("MeshData database HTTP server listening on %s", *httpPtr)

		http.ListenAndServe(*httpPtr, HTTPLog(HTTPError(HTTPGzip(http.DefaultServeMux))))
	}
}

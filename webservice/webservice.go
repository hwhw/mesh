package webservice

import (
	"compress/gzip"
	"github.com/boltdb/bolt"
	"github.com/gorilla/mux"
	"github.com/hwhw/mesh/alfred"
	"github.com/hwhw/mesh/nodedb"
	"io"
	"log"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"time"
)

type Webservice struct {
	db                  *nodedb.NodeDB
	nodeOfflineDuration time.Duration
}

func (ws *Webservice) handler_nodes_json(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")
	ws.db.GenerateNodesJSON(w, ws.nodeOfflineDuration)
}

func (ws *Webservice) handler_graph_json(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")
	ws.db.GenerateGraphJSON(w)
}

func (ws *Webservice) handler_nodes_old_json(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")
	ws.db.GenerateNodesOldJSON(w, ws.nodeOfflineDuration)
}

func (ws *Webservice) handler_export_nodeinfo_json(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")
	ws.db.ExportNodeInfo(w)
}

func (ws *Webservice) handler_export_statistics_json(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")
	ws.db.ExportStatistics(w)
}

func (ws *Webservice) handler_export_visdata_json(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")
	ws.db.ExportVisData(w)
}

func (ws *Webservice) handler_export_aliases_json(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")
	ws.db.ExportAliases(w)
}

func (ws *Webservice) handler_loglist_json(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")
	ws.db.GenerateLogList(w)
}

func (ws *Webservice) handler_logdata_json(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var counter nodedb.Counter
	switch vars["id"] {
	case "clients":
		counter = &nodedb.CountMeshClients{}
	case "nodes":
		counter = &nodedb.CountMeshNodes{}
	default:
		addr := &alfred.HardwareAddr{}
		err := addr.Parse(vars["id"])
		if err != nil {
			http.Error(w, "Bad Request", 400)
			return
		}
		realaddr := alfred.HardwareAddr{}
		ws.db.Main.View(func(tx *bolt.Tx) error {
			realaddr = ws.db.ResolveAlias(tx, *addr)
			return nil
		})
		counter = &nodedb.CountNodeClients{Node: realaddr}
	}
	w.Header().Set("Content-type", "application/json")
	ws.db.GenerateLogJSON(w, counter)
}

func (ws *Webservice) handler_logsamples_json(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	over, err := time.ParseDuration(vars["duration"])
	if err != nil {
		http.Error(w, "Bad Request", 400)
		return
	}
	samples, err := strconv.Atoi(vars["samples"])
	if err != nil {
		http.Error(w, "Bad Request", 400)
		return
	}
	start := time.Now().Round(time.Duration(int64(over) / int64(samples)))
	var counter nodedb.Counter
	switch vars["id"] {
	case "clients":
		counter = &nodedb.CountMeshClients{}
	case "nodes":
		counter = &nodedb.CountMeshNodes{}
	default:
		addr := &alfred.HardwareAddr{}
		err := addr.Parse(vars["id"])
		if err != nil {
			http.Error(w, "Bad Request", 400)
			return
		}
		counter = &nodedb.CountNodeClients{Node: *addr}
	}
	w.Header().Set("Content-type", "application/json")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	ws.db.GenerateLogitemSamplesJSON(w, counter, start, over, samples)
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

func Run(db *nodedb.NodeDB, addr string, staticDir string, nodeOfflineDuration time.Duration) {
	ws := &Webservice{db: db, nodeOfflineDuration: nodeOfflineDuration}

	r := mux.NewRouter().StrictSlash(false)

	r.HandleFunc("/json/export/nodeinfo.json", ws.handler_export_nodeinfo_json)
	r.HandleFunc("/json/export/statistics.json", ws.handler_export_statistics_json)
	r.HandleFunc("/json/export/visdata.json", ws.handler_export_visdata_json)
	r.HandleFunc("/json/export/aliases.json", ws.handler_export_aliases_json)
	r.HandleFunc("/json/log/data-{id}.json", ws.handler_logdata_json)
	r.HandleFunc("/json/log/samples-{id}-{duration}-{samples}.json", ws.handler_logsamples_json)
	r.HandleFunc("/json/log/nodes.json", ws.handler_loglist_json)
	r.HandleFunc("/json/old/nodes.json", ws.handler_nodes_old_json)
	r.HandleFunc("/json/nodes.json", ws.handler_nodes_json)
	r.HandleFunc("/json/graph.json", ws.handler_graph_json)

	r.PathPrefix("/").Handler(http.FileServer(http.Dir(staticDir)))

	http.Handle("/", r)

	log.Printf("HTTP: server listening on %s", addr)

	http.ListenAndServe(addr, HTTPLog(HTTPError(HTTPGzip(http.DefaultServeMux))))
}

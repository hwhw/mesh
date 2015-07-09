package webservice

import (
	"compress/gzip"
	"github.com/hwhw/mesh/nodedb"
	"github.com/hwhw/mesh/alfred"
    "net/http"
    "time"
    "io"
    "log"
    "strings"
    "strconv"
    "runtime/debug"
    "github.com/gorilla/mux"
)

type Webservice struct {
    db *nodedb.NodeDB
    nodeOfflineDuration time.Duration
}

func (ws *Webservice) handler_dyn_nodes_json(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")
	if err := ws.db.GenerateNodesJSON(w, ws.nodeOfflineDuration); err != nil {
		panic(err)
	}
}

func (ws *Webservice) handler_dyn_graph_json(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")
	if err := ws.db.GenerateGraphJSON(w); err != nil {
		panic(err)
	}
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
        counter = &nodedb.CountNodeClients{Node: *addr}
    }
	w.Header().Set("Content-type", "application/json")
    ws.db.GenerateLogJSON(w, counter)
}

func (ws *Webservice) handler_logsamples_json(w http.ResponseWriter, r *http.Request) {
    vars := mux.Vars(r)
    start := time.Now()
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

func Run(db *nodedb.NodeDB, addr string, staticDir string, jsonDir string, nodeOfflineDuration time.Duration) {
    ws := &Webservice{db:db, nodeOfflineDuration:nodeOfflineDuration}

    r := mux.NewRouter().StrictSlash(false)

    r.HandleFunc("/json/export/nodeinfo.json", ws.handler_export_nodeinfo_json)
    r.HandleFunc("/json/export/statistics.json", ws.handler_export_statistics_json)
    r.HandleFunc("/json/export/visdata.json", ws.handler_export_visdata_json)

    r.HandleFunc("/json/log/data/{id}.json", ws.handler_logdata_json)
    r.HandleFunc("/json/log/samples/{id}/{duration}/{samples}.json", ws.handler_logsamples_json)
    r.HandleFunc("/json/log/nodes.json", ws.handler_loglist_json)

    if jsonDir != "" {
        r.Handle("/json/", http.StripPrefix("/json/", http.FileServer(http.Dir(jsonDir))))
    } else {
        // generate JSON data on the fly
        r.HandleFunc("/json/nodes.json", ws.handler_dyn_nodes_json)
        r.HandleFunc("/json/graph.json", ws.handler_dyn_graph_json)
    }

    r.Handle("/", http.FileServer(http.Dir(staticDir)))

    http.Handle("/", r)

    log.Printf("HTTP: server listening on %s", addr)

    http.ListenAndServe(addr, HTTPLog(HTTPError(HTTPGzip(http.DefaultServeMux))))
}

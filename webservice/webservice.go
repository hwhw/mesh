package webservice

import (
	"compress/gzip"
	"github.com/gorilla/mux"
	"github.com/hwhw/mesh/nodedb"
	"io"
	"log"
	"net/http"
	"runtime/debug"
	"strings"
	"time"
)

type Webservice struct {
	db                  *nodedb.NodeDB
	nodeOfflineDuration time.Duration
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

func HTTPLog(handler http.Handler, server string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		handler.ServeHTTP(w, r)
		end := time.Now()
		log.Printf("HTTP (%s): %s %s %s (%s)", server, r.RemoteAddr, r.Method, r.URL, end.Sub(start))
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

func Run(db *nodedb.NodeDB, addr string, addradmin string, staticDir string, nodeOfflineDuration time.Duration) {
	ws := &Webservice{db: db, nodeOfflineDuration: nodeOfflineDuration}

	r := mux.NewRouter().StrictSlash(false)
	r.HandleFunc("/json/log/samples-{id}-{duration}-{samples}.json", ws.handler_logsamples_json)
	r.HandleFunc("/json/old/nodes.json", ws.handler_nodes_old_json)
	r.HandleFunc("/json/nodes.json", ws.handler_nodes_json)
	r.HandleFunc("/json/graph.json", ws.handler_graph_json)
	r.PathPrefix("/").Handler(http.FileServer(http.Dir(staticDir)))

	ra := mux.NewRouter().StrictSlash(false)
	ra.HandleFunc("/export/nodeinfo.json", ws.handler_export_nodeinfo_json)
	ra.HandleFunc("/export/statistics.json", ws.handler_export_statistics_json)
	ra.HandleFunc("/export/visdata.json", ws.handler_export_visdata_json)
	ra.HandleFunc("/export/aliases.json", ws.handler_export_aliases_json)
	ra.HandleFunc("/log/{id}", ws.handler_logdata_json).Methods("GET")
	ra.HandleFunc("/log/{id}/{timestamp}", ws.handler_logdata_delete).Methods("DELETE")
    ra.HandleFunc("/log/{what}", ws.handler_logdata_post).Methods("POST")
	ra.HandleFunc("/log/", ws.handler_loglist_json).Methods("GET")

	log.Printf("HTTP: server listening on %s", addr)

    go http.ListenAndServe(addr, HTTPLog(HTTPError(HTTPGzip(r)), "public"))
    http.ListenAndServe(addradmin, HTTPLog(HTTPError(HTTPGzip(ra)), "admin"))
}

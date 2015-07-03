package main

import (
    "github.com/hwhw/mesh/nodedb"
    "log"
    "net/http"
    "time"
    "flag"
    "runtime/debug"
    "compress/gzip"
    "io"
    "strings"
)

var db *nodedb.NodeDB

func handler_dyn_nodes_json(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-type", "application/json")
    if err := db.GenerateNodesJSON(w); err != nil {
        panic(err)
    }
}

func handler_dyn_graph_json(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-type", "application/json")
    if err := db.GenerateGraphJSON(w); err != nil {
        panic(err)
    }
}

func handler_log_json(w http.ResponseWriter, r *http.Request) {
    w.Header().Set("Content-type", "application/json")
    err := db.JSONClientsTotal(w, r.URL.Query().Get("node"), time.Now(), time.Hour * 24)
    if err != nil {
        panic(err)
    }
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
        log.Printf("HTTP %s %s %s (%s)", r.RemoteAddr, r.Method, r.URL, end.Sub(start))
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
    httpPtr := flag.String(
        "http",
        ":8080",
        "listen on this address for HTTP requests, leave empty to disable")
    jsonDirPtr := flag.String(
        "jsondir",
        "",
        "write nodes.json and graph.json to this directory")
    jsonIntervalPtr := flag.Duration(
        "jsonint",
        time.Second * 60,
        "time between two JSON file generations")
    updateWaitPtr := flag.Duration(
        "updatewait",
        time.Second * 60,
        "wait for this duration between update runs")
    retryWaitPtr := flag.Duration(
        "retrywait",
        time.Second * 10,
        "wait this for this duration after failed update attempt until retrying")
    clientNetworkPtr := flag.String(
        "clientnetwork",
        "unix",
        "use this type of socket (unix, tcp)")
    clientAddressPtr := flag.String(
        "clientaddress",
        "/var/run/alfred.sock",
        "use this socket address (e.g. unix domain socket, \"host:port\")")
    httpdStaticPtr := flag.String(
        "staticroot",
        "/opt/meshviewer/build",
        "serve static files from this directory")
    nodeOfflineDuration := flag.Duration(
        "offlineafter",
        time.Second * 300,
        "consider node offline after not hearing of it for this time")
    gluonPurgePtr := flag.Duration(
        "gluonpurge",
        time.Hour * 24 * 21,
        "purge Gluon node data older than this")
    gluonPurgeIntPtr := flag.Duration(
        "gluonpurgeint",
        time.Hour * 1,
        "purge interval for Gluon node data")
    batAdvVisPurgePtr := flag.Duration(
        "batadvvispurge",
        time.Minute * 5,
        "purge batman-adv vis data older than this")
    batAdvVisPurgeIntPtr := flag.Duration(
        "batadvvispurgeint",
        time.Minute * 1,
        "purge interval for batman-adv vis data")
    storePtr := flag.String(
        "store",
        "/tmp/mesh.db",
        "backing store for mesh database")
    logPtr := flag.String(
        "datalog",
        "/tmp/meshlog.db",
        "backing store for mesh data logging")
    importNodesPtr := flag.String(
        "importnodes",
        "",
        "read nodes from this nodes.json compatible file")
    importNodesPersistentPtr := flag.String(
        "importnodespersistent",
        "",
        "read nodes from this nodes.json compatible file and do not ever drop the records in there")

    flag.Parse()

    var err error
    db, err = nodedb.New(
        *nodeOfflineDuration,
        gluonPurgePtr,
        gluonPurgeIntPtr,
        batAdvVisPurgePtr,
        batAdvVisPurgeIntPtr,
        *storePtr,
        *logPtr)

    if err != nil {
        log.Fatalf("Error opening database: %v", err)
    }

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

    db.NewUpdateClient(*clientNetworkPtr, *clientAddressPtr, *updateWaitPtr, *retryWaitPtr)

    if *httpPtr == "" {
        if *jsonDirPtr == "" {
            log.Printf("no JSON output directory and no HTTP server: this will be a very boring operation")
        } else {
            db.GeneratorJSON(*jsonDirPtr, *jsonIntervalPtr)
        }
    } else {
        if *jsonDirPtr != "" {
            go db.GeneratorJSON(*jsonDirPtr, *jsonIntervalPtr)
            http.Handle("/json/", http.StripPrefix("/json/", http.FileServer(http.Dir(*jsonDirPtr))))
        } else {
            // generate JSON data on the fly
            http.HandleFunc("/json/nodes.json", handler_dyn_nodes_json)
            http.HandleFunc("/json/graph.json", handler_dyn_graph_json)
        }
        http.HandleFunc("/json/log.json", handler_log_json)
        http.Handle("/", http.FileServer(http.Dir(*httpdStaticPtr)))

        log.Printf("MeshData database HTTP server listening on %s", *httpPtr)

        http.ListenAndServe(*httpPtr, HTTPLog(HTTPError(HTTPGzip(http.DefaultServeMux))))
    }
}

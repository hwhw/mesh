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
    "os"
    "strings"
)

var db nodedb.NodeDB

func handler_dyn_nodes_json(w http.ResponseWriter, r *http.Request) {
    defer func() {
        if r := recover(); r != nil {
            http.Error(w, "Internal Server Error", 500)
            log.Printf("Error: %+v\nTrace:\n%s", r, debug.Stack())
        }
    }()
    w.Header().Set("Content-type", "application/json")
    if err := db.GenerateNodesJSON(w); err != nil {
        panic(err)
    }
}

func handler_dyn_graph_json(w http.ResponseWriter, r *http.Request) {
    defer func() {
        if r := recover(); r != nil {
            http.Error(w, "Internal Server Error", 500)
            log.Printf("Error: %+v\nTrace:\n%s", r, debug.Stack())
        }
    }()
    w.Header().Set("Content-type", "application/json")
    if err := db.GenerateGraphJSON(w); err != nil {
        panic(err)
    }
}

func Log(handler http.Handler) http.Handler {
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

func Gzip(handler http.Handler) http.Handler {
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
    addressPtr := flag.String("address", ":8080", "listen on this address for HTTP requests")
    updateWaitPtr := flag.Duration("updatewait", time.Second * 60, "wait for this duration between update runs")
    retryWaitPtr := flag.Duration("retrywait", time.Second * 10, "wait this for this duration after failed update attempt until retrying")
    clientNetworkPtr := flag.String("clientnetwork", "unix", "use this type of socket (unix, tcp)")
    clientAddressPtr := flag.String("clientaddress", "/var/run/alfred.sock", "use this socket address (e.g. unix domain socket, \"host:port\")")
    httpdStaticPtr := flag.String("staticroot", "static/", "serve static files from this directory")
    initialDatabasePtr := flag.String("initialdb", "", "read initial database from this nodes.json file")
    persistentDatabasePtr := flag.String("persistentdb", "", "read this nodes.json compatible file and do not ever drop the records in there")
    nodeOfflineDuration := flag.Duration("offlineafter", time.Second * 300, "consider node offline after not hearing of it for this time")
    gluonPurgePtr := flag.Duration("gluonpurge", time.Hour * 24 * 21, "purge Gluon node data older than this")
    gluonPurgeIntPtr := flag.Duration("gluonpurgeint", time.Hour * 1, "purge interval for Gluon node data")
    batAdvVisPurgePtr := flag.Duration("batadvvispurge", time.Minute * 20, "purge batman-adv vis data older than this")
    batAdvVisPurgeIntPtr := flag.Duration("batadvvispurgeint", time.Minute * 2, "purge interval for batman-adv vis data")
    logPtr := flag.String("datalog", "/tmp/meshdatalog.db", "backing store for mesh data log")

    flag.Parse()

    db = nodedb.New(
        *nodeOfflineDuration,
        *gluonPurgePtr,
        *gluonPurgeIntPtr,
        *batAdvVisPurgePtr,
        *batAdvVisPurgeIntPtr,
        *logPtr)

    if *initialDatabasePtr != "" {
        f, err := os.Open(*initialDatabasePtr)
        if err != nil {
            panic(err)
        }
        if err := db.ReadNodesJSONtoDB(f, false); err != nil {
            log.Printf("Error while reading initial database: %v, continuing", err)
        }
        f.Close()
    }
    if *persistentDatabasePtr != "" {
        f, err := os.Open(*persistentDatabasePtr)
        if err != nil {
            panic(err)
        }
        if err := db.ReadNodesJSONtoDB(f, true); err != nil {
            log.Printf("Error while reading persistent database: %v, continuing", err)
        }
        f.Close()
    }
    db.NewUpdateClient(*clientNetworkPtr, *clientAddressPtr, *updateWaitPtr, *retryWaitPtr, nil)

    http.HandleFunc("/nodes.json", handler_dyn_nodes_json)
    http.HandleFunc("/graph.json", handler_dyn_graph_json)
    http.Handle("/", http.FileServer(http.Dir(*httpdStaticPtr)))
    log.Printf("MeshData database server listening on %s", *addressPtr)
    http.ListenAndServe(*addressPtr, Log(Gzip(http.DefaultServeMux)))
}

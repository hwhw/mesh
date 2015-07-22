package webservice

import (
    "net/http"
	"github.com/gorilla/mux"
    "github.com/hwhw/mesh/alfred"
    "github.com/hwhw/mesh/nodedb"
    "time"
    "strconv"
    "encoding/json"
    "github.com/boltdb/bolt"
)

func (ws *Webservice) handler_loglist_json(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-type", "application/json")
	ws.db.GenerateLogList(w)
}

func (ws *Webservice) handler_logdata_post(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	var counter nodedb.Counter
	switch vars["what"] {
	case "clients":
		counter = &nodedb.CountMeshClients{}
	case "nodes":
		counter = &nodedb.CountMeshNodes{}
    case "node":
        counter = &nodedb.CountNodeClients{Node:alfred.HardwareAddr{}}
	default:
        http.Error(w, "Bad Request", 400)
        return
	}
    dec := json.NewDecoder(r.Body)
    err := dec.Decode(counter)
	if err == nil {
        err = ws.db.Logs.Batch(func(tx *bolt.Tx) error {
            return ws.db.Logs.Put(tx, counter)
        })
    }
    if err != nil {
        http.Error(w, "Bad Request", 400)
        return
    }
	w.Header().Set("Content-type", "application/json")
    w.WriteHeader(http.StatusCreated)
	ws.db.GenerateLogJSON(w, counter)
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

func (ws *Webservice) handler_logdata_delete(w http.ResponseWriter, r *http.Request) {
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
    var timestamp time.Time
    err := timestamp.UnmarshalText([]byte(vars["timestamp"]))
    if err != nil {
        http.Error(w, "Bad Request", 400)
        return
    }
    counter.SetTimestamp(timestamp)
    err = ws.db.Logs.Update(func (tx *bolt.Tx) error {
        return ws.db.Logs.Delete(tx, counter)
    })
    if err != nil {
        http.Error(w, "Not Found", 404)
        return
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


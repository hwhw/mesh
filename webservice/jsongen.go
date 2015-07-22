package webservice

import (
    "net/http"
)

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

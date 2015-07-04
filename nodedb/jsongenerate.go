package nodedb

import (
	"io"
	"log"
	"os"
	"path/filepath"
    "time"
)

func (db *NodeDB) GenerateJSON(directory string, offlineDuration time.Duration) {
	quit := make(chan interface{})
	db.NotifyQuitGenerateJSON.Register(quit)
	defer db.NotifyQuitGenerateJSON.Unregister(quit)

	updateni := make(chan interface{})
	db.NotifyUpdateNodeInfo.Register(updateni)
	defer db.NotifyUpdateNodeInfo.Unregister(updateni)

	updatevis := make(chan interface{})
	db.NotifyUpdateNodeInfo.Register(updatevis)
	defer db.NotifyUpdateNodeInfo.Unregister(updatevis)

    doneni := make(chan interface{})
    donevis := make(chan interface{})

    runni := 0
    runvis := 0

    generatenodesjson := func(w io.Writer) error {
        return db.GenerateNodesJSON(w, offlineDuration)
    }
    for {
        select {
        case <-quit:
            return
        case <-updateni:
            if runni == 0 {
                go db.generateJSONFile(directory, "nodes.json", "nodes.json.new", generatenodesjson)
            }
            runni++
        case <-doneni:
            if runni > 1 {
                runni = 1
                go db.generateJSONFile(directory, "nodes.json", "nodes.json.new", generatenodesjson)
            } else {
                runni = 0
            }
        case <-updatevis:
            if runvis == 0 {
                go db.generateJSONFile(directory, "graph.json", "graph.json.new", db.GenerateGraphJSON)
            }
            runvis++
        case <-donevis:
            if runvis > 1 {
                runvis = 1
                go db.generateJSONFile(directory, "graph.json", "graph.json.new", db.GenerateGraphJSON)
            } else {
                runvis = 0
            }
        }
    }
}

func (db *NodeDB) generateJSONFile(directory string, name string, tmpname string, generator func(io.Writer) error) {
	fg, err := os.Create(filepath.Join(directory, tmpname))
	if err != nil {
        log.Printf("GenerateJSON: cannot open file in %v: %v", directory, err)
		return
	}
	err = generator(fg)
	fg.Close()
	if err != nil {
        log.Printf("GenerateJSON: cannot write JSON data: %v", err)
		return
	}
	err = os.Rename(filepath.Join(directory, tmpname), filepath.Join(directory, name))
	if err != nil {
        log.Printf("GenerateJSON: cannot update JSON data: %v", err)
	}
}

package nodedb

import (
    "github.com/hwhw/mesh/boltdb"
    "time"
    "os"
    "io"
    "log"
    "path/filepath"
)

func (db *NodeDB) GeneratorJSON(directory string, interval time.Duration) {
    notifications := db.store.Subscribe()
    for {
        n := <-notifications
        switch n.(type) {
        case boltdb.QuitNotification:
            return
        case UpdateNodeInfoNotification:
            db.GenerateJSONFile(directory, "nodes.json", "nodes.json.new", db.GenerateNodesJSON)
        case UpdateVisDataNotification:
            db.GenerateJSONFile(directory, "graph.json", "graph.json.new", db.GenerateGraphJSON)
        }
    }
}

func (db *NodeDB) GenerateJSONFile(directory string, name string, tmpname string, generator func(io.Writer) error) {
    fg, err := os.Create(filepath.Join(directory, tmpname))
    if err != nil {
        log.Printf("cannot open file in %v: %v", directory, err)
        return
    }
    err = generator(fg)
    fg.Close()
    if err != nil {
        log.Printf("cannot write JSON data: %v", err)
        return
    }
    err = os.Rename(filepath.Join(directory, tmpname), filepath.Join(directory, name))
    if err != nil {
        log.Printf("cannot update JSON data: %v", err)
    }
}

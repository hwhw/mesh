package main

import (
    "github.com/hwhw/mesh/nodedb"
    "github.com/hwhw/mesh/alfred"
    "flag"
    "net/http"
    "fmt"
    "os"
    "encoding/json"
    "time"
    "strconv"
    "bytes"
)

var webadmin = flag.String("w", "127.0.0.1:8081", "meshweb admin server address")

func failure(msg string, args ...interface{}) {
	fmt.Fprintf(os.Stderr, msg, args...)
	os.Exit(-1)
}

func usage() {
	failure(`
Usage:

meshctl [options] <command> <command options>

available options:
-w <address>   meshweb admin server address

available commands and additional options if any:

  listnodes

  importrrdxml <what>

        <what> is either "nodes", "clients" or
        the ID of a node.
`)
}

func getjson(url string, dest interface{}) error {
    resp, err := http.Get(url)
    if err != nil {
        return err
    }
    dec := json.NewDecoder(resp.Body)
    err = dec.Decode(dest)
    return err
}

func cmd_log() error {
    switch flag.Arg(1) {
    case "l", "ls", "list":
        list := make([]string, 0, 100)
        err := getjson(fmt.Sprintf("http://%s/log/", *webadmin), &list)
        if err != nil {
            return err
        }
        for _, id := range list {
            fmt.Println(id)
        }
    case "s", "show":
        switch flag.Arg(2) {
        case "":
            failure("must specify which logs you want\n")
        case "nodes":
            list := make([]nodedb.CountMeshNodes, 0, 100)
            err := getjson(fmt.Sprintf("http://%s/log/%s", *webadmin, flag.Arg(2)), &list)
            if err != nil {
                return err
            }
            for _, logitem := range list {
                fmt.Printf("%v; %v\n", logitem.Timestamp.Format(time.RFC3339Nano), logitem.Count.Count)
            }
        case "clients":
            list := make([]nodedb.CountMeshClients, 0, 100)
            err := getjson(fmt.Sprintf("http://%s/log/%s", *webadmin, flag.Arg(2)), &list)
            if err != nil {
                return err;
            }
            for _, logitem := range list {
                fmt.Printf("%v; %v\n", logitem.Timestamp.Format(time.RFC3339Nano), logitem.Count.Count)
            }
        default:
            list := make([]nodedb.CountNodeClients, 0, 100)
            err := getjson(fmt.Sprintf("http://%s/log/%s", *webadmin, flag.Arg(2)), &list)
            if err != nil {
                return err
            }
            for _, logitem := range list {
                fmt.Printf("%v; %v\n", logitem.Timestamp.Format(time.RFC3339Nano), logitem.Count.Count)
            }
        }
    case "d", "del", "delete":
        t, err := time.Parse(time.RFC3339Nano, flag.Arg(3))
        if err != nil {
            return err
        }
        switch flag.Arg(2) {
        case "":
            failure("must specify which log type you want to delete\n")
        default:
            req, err := http.NewRequest("DELETE", fmt.Sprintf("http://%s/log/%s/%s", *webadmin, flag.Arg(2), t.Format(time.RFC3339Nano)), nil)
            if err != nil {
                return err
            }
            _, err = http.DefaultClient.Do(req)
            return err
        }
    case "n", "new", "a", "add", "u", "update":
        t, err := time.Parse(time.RFC3339Nano, flag.Arg(3))
        if err != nil {
            return err
        }
        c, err := strconv.Atoi(flag.Arg(4))
        if err != nil {
            return err
        }
        var counter nodedb.Counter
        what := flag.Arg(2)
        switch what {
        case "":
            failure("must specify which log type you want to add/update\n")
        case "clients":
            counter = &nodedb.CountMeshClients{}
        case "nodes":
            counter = &nodedb.CountMeshNodes{}
        default:
            ctr := &nodedb.CountNodeClients{Node:alfred.HardwareAddr{}}
            err := ctr.Node.Parse(what)
            if err != nil {
                return err
            }
            counter = ctr
            what = "node"
        }
        counter.SetTimestamp(t)
        counter.SetCount(c)
        buf := new(bytes.Buffer)
        enc := json.NewEncoder(buf)
        err = enc.Encode(counter)
        if err != nil {
            return err
        }
        _, err = http.Post(fmt.Sprintf("http://%s/log/%s", *webadmin, what), "application/json", buf)
        if err != nil {
            return err
        }
	case "import":
    default:
        failure("unknown/unspecified log subcommand\n")
    }
    return nil
}

func main() {
	flag.Parse()
	var reterr error
	switch flag.Arg(0) {
    case "log":
        reterr = cmd_log()
	default:
		usage()
	}
	if reterr != nil {
		failure("error: %v\n", reterr)
	}
}

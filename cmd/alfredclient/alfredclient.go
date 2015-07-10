package main

import (
    "github.com/hwhw/mesh/alfred"
    "flag"
    "os"
    "fmt"
    "strconv"
    "io"
    "io/ioutil"
    "compress/gzip"
    "compress/zlib"
    "bytes"
)

var network = flag.String("p", "unix", "network (unix, tcp) to use for connecting server")
var address = flag.String("a", "/var/run/alfred.sock", "address to connect to")
var zlibpipe = flag.Bool("z", false, "zlib compress/uncompress data")
var gzippipe = flag.Bool("g", false, "gzip/gunzip data")

func failure(msg string, args... interface{}) {
    fmt.Fprintf(os.Stderr, msg, args...)
    os.Exit(-1)
}

func usage() {
    failure(`
Usage:

alfredclient [options] <command> <command options>

available options:
 -p<network>  network to use (unix, tcp)
 -a<address>  address to use for connecting server

available commands and additional options if any:

 set <type>    will set local data for type <type> (1-255)
               data will be read from stdin.
     -g        gzip compress data before storing
     -z        zlib compress data before storing

 get <type>    will fetch and output data
     -g        gzip uncompress data before outputting
     -z        zlib uncompress data before outputting

 mode <modeid> will request server to switch to operation mode <modeid>
               0: slave mode
               1: master mode
               2: stealth master mode (only go-based alfred implementation)

 interfaces <iface list>
               request server to listen on given interfaces.
`)
}

func compress(data []byte) []byte {
    if *zlibpipe || *gzippipe {
        zipped := new(bytes.Buffer)
        var zwriter io.WriteCloser
        if *zlibpipe {
            zwriter = zlib.NewWriter(zipped)
        } else {
            zwriter = gzip.NewWriter(zipped)
        }
        _, err := zwriter.Write(data)
        if err == nil {
            err = zwriter.Close()
        }
        if err != nil {
            failure("error gzipping data: %v\n", err)
        }
        return zipped.Bytes()
    } else {
        return data
    }
}

func uncompress(data []byte) []byte {
    if *zlibpipe || *gzippipe {
        var err error
        var n int
        unzipped := make([]byte, 0, 1024)
        out := make([]byte, 1024)
        zreader := bytes.NewReader(data)
        var uzreader io.Reader
        if *zlibpipe {
            uzreader, err = zlib.NewReader(zreader)
        } else {
            uzreader, err = gzip.NewReader(zreader)
        }
        pos := 0
        for err == nil {
            n, err = uzreader.Read(out[0:1024])
            pos += n
            unzipped = append(unzipped, out[0:n]...)
        }
        return unzipped
    } else {
        return data
    }
}

func main() {
    flag.Parse()
    client := alfred.NewClient(*network, *address, nil)
    var reterr error
    switch flag.Arg(0) {
    case "set":
        id, err := strconv.Atoi(flag.Arg(1))
        if err != nil || id < 1 || id > 255 {
            failure("error: invalid type %v\n", flag.Arg(1))
            goto failure
        }
        buf, err := ioutil.ReadAll(os.Stdin)
        if err != nil {
            failure("error: reading data from stdin, %v\n", err)
        }
        reterr = client.PushData(uint8(id), compress(buf))
    case "get":
        id, err := strconv.Atoi(flag.Arg(1))
        if err != nil || id < 1 || id > 255 {
            failure("error: invalid type %v\n", flag.Arg(1))
            goto failure
        }
        reterr = client.Request(uint8(id), func(d alfred.Data) error {
            fmt.Printf("{%s, \"", d.Source)
            buf := uncompress(d.Data)
            for _, c := range buf {
                switch {
                case c == 0x5C: // backslash
                    os.Stdout.Write([]byte{c,c})
                case c == 0x22: // parenthesis
                    os.Stdout.Write([]byte{0x5C,c})
                case 0x20 <= c && c <= 0x7E:
                    os.Stdout.Write([]byte{c})
                default:
                    fmt.Printf("\\x%02x", c)
                }
            }
            fmt.Print("\"},\n")
            return nil
        })
    case "mode":
        mode, err := strconv.Atoi(flag.Arg(1))
        if err != nil || mode < 0 || mode > 2 {
            failure("error: invalid mode %v\n", flag.Arg(1))
            goto failure
        }
        reterr = client.ModeSwitch(uint8(mode))
    case "interfaces":
        ifaces := []byte(flag.Arg(1))
        if len(ifaces) < 1 {
            failure("you need to specify interface(s)")
        }
        reterr = client.ChangeInterface(ifaces)
    default:
        usage()
    }
    if reterr != nil {
        failure("error: %v\n", reterr)
    }
    return
failure:
    os.Exit(-1)
}

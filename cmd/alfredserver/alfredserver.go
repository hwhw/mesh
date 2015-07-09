package main

import (
    "github.com/hwhw/mesh/alfred"
    "flag"
    "log"
    "os"
    "os/signal"
    "syscall"
)

func main() {
    modePtr := flag.String(
		"m",
		"slave",
		"operation mode (slave, master, stealthmaster)")
    ifacePtr := flag.String(
		"i",
		"bat0",
		"interface to listen on")
    tcpaddrPtr := flag.String(
        "t",
        "",
        "address/port to listen on for TCP requests")
    unixaddrPtr := flag.String(
        "u",
        "/var/run/alfred.sock",
        "unix socket to listen on")
    addrPtr := flag.String(
        "a",
        "[ff02::1]:16962",
        "address/port to listen on for UDP requests")
    flag.Parse()

    var mode int
    switch *modePtr {
    case "slave":
        mode = alfred.SERVER_MODE_SLAVE
    case "master":
        mode = alfred.SERVER_MODE_MASTER
    case "stealthmaster":
        mode = alfred.SERVER_MODE_STEALTH_MASTER
    default:
        log.Fatalf("invalid mode specified")
    }

    server := alfred.NewServer(mode)
    if *tcpaddrPtr != "" {
        err := server.NewListenerStream("tcp", *tcpaddrPtr)
        if err != nil {
            log.Fatalf("error listening on TCP address %v: %v", *tcpaddrPtr, err)
        }
    }
    if *unixaddrPtr != "" {
        err := server.NewListenerStream("unix", *unixaddrPtr)
        if err != nil {
            log.Fatalf("error listening on Unix socket %v: %v", *unixaddrPtr, err)
        }
    }
    err := server.NewListenerUDP(*addrPtr, *ifacePtr)
    if err != nil {
        log.Fatalf("error listening on interface %v, address %v: %v", *ifacePtr, *addrPtr, err)
    }
    log.Printf("now running!")
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt, os.Kill, syscall.SIGTERM)
    <-c
    server.Shutdown()
}

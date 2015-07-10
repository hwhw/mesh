# Go-based backend for batman-adv/Gluon mesh metadata

## Installation

```
go install github.com/hwhw/mesh/...
```

## running

see `meshbackend --help`

## General code structure

alfred:
    code related to Almighty Lightweight Fact Remote Exchange Daemon (A.L.F.R.E.D.), the information broker from the developers of the B.A.T.M.A.N. Advanced layer 2 mesh routing protocol implementation. Contains the data model as well as a client implementation that can fetch data from a running server instance of "alfred" and also a server, able to fully replace the C variant "alfred" - except of course of its small resource footprint.

batadvvis:
    data model of the "vis" data that is distributed via A.L.F.R.E.D. by nodes running both that and the batadv-vis daemon.

gluon:
    data model of the node information and statistics distributed by mesh nodes running the "Gluon" based firmware used in many "Freifunk" communities.

store:
    storage abstraction using the Bolt database

nodedb:
    Where it all comes together

webservice:
    Base for all offered webservices

cmd:
    The utilities

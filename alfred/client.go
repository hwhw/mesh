package alfred

import (
    "net"
    "io"
    "encoding/binary"
    "crypto/rand"
    "bufio"
    "errors"
    "time"
//    "log"
)

var ErrStatus = errors.New("A.L.F.R.E.D. server reported an error")
var ErrProtocol = errors.New("Bad data received from A.L.F.R.E.D. server")

// wrapper for getting a random uint16 used for transaction IDs
func getRandomId() (randval uint16) {
    binary.Read(rand.Reader, binary.LittleEndian, &randval)
    return
}

// An A.L.F.R.E.D. client.
//
// It will create a new connection for each request - since the
// C implementation will close the connection when it has handled
// a request.
type Client struct {
    network string
    address string
    timeout time.Duration
}

// Return a new client instance
func NewClient(network string, address string, timeout time.Duration) Client {
    return Client{network: network, address: address, timeout: timeout}
}

// Request data of a given type.
//
// Returns the data received, and possibly an error if there was an
// error at some point.
func (c Client) Request(requestedtype uint8) (data []Data, err error) {
    data = make([]Data, 0, 100)
    conn, err := net.Dial(c.network, c.address)
    if err != nil {
        return data, err
    }
    defer conn.Close()
    conn.SetDeadline(time.Now().Add(c.timeout))
    buf := bufio.NewWriter(conn)
    req := NewRequestV0(requestedtype, getRandomId())
    if err = req.Write(buf); err != nil {
        return data, err
    }
    if err = buf.Flush(); err != nil {
        return data, err
    }
    for {
        pkg, err, _ := Read(conn)
        switch err {
        case nil:
            conn.SetDeadline(time.Now().Add(c.timeout))
            if pd, ok := pkg.(*PushDataV0); ok {
                for _, d := range pd.Data {
                    data = append(data, d)
                }
            } else if status, ok := pkg.(*StatusV0); ok && status.Header.Type == ALFRED_STATUS_ERROR {
                return data, ErrStatus
            } else {
                return data, ErrProtocol
            }
        case io.EOF:
            // don't pass through this error, it is just the
            // end of the transaction
            return data, nil
        default:
            // pass through other errors
            return data, err
        }
    }
}

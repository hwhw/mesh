package alfred

import (
	"bufio"
	"errors"
	"github.com/tv42/topic"
	"io"
	"log"
	"net"
	"time"
)

var ErrStatus = errors.New("A.L.F.R.E.D. server reported an error")
var ErrProtocol = errors.New("Bad data received from A.L.F.R.E.D. server")

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

var defaultTimeout = time.Second * 10

// Return a new client instance
func NewClient(network string, address string, timeout *time.Duration) *Client {
    t := defaultTimeout
    if timeout != nil {
        t = *timeout
    }
    return &Client{network: network, address: address, timeout: t}
}

func (c *Client) Connect(handler func(net.Conn, *bufio.Writer) error) error {
	conn, err := net.Dial(c.network, c.address)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(c.timeout))
	buf := bufio.NewWriter(conn)
    return handler(conn, buf)
}

func (c *Client) PushData(packettype uint8, data []byte) error {
    return c.Connect(func(conn net.Conn, buf *bufio.Writer) error {
        tm := &TransactionMgmt{Id: getRandomId(), SeqNo: 0}
        pdata := []Data{Data{Source: NullHardwareAddr, Header: &TLV{Type: packettype}, Data: data}}
        pd := NewPushDataV0(tm, pdata)
        err := pd.Write(buf)
        if err == nil {
            err = buf.Flush()
        }
        return err
    })
}

// Request data of a given type.
func (c *Client) Request(contentitem Content, handler func() error) error {
    return c.Connect(func(conn net.Conn, buf *bufio.Writer) (err error) {
        req := NewRequestV0(contentitem.GetPacketType(), getRandomId())
        if err = req.Write(buf); err != nil {
            return err
        }
        if err = buf.Flush(); err != nil {
            return err
        }
        for {
            pkg, err, _ := Read(conn)
            switch err {
            case nil:
                if pd, ok := pkg.(*PushDataV0); ok {
                    for _, d := range pd.Data {
                        err = contentitem.ReadAlfred(d)
                        if err != nil {
                            // just skip
                            continue
                        }
                        err = handler()
                        if err != nil {
                            return err
                        }
                    }
                } else if status, ok := pkg.(*StatusV0); ok && status.Header.Type == ALFRED_STATUS_ERROR {
                    return ErrStatus
                } else {
                    return ErrProtocol
                }
            case io.EOF:
                // don't pass through this error, it is just the
                // end of the transaction
                return nil
            default:
                // pass through other errors
                return err
            }
        }
    })
}

// Create a new update client.
// the time to wait between updates in updatewait and the time to wait after failure
// before retrying in retrywait.
// The updatewait duration is also the timeout duration for the actual
// network connections.
func (c *Client) Updater(
	contentitem Content,
	updatewait time.Duration, retrywait time.Duration,
	notifyQuit *topic.Topic,
	notifySuccess *topic.Topic,
	handler func() error) {

	quit := make(chan interface{})
	notifyQuit.Register(quit)
	defer notifyQuit.Unregister(quit)

	for {
		timeout := updatewait
		log.Printf("UpdateClient: Updating data from alfred server for type %d", contentitem.GetPacketType())
		err := c.Request(contentitem, handler)
		if err != nil {
			log.Printf("UpdateClient: type %d, error fetching data: %v", contentitem.GetPacketType(), err)
			timeout = retrywait
		} else {
			notifySuccess.Broadcast <- struct{}{}
		}
		select {
		case <-quit:
			break
		case <-time.After(timeout):
			continue
		}
	}
}

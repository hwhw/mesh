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

// wrapper for the network connection
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

// switch server mode
func (c *Client) ModeSwitch(mode uint8) error {
	return c.Connect(func(conn net.Conn, buf *bufio.Writer) error {
		err := NewModeSwitchV0(mode).Write(buf)
		if err == nil {
			err = buf.Flush()
		}
		return err
	})
}

// change interface(s) the server listens on for UDP connections
func (c *Client) ChangeInterface(interfaces []byte) error {
	return c.Connect(func(conn net.Conn, buf *bufio.Writer) error {
		err := NewChangeInterfaceV0(interfaces).Write(buf)
		if err == nil {
			err = buf.Flush()
		}
		return err
	})
}

// push data of a given type
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

// Request data of a given type
func (c *Client) Request(packettype uint8, handler func(Data) error) error {
	return c.Connect(func(conn net.Conn, buf *bufio.Writer) error {
		req := NewRequestV0(packettype, getRandomId())
		err := req.Write(buf)
		if err == nil {
			err = buf.Flush()
		}
		if err != nil {
			return err
		}
		for {
			pkg, err, _ := Read(conn)
			switch err {
			case nil:
				if pd, ok := pkg.(*PushDataV0); ok {
					for _, d := range pd.Data {
						err = handler(d)
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

// Request data and put it into structured data conforming to the Content interface
func (c *Client) RequestContent(contentitem Content, handler func() error) error {
	return c.Request(contentitem.GetPacketType(), func(data Data) error {
		err := contentitem.ReadAlfred(data)
		if err != nil {
			// just skip
			return nil
		}
		return handler()
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
		err := c.RequestContent(contentitem, handler)
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

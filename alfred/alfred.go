// Package A.L.F.R.E.D. contains functionality resembling that of
// the C variant, to be found here:
// http://git.open-mesh.org/alfred.git/
// This is a pure Go reimplementation.
package alfred

import (
	"bytes"
	"errors"
	"io"
	"net"
	"crypto/rand"
)

const (
	ALFRED_IFNAMSIZ          = 16
	ALFRED_VERSION           = 0
	ALFRED_PORT              = 0x4242
	ALFRED_MAX_RESERVED_TYPE = 64
	ALFRED_MAX_PAYLOAD       = 65535 - 20
)

// packet types
const (
	ALFRED_PUSH_DATA        = 0
	ALFRED_ANNOUNCE_MASTER  = 1
	ALFRED_REQUEST          = 2
	ALFRED_STATUS_TXEND     = 3
	ALFRED_STATUS_ERROR     = 4
	ALFRED_MODESWITCH       = 5
	ALFRED_CHANGE_INTERFACE = 6
)

// operation modes
const (
	ALFRED_MODESWITCH_SLAVE  = 0
	ALFRED_MODESWITCH_MASTER = 1
)

var ErrTooLarge = errors.New("data chunk is too large to fit into packet")
var ErrUnknownType = errors.New("unknown data type")
var ErrParseMAC = errors.New("error parsing MAC address")
var ErrRead = errors.New("cannot read data")

// interface for data that is being transported via A.L.F.R.E.D.
type Content interface {
	GetPacketType() uint8
	ReadAlfred(Data) error
}

type Packet interface {
	// write the packet to the io.Writer, return an error if anything
	// went wrong.
	Write(io.Writer) error
	// return the size of the packet when put onto wire.
	Size() int
}

// wrapper for getting a random uint16 used for transaction IDs
func getRandomId() (randval uint16) {
    data := make([]byte, 2)
    n, err := rand.Reader.Read(data)
    if err != nil {
        panic(err)
    }
    if n < 2 {
        panic("got not enough random data!")
    }
    return uint16(data[0]) << 8 + uint16(data[1])
}

// A type-length-version data element.
// It is the main descriptor used in the data stream
// provided or received by an instance of A.L.F.R.E.D.
type TLV struct {
	// type of the data following (or the information expressed by
	// just the TLV itself)
	Type uint8
	// version of the typed data
	Version uint8
	// length of the data following
	Length uint16
}

// Read a TLV packet.
// Returns the packet, an error if anything went wrong, and the number
// of bytes read from the io.Reader
func ReadTLV(r io.Reader) (*TLV, error, int) {
	tlv := TLV{}
    data := make([]byte, 4)
    n, err := r.Read(data)
    if err != nil {
        return &tlv, err, n
    }
    if n < 4 {
        return &tlv, ErrRead, n
    }
    tlv.Type = data[0]
    tlv.Version = data[1]
    tlv.Length = uint16(data[2]) << 8 + uint16(data[3])
	return &tlv, nil, n
}

func (t *TLV) Write(w io.Writer) error {
	_, err := w.Write([]byte{
		t.Type, t.Version, byte(t.Length >> 8), byte(t.Length & 0xFF)})
	return err
}

func (t *TLV) Size() int {
	return 4
}

// A wrapper for data received from or transmitted to another instance
// of A.L.F.R.E.D.
type Data struct {
	// origin of the data
	Source HardwareAddr
	// descriptor for the data
	Header *TLV
	// the actual data
	Data []byte
}

// Read a Data packet
// Returns the packet, an error if anything went wrong, and the number
// of bytes read from the io.Reader
func ReadData(r io.Reader) (*Data, error, int) {
	var n, c int
	var err error
	data := Data{Source: make([]byte, 6)}
	if n, err = io.ReadFull(r, data.Source); err != nil {
		return &data, err, n
	}
	if data.Header, err, c = ReadTLV(r); err != nil {
		return &data, err, n
	} else {
		n += c
	}
	data.Data = make([]byte, data.Header.Length)
	c, err = io.ReadFull(r, data.Data)
	return &data, err, n + c
}

// check for equality
func (d *Data) Equals(other *Data) bool {
    if d == nil {
        return false
    }
    if other == nil {
        return false
    }
    if bytes.Compare(d.Source, other.Source) == 0 &&
        d.Header.Type == other.Header.Type &&
        d.Header.Version == other.Header.Version &&
        d.Header.Length == other.Header.Length &&
        bytes.Compare(d.Data, other.Data) == 0 {
        return true
    }
    return false
}

func (d *Data) Write(w io.Writer) error {
	if len(d.Data) > 0xFFFF {
		return ErrTooLarge
	}
	d.Header.Length = (uint16)(len(d.Data))
	if _, err := w.Write(d.Source); err != nil {
		return err
	}
	if err := d.Header.Write(w); err != nil {
		return err
	}
	_, err := w.Write(d.Data)
	return err
}

func (d *Data) Size() int {
	return 6 + d.Header.Size() + len(d.Data)
}

// Information referring to a transaction between two instances of
// A.L.F.R.E.D.
type TransactionMgmt struct {
	// Transaction ID
	Id uint16
	// Sequence number
	SeqNo uint16
}

// Read a TransactionMgmt packet
// Returns the packet, an error if anything went wrong, and the number
// of bytes read from the io.Reader
func ReadTransactionMgmt(r io.Reader) (*TransactionMgmt, error, int) {
	tx := TransactionMgmt{}
    data := make([]byte, 4)
    n, err := r.Read(data)
    if err != nil {
        return &tx, err, n
    }
    if n < 4 {
        return &tx, ErrRead, n
    }
    tx.Id = uint16(data[0]) << 8 + uint16(data[1])
    tx.SeqNo = uint16(data[2]) << 8 + uint16(data[3])
	return &tx, nil, n
}

func (t *TransactionMgmt) Write(w io.Writer) error {
	_, err := w.Write([]byte{
		byte(t.Id >> 8), byte(t.Id & 0xFF),
		byte(t.SeqNo >> 8), byte(t.SeqNo & 0xFF)})
	return err
}

func (t *TransactionMgmt) Size() int {
	return 4
}

// Wrapper for a data "push" transaction, to be sent from the
// "pushing" instance of A.L.F.R.E.D. to a receiving instance.
// It refers to a transaction and contains the actually pushed
// data.
type PushDataV0 struct {
	Header *TLV
	Tx     *TransactionMgmt
	Data   []Data
}

func NewPushDataV0(tx *TransactionMgmt, data []Data) *PushDataV0 {
	return &PushDataV0{
		Header: &TLV{
			Type:    ALFRED_PUSH_DATA,
			Version: 0,
		},
		Tx:   tx,
		Data: data,
	}
}

// Read a TransactionMgmt packet
// Returns the packet, an error if anything went wrong, and the number
// of bytes read from the io.Reader
func ReadPushDataV0(r io.Reader, header *TLV) (*PushDataV0, error, int) {
	var read int
	var err error
	pd := PushDataV0{Header: header}
	if pd.Tx, err, read = ReadTransactionMgmt(r); err != nil {
		return &pd, err, read
	}
	for read < (int)(pd.Header.Length) {
		if data, err, c := ReadData(r); err != nil {
			return &pd, err, read
		} else {
			pd.Data = append(pd.Data, *data)
			read += c
		}
	}
	return &pd, nil, read
}

func (p *PushDataV0) Write(w io.Writer) error {
	length := p.Tx.Size() + p.SizeData()
	if length > 0xFFFF {
		return ErrTooLarge
	}
	p.Header.Length = (uint16)(length)
	if err := p.Header.Write(w); err != nil {
		return err
	}
	if err := p.Tx.Write(w); err != nil {
		return err
	}
	for _, d := range p.Data {
		if err := d.Write(w); err != nil {
			return err
		}
	}
	return nil
}

// As a PushDataV0 packet can contain many Data packets in its
// data part, this convenience method can be used to query their
// combined size.
func (p *PushDataV0) SizeData() int {
	datasize := 0
	for _, d := range p.Data {
		datasize += d.Size()
	}
	return datasize
}

func (p *PushDataV0) Size() int {
	return p.Header.Size() + p.Tx.Size() + p.SizeData()
}

// Header-only packet sent by an A.L.F.R.E.D. server to announce
// that it is an available master server and ready to receive
// data from slave servers or to be queried for collected data
type AnnounceMasterV0 struct {
	Header *TLV
}

func NewAnnounceMasterV0() *AnnounceMasterV0 {
	return &AnnounceMasterV0{
		Header: &TLV{
			Type:    ALFRED_ANNOUNCE_MASTER,
			Version: 0,
			Length:  0,
		},
	}
}

func (a *AnnounceMasterV0) Write(w io.Writer) error {
	return a.Header.Write(w)
}

func (a *AnnounceMasterV0) Size() int {
	return a.Header.Size()
}

// Query send either from client to server or from server to server
// for requesting (all) data of a given type, initializing a
// transaction.
type RequestV0 struct {
	Header        *TLV
	RequestedType uint8
	TxId          uint16
}

func NewRequestV0(requestedtype uint8, txid uint16) *RequestV0 {
	return &RequestV0{
		Header: &TLV{
			Type:    ALFRED_REQUEST,
			Version: 0,
			Length:  3,
		},
		RequestedType: requestedtype,
		TxId:          txid,
	}
}

// Read a RequestV0 packet
// Returns the packet, an error if anything went wrong, and the number
// of bytes read from the io.Reader
func ReadRequestV0(r io.Reader, header *TLV) (*RequestV0, error, int) {
	rq := RequestV0{Header: header}
    data := make([]byte, 3)
    n, err := r.Read(data)
    if err != nil {
		return &rq, err, n
	}
    if n < 3 {
        return &rq, ErrRead, n
    }
    rq.RequestedType = data[0]
    rq.TxId = uint16(data[1]) << 8 + uint16(data[2])
	return &rq, nil, 3
}

func (r *RequestV0) Write(w io.Writer) error {
	if err := r.Header.Write(w); err != nil {
		return err
	}
	if _, err := w.Write([]byte{r.RequestedType}); err != nil {
		return err
	}
	_, err := w.Write([]byte{byte(r.TxId >> 8), byte(r.TxId & 0xFF)})
	return err
}

func (r *RequestV0) Size() int {
	return r.Header.Size() + 1 + 2
}

// A client sends this to a server to request a switch of the
// operation mode
type ModeSwitchV0 struct {
	Header *TLV
	Mode   uint8
}

func NewModeSwitchV0(mode uint8) *ModeSwitchV0 {
	return &ModeSwitchV0{
		Header: &TLV{
			Type:    ALFRED_MODESWITCH,
			Version: 0,
			Length:  1,
		},
		Mode: mode,
	}
}

// Read a ModeSwitchV0 packet
// Returns the packet, an error if anything went wrong, and the number
// of bytes read from the io.Reader
func ReadModeSwitchV0(r io.Reader, header *TLV) (*ModeSwitchV0, error, int) {
	m := ModeSwitchV0{Header: header}
    data := make([]byte, 1)
    n, err := r.Read(data)
    if err != nil {
		return &m, err, n
	}
    m.Mode = data[0]
	return &m, nil, n
}

func (m *ModeSwitchV0) Write(w io.Writer) error {
	if err := m.Header.Write(w); err != nil {
		return err
	}
	_, err := w.Write([]byte{m.Mode})
	return err
}

func (m *ModeSwitchV0) Size() int {
	return m.Header.Size() + 1
}

// A client sends this to a server to request the server to (re-)bind
// its interfaces to send/receive on.
type ChangeInterfaceV0 struct {
	Header *TLV
	Ifaces *[ALFRED_IFNAMSIZ * 16]byte
}

func NewChangeInterfaceV0(interfaces []byte) *ChangeInterfaceV0 {
	c := ChangeInterfaceV0{
		Header: &TLV{
			Type:    ALFRED_CHANGE_INTERFACE,
			Version: 0,
			Length:  ALFRED_IFNAMSIZ * 16,
		},
	}
	copy(c.Ifaces[:], interfaces)
	return &c
}

// Read a ChangeInterfaceV0 packet
// Returns the packet, an error if anything went wrong, and the number
// of bytes read from the io.Reader
func ReadChangeInterfaceV0(r io.Reader, header *TLV) (*ChangeInterfaceV0, error, int) {
	c := ChangeInterfaceV0{Header: header}
	n, err := io.ReadFull(r, c.Ifaces[:])
	return &c, err, n
}

func (c *ChangeInterfaceV0) Write(w io.Writer) error {
	if err := c.Header.Write(w); err != nil {
		return err
	}
	_, err := w.Write(c.Ifaces[:])
	return err
}

func (c *ChangeInterfaceV0) Size() int {
	return c.Header.Size() + ALFRED_IFNAMSIZ*16
}

// This is used as the packet format for various status reports
// which differ in their numerical packet type.
type StatusV0 struct {
	Header *TLV
	Tx     *TransactionMgmt
}

func NewStatusV0(statustype uint8, tx *TransactionMgmt) *StatusV0 {
	return &StatusV0{
		Header: &TLV{
			Type:    statustype,
			Version: 0,
			Length:  (uint16)(tx.Size()),
		},
		Tx: tx,
	}
}

// Read a StatusV0 packet
// Returns the packet, an error if anything went wrong, and the number
// of bytes read from the io.Reader
func ReadStatusV0(r io.Reader, header *TLV) (*StatusV0, error, int) {
	var err error
	var n int
	s := StatusV0{Header: header}
	s.Tx, err, n = ReadTransactionMgmt(r)
	return &s, err, n
}

func (s *StatusV0) Write(w io.Writer) error {
	if err := s.Header.Write(w); err != nil {
		return err
	}
	return s.Tx.Write(w)
}

func (s *StatusV0) Size() int {
	return s.Header.Size() + s.Tx.Size()
}

// Read a packet and all its contained data from an io.Reader.
// Returns the packet, an error if anything went wrong, and the number
// of bytes read from the io.Reader
func Read(r io.Reader) (Packet, error, int) {
	tlv, err, read := ReadTLV(r)
	if err != nil {
		return tlv, err, read
	}
	switch {
	case tlv.Type == ALFRED_PUSH_DATA && tlv.Version == 0:
		p, err, c := ReadPushDataV0(r, tlv)
		return p, err, read + c
	case tlv.Type == ALFRED_ANNOUNCE_MASTER && tlv.Version == 0:
		return &AnnounceMasterV0{Header: tlv}, nil, read
	case tlv.Type == ALFRED_REQUEST && tlv.Version == 0:
		p, err, c := ReadRequestV0(r, tlv)
		return p, err, read + c
	case (tlv.Type == ALFRED_STATUS_TXEND || tlv.Type == ALFRED_STATUS_ERROR) && tlv.Version == 0:
		p, err, c := ReadStatusV0(r, tlv)
		return p, err, read + c
	case tlv.Type == ALFRED_MODESWITCH && tlv.Version == 0:
		p, err, c := ReadModeSwitchV0(r, tlv)
		return p, err, read + c
	case tlv.Type == ALFRED_CHANGE_INTERFACE && tlv.Version == 0:
		p, err, c := ReadChangeInterfaceV0(r, tlv)
		return p, err, read + c
	default:
		return tlv, ErrUnknownType, read
	}
}

// Wrapper for the MAC addresses found as main identifier.
type HardwareAddr net.HardwareAddr

var unsetAddr = HardwareAddr([]byte{0,0,0,0,0,0})

// check if it is unset - i.e. nil pointer or 00:00:00:00:00:00
func (i *HardwareAddr) IsUnset() bool {
    if i == nil {
        return true
    }
    if bytes.Compare(unsetAddr, *i) == 0 {
        return true
    }
    return false
}

// wrap printing from net.HardwareAddr
func (i HardwareAddr) String() string {
	return net.HardwareAddr(i).String()
}

// parse address from a string
func (i *HardwareAddr) Parse(addr string) error {
    return i.UnmarshalJSON([]byte(addr))
}

// JSON encoder for MAC addresses
func (i HardwareAddr) MarshalJSON() ([]byte, error) {
	buf := new(bytes.Buffer)
	buf.WriteByte('"')
	buf.WriteString(net.HardwareAddr(i).String())
	buf.WriteByte('"')
	return buf.Bytes(), nil
}

// very forgiving parser for MAC addresses
func (i *HardwareAddr) UnmarshalJSON(data []byte) error {
	addr := make([]byte, 6)
	n := 0
	v := byte(0)
	for _, c := range data {
		switch {
		case c >= 0x30 && c <= 0x39:
			v = c - 0x30
		case c >= 0x41 && c <= 0x46:
			v = c - 0x41 + 0xA
		case c >= 0x61 && c <= 0x66:
			v = c - 0x61 + 0xA
		default:
			continue
		}
		if n >= 12 {
			// more than 12 hex chars
			return ErrParseMAC
		}
		if 0 == n%2 {
			addr[n>>1] = v << 4
		} else {
			addr[n>>1] += v
		}
		n++
	}
	if n == 12 {
		*i = HardwareAddr(addr)
		return nil
	}
	return ErrParseMAC
}

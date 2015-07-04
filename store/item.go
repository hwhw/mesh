package store

import (
	"time"
)

// For convenient storage, special methods are provided that apply to
// data which fulfills the Item interface
type Item interface {
	// return a key for the data item
	Key() []byte
	// return a store id for the data item
	StoreID() []byte
	// serialize data item
	Bytes() ([]byte, error)
	// unserialize data item
	DeserializeFrom([]byte) error
	// set key when reading
	SetKey([]byte)
}

// Database entry metadata
type Meta struct {
	// first creation of an item with this key in this store
	Created time.Time
	// last update of the item with this key in this store
	Updated time.Time
	// date when this item becomes invalid and may be purged
	Invalid time.Time
	// unparsed content
	content []byte
	// typed/parsed content (will get unserialized lazily)
	item *Item
    // key cache
    key []byte
}

// these are needed for binary size calculation for Meta
var timeMarshaled, _ = time.Time{}.MarshalBinary()
var timeSize = len(timeMarshaled)
var metaSize = 3 * timeSize

// time value to check for "unset" metadata time
var Never = time.Time{}

// Create a new Meta element
func NewMeta(item Item) *Meta {
	return &Meta{item: &item}
}

// Set invalidation date via duration
func (m *Meta) InvalidateIn(in time.Duration) {
	m.Invalid = time.Now().Add(in)
}

// Access contained item
func (m *Meta) GetItem(item Item) error {
	if m.item != nil {
		item = *m.item
		return nil
	}
	err := item.DeserializeFrom(m.content)
	if err == nil {
		m.item = &item
	}
	return err
}

// pass through a call to the Key method
func (m *Meta) Key() []byte {
    if m.item != nil {
	    return (*m.item).Key()
    } else if m.key != nil {
        return m.key
    } else {
        return nil
    }
}

// pass through a call to the SetKey method
func (m *Meta) SetKey(key []byte) {
    if m.item != nil {
        (*m.item).SetKey(key)
    } else {
        m.key = key
    }
}

// fast Meta binary encoder
func (m *Meta) Bytes() ([]byte, error) {
	e := make([]byte, metaSize)

	now := time.Now()
	if m.Created.Equal(Never) {
		m.Created = now
	}
	m.Updated = now

	t, err := m.Created.MarshalBinary()
	if err != nil {
		return e, err
	}
	copy(e, t)
	t, err = m.Updated.MarshalBinary()
	if err != nil {
		return e, err
	}
	copy(e[timeSize:], t)
	t, err = m.Invalid.MarshalBinary()
	if err != nil {
		return e, err
	}
	copy(e[timeSize*2:], t)
	if m.item != nil {
		bytes, err := (*m.item).Bytes()
		return append(e, bytes...), err
	}
	return append(e, m.content...), nil
}

// fast Meta binary decoder
func (m *Meta) DeserializeFrom(d []byte) error {
    m.key = nil
	err := m.Created.UnmarshalBinary(d[:timeSize])
	if err == nil {
		err = m.Updated.UnmarshalBinary(d[timeSize : 2*timeSize])
	}
	if err == nil {
		err = m.Invalid.UnmarshalBinary(d[2*timeSize : 3*timeSize])
	}
	if err == nil {
		m.content = d[3*timeSize:]
		m.item = nil
	}
	return err
}

// return StoreID
func (m *Meta) StoreID() []byte {
	m.enforceItem()
	return (*m.item).StoreID()
}

// helper: will trigger a panic when the item element is not set
func (m *Meta) enforceItem() {
	if m.item == nil {
		panic("trying to access store ID before fixing item type")
	}
}

// For data import/export, we need to make the content accessible.
type MetaTransfer struct {
	Metadata *Meta
	Record   *Item
}

// Get exportable metadata/data struct
func (m *Meta) GetTransfer() *MetaTransfer {
	m.enforceItem()
	return &MetaTransfer{
		Metadata: m,
		Record:   m.item,
	}
}

// wrapper for checking validity via metadata
func (m *Meta) IsValid() bool {
	if m.Invalid.Equal(Never) {
		return true
	}
	return time.Now().Before(m.Invalid)
}

// A no-op wrapper for items that generate/contain their own
// key
type ContainedKey struct{}

// no operation for the SetKey method
func (c *ContainedKey) SetKey(key []byte) {}

// Use this to wrap items that do not generate a key from
// their own data.
// Note that upon creating new items, you must call SetKey()
// to set up the key value.
type BasicKey struct {
	key []byte
}

// return key
func (b *BasicKey) Key() []byte {
	return b.key
}

// set key
func (b *BasicKey) SetKey(key []byte) {
	b.key = key
}

// You can use this to implement simple Flag items.
// These will have to embed this struct and define their own
// method StoreID.
type Flag struct{ BasicKey }

// return empty []byte as encoded data
func (f *Flag) Bytes() ([]byte, error) {
	return []byte{}, nil
}

// no-op deserialization
func (f *Flag) DeserializeFrom(d []byte) error {
	return nil
}

// Use this to wrap pure binary data items.
// These will have to embed this struct and define their own
// methods StoreID and Key.
type Byte struct {
	BasicKey
	data []byte
}

// return data content
func (b *Byte) Bytes() ([]byte, error) {
	return b.data, nil
}

// passthrough deserialization
func (b *Byte) DeserializeFrom(d []byte) error {
	b.data = d
	return nil
}

// set contained data
func (b *Byte) Set(d []byte) {
	b.data = d
}

// get contained data
func (b *Byte) Get() []byte {
	return b.data
}

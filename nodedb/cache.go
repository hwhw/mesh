package nodedb

import "sync"

type Cache struct {
    object []byte
    sync.Mutex
}

func (c *Cache) get(getter func() []byte) []byte {
    c.Lock()
    defer c.Unlock()
    if c.object == nil {
        c.object = getter()
    }
    return c.object
}

func (c *Cache) invalidate() {
    c.Lock()
    c.object = nil
    c.Unlock()
}

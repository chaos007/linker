package linker

import (
	"sync"
	"time"
)

var (
	rt   *ReTry
	once sync.Once
)

type ReTry struct {
	items   sync.Map
	timeout time.Duration
}

type Item struct {
	Channel string
	Ctx     Context
	Header  map[string]string
	Value   interface{}
}

func NewRetry(timeout time.Duration) *ReTry {
	once.Do(func() {
		rt = &ReTry{items: sync.Map{}, timeout: timeout}
	})

	return rt
}

func (rt *ReTry) Put(key interface{}, value *Item) {
	rt.items.Store(key, value)

	go func(rt *ReTry) {
		<-time.NewTimer(rt.timeout).C
		if v, ok := rt.items.Load(key); ok {
			if i, ok := v.(*Item); ok {
				for k, v := range i.Header {
					i.Ctx.SetRequestProperty(k, v)
				}

				i.Ctx.Write(i.Channel, i.Value)
				rt.Delete(key)
			}
		}

		return
	}(rt)
}

func (rt *ReTry) Delete(key interface{}) {
	rt.items.Delete(key)
}

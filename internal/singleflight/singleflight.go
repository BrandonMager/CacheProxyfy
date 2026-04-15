package singleflight

import (
	"fmt"
	"sync"
)

type call struct {
	wg sync.WaitGroup
	val []byte
	err error
}

// Allows concurrent upstream fetches of the same package so only one upstream runs and the rest share
type Group struct {
	mu sync.Mutex
	calls map[string]*call
}

func NewGroup() *Group {
	return &Group{ calls: make(map[string]*call)}
}

func key(ecosystem, name, version string) string {
	return fmt.Sprintf("%s:%s:%s", ecosystem, name, version)
}

/* Runs goroutine fn if no in-flight fetch exists. If fetch is running, the Do function blocks until it completes and returns same result to all callers */
func (g *Group) Do(ecosystem, name, version string, fn func() ([]byte, error)) ([]byte, bool, error){
	k := key(ecosystem, name, version)
	g.mu.Lock()
	if c, ok := g.calls[k]; ok {
		g.mu.Unlock()
		c.wg.Wait()
		return c.val, false, c.err
	}

	// First caller for this key
	c := &call{}
	c.wg.Add(1)
	g.calls[k] = c
	g.mu.Unlock()

	c.val, c.err = fn()
	c.wg.Done()

	// Remove key so future cache misses trigger a fresh fetch
	g.mu.Lock()
	delete(g.calls, k)
	g.mu.Unlock()

	return c.val, true, c.err

}
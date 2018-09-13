package arraymap

import (
	"encoding/json"
	"sync"
)

type ArrayMap struct {
	positions map[interface{}]int
	keys      []interface{}
	values    []interface{}
	rwl       sync.RWMutex
}

func NewArrayMap() *ArrayMap {
	return &ArrayMap{
		positions: make(map[interface{}]int),
		keys:      make([]interface{}, 0),
		values:    make([]interface{}, 0),
	}
}

func (a *ArrayMap) Len() int {
	return len(a.keys)
}

func (a *ArrayMap) Add(key, value interface{}) (oldValue interface{}) {
	if p, existed := a.positions[key]; existed {
		oldValue = a.values[p]
		a.values[p] = value
		return
	}
	a.keys = append(a.keys, key)
	a.values = append(a.values, value)
	a.positions[key] = len(a.keys) - 1
	return
}

func (a *ArrayMap) GetKeyAt(i int) interface{} {
	return a.keys[i]
}

func (a *ArrayMap) GetValueAt(i int) interface{} {
	return a.values[i]
}

func (a *ArrayMap) GetValueOf(key interface{}) interface{} {
	return a.values[a.positions[key]]
}

func (a *ArrayMap) Has(key interface{}) bool {
	_, existed := a.positions[key]
	return existed
}

func (a *ArrayMap) removeAt(i int) {
	removingKey, lastKey := a.keys[i], a.keys[len(a.keys)-1]
	// Swap the removing item and the last.
	a.keys[i], a.keys[len(a.keys)-1] = a.keys[len(a.keys)-1], a.keys[i]
	a.values[i], a.values[len(a.values)-1] = a.values[len(a.values)-1], a.values[i]

	// Update the position.
	a.positions[lastKey] = i

	// Removing.
	a.keys = a.keys[:len(a.keys)-1]
	a.values = a.values[:len(a.values)-1]
	delete(a.positions, removingKey)
}

func (a *ArrayMap) Remove(key interface{}) bool {
	if p, exisited := a.positions[key]; exisited {
		a.removeAt(p)
		return true
	}
	return false
}

func (a *ArrayMap) RemoveAll() {
	for k := range a.positions {
		delete(a.positions, k)
	}
	a.keys = a.keys[:0]
	a.values = a.values[:0]
}

func (a *ArrayMap) Lock() {
	a.rwl.Lock()
	return
}

func (a *ArrayMap) Unlock() {
	a.rwl.Unlock()
	return
}

func (a *ArrayMap) RLock() {
	a.rwl.RLock()
	return
}

func (a *ArrayMap) RUnlock() {
	a.rwl.RUnlock()
	return
}

func (a *ArrayMap) Values() []interface{} {
	return a.values
}

func (a *ArrayMap) MarshalJSON() ([]byte, error) {
	return json.Marshal(a.values)
}

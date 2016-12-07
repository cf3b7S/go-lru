package lru

import (
	"sync"

	"go-lru/list"
	"go-lru/spinlock"
)

type lruShard struct {
	spinlock.SpinLock
	// sync.Mutex
	lst       *list.List
	table     map[string]*list.Element
	lstElPool *sync.Pool
}

func newShard() *lruShard {
	return &lruShard{
		lst:       list.New(),
		table:     make(map[string]*list.Element),
		lstElPool: &sync.Pool{New: func() interface{} { return &list.Element{} }},
	}
}

func (s *lruShard) add(entry *Entry) {
	el := s.lstElPool.Get().(*list.Element)
	el.Value = entry
	s.table[entry.Key] = s.lst.PushFrontElement(el)
}

func (s *lruShard) get(key string) (el *list.Element, found bool) {
	el, found = s.table[key]
	return
}

func (s *lruShard) putIfAbsent(entry *Entry) {
	if _, found := s.table[entry.Key]; !found {
		s.add(entry)
	}
}

func (s *lruShard) oldest() (el *list.Element) {
	return s.lst.Back()
}

func (s *lruShard) remove(el *list.Element) {
	entry := el.Value.(*Entry)
	delete(s.table, entry.Key)
	s.lst.Remove(el)
	s.lstElPool.Put(el)
}

func (s *lruShard) offer(el *list.Element) {
	s.lst.MoveToFront(el)
}

func (s *lruShard) len() int {
	return s.lst.Len()
}

package lru

import (
	"encoding/gob"
	"fmt"
	"hash"
	"hash/fnv"
	"io"
	"list"
	"os"
	"sync"
	"time"
)

const NoExpiration time.Duration = 0
const SHARD_SIZE_DEFAULT int = 64

type Callback func(entry *Entry)

type LRU struct {
	TTL       time.Duration
	OnRemove  Callback
	shards    []*lruShard
	shardSize int // how much shard
	eps       int // entries per shard
	hashPool  *sync.Pool
	entryPool *sync.Pool
}

// New create a lru cache with default shard size and no expiration. If entry size is less than 128, there will be only one shard.
func New(entrySize int) *LRU {
	return NewLRU(entrySize, SHARD_SIZE_DEFAULT, NoExpiration)
}

// NewLRU create a lru cache with params. If entry size is less than 128, there will be only one shard.
func NewLRU(entrySize int, shardSize int, ttl time.Duration) *LRU {
	if entrySize < 128 {
		shardSize = 1
	}
	eps := entrySize / shardSize
	if (entrySize % shardSize) > 0 {
		eps += 1
	}
	shards := make([]*lruShard, shardSize, shardSize)
	for i := 0; i < shardSize; i++ {
		shards[i] = newShard()
	}

	hashPool := &sync.Pool{New: func() interface{} { return fnv.New32() }}
	entryPool := &sync.Pool{New: func() interface{} { return &Entry{} }}

	return &LRU{
		TTL:       ttl,
		shards:    shards,
		shardSize: shardSize,
		eps:       eps,
		hashPool:  hashPool,
		entryPool: entryPool,
	}
}

func (c *LRU) getShard(key string) *lruShard {
	hasher := c.hashPool.Get().(hash.Hash32)
	hasher.Write([]byte(key))
	shard := c.shards[uint(hasher.Sum32())%uint(c.shardSize)]
	hasher.Reset()
	c.hashPool.Put(hasher)
	return shard
}

func (c *LRU) getEntry(shard *lruShard, key string, now time.Time, exp *time.Time) (entry *Entry, found bool) {
	var el *list.Element
	if el, found = shard.get(key); !found {
		return
	}
	entry = el.Value.(*Entry)
	if c.TTL != NoExpiration && entry.Expiration.Before(now) {
		c.remove(shard, el)
		return nil, false
	}
	entry.Expiration = exp
	shard.offer(el)
	return
}

// Set insert key value pair into the cache with default expiration. If key exist and not expired, the value will be reset, otherwise new key will be added.
func (c *LRU) Set(key string, value interface{}) {
	c.SetWithTTL(key, value, c.TTL)
}

// Set insert key value pair into the cache with expiration. If key exist and not expired, the value will be reset, otherwise new key will be added.
func (c *LRU) SetWithTTL(key string, value interface{}, ttl time.Duration) {
	now, exp := genExp(ttl)
	shard := c.getShard(key)
	shard.Lock()
	if entry, found := c.getEntry(shard, key, now, exp); found {
		entry.Value = value
	} else {
		entry := c.entryPool.Get().(*Entry)
		entry.Key = key
		entry.Value = value
		if ttl != NoExpiration {
			entry.Expiration = exp
		}
		shard.add(entry)
		if shard.len() > c.eps {
			c.remove(shard, shard.oldest())
		}
	}
	shard.Unlock()
}

// Get return the value corresponding to key and if the key was founded. If key is expired, found will be false.
func (c *LRU) Get(key string) (value interface{}, found bool) {
	now, exp := genExp(c.TTL)
	shard := c.getShard(key)
	shard.Lock()
	entry, found := c.getEntry(shard, key, now, exp)
	shard.Unlock()
	if found {
		value = entry.Value
	}
	return
}

// GetEntry return the entry corresponding to key and if the key was founded. If key is expired, found will be false.
func (c *LRU) GetEntry(key string) (entry *Entry, found bool) {
	now, exp := genExp(c.TTL)
	shard := c.getShard(key)
	shard.Lock()
	entry, found = c.getEntry(shard, key, now, exp)
	shard.Unlock()
	return
}

// Update find the entry corresponding to key, pass it from callback's param. The whole update will be thread safe, and should not use other operation with lock like Get, Set and Remove etc.
func (c *LRU) Update(key string, cb Callback) (entry *Entry, found bool) {
	now, exp := genExp(c.TTL)
	shard := c.getShard(key)
	shard.Lock()
	if entry, found = c.getEntry(shard, key, now, exp); found {
		cb(entry)
	}
	shard.Unlock()
	return
}

// Remove will delete the key value pair from cache
func (c *LRU) Remove(key string) {
	shard := c.getShard(key)
	shard.Lock()
	if el, found := shard.get(key); found {
		c.remove(shard, el)
	}
	shard.Unlock()
}

func (c *LRU) remove(shard *lruShard, el *list.Element) {
	shard.remove(el)
	entry := el.Value.(*Entry)
	if c.OnRemove != nil {
		//dead lock?
		c.OnRemove(entry)
	}
	entry.Expiration = nil
	c.entryPool.Put(entry)
}

// Len return the total number of entries in cache
func (c *LRU) Len() int {
	len := 0
	for i := 0; i < c.shardSize; i++ {
		shard := c.shards[i]
		shard.Lock()
		len += shard.len()
		shard.Unlock()
	}
	return len
}

// Len return all keys in the cache
func (c *LRU) Keys() []string {
	var keys []string
	for i := 0; i < c.shardSize; i++ {
		shard := c.shards[i]
		shard.Lock()
		for key := range shard.table {
			keys = append(keys, key)
		}
		shard.Unlock()
	}
	return keys
}

// Iter return a channel which can read every entry from it
func (c *LRU) Iter() <-chan *Entry {
	ch := make(chan *Entry)
	go func() {
		for i := 0; i < c.shardSize; i++ {
			shard := c.shards[i]
			shard.Lock()
			for el := shard.oldest(); el != nil; el = el.Prev() {
				ch <- el.Value.(*Entry)
			}
			shard.Unlock()
		}
		close(ch)
	}()
	return ch
}

// Flush remove all the keys
func (c *LRU) Flush() {
	for i := 0; i < c.shardSize; i++ {
		shard := c.shards[i]
		shard.Lock()
		oldLst := shard.lst
		shard.lst = list.New()
		shard.table = make(map[string]*list.Element)
		shard.Unlock()

		for el := oldLst.Front(); el != nil; el = el.Next() {
			if c.OnRemove != nil {
				c.OnRemove(el.Value.(*Entry))
			}
		}
	}
}

// Load load cache from io reader
func (c *LRU) Load(r io.Reader) error {
	dec := gob.NewDecoder(r)
	for {
		var entry Entry
		if err := dec.Decode(&entry); err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		} else {
			c.getShard(entry.Key).putIfAbsent(&entry)
		}
	}
}

// LoadFile load cache from saved file path
func (c *LRU) LoadFile(fname string) error {
	f, err := os.Open(fname)
	defer f.Close()
	if err != nil {
		return err
	}
	return c.Load(f)
}

// NewWithReader create new cache load cache from io reader
func NewWithReader(entrySize int, shardSize int, ttl time.Duration, r io.Reader) (c *LRU, err error) {
	c = NewLRU(entrySize, shardSize, ttl)
	err = c.Load(r)
	return
}

// NewWithFile create new cache load cache from saved file path
func NewWithFile(entrySize int, shardSize int, ttl time.Duration, fname string) (c *LRU, err error) {
	f, err := os.Open(fname)
	defer f.Close()
	if err != nil {
		return
	}
	return NewWithReader(entrySize, shardSize, ttl, f)
}

// Save write cache to the given io writer
func (c *LRU) Save(w io.Writer) (err error) {
	gob.Register(Entry{})
	enc := gob.NewEncoder(w)
	defer func() {
		if x := recover(); x != nil {
			err = fmt.Errorf("Error registering item types with Gob library")
		}
	}()
	for entry := range c.Iter() {
		gob.Register(entry.Value)
		err := enc.Encode(entry)
		if err != nil {
			return err
		}
	}
	return err
}

// Save write cache to the given file path
func (c *LRU) SaveFile(fname string) error {
	fp, err := os.Create(fname)
	if err != nil {
		return err
	}
	err = c.Save(fp)
	if err != nil {
		fp.Close()
		return err
	}
	return fp.Close()
}

func genExp(ttl time.Duration) (time.Time, *time.Time) {
	var exp *time.Time
	now := time.Now()
	if ttl != NoExpiration {
		_exp := now.Add(ttl)
		exp = &_exp
	}
	return now, exp
}

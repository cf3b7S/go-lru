# go-lru
golang lru cache with thread safe.

### Installation

`go get github.com/khowarizmi/go-lru`

### Document
[doc](https://godoc.org/github.com/khowarizmi/go-lru)


### Usage

```
package main

import (
	"fmt"
	"github.com/khowarizmi/go-lru"
)

func inc(entry *lru.Entry) {
	entry.Value = entry.Value.(int) + 1
}

func callback(entry *lru.Entry) {
	fmt.Println("Remove", entry.Key, entry.Value.(int))
}

func main() {
	// create a lru with 960 entries
	cache := lru.New(960)

	// Set key1 with int 1 to the cache
	cache.Set("key1", 1)

	// Set key2 with string "value" to the cache
	cache.Set("key2", "value")

	// Get the key1 from cache and convert to int
	key1, found := cache.Get("key1")
	if found {
		fmt.Println("found key1", key1.(int))
	}

	// Update key1 with func inc
	cache.Update("key1", inc)

	// Set evict callback
	cache.OnRemove = callback
	cache.Remove("key1")
}
```

### Test

`go test .`

### BenchMark

`go test -bench . -benchtime 10s`

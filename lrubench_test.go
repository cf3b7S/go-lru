package lru

import (
	// "fmt"
	"runtime"
	"strconv"
	"sync"
	"testing"
	"time"
)

func BenchmarkSet(b *testing.B) {
	var wg sync.WaitGroup
	// c := New(10000)
	c := NewLRU(10000, 100, NoExpiration)
	core := runtime.NumCPU()
	chanNum := core
	ch := make(chan int, b.N)
	for i := 0; i < b.N; i++ {
		ch <- i
	}
	time.Sleep(5 * time.Millisecond)
	count := 0
	b.ResetTimer()
	for i := 0; i < chanNum; i++ {
		wg.Add(1)
		go func() {
		loop:
			for {
				select {
				case j := <-ch:
					key := strconv.Itoa(j)
					c.Set(key, j)
				default:
					count += 1
					if count == chanNum {
						close(ch)
					}
					break loop
				}
			}
			wg.Done()
		}()
	}
	wg.Wait()
}

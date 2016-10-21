package lru

import (
	// "fmt"
	"os"
	"testing"
	"time"
)

type TestStruct struct {
	N int
	S string
}

func Inc(entry *Entry) {
	entry.Value = entry.Value.(int) + 1
}

func TestGet(t *testing.T) {
	c := NewLRU(100, 1, time.Second*5)
	c.Set("key", 123)
	if v, ok := c.Get("key"); !ok || v.(int) != 123 {
		t.Fatalf("expected get to return 123 but got %v, %v", v, ok)
	}

	if _, ok := c.Get("key2"); ok {
		t.Fatal("it shouldn't exist")
	}

	s := TestStruct{1, "one"}
	c.Set("key", &s)

	r, ok := c.Get("key")
	if !ok {
		t.Fatal("data is lost")
	}

	e1, ok := c.GetEntry("key")
	exp1 := e1.Expiration
	e2, ok := c.GetEntry("key")
	exp2 := e2.Expiration
	if exp1 == exp2 {
		t.Fatal("ATime eq CTime, %v, %v", exp1, exp2)
	}

	rs := r.(*TestStruct)
	if rs.N != 1 || rs.S != "one" {
		t.Fatal("wtf!")
	}
}

func TestEvict(t *testing.T) {
	evicted := 0
	onRemove := func(e *Entry) {
		t.Logf("on evicted %d", e.Value)
		evicted = e.Value.(int)
	}
	c := New(1)
	c.OnRemove = onRemove
	c.Set("key", 1)
	c.Remove("key")

	if evicted != 1 {
		t.Fatal("expected pop 1")
	}
	if _, ok := c.Get("key"); ok {
		t.Fatal("it shouldn't exist")
	}
}

func TestUpdate(t *testing.T) {
	c := New(1)
	c.Set("key", 1)
	if e, ok := c.Update("key", Inc); !ok || e.Value.(int) != 2 {
		t.Fatal("it should be 2")
	}
	if found := c.SetOrUpdate("newKey", 3, Inc); found {
		t.Fatal("newKey should not exist")
	}
	if value, found := c.Get("newKey"); !found || value != 3 {
		t.Errorf("found: %v, value: %d", found, value)
		t.Fatal("newKey should exist, value should be 3")
	}
	if found := c.SetOrUpdate("newKey", 3, Inc); !found {
		t.Fatal("newKey should exist")
	}
	if value, found := c.Get("newKey"); !found || value != 4 {
		t.Errorf("found: %v, value: %d", found, value)
		t.Fatal("newKey should exist, value should be 4")
	}
}

func TestTTL(t *testing.T) {
	c := NewLRU(1, 1, 100*time.Millisecond)
	c.Set("key", 1)
	time.Sleep(100 * time.Millisecond)
	if _, ok := c.Get("key"); ok {
		t.Fatal("expiration not work")
	}

	c.SetWithTTL("key1", 2, NoExpiration)
	time.Sleep(100 * time.Millisecond)
	if _, ok := c.Get("key1"); !ok {
		t.Fatal("expiration not work")
	}

}

func TestFlush(t *testing.T) {
	c := New(100)
	c.Set("k1", "v1")
	c.Set("k2", "v2")
	c.Set("k3", "v3")
	if c.Len() != 3 {
		t.Fatal("lenght should be 3, %v", c.Len())
	}
	c.Flush()
	if c.Len() != 0 {
		t.Fatal("flush not work, %v", c.Len())
	}
}

func TestSerialize(t *testing.T) {
	var err error
	c := New(10)

	c.Set("key1", 1)
	c.Set("key2", &TestStruct{1, "one"})

	fname := os.TempDir() + "/lru.test.data"

	if err = c.SaveFile(fname); err != nil {
		t.Fatal("save failed", err)
	}

	if err = c.LoadFile(fname); err != nil {
		t.Fatal("load file failed", err)
	}

	if c, err = NewWithFile(1, 1, NoExpiration, fname); err != nil {
		t.Fatal("new with file failed", err)
	}

	if v, ok := c.Get("key1"); !ok || v.(int) != 1 {
		t.Fatal("expected get to return 1")
	}

	r, _ := c.Get("key2")
	rs := r.(*TestStruct)
	if rs.N != 1 || rs.S != "one" {
		t.Fatal("wtf!")
	}
}

package lru

import (
	"time"
)

type Entry struct {
	Key        string
	Value      interface{}
	Expiration *time.Time
}

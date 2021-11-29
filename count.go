package main

import (
	"sync"
)

type countDso struct {
	mutex sync.Mutex
	value uint
}

func newCount() *countDso {
	return &countDso{}
}

func (dso *countDso) increment() uint {
	defer dso.mutex.Unlock()
	dso.mutex.Lock()
	dso.value++
	value := dso.value
	return value
}

func (dso *countDso) decrement() uint {
	defer dso.mutex.Unlock()
	dso.mutex.Lock()
	value := dso.value
	dso.value--
	return value
}

package main

import (
	//"bytes"
	"container/list"
	"errors"
	"flag"
	"github.com/julienschmidt/httprouter"
	"io/ioutil"
	"log"
	"net/http"
	_ "net/http/pprof"
	"runtime"
	"runtime/debug"
	"time"
)

var cachesize = flag.Int("cachesize", 1024000000, "Max cache size in bytes")

type Cache struct {
	// Maxsize is the max size of the cache in bytes
	maxSize int
	curSize int

	// ll is the least-recently used list. objects in the back are evicted
	lru *list.List

	// cache is the actual cache lookup map.
	cache map[interface{}]*list.Element
}

type Key interface{}

type entry struct {
	Key   Key
	Value *[]byte
}

func NewCache(maxSize int) *Cache {
	return &Cache{
		maxSize: maxSize,
		curSize: 0,
		lru:     list.New(),
		cache:   make(map[interface{}]*list.Element),
	}
}

func (c *Cache) Add(key Key, value *[]byte) {
	for (c.curSize + len(*value)) > c.maxSize {
		log.Printf("CACHE_DEL: Removing oldest")
		c.RemoveOldest()
	}
	if ele, ok := c.cache[key]; ok {
		log.Printf("CACHE_ADD: Readding: %s", key)
		c.curSize = c.curSize + len(*value)
		c.lru.MoveToFront(ele)
		ele.Value.(*entry).Value = value
		return
	}
	log.Printf("CACHE_ADD: Adding: %s", key)
	ele := c.lru.PushFront(&entry{key, value})
	//c.lru.PushFront(&entry{key, value})
	c.cache[key] = ele
	c.curSize = c.curSize + len(*value)
}

func (c *Cache) Get(key Key) (*[]byte, error) {
	if ele, ok := c.cache[key]; ok {
		c.lru.MoveToFront(ele)
		return ele.Value.(*entry).Value, nil
	}
	return nil, errors.New("Key not found")
}

func (c *Cache) Remove(key Key) {
	if ele, ok := c.cache[key]; ok {
		c.removeElement(ele)
	}
}

func (c *Cache) RemoveOldest() {
	ele := c.lru.Back()
	if ele != nil {
		c.removeElement(ele)
	}
}

func (c *Cache) removeElement(ele *list.Element) {
	c.curSize = c.curSize - len(*ele.Value.(*entry).Value)
	c.lru.Remove(ele)
	e := ele.Value.(*entry)
	delete(c.cache, e.Key)
}

func (c *Cache) Len() int {
	return c.lru.Len()
}

func (c *Cache) Size() int {
	return c.curSize
}

func (c *Cache) PUT(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	key := ps.ByName("key")
	data, err := ioutil.ReadAll(r.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
	}
	c.Add(key, &data)

	log.Printf("Added object: %s", key)
	log.Printf("Cache size: %d", c.Size())
	log.Printf("Cache length: %d", c.Len())
}

func (c *Cache) GET(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	//time.Sleep(1 * time.Second)
	key := ps.ByName("key")
	data, err := c.Get(key)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	w.Write(*data)
	//log.Printf("Served object: %s", key)
}

func (c *Cache) WIPE(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	for c.Len() > 0 {
		c.RemoveOldest()
	}
	runtime.GC()
}

func (c *Cache) GC(w http.ResponseWriter, r *http.Request, ps httprouter.Params) {
	for k := range c.cache {
		log.Println("Keys: ", k)
	}
	runtime.GC()
}
func GoRuntimeStats() {
	for {
		m := &runtime.MemStats{}

		log.Println("# goroutines: ", runtime.NumGoroutine())
		runtime.ReadMemStats(m)
		log.Println("Memory Acquired: ", m.Sys)
		log.Println("Memory Used    : ", m.Alloc)
		time.Sleep(2 * time.Second)
	}
}

func main() {
	flag.Parse()
	log.Println("Default GC: ", debug.SetGCPercent(10), "New GC: 10")
	go GoRuntimeStats()
	go func() {
		log.Println(http.ListenAndServe("localhost:6060", nil))
	}()
	c := NewCache(*cachesize)
	router := httprouter.New()
	router.POST("/wipe", c.WIPE)
	router.POST("/gc", c.GC)
	router.GET("/*key", c.GET)
	router.PUT("/*key", c.PUT)

	log.Fatal(http.ListenAndServe(":8080", router))
}

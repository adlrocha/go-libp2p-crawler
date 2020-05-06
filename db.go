package main

import (
	"encoding/json"
	"fmt"
	"log"
	"strconv"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
)

func initDB(dbPath string) (db *leveldb.DB) {

	var err error
	db, err = leveldb.OpenFile(dbPath, nil)
	if err != nil {
		log.Fatal(err, "Could not initialize database...")
	}

	return db

}

func currentDate() string {
	date := fmt.Sprintf("%s", time.Now().Format("20060102"))
	return date
}

func yesterday() string {
	date := fmt.Sprintf("%s", time.Now().Add(-24*time.Hour).Format("20060102"))
	return date
}

func getCount(db *leveldb.DB, key string) (int, error) {
	data, err := db.Get([]byte(key), nil)
	// If counter not set yet make it zero.
	var counter int
	if string(data) == "" {
		counter = 0
	} else {
		counter, _ = strconv.Atoi(string(data))
	}

	return counter, err
}

func (c *Crawler) storeSeenNode(key string, node SeenNode) error {
	nodeRaw, _ := json.Marshal(node)
	err := c.db.Put([]byte(key), nodeRaw, nil)
	return err
}

func (c *Crawler) getSeenNode(key string) (SeenNode, error) {
	var node SeenNode
	data, err := c.db.Get([]byte(key), nil)
	json.Unmarshal(data, &node)

	return node, err
}

func (c *Crawler) updateCount(key string, add bool) (int, error) {

	data, err := c.db.Get([]byte(key), nil)

	var counter int
	// If counter not set yet
	if string(data) == "" {
		// If total count set to zero
		if key == "total.count" {
			counter = 0
		} else if key == fmt.Sprintf("%s.count", currentDate()) {
			// If no data for today start with previous total nodes
			// (baseline for the day)
			data, err = c.db.Get([]byte("total.count"), nil)

			if string(data) == "" {
				counter = 0
			} else {
				counter, _ = strconv.Atoi(string(data))
			}
		}
	} else {
		// If counter exists just assign the previous value as start.
		counter, _ = strconv.Atoi(string(data))
	}

	// Update counter
	if add {
		counter++
	} else {
		counter--
	}
	cString := strconv.Itoa(counter)
	err = c.db.Put([]byte(key), []byte(cString), nil)

	return counter, err
}

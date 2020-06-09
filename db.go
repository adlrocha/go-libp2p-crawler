package main

import (
	"fmt"
	"log"
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

func (c *Crawler) updateCountersToday(counter string, increment bool) {
	if c.counters.startingDate != currentDate() {
		c.counters.startingDate = currentDate()
		c.counters.leftNodesToday = 0
		c.counters.seenNodesToday = c.counters.activeNodes
	}

	if counter == "seen" {
		if increment {
			c.counters.seenNodesToday++
		} else {
			c.counters.seenNodesToday--
		}
	} else if counter == "left" {
		if increment {
			c.counters.leftNodesToday++
		} else {
			c.counters.leftNodes--
		}
	} else {
		panic("Passed wrong counter type to update data")
	}
}

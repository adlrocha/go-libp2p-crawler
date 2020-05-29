package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/syndtr/goleveldb/leveldb"
)

const (
	reportingTime            = 10
	timeEphemeralConnections = 5
	timeClosestPeers         = 5
	dbPath                   = "./db"
)

func reporting(ctx context.Context, db *leveldb.DB) {
	for {
		select {
		case <-ctx.Done():
			return

		default:
			//TODO: We can show more metrics here if needed.
			// It would be easy to add number of nodes behind NAT or other metrics.
			totalCount, _ := getCount(db, "total.count")
			totalLeft, _ := getCount(db, "total.left")
			todayCount, _ := getCount(db, fmt.Sprintf("%s.count", currentDate()))
			// yesterdayCount, _ := getCount(db, fmt.Sprintf("%s.count", yesterday()))
			todayLeft, _ := getCount(db, fmt.Sprintf("%s.left", currentDate()))
			var churn float32

			if totalCount == 0 {
				churn = 0
			} else {
				churn = (float32(todayLeft) / float32(todayCount)) * 100
			}
			log.Printf("==== Total nodes active in run: %d, Total nodes left in run: %d, Total nodes seen today: %d, Total nodes gone today: %d,  Daily churn: %f%%====",
				totalCount, totalLeft, todayCount, todayLeft, churn)
			time.Sleep(reportingTime * time.Second)
		}
	}
}

func main() {
	numLiveliness := flag.Int("liveliness", 1, "Number of liveliness nodes")
	numCrawlers := flag.Int("crawler", 1, "Number of crawler nodes")
	verbose := flag.Bool("verbose", false, "Verbose mode")
	flag.Parse()

	log.Printf("Running crawler with %v crawler nodes and %v liveliness nodes", *numCrawlers, *numLiveliness)

	if *numLiveliness > *numCrawlers {
		log.Fatal("Wrong number of liveliness nodes provided. There should be less liveliness nodes or equal to crawler nodes")
	}

	// fmt.Println(*numCrawlers, *numLiveliness, *verbose)
	var err error

	log.Println("Removing state from previous runs...")
	err = os.RemoveAll(dbPath) // delete an entire directory
	if err != nil {
		log.Fatal("Error removing previous run databases", err)
	}

	BootstrapNodes := []string{
		// IPFS Bootstrapper nodes.
		// TODO: Use ipfs icore.CoreAPI and err := ipfs.Swarm().Connect(ctx, *peerInfo) to resolve these peers.
		// "/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
		// "/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
		// "/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
		// "/dnsaddr/bootstrap.libp2p.io/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",

		// IPFS Cluster Pinning nodes
		"/ip4/138.201.67.219/tcp/4001/p2p/QmUd6zHcbkbcs7SMxwLs48qZVX3vpcM8errYS7xEczwRMA",
		"/ip4/138.201.67.220/tcp/4001/p2p/QmNSYxZAiJHeLdkBg38roksAR9So7Y5eojks1yjEcUtZ7i",
		"/ip4/138.201.68.74/tcp/4001/p2p/QmdnXwLrC8p1ueiq2Qya8joNvk3TVVDAut7PrikmZwubtR",
		"/ip4/94.130.135.167/tcp/4001/p2p/QmUEMvxS2e7iDrereVYc5SWPauXPyNwxcy9BXZrC1QTcHE",

		// IPFS Bootstrapper nodes.
		"/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
		"/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
		"/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
		"/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
	}

	ctx, cancel := context.WithCancel(context.Background())

	mux := &sync.Mutex{}

	db := initDB(dbPath)
	defer db.Close()
	log.Println("Bootstrapping crawlers...")
	// Create and initialize crawlers
	crawlers := make([]*Crawler, *numCrawlers)
	for i := 0; i < *numCrawlers; i++ {
		crawlers[i] = newCrawler(db, mux)
		go crawlers[i].initCrawler(BootstrapNodes, *verbose)
	}

	// Start reporting after 30 seconds to let bootstrap happen
	// TODO: We could use a waitGroup here for sync purposes.
	time.Sleep(30 * time.Second)

	// Liveliness started just in the first crawler. This can
	// be easily changed setting an additional argument.
	for i := 0; i < *numLiveliness; i++ {
		go crawlers[i].liveliness(*verbose)
	}

	// Start reporting
	go reporting(ctx, db)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT)

	select {
	case <-stop:
		cancel()
		for i := 0; i < *numCrawlers; i++ {
			crawlers[i].close()
		}
		os.Exit(0)
	case <-ctx.Done():
		return
	}
}

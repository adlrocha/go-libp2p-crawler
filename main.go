package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/routing"
	"github.com/libp2p/go-libp2p-core/test"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	secio "github.com/libp2p/go-libp2p-secio"
	tcp "github.com/libp2p/go-tcp-transport"
	"github.com/multiformats/go-multiaddr"
	"github.com/syndtr/goleveldb/leveldb"
)

const reportingTime = 10

// SeenNode struct
type SeenNode struct {
	NAT      bool
	lastSeen string
}

// Crawler node structure
type Crawler struct {
	host   host.Host
	ctx    context.Context
	cancel context.CancelFunc
	dht    *kaddht.IpfsDHT
	// peers     map[peer.ID]SeenNode
	// count     int
	seenQueue []SeenNode
	db        *leveldb.DB
	// routingDisc disc.Discoverer
	// wg     sync.WaitGroup
	routing routing.PeerRouting
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
			// If yesterday data start with yesterday's nodes
			data, err = c.db.Get([]byte(fmt.Sprintf("%s.count", yesterday())), nil)
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

// Make ephemeral connections to nodes.
func (c *Crawler) ephemeralConnection(pInfo *peer.AddrInfo) error {

	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)

	err := c.host.Connect(ctx, *pInfo)
	// TODO: Make a way of traversing NATs.
	if err != nil {
		fmt.Fprintf(os.Stderr, "connecting to node: %s\n", err)
	} else {
		fmt.Println("Connected to", pInfo.ID)
	}
	cancel()

	return err
}

// func (c *Crawler) crawlPeer(p peer.ID) {
// 	_, ok := c.peers[p]
// 	if ok {
// 		return
// 	}

// 	log.Printf("Crawling peer %s", p.Pretty())

// 	ctx, cancel := context.WithTimeout(c.ctx, 60*time.Second)
// 	pi, err := c.dht.FindPeer(ctx, p)
// 	cancel()

// 	if err != nil {
// 		log.Printf("Peer not found %s: %s", p.Pretty(), err.Error())
// 		return
// 	}

// 	c.peers[p] = struct{}{}
// 	// select {
// 	// case c.work <- pi:
// 	// case <-c.ctx.Done():
// 	// 	return
// 	// }

// 	ctx, cancel = context.WithTimeout(c.ctx, 60*time.Second)
// 	pch, err := c.dht.GetClosestPeers(ctx, p)

// 	if err != nil {
// 		log.Printf("Can't find peers connected to peer %s: %s", p.Pretty(), err.Error())
// 		cancel()
// 		return
// 	}

// 	var ps []peer.ID
// 	for pip := range pch {
// 		ps = append(ps, pip.ID)
// 	}
// 	cancel()

// 	log.Printf("Peer %s is connected to %d peers", p.Pretty(), len(ps))

// 	for _, p := range ps {
// 		c.crawlPeer(p)
// 	}
// }

func (c *Crawler) crawlFromKey(key string, ping bool) {

	// log.Printf("Crawling from key %s\n", key)

	ctx, cancel := context.WithTimeout(c.ctx, 60*time.Second)
	pch, err := c.dht.GetClosestPeers(ctx, key)

	if err != nil {
		// log.Fatal(err)
		// log.Printf(">> No peers found... deadline reached.")
		// return
	}

	var ps []peer.ID
	for p := range pch {
		ps = append(ps, p)
	}
	cancel()

	// log.Printf("Found %d peers", len(ps))
	for _, pID := range ps {

		// Used to check if behind NAT or not.
		var canConnect error

		// Test connection of found nodes
		if !ping {
			pString := fmt.Sprintf("/p2p/%s", pID.String())

			fmt.Println(pString)
			p, err := multiaddr.NewMultiaddr(pString)
			if err != nil {
				panic(err)
			}

			pInfo, err := peer.AddrInfoFromP2pAddr(p)

			fmt.Println("Trying ", pInfo)
			canConnect = c.ephemeralConnection(pInfo)
			// if err == nil {
			// 	fmt.Println("Connected peer", pInfo.String())
			// }
		} else {
			canConnect = c.dht.Ping(c.ctx, pID)
			if err != nil {
				log.Println("Couldn't ping host")
			}
			pString := fmt.Sprintf("/p2p/%s", pID.String())

			fmt.Println(pString)
			p, err := multiaddr.NewMultiaddr(pString)
			if err != nil {
				panic(err)
			}

			pInfo, err := peer.AddrInfoFromP2pAddr(p)
			fmt.Println("Trying ", pInfo)
			canConnect = c.ephemeralConnection(pInfo)
			if err == nil {
				fmt.Println("Connected peer", pInfo.String())
			}
		}

		var aux SeenNode
		timestamp := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)

		if stored, _ := c.getSeenNode(pID.String()); stored == aux {
			hasNat := false
			if canConnect != nil {
				hasNat = true
			}
			aux = SeenNode{NAT: hasNat, lastSeen: timestamp}
			// Store node in database
			c.storeSeenNode(pID.String(), aux)
			// Update counter
			// counter, _ := c.updateCount(true)
			c.updateCount("total.count", true)
			c.updateCount(fmt.Sprintf("%s.count", currentDate()), true)

			// Add to seenQueue for liveliness check
			// TODO: This should be added to leveldb?
			c.seenQueue = append(c.seenQueue, aux)

			// fmt.Println("=========")
			// fmt.Println("Count: ", counter)
			// fmt.Println("=========")
		} else {
			// If node already seen only update lastSeen

			stored.lastSeen = timestamp
			c.storeSeenNode(pID.String(), stored)
		}

		// // TODO: Check if it exists. If it exists update timestamp, if not add 1 and add key.
		// if _, ok := c.peers[pID]; !ok {
		// 	c.count++
		// 	hasNat := false
		// 	if canConnect != nil {
		// 		hasNat = true
		// 	}
		// 	timestamp := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)
		// 	c.peers[pID] = SeenNode{NAT: hasNat, lastSeen: timestamp}
		// }
	}
}

func (c *Crawler) randomWalk(ping bool) {

	// Choose a random ID
	id, err := test.RandPeerID()
	if err != nil {
		panic(err)
	}
	key := id.String()

	c.crawlFromKey(key, ping)
}

func (c *Crawler) discoveryNodes() {}
func (c *Crawler) findNodes() {
	id, err := test.RandPeerID()
	if err != nil {
		panic(err)
	}
	// fmt.Printf("looking for peer: %s\n", id)
	// ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	// addr, _ := c.dht.FindPeer(ctx, id)
	// fmt.Println("Found peer: ", addr.String())
	// cancel()
	pString := fmt.Sprintf("/p2p/%s", id.String())
	fmt.Println(pString)
	p, err := multiaddr.NewMultiaddr(pString)
	if err != nil {
		panic(err)
	}

	pInfo, err := peer.AddrInfoFromP2pAddr(p)
	if err != nil {
		panic(err)
	}

	// err = host.Connect(ctx, *pInfo)
	fmt.Println("Trying ", pInfo)
	err = c.ephemeralConnection(pInfo)
	if err == nil {
		fmt.Println("Connected peer", pInfo.String())
	}

	// peer, _ := c.routing.FindPeer(c.ctx, pInfo)

}

func (c *Crawler) initCrawler(BootstrapNodes []string) {

	// List of seenNodes for liveliness.
	c.seenQueue = make([]SeenNode, 0)

	transports := libp2p.ChainOptions(
		libp2p.Transport(tcp.NewTCPTransport),
	)

	security := libp2p.Security(secio.ID, secio.New)
	// priv, _, _ := crypto.GenerateKeyPair(crypto.RSA, 2024)
	// opts := []libp2p.Option{libp2p.Identity(priv)}

	// Listen TCP on any available port.
	listenAddrs := libp2p.ListenAddrStrings(
		"/ip4/0.0.0.0/tcp/0",
	)

	//TODO: Share DHT by all crawlers for faster discovery?
	// var dht *kaddht.IpfsDHT
	newDHT := func(h host.Host) (routing.PeerRouting, error) {
		var err error
		c.dht, err = kaddht.New(c.ctx, h)
		return c.dht, err
	}
	// c.dht = dht
	routing := libp2p.Routing(newDHT)
	var err error
	c.host, err = libp2p.New(
		c.ctx,
		transports,
		listenAddrs,
		security,
		routing,
	)
	if err != nil {
		panic(err)
	}

	for _, addr := range c.host.Addrs() {
		fmt.Println("Listening on", addr)
	}

	// Create routingDiscovery
	// c.routingDisc = disc.NewRoutingDiscovery(c.dht)

	for _, pString := range BootstrapNodes {
		p, err := multiaddr.NewMultiaddr(pString)
		if err != nil {
			panic(err)
		}

		pInfo, err := peer.AddrInfoFromP2pAddr(p)
		if err != nil {
			panic(err)
		}

		err = c.host.Connect(c.ctx, *pInfo)
		// err = c.ephemeralConnection(pInfo)
		if err != nil {
			fmt.Fprintf(os.Stderr, "connecting to bootstrap: %s\n", err)
		} else {
			fmt.Println("Connected to bootstrap", pInfo.ID)
		}

		// Node in bootstrapped state.
		err = c.dht.Bootstrap(c.ctx)
		if err != nil {
			panic(err)
		}
	}

	fmt.Println("Crawler has been bootstrapped...")

	for {
		c.randomWalk(true)
	}

	// return Crawler{host: host}
}

func newCrawler(db *leveldb.DB) *Crawler {
	ctx, cancel := context.WithCancel(context.Background())

	c := &Crawler{
		ctx:    ctx,
		cancel: cancel,
	}

	// Initialize database
	c.db = db

	return c
}

func initDB(dbPath string) (db *leveldb.DB) {

	var err error
	db, err = leveldb.OpenFile(dbPath, nil)
	if err != nil {
		log.Fatal(err, "Could not initialize database...")
	}

	return db

}

func currentDate() string {
	date := fmt.Sprintf("%s", time.Now().Format("20060202"))
	return date
}

func yesterday() string {
	date := fmt.Sprintf("%s", time.Now().Add(-24*time.Hour).Format("2006-01-02"))
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

func liveliness(ctx context.Context, db *leveldb.DB) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			// TODO: Iterate from list to see churn

		}
	}
}

func reporting(ctx context.Context, db *leveldb.DB) {
	for {
		select {
		case <-ctx.Done():
			return

		default:
			//TODO: We can show more metrics here if needed.
			totalCount, _ := getCount(db, "total.count")
			// todayCount, _ := getCount(db, fmt.Sprintf("%s.count", currentDate()))
			// yesterdayCount, _ := getCount(db, fmt.Sprintf("%s.count", yesterday()))
			todayLeft, _ := getCount(db, fmt.Sprintf("%s.left", currentDate()))
			var churn float32

			if totalCount == 0 {
				churn = 0
			} else {
				churn = (float32(todayLeft / totalCount)) * 100
			}

			log.Printf("==== Total nodes seen: %d,  Daily churn: %f ====", totalCount, churn)
			time.Sleep(reportingTime * time.Second)
		}
	}
}

func main() {

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

		"/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
		"/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
		"/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
		"/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",

		// You can add more nodes here, for example, another IPFS node you might have running locally, mine was:
		// "/ip4/127.0.0.1/tcp/4010/p2p/QmZp2fhDLxjYue2RiUvLwT9MWdnbDxam32qYFnGmxZDh5L",
	}

	// routingDiscovery := disc.NewRoutingDiscovery(dht)
	// peers, err := disc.FindPeers(ctx, routingDiscovery, "NODESTRING")
	//TODO: Do something with peers found

	// Stop crawlers

	ctx, cancel := context.WithCancel(context.Background())
	dbPath := "./db"
	db := initDB(dbPath)

	a := newCrawler(db)
	// b := newCrawler(db)
	go a.initCrawler(BootstrapNodes)
	// go b.initCrawler(BootstrapNodes)
	// go initCrawler(ctx, BootstrapNodes)

	// Start reporting after 30 seconds to let bootstrap happen
	time.Sleep(30 * time.Second)
	go reporting(ctx, db)

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT)

	select {
	case <-stop:
		cancel()
		os.Exit(0)
	case <-ctx.Done():
		return
	}

}

// func main() {

// 	BootstrapNodes := []string{
// 		// IPFS Bootstrapper nodes.
// 		// TODO: Use ipfs icore.CoreAPI and err := ipfs.Swarm().Connect(ctx, *peerInfo) to resolve these peers.
// 		// "/dnsaddr/bootstrap.libp2p.io/p2p/QmNnooDu7bfjPFoTZYxMNLWUQJyrVwtbZg5gBMjTezGAJN",
// 		// "/dnsaddr/bootstrap.libp2p.io/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
// 		// "/dnsaddr/bootstrap.libp2p.io/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
// 		// "/dnsaddr/bootstrap.libp2p.io/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",

// 		// IPFS Cluster Pinning nodes
// 		"/ip4/138.201.67.219/tcp/4001/p2p/QmUd6zHcbkbcs7SMxwLs48qZVX3vpcM8errYS7xEczwRMA",
// 		"/ip4/138.201.67.220/tcp/4001/p2p/QmNSYxZAiJHeLdkBg38roksAR9So7Y5eojks1yjEcUtZ7i",
// 		"/ip4/138.201.68.74/tcp/4001/p2p/QmdnXwLrC8p1ueiq2Qya8joNvk3TVVDAut7PrikmZwubtR",
// 		"/ip4/94.130.135.167/tcp/4001/p2p/QmUEMvxS2e7iDrereVYc5SWPauXPyNwxcy9BXZrC1QTcHE",

// 		"/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",
// 		"/p2p/QmQCU2EcMqAqQPR2i9bChDtGNJchTbq5TbXJJ16u19uLTa",
// 		"/p2p/QmbLHAnMoJPWSCR5Zhtx6BHJX9KiKNN6tpvbUcqanj75Nb",
// 		"/p2p/QmcZf59bWwK5XFi76CZX8cbJ4BhTzzA3gU1ZjYZcYW3dwt",

// 		// You can add more nodes here, for example, another IPFS node you might have running locally, mine was:
// 		// "/ip4/127.0.0.1/tcp/4010/p2p/QmZp2fhDLxjYue2RiUvLwT9MWdnbDxam32qYFnGmxZDh5L",
// 	}

// 	dbPath := "./db"
// 	db := initDB(dbPath)

// 	a := newCrawler(db)
// 	go a.initCrawler(BootstrapNodes)

// 	iter := db.NewIterator(nil, nil)
// 	for iter.Next() {
// 		// Remember that the contents of the returned slice should not be modified, and
// 		// only valid until the next call to Next.
// 		key := string(iter.Key())
// 		value := string(iter.Value())
// 		if len(strings.Split(key, ".")) == 1 {
// 			fmt.Println(key, value)
// 			_, err := a.dht.FindPeer(a.ctx, key)
// 			fmt.Println(err)
// 		}
// 	}
// 	iter.Release()
// }

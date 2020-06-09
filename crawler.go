package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"sync"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p-core/host"
	"github.com/libp2p/go-libp2p-core/peer"
	"github.com/libp2p/go-libp2p-core/test"
	kaddht "github.com/libp2p/go-libp2p-kad-dht"
	routing "github.com/libp2p/go-libp2p-routing"
	secio "github.com/libp2p/go-libp2p-secio"
	"github.com/libp2p/go-tcp-transport"
	"github.com/multiformats/go-multiaddr"
)

// SeenNode struct
// TODO: More info from seen nodes could be extracted.
type SeenNode struct {
	NAT      bool
	lastSeen string
}

// Crawler node structure
type Crawler struct {
	host     host.Host
	ctx      context.Context
	cancel   context.CancelFunc
	dht      *kaddht.IpfsDHT
	mux      *sync.Mutex
	counters *Counters
}

// Counters shared by all routines
type Counters struct {
	activeNodes    int64
	seenNodesToday int64
	leftNodes      int64
	leftNodesToday int64
	listNodes      map[string]int //0 - not seen; 1 - Behind NAT; 2 - Seen
	startingDate   string
}

// Creates a new crawler node.
func newCrawler(mux *sync.Mutex, counters *Counters) *Crawler {
	ctx, cancel := context.WithCancel(context.Background())

	c := &Crawler{
		ctx:    ctx,
		cancel: cancel,
	}

	// Initialize mux and counters
	c.mux = mux
	c.counters = counters

	return c
}

// Liveliness process to check if nodes are still alive.
func (c *Crawler) liveliness(verbose bool) {
	// for {
	select {
	// Return if context done.
	case <-c.ctx.Done():
		return
	default:
	Alive:

		// Iterate through all seen nodes to check if alive.
		for key, value := range c.counters.listNodes {

			// Used to check if behind NAT or not.
			var canConnectErr error

			// Test connection of found nodes
			pString := fmt.Sprintf("/p2p/%s", key)

			p, err := multiaddr.NewMultiaddr(pString)
			if err != nil {
				panic(err)
			}

			pInfo, err := peer.AddrInfoFromP2pAddr(p)

			// fmt.Println("Checking if alive ", pInfo)
			canConnectErr = c.ephemeralConnection(pInfo)
			// if canConnectErr == nil {
			// 	fmt.Println("Connected peer", pInfo.String())
			// }

			if value == 2 && canConnectErr != nil {
				c.mux.Lock()
				if verbose {
					log.Println("[Liveliness] Node left:", key, canConnectErr)
				}
				c.counters.listNodes[key] = 0
				c.counters.leftNodes++
				c.updateCountersToday("left", true)
				c.counters.activeNodes--
				// fmt.Println("Status of counters", c.counters.activeNodes, c.counters.leftNodes)

				// Remove node from list
				// c.db.Delete([]byte(key), nil)
				c.mux.Unlock()
			}
		}

		goto Alive
	}
	// }
}

// Initializes a crawling node.
func (c *Crawler) initCrawler(BootstrapNodes []string, verbose bool) {

	transports := libp2p.ChainOptions(
		libp2p.Transport(tcp.NewTCPTransport),
		// TODO: Add more transport interfaces for enhanced connectivity??
		// libp2p.Transport()
	)

	security := libp2p.Security(secio.ID, secio.New)

	// Listen TCP on any available port.
	listenAddrs := libp2p.ListenAddrStrings(
		"/ip4/0.0.0.0/tcp/0",
	)

	//TODO: Share DHT by all crawlers for faster discovery?
	newDHT := func(h host.Host) (routing.PeerRouting, error) {
		var err error
		c.dht, err = kaddht.New(c.ctx, h)
		return c.dht, err
	}

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

	// for _, addr := range c.host.Addrs() {
	// 	fmt.Println("Listening on", addr)
	// }

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

		// Stay connected to bootstraps
		err = c.host.Connect(c.ctx, *pInfo)

		if err != nil {
			fmt.Fprintf(os.Stderr, "connecting to bootstrap: %s\n", err)
		} // else {
		// 	fmt.Println("Connected to bootstrap", pInfo.ID)
		// }

		// Node in bootstrapped state. Ready to crawl.
		err = c.dht.Bootstrap(c.ctx)
		if err != nil {
			panic(err)
		}
	}

	fmt.Println("Crawler has been bootstrapped...")

	for {
		select {
		// Return if context done.
		case <-c.ctx.Done():
			// Close host and cancel context.
			c.close()
			return
		default:
			// Start random walk
			c.randomWalk(verbose)
		}
	}
}

// Random DHT walk performed by crawler.
func (c *Crawler) randomWalk(verbose bool) {

	// Choose a random ID
	id, err := test.RandPeerID()
	if err != nil {
		panic(err)
	}
	key := id.String()

	// Start crawling from key starting from random node.
	c.crawlFromKey(key, verbose)
}

func (c *Crawler) crawlFromKey(key string, verbose bool) {

	// Make 60 seconds crawls
	ctx, cancel := context.WithTimeout(c.ctx, timeClosestPeers*time.Second)
	pch, _ := c.dht.GetClosestPeers(ctx, key)

	// No peers found
	cancel()

	var ps []peer.ID
	for p := range pch {
		ps = append(ps, p)
	}

	// log.Printf("Found %d peers", len(ps))
	for _, pID := range ps {

		// Used to check if behind NAT or not.
		var canConnectErr error

		// Test connection of found nodes
		pString := fmt.Sprintf("/p2p/%s", pID.String())

		// fmt.Println(pString)
		p, err := multiaddr.NewMultiaddr(pString)
		if err != nil {
			panic(err)
		}

		pInfo, err := peer.AddrInfoFromP2pAddr(p)

		// fmt.Println("Trying ", pInfo)
		canConnectErr = c.ephemeralConnection(pInfo)
		// if canConnectErr == nil {
		// 	fmt.Println("Connected peer", pInfo.String())
		// }

		// Enforce atomic update
		c.mux.Lock()
		// If the key is empty in db we haven't seen it.
		if c.counters.listNodes[pID.String()] == 0 {
			// hasNat := false
			nat := 2
			if canConnectErr != nil {
				// hasNat = true
				nat = 1
			}
			// Store node in database
			c.counters.listNodes[pID.String()] = nat
			// Update counters
			c.counters.activeNodes++
			c.updateCountersToday("seen", true)
			// fmt.Println("Status of counters", c.counters.activeNodes, c.counters.leftNodes)

			if verbose {
				log.Println("[Random Walk] New Node: ", pID.String())
			}

		} else {
			// If we could see the node but not anymore it means is out.
			if c.counters.listNodes[pID.String()] == 2 && canConnectErr != nil {
				// fmt.Println("RandomWalk LEFT!!", pID.String(), stored.NAT, canConnectErr)
				// Remove node from list
				c.counters.listNodes[pID.String()] = 0
				// Update counters
				c.counters.leftNodes++
				c.counters.activeNodes--
				c.updateCountersToday("left", true)
			}
		}
		c.mux.Unlock()

	}
}

// Make ephemeral connections to nodes.
func (c *Crawler) ephemeralConnection(pInfo *peer.AddrInfo) error {

	ctx, cancel := context.WithTimeout(context.Background(), timeEphemeralConnections*time.Second)

	// TODO: Make a way of traversing NATs. Important
	err := c.host.Connect(ctx, *pInfo)
	errString := fmt.Sprintf("%v", err)
	// If there is a context deadline, retry with a longer deadline.
	if errString == "context deadline exceeded" {
		ctx, cancel := context.WithTimeout(context.Background(), 4*timeEphemeralConnections*time.Second)
		err = c.host.Connect(ctx, *pInfo)
		cancel()
	}
	// if err != nil {
	// 	fmt.Fprintf(os.Stderr, "connecting to node: %s\n", err)
	// } else {
	// 	fmt.Println("Connected to", pInfo.ID)
	// }
	cancel()

	return err
}

func (c *Crawler) close() {
	c.cancel()
	c.host.Close()
}

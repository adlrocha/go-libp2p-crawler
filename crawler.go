package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
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
	"github.com/syndtr/goleveldb/leveldb"
)

// SeenNode struct
// TODO: More info from seen nodes could be extracted.
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
	db     *leveldb.DB
}

// Creates a new crawler node.
func newCrawler(db *leveldb.DB) *Crawler {
	ctx, cancel := context.WithCancel(context.Background())

	c := &Crawler{
		ctx:    ctx,
		cancel: cancel,
	}

	// Gets database handler
	c.db = db

	return c
}

// Liveliness process to check if nodes are still alive.
func (c *Crawler) liveliness() {
	// for {
	select {
	// Return if context done.
	case <-c.ctx.Done():
		return
	default:
	Alive:
		iter := c.db.NewIterator(nil, nil)
		// Iterate through all seen nodes to check if alive.
		for iter.Next() {
			key := string(iter.Key())
			value := string(iter.Value())
			if len(strings.Split(key, ".")) == 1 {
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

				var node SeenNode
				json.Unmarshal([]byte(value), &node)
				timestamp := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)

				// If we could see the node but not anymore it means is out.

				if node.NAT == false && canConnectErr != nil {
					// fmt.Println("Liveliness", key, node.NAT, canConnectErr)
					c.updateCount(fmt.Sprintf("%s.left", currentDate()), true)
					// c.updateCount(fmt.Sprintf("%s.count", currentDate()), false)
					c.updateCount("total.count", false)

					// Remove node from list
					c.db.Delete([]byte(key), nil)

				} else {
					// If node already seen only update lastSeen
					node.lastSeen = timestamp
					if canConnectErr != nil {
						node.NAT = true
					} else {
						node.NAT = false
					}
					c.storeSeenNode(key, node)
				}
			}
		}
		iter.Release()
		goto Alive
	}
	// }
}

// Initializes a crawling node.
func (c *Crawler) initCrawler(BootstrapNodes []string) {

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

		// Stay connected to bootstraps
		err = c.host.Connect(c.ctx, *pInfo)

		if err != nil {
			fmt.Fprintf(os.Stderr, "connecting to bootstrap: %s\n", err)
		} else {
			fmt.Println("Connected to bootstrap", pInfo.ID)
		}

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
			c.randomWalk()
		}
	}
}

// Random DHT walk performed by crawler.
func (c *Crawler) randomWalk() {

	// Choose a random ID
	id, err := test.RandPeerID()
	if err != nil {
		panic(err)
	}
	key := id.String()

	// Start crawling from key starting from random node.
	c.crawlFromKey(key)
}

func (c *Crawler) crawlFromKey(key string) {

	// Make 60 seconds crawls
	ctx, cancel := context.WithTimeout(c.ctx, 60*time.Second)
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

		var aux SeenNode
		timestamp := strconv.FormatInt(time.Now().UTC().UnixNano(), 10)

		if stored, _ := c.getSeenNode(pID.String()); stored == aux {
			hasNat := false
			if canConnectErr != nil {
				hasNat = true
			}
			aux = SeenNode{NAT: hasNat, lastSeen: timestamp}
			// Store node in database
			c.storeSeenNode(pID.String(), aux)
			// Update counters
			c.updateCount(fmt.Sprintf("%s.count", currentDate()), true)
			c.updateCount("total.count", true)

		} else {

			// If we could see the node but not anymore it means is out.
			if stored.NAT == false && canConnectErr != nil {
				// fmt.Println("RandomWalk", pID.String(), stored.NAT, canConnectErr)

				c.updateCount(fmt.Sprintf("%s.left", currentDate()), true)
				// c.updateCount(fmt.Sprintf("%s.count", currentDate()), false)
				c.updateCount("total.count", false)

				// Remove node from list
				c.db.Delete([]byte(pID.String()), nil)

			} else {
				// If node already seen only update lastSeen
				stored.lastSeen = timestamp
				c.storeSeenNode(pID.String(), stored)
			}
		}

	}
}

// Make ephemeral connections to nodes.
func (c *Crawler) ephemeralConnection(pInfo *peer.AddrInfo) error {

	ctx, cancel := context.WithTimeout(context.Background(), timeEphemeralConnections*time.Second)

	// TODO: Make a way of traversing NATs. Important
	err := c.host.Connect(ctx, *pInfo)
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

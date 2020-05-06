# Libp2p node crawler.
Quick and dirty solution to crawl the number of nodes and the
daily churn of a libp2p network. The tool is conformed by the following
processes:

* RandomWalk: Crawlers randomWalk their DHT in search for new nodes.
* Liveliness: Some crawlers may enable their liveliness process. This process
traverses all the seen nodes and check the ones that are still alive in order
to compute the daily churn.
* Reporting: Periodically reports the status of the crawling.

To run the tool run the following command. The default number of workers is 1, but
you can assign more if needed. Currently only one worker runs the liveliness process.
In future version an argument to determine the number of workers that will run
the liveliness process will be added:

```
./go-libp2p-crawler <num_workers>
```

# Design decisions and future work

* It uses LevelDB to store the data. I could have used other databases such as a MongoDB
in order to have "smarter" querying and more functionalities, but I wanted the tool to
be self-contained (no need for other processes). Also, levelDB makes concurrency easy,
so I wouldn't have to worry so much about synchronization and mutex for this quick
development.
(See LeveldB Concurrency [docs](https://github.com/google/leveldb/blob/master/doc/index.md) for further
details on its concurrency).

* The computation of the daily churn has some accuracy problems. To check if the node
is still alive we try to make an `ephemeralConnection`. The problem comes when the node
is behind a NAT, as when we found it in the crawling process we couldn't dial it, and the
problem will persist when checking if alive. This needs to be fixed for an accurate measure.

* I may have missed some of the design decisions and future work.
Throughout the code you will see TODO comments with questions and potential enhancements.

* And of course, the tool needs unit tests to be implemented whenever I have the time.

### Useful resources
Some useful resources in case you want to understand or build upon the code:
* https://github.com/libp2p/go-libp2p-examples
* https://libp2p.io/implementations/
* https://godoc.org/github.com/libp2p/go-libp2p-kad-dht
* https://docs.libp2p.io/concepts/
* https://github.com/jmantas/ipfs-nodes-crawler/blob/master/ipfs-nodes-crawler.py
* https://github.com/raulk/dht-hawk/blob/3501c82be982e2a9c6cfe430497cc43371e75714/hawk.go
* https://godoc.org/github.com/libp2p/go-libp2p-core/discovery

### Questions
* How to "easily" make connections to nodes behind NATs?
* How to find specific peers with libp2p, or ask for the connected peers of a specific one?
* Why [dht.FindPeer](https://godoc.org/github.com/libp2p/go-libp2p-kad-dht#IpfsDHT.FindPeer) 
returns empty AddrInfo?
* What is the best way to find new nodes in the network without specific application with libp2p?
Notifee to listen to incoming connections? Using routingDiscovery? Or RandomWalk?



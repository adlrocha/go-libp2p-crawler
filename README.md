# Libp2p node crawler.
Quick and dirty solution to crawl the number of nodes and the
daily churn of a IPFS/libp2p network. The tool is conformed by the following
processes:

* RandomWalk: Crawlers randomWalk their DHT in search for new nodes.
* Liveliness: Some crawlers may enable their liveliness process. This process
traverses all the seen nodes and check the ones that are still alive in order
to compute the daily churn.
* Reporting: Periodically reports the status of the crawling.

To run the tool, run the following command. The default number of workers is 1, but
you can assign more if needed. You can also choose the number of workers to run the liveliness process.
```
go build
./go-libp2p-crawler -crawler=<num_workers> -liveliness=<num_liveliness> -verbose
```
By defualt, database information is stored in ./db. There is no way of configuring this path yet, but it should be easy for you to change this in `main.go`.

### Design decisions and future work

* The tool uses LevelDB to store the data. I could have used other databases such as a MongoDB
in order to have "smarter" querying and more functionalities, but I wanted the tool to
be self-contained (no need for other processes). Also, levelDB makes concurrency easy,
so I wouldn't have to worry so much about synchronization and mutex for this quick
development.
(See LeveldB Concurrency [docs](https://github.com/google/leveldb/blob/master/doc/index.md) for further
details on its concurrency).

* The daily churn is computed in real time as the number of nodes that have left the network divided by
the total number of nodes seen throughout the day. 
```
churn = (float32(todayLeft) / float32(todayCount)) * 100
```
The number of active nodes is also shown in real time.

* The computation of the daily churn and active nodes has some accuracy problems. To check if the node
is still alive we try to make an `ephemeralConnection`. The problem comes when the node
is behind a NAT, as when we found it in the crawling process we couldn't dial it ditectly, and this issue
persists when checking if alive. This needs to be fixed for an accurate measure.
Moreover, the ephemeralConnection timesout after `timeEphemeralConnections` seconds.
Lowering this number too much may lead to timeouts in rachable nodes.

* A really flat DHT random walk has been implemented for node discovery. Maybe it would have
been more efficient to compute deeper random walks (when a node is found, we check its 
connected nodes and use them as anchor of additional walks). A more complex discovery
of nodes based on libp2p's routing discovery may had also been a good option. I
leave it as a future work.

* I used flat p2p connections to "ping" nodes without determining an specific application. 
I tried using directly a DHT ping, but some peers were blocking it. 

* With the data stored, enhanced metrics could be configured easily in the reporting process.

I may have missed in this README file some of the design decisions and future work. Throughout the code you will find some *TODO* comments with questions and potential enhancements. And of course, the tool needs unit tests to be complete.

### Useful resources
Some useful resources in case you want to understand or build upon the code:
* https://github.com/libp2p/go-libp2p-examples
* https://libp2p.io/implementations/
* https://godoc.org/github.com/libp2p/go-libp2p-kad-dht
* https://docs.libp2p.io/concepts/
* https://github.com/jmantas/ipfs-nodes-crawler/blob/master/ipfs-nodes-crawler.py
* https://github.com/raulk/dht-hawk/blob/3501c82be982e2a9c6cfe430497cc43371e75714/hawk.go
* https://godoc.org/github.com/libp2p/go-libp2p-core/discovery
* https://godoc.org/github.com/libp2p/go-libp2p-core/network#Notifiee

### Questions
* How to "easily" make connections to nodes behind NATs?
* How to find specific peers with libp2p, or ask for the connected peers of a specific one?
* Why [dht.FindPeer](https://godoc.org/github.com/libp2p/go-libp2p-kad-dht#IpfsDHT.FindPeer) 
returns empty AddrInfo?
* What is the best way to find new nodes in the network without specific application with libp2p?
Notifee to listen to incoming connections? Using routingDiscovery? Or RandomWalk?



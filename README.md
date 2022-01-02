# timespotter
First seen / last seen web service based on sha256

Values is stored in memory, saved to disk with SIGHUP or /save API call.
Keep track of hash value, first seen, last seen and count.

```
Usage of ./timespotter:
  -address string
        listen address (default "127.0.0.1")
  -dnsport string
        dns port (default "5300")
  -port string
        listen port (default "5000")
  -state string
        statefile name (default "timespotter.state")
```
State is saved as gzip'ed GOB.
State will be saved if interupt is caught or if /save endpoint is called.


# API

## POST
* /seen/value/\<value\>      - add <value> (string) to seen map
* /seen/hash/\<hash\>       - add <hash> (sha256) to seen map
* /post/value              - add vaules in post to seen map, one value pr. line
* /post/hash               - add hashes in post to seen map, one hash pr. line
* /unseen/value/\<value\>    - remove value (string) from seen map
* /unseen/hash/\<hash\>      - remove hash (sha256) from seen map
* /save                    - save state to statefile
* /load                    - load state from (last) saved statefile
* /expire/first/\<limit\>    - remove all entries with firstseen < limit (unixtime)
* /expire/last/\<limit\>     - remove all entries with lastseen < limit  (unixtime)

## GET
* /check/value/\<value\>    - check if value is seen
* /check/hash/\<hash\>      - check if hash is seen
* /info                   - list info
* /dump                   - dump all entries

## DNS

Values can be queried by DNS.

Query: <0-31>.<32-63>.some.random.name
  
If found, return 127.0.0.1, if not SERVFAIL
  

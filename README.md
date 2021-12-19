# timespotter
First seen / last seen web service based on sha256

Values is stored in memory, saved to disk with SIGUP or /save API call.
Keep track of hash value, first seen, last seen and count.


# API

## POST
* /seen/value/<value>      - add <value> to seen map
* /seen/hash/<hash>.       - add <hash> to seen map
* /post/value              - add vaules in post to seen map, one value pr. line
* /post/hash               - add hashes in post to seen map, one hash pr. line
* /unseen/value/<value>    - remove value from seen map
* /unseen/hash/<hash>      - remove hash from seen map
* /save                    - save state to statefile
* /load                    - load state from (last) saved statefile
* /expire/first/<limit>    - remove all entries with firstseen < limit (unixtime)
* /expire/last/<limit>     - remove all entries with lastseen < limit  (unixtime)

## GET
* /check/value/<value>    - check if value is seen
* /check/hash/<hash>      - check if hash is seen
* /info                   - list info
* /dump                   - dump all entries

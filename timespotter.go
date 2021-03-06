package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"github.com/julienschmidt/httprouter"
	"io/ioutil"
	"log"
	"log/syslog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Global config struct
type Config struct {
	StateName     string
	Port          string
	DNSPort       string
	ListenAddress string
}

// Global config
var config = new(Config)

// Global mutex for map
var maplock = &sync.Mutex{}

// Hashelements
type HashElement struct {
	First int
	Last  int
	Count int
}

type Hashmap map[[32]byte]HashElement

// Global hashmap
var gmap = make(Hashmap)

func main() {
	// Set up logging to syslog
	logwriter, err := syslog.New(syslog.LOG_NOTICE, "timespotter")
	if err == nil {
		log.SetOutput(logwriter)
	}

	// Config
	config_state := flag.String("state", "timespotter.state", "statefile name")
	config_port := flag.String("port", "5000", "listen port")
	config_addr := flag.String("address", "127.0.0.1", "listen address")
	config_dns := flag.String("dnsport", "5300", "dns port")
	flag.Parse()
	if len(*config_state) > 0 {
		config.StateName = *config_state
	}
	if len(*config_port) > 0 {
		config.Port = *config_port
	}
	if len(*config_addr) > 0 {
		config.ListenAddress = *config_addr
	}
	if len(*config_dns) > 0 {
		config.DNSPort = *config_dns
	}

	// Check write permissions on filterstate
	file, err := os.OpenFile(config.StateName, os.O_WRONLY, 0666)
	if err != nil {
		if os.IsPermission(err) {
			log.Println("Unable to write to ", config.StateName)
			log.Println(err)
			fmt.Println("Unable to write to ", config.StateName)
			fmt.Println(err)
			os.Exit(1)
		}
	}
	file.Close()

	// Load state
	log.Printf("Loading state from statefile: %v\n", config.StateName)
	fmt.Printf("Loading state from statefile: %v\n", config.StateName)
	gmap.Load(config.StateName)
	log.Printf("Loaded %v items from state\n", len(gmap))
	fmt.Printf("Loaded %v items from state\n", len(gmap))


	// Catch signals - save state
	c := make(chan os.Signal)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-c
		log.Printf("Signal caught, saving state to file: %v\n", config.StateName)
		fmt.Printf("Signal caught, saving state to file: %v\n", config.StateName)
		gmap.Save(config.StateName)
		fmt.Printf("Saved %v items to state\n", len(gmap))
		os.Exit(1)
	}()

	// Start ServDNS
	dnssocket := config.ListenAddress + ":" + config.DNSPort
	fmt.Printf("Starting dns listner on: %v\n", dnssocket)
	log.Printf("Starting dns listner on: %v\n", dnssocket)
	go ServDNS(dnssocket)

	// Set up handlers
	mux := httprouter.New()
	mux.POST("/seen/value/:value", seenhandler)
	mux.POST("/seen/hash/:value", seenbyhashhandler)
	mux.POST("/post/value", posthandler)
	mux.POST("/post/hash", postbyhashhandler)
	mux.POST("/unseen/value/:value", unseenhandler)
	mux.POST("/unseen/hash/:value", unseenbyhashhandler)
	mux.POST("/save", savehandler)
	mux.POST("/load", loadhandler)
	mux.POST("/expire/first/:limit", expirefirsthandler)
	mux.POST("/expire/last/:limit", expirelasthandler)
	// Overridehandlers to correct values
	mux.POST("/override/first/:hash/:time", overridefirsthandler)
	mux.POST("/override/last/:hash/:time", overridelasthandler)
	mux.POST("/override/count/:hash/:count", overridecounthandler)
	// Check values
	mux.GET("/check/value/:value", checkhandler)
	mux.GET("/check/hash/:value", checkbyhashhandler)
	// Method to allow logging of access of link httpX://<url>/linkspotter/<sha256>
	mux.GET("/linkspotter/:value", seenbyhashhandler)
	mux.GET("/info", infohandler)
	mux.GET("/dump", dumphandler)


	// Start server process
	server := http.Server{
		Addr:    config.ListenAddress + ":" + config.Port,
		Handler: mux,
	}
	fmt.Printf("Starting service on: %v\n", config.Port)
	log.Printf("Starting service on: %v\n", config.Port)
	server.ListenAndServe()
}

// Handler functions

// Record sha256 hash of single value
func seenhandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// Get value
	datavalue := string(p.ByName("value"))
	// Make SHA256 sum and add to seen
	hash := sha256.Sum256([]byte(datavalue))
	seenbyhash(hash, w)
}

// Record single sha256
func seenbyhashhandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// Get hashvalue and encode to [32]byte
	datavalue := string(p.ByName("value"))
	datavalue = strings.ToLower(datavalue)
	bytes, err := hex.DecodeString(datavalue)
	if !( len(datavalue) == 64 && err == nil ) {
		w.WriteHeader(500)
		fmt.Fprintf(w, "Error decoding hash!\n")
		return
	}
	var hash [32]byte
	copy(hash[:], bytes)
	// Add to seen
	seenbyhash(hash, w)
}

// Helper for seenhandler and seenbyhashhandler
func seenbyhash(hash [32]byte, w http.ResponseWriter) {
	// Get current unix time
	atime := int(time.Now().Unix())
	// Lock global map
	maplock.Lock()
	defer maplock.Unlock()
	// If first not set, set to current time
	first := gmap[hash].First
	if first == 0 {
		first = atime
	}
	// Update last time and increment seen counter
	last := atime
	count := gmap[hash].Count
	count = count + 1
	gmap[hash] = HashElement{first, last, count}
	// Write http response
	w.WriteHeader(200)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, "{\n\"status\": \"OK\",\n")
	fmt.Fprintf(w, "\"hash\": \"%x\",\n", hash)
	fmt.Fprintf(w, "\"count\": \"%v\",\n", count)
	fmt.Fprintf(w, "\"first\": \"%v\",\n", first)
	fmt.Fprintf(w, "\"last\": \"%v\"\n}\n", last)
}

// Record sha256 of multiple values, one string pr line
func posthandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// Lock global map
	maplock.Lock()
	defer maplock.Unlock()
	// Close Body after exit to avoid memory leak
	defer r.Body.Close()
	// Read body and split on lines
	resbody, _ := ioutil.ReadAll(r.Body)
	lines := bytes.Split(resbody, []byte("\n"))
	count := 0
	// Iterate over lines
	for _, line := range lines {
		if len(string(line)) > 0 {
			// Update entry if line length > 0
			atime := int(time.Now().Unix())
			datavalue := string(line)
			hash := sha256.Sum256([]byte(datavalue))
			first := gmap[hash].First
			if first == 0 {
				first = atime
			}
			last := atime
			count := gmap[hash].Count
			count = count + 1
			gmap[hash] = HashElement{first, last, count}
		}
		count = count + 1
	}
	// Write http response
	w.WriteHeader(200)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, "{\n\"status\": \"OK\",\n")
	fmt.Fprintf(w, "\"added\": \"%v\"\n}\n", count)
	log.Printf("OK, added %v values to map\n", count)
}

// Record multiple sha256 values, one hash pr line
func postbyhashhandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// Lock global map
	maplock.Lock()
	defer maplock.Unlock()
	// Close Body after exit to avoid memory leak
	defer r.Body.Close()
	// Read body and split on line
	resbody, _ := ioutil.ReadAll(r.Body)
	lines := bytes.Split(resbody, []byte("\n"))
	count := 0
	// Iterate over lines
	for _, line := range lines {
		if len(string(line)) > 0 {
			// If length > 0, add hash to seen
			atime := int(time.Now().Unix())
			datavalue := string(line)
			datavalue = strings.ToLower(datavalue)
			bytes, err := hex.DecodeString(datavalue)
			if err == nil {
				var hash [32]byte
				copy(hash[:], bytes)
				first := gmap[hash].First
				if first == 0 {
					first = atime
				}
				last := atime
				count := gmap[hash].Count
				count = count + 1
				gmap[hash] = HashElement{first, last, count}
			}
			count = count + 1
		}
	}
	// Write http response
	w.WriteHeader(200)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, "{\n\"status\": \"OK\",\n")
	fmt.Fprintf(w, "\"added\": \"%v\"\n}\n", count)
	log.Printf("OK, added %v values to map\n", count)
}

// Remove sha256 from spotted values, by string
func unseenhandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	datavalue := string(p.ByName("value"))
	hash := sha256.Sum256([]byte(datavalue))
	unseenbyhash(hash, w)
}

// Remove sha256 from spotted values
func unseenbyhashhandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	datavalue := string(p.ByName("value"))
	datavalue = strings.ToLower(datavalue)
	bytes, err := hex.DecodeString(datavalue)
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "Error decoding hash!\n")
		return
	}
	var hash [32]byte
	copy(hash[:], bytes)
	unseenbyhash(hash, w)
}

// Unseen helper function
func unseenbyhash(hash [32]byte, w http.ResponseWriter) {
	maplock.Lock()
	defer maplock.Unlock()
	delete(gmap, hash)
	w.WriteHeader(200)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, "{\n\"status\": \"OK\"\n}\n")
}

// Check if value is in spotted values, by string
func checkhandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	maplock.Lock()
	defer maplock.Unlock()
	datavalue := string(p.ByName("value"))
	hash := sha256.Sum256([]byte(datavalue))
	if val, ok := gmap[hash]; ok {
		w.WriteHeader(200)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "{\n\"status\": \"OK\",\n\"found\": true,\n")
		fmt.Fprintf(w, "\"first\": \"%v\",\n", val.First)
		fmt.Fprintf(w, "\"last\": \"%v\",\n", val.Last)
		fmt.Fprintf(w, "\"count\": \"%v\",\n", val.Count)
		fmt.Fprintf(w, "\"hash\": \"%x\"\n", hash)
		fmt.Fprintf(w, "}\n")
	} else {
		w.WriteHeader(404)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "{\n\"status\": \"NOT FOUND\",\n\"found\": false\n}\n")
	}

}

// Check if sha256 is in spotted values
func checkbyhashhandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	maplock.Lock()
	defer maplock.Unlock()
	datavalue := string(p.ByName("value"))
	datavalue = strings.ToLower(datavalue)
	bytes, err := hex.DecodeString(datavalue)
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "Error decoding hash!\n")
		return
	}
	var hash [32]byte
	copy(hash[:], bytes)
	if val, ok := gmap[hash]; ok {
		w.WriteHeader(200)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "{\n\"status\": \"OK\",\n\"found\": true,\n")
		fmt.Fprintf(w, "\"first\": \"%v\",\n", val.First)
		fmt.Fprintf(w, "\"last\": \"%v\",\n", val.Last)
		fmt.Fprintf(w, "\"count\": \"%v\",\n", val.Count)
		fmt.Fprintf(w, "\"hash\": \"%x\"\n", hash)
		fmt.Fprintf(w, "}\n")
	} else {
		w.WriteHeader(404)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "{\n\"status\": \"NOT FOUND\",\n\"found\": false\n}\n")
	}

}

// Dump nr of values spotted
func infohandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	maplock.Lock()
	defer maplock.Unlock()
	w.WriteHeader(200)
	w.Header().Set("Content-Type", "application/json")
	count := 0
	for range gmap {
		count = count + 1
	}
	fmt.Fprintf(w, "{\n\"status\": \"OK\",\n\"keys\": \"%v\"\n}\n", count)
}

// Dump all spotted values
func dumphandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	maplock.Lock()
	defer maplock.Unlock()
	w.WriteHeader(200)
	w.Header().Set("Content-Type", "text/plain")
	count := 0
	for i:= range gmap {
		fmt.Fprintf(w, "%v,%v,%v,%v\n", hex.EncodeToString(i[:]), gmap[i].First, gmap[i].Last, gmap[i].Count)
		count = count + 1
	}
}

// Save spotted values to statefile
func savehandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	gmap.Save(config.StateName)
	w.WriteHeader(200)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, "{\n\"status\": \"OK\"\n}\n")
}

// Load values from statefile
func loadhandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	gmap.Load(config.StateName)
	w.WriteHeader(200)
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, "{\n\"status\": \"OK\"\n}\n")
}

// Expire evry entry with firstseen > limit
func expirefirsthandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	maplock.Lock()
	defer maplock.Unlock()
	expirevalue, err := strconv.Atoi(string(p.ByName("limit")))
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "Error decoding limit!\n")
		return
	}
	count := 0
	for i := range gmap {
		if gmap[i].First > expirevalue {
			delete(gmap, i)
			count = count + 1
		}
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, "{\n\"status\": \"OK\",")
	fmt.Fprintf(w, "\"expired\": \"%v\"\n}\n", count)
}

// Expire evry entry with lastseen > limit
func expirelasthandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	maplock.Lock()
	defer maplock.Unlock()
	expirevalue, err := strconv.Atoi(string(p.ByName("limit")))
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "Error decoding limit!\n")
		return
	}
	count := 0
	for i := range gmap {
		if gmap[i].Last > expirevalue {
			delete(gmap, i)
			count = count + 1
		}
	}
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprintf(w, "{\n\"status\": \"OK\",")
	fmt.Fprintf(w, "\"expired\": \"%v\"\n}\n", count)
}


// Override first value for sha256
func overridefirsthandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// Get hashvalue and encode to [32]byte
	hashvalue := string(p.ByName("hash"))
	hashvalue = strings.ToLower(hashvalue)
	bytes, err := hex.DecodeString(hashvalue)
	if !( len(hashvalue) == 64 && err == nil ) {
		w.WriteHeader(500)
		fmt.Fprintf(w, "Error decoding hash!\n")
		return
	}
	var hash [32]byte
	copy(hash[:], bytes)

	newtime, err := strconv.Atoi(string(p.ByName("time")))
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "Error decoding time!\n")
		return
	}

	if val, ok := gmap[hash]; ok {
		w.WriteHeader(200)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "{\n\"status\": \"OK\",\n\"found\": true,\n")
		fmt.Fprintf(w, "\"first\": \"%v\",\n", newtime)
		fmt.Fprintf(w, "\"oldfirst\": \"%v\",\n", val.First)
		fmt.Fprintf(w, "\"last\": \"%v\",\n", val.Last)
		fmt.Fprintf(w, "\"count\": \"%v\",\n", val.Count)
		fmt.Fprintf(w, "\"hash\": \"%x\"\n", hash)
		fmt.Fprintf(w, "}\n")

		last := gmap[hash].Last
		count := gmap[hash].Count
		gmap[hash] = HashElement{newtime, last, count}
	} else {
		w.WriteHeader(404)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "{\n\"status\": \"NOT FOUND\",\n\"found\": false\n}\n")
	}
}


// Override last value for sha256
func overridelasthandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// Get hashvalue and encode to [32]byte
	hashvalue := string(p.ByName("hash"))
	hashvalue = strings.ToLower(hashvalue)
	bytes, err := hex.DecodeString(hashvalue)
	if !( len(hashvalue) == 64 && err == nil ) {
		w.WriteHeader(500)
		fmt.Fprintf(w, "Error decoding hash!\n")
		return
	}
	var hash [32]byte
	copy(hash[:], bytes)

	newtime, err := strconv.Atoi(string(p.ByName("time")))
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "Error decoding time!\n")
		return
	}

	if val, ok := gmap[hash]; ok {
		w.WriteHeader(200)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "{\n\"status\": \"OK\",\n\"found\": true,\n")
		fmt.Fprintf(w, "\"first\": \"%v\",\n", val.First)
		fmt.Fprintf(w, "\"last\": \"%v\",\n", newtime)
		fmt.Fprintf(w, "\"oldfirst\": \"%v\",\n", val.Last)
		fmt.Fprintf(w, "\"count\": \"%v\",\n", val.Count)
		fmt.Fprintf(w, "\"hash\": \"%x\"\n", hash)
		fmt.Fprintf(w, "}\n")

		first := gmap[hash].First
		count := gmap[hash].Count
		gmap[hash] = HashElement{first, newtime, count}
	} else {
		w.WriteHeader(404)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "{\n\"status\": \"NOT FOUND\",\n\"found\": false\n}\n")
	}
}

// Override count value for sha256
func overridecounthandler(w http.ResponseWriter, r *http.Request, p httprouter.Params) {
	// Get hashvalue and encode to [32]byte
	hashvalue := string(p.ByName("hash"))
	hashvalue = strings.ToLower(hashvalue)
	bytes, err := hex.DecodeString(hashvalue)
	if !( len(hashvalue) == 64 && err == nil ) {
		w.WriteHeader(500)
		fmt.Fprintf(w, "Error decoding hash!\n")
		return
	}
	var hash [32]byte
	copy(hash[:], bytes)

	newcount, err := strconv.Atoi(string(p.ByName("count")))
	if err != nil {
		w.WriteHeader(500)
		fmt.Fprintf(w, "Error decoding count!\n")
		return
	}

	if val, ok := gmap[hash]; ok {
		w.WriteHeader(200)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "{\n\"status\": \"OK\",\n\"found\": true,\n")
		fmt.Fprintf(w, "\"first\": \"%v\",\n", val.First)
		fmt.Fprintf(w, "\"last\": \"%v\",\n", val.Last)
		fmt.Fprintf(w, "\"count\": \"%v\",\n", newcount)
		fmt.Fprintf(w, "\"oldcount\": \"%v\",\n", val.Count)
		fmt.Fprintf(w, "\"hash\": \"%x\"\n", hash)
		fmt.Fprintf(w, "}\n")

		first := gmap[hash].First
		last := gmap[hash].Last
		gmap[hash] = HashElement{first, last, newcount}
	} else {
		w.WriteHeader(404)
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, "{\n\"status\": \"NOT FOUND\",\n\"found\": false\n}\n")
	}
}


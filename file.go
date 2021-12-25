package main

import (
	"compress/gzip"
	"encoding/gob"
	"log"
	"os"
)

// Savestate
func (t *Hashmap) Save(filename string) error {
	// Lock hash while saving
	maplock.Lock()
	defer maplock.Unlock()

	// Create file
	fi, err := os.Create(filename)
	if err != nil {
		log.Printf("Unable to create file: %v\n", filename)
		return err
	}
	defer fi.Close()

	// Make gzip writer
	fz := gzip.NewWriter(fi)
	defer fz.Close()

	// Make new Gob encoder
	encoder := gob.NewEncoder(fz)
	// Encode file
	err = encoder.Encode(t)
	if err != nil {
		log.Printf("Unable to encode file: %v\n", filename)
		return err
	}
	return nil
}

// Loadstate
func (t *Hashmap) Load(filename string) error {
	// Lock hash while loading
	maplock.Lock()
	defer maplock.Unlock()

	// Open file
	fi, err := os.Open(filename)
	if err != nil {
		log.Printf("Unable to open file: %v\n", filename)
		return err
	}
	defer fi.Close()

	// Make new gzip reader
	fz, err := gzip.NewReader(fi)
	if err != nil {
		log.Printf("Unable to create gzip reader: %v\n", filename)
		return err
	}
	defer fz.Close()

	// Make new Gob decoder
	decoder := gob.NewDecoder(fz)
	// Decode file to gmap
	err = decoder.Decode(&t)
	if err != nil {
		log.Printf("Unable to decode: %v\n", err)
		return err
	}
	return nil
}


package main

import (
	"github.com/miekg/dns"
	"log"
	"net"
	"strings"
	"encoding/hex"
)

func ServDNS(socket string) {
	dns.HandleFunc(".", func(w dns.ResponseWriter, req *dns.Msg) {
		// Lock global map
		defer maplock.Unlock()
		maplock.Lock()

		var resp dns.Msg
		resp.SetReply(req)
		// Iterate questions
		for _, q := range req.Question {

			// Split question on .
			qparts := strings.Split(q.Name, ".")
			// To few parts
			if len(qparts) <2 {
				dns.HandleFailed(w, req)
				return
			}
			// Build hashstring from leftmost part 0 and 1
			data1 := strings.ToLower(qparts[0])
			data2 := strings.ToLower(qparts[1])
			datavalue := data1 + data2

			// Encode to hex
			bytes, err := hex.DecodeString(datavalue)
			if err != nil {
				dns.HandleFailed(w, req)
				return
			}
			// Convert to [32]byte
			var hash [32]byte
			copy(hash[:], bytes)
			// If key is found
			if _, ok := gmap[hash]; ok {
				// Build answer
				a := dns.A{
					Hdr: dns.RR_Header{
						Name:   q.Name,
						Rrtype: dns.TypeA,
						Class:  dns.ClassINET,
						Ttl:    0,
					},
					A: net.ParseIP("127.0.0.1").To4(),
				}
				resp.Answer = append(resp.Answer, &a)
			} else {
				//Else SERVFAIL
				dns.HandleFailed(w, req)
				return
			}
		}
		w.WriteMsg(&resp)

	})
	// Start DNS listner
	log.Fatal(dns.ListenAndServe(socket, "udp", nil))
}

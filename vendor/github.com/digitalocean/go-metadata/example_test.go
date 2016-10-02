package metadata_test

import (
	"fmt"
	"log"

	"github.com/digitalocean/go-metadata"
)

// Create a client and query it for all available metadata.
func Example() {
	// Create a client
	client := metadata.NewClient(opts)

	// Request all the metadata about the current droplet
	all, err := client.Metadata()
	if err != nil {
		log.Fatal(err)
	}

	// Lookup what our IPv4 address is on our first public
	// network interface.
	publicIPv4Addr := all.Interfaces["public"][0].IPv4.IPAddress

	fmt.Println(publicIPv4Addr)
	// Output:
	// 192.168.0.100
}

package metadata_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/digitalocean/go-metadata"
)

var opts = stubMetadata()

func stubMetadata() metadata.ClientOption {
	srv := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, req *http.Request) {
		rw.Write([]byte(`{"interfaces":{"public":[{"ipv4":{"ip_address":"192.168.0.100"}}]}}`))
	}))
	u, err := url.Parse(srv.URL)
	if err != nil {
		panic(err)
	}
	// the server is not closed since the Example process is about to die anyways.
	// makes for a cleaner Example in the docs.
	return metadata.WithBaseURL(u)
}

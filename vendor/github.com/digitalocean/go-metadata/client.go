// Package metadata implements a client for the DigitalOcean metadata
// API. This API allows a droplet to inspect information about itself,
// like it's region, droplet ID, and so on.
//
// Documentation for the API is available at:
//
//    https://developers.digitalocean.com/documentation/metadata/
package metadata

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net/http"
	"net/url"
	"path"
	"time"
)

const (
	maxErrMsgLen = 128 // arbitrary max length for error messages

	defaultTimeout = 2 * time.Second
	defaultPath    = "/metadata/v1/"
)

var (
	defaultBaseURL = func() *url.URL {
		u, err := url.Parse("http://169.254.169.254")
		if err != nil {
			panic(err)
		}
		return u
	}()
)

// ClientOption modifies the default behavior of a metadata client. This
// is usually not needed.
type ClientOption func(*Client)

// WithHTTPClient makes the metadata client use the given HTTP client.
func WithHTTPClient(client *http.Client) ClientOption {
	return func(metaclient *Client) { metaclient.client = client }
}

// WithBaseURL makes the metadata client reach the metadata API using the
// given base URL.
func WithBaseURL(base *url.URL) ClientOption {
	return func(metaclient *Client) { metaclient.baseURL = base }
}

// Client to interact with the DigitalOcean metadata API, from inside
// a droplet.
type Client struct {
	client  *http.Client
	baseURL *url.URL
}

// NewClient creates a client for the metadata API.
func NewClient(opts ...ClientOption) *Client {
	client := &Client{
		client:  &http.Client{Timeout: defaultTimeout},
		baseURL: defaultBaseURL,
	}
	for _, opt := range opts {
		opt(client)
	}
	return client
}

// Metadata contains the entire contents of a Droplet's metadata.
// This method is unique because it returns all of the
// metadata at once, instead of individual metadata items.
func (c *Client) Metadata() (*Metadata, error) {
	metadata := new(Metadata)
	err := c.doGetURL(c.resolve("/metadata/v1.json"), func(r io.Reader) error {
		return json.NewDecoder(r).Decode(metadata)
	})
	return metadata, err
}

// DropletID returns the Droplet's unique identifier. This is
// automatically generated upon Droplet creation.
func (c *Client) DropletID() (int, error) {
	dropletID := new(int)
	err := c.doGet("id", func(r io.Reader) error {
		_, err := fmt.Fscanf(r, "%d", dropletID)
		return err
	})
	return *dropletID, err
}

// Hostname returns the Droplet's hostname, as specified by the
// user during Droplet creation.
func (c *Client) Hostname() (string, error) {
	var hostname string
	err := c.doGet("hostname", func(r io.Reader) error {
		hostnameraw, err := ioutil.ReadAll(r)
		hostname = string(hostnameraw)
		return err
	})
	return hostname, err
}

// UserData returns the user data that was provided by the user
// during Droplet creation. User data can contain arbitrary data
// for miscellaneous use or, with certain Linux distributions,
// an arbitrary shell script or cloud-config file that will be
// consumed by a variation of cloud-init upon boot. At this time,
// cloud-config support is included with CoreOS, Ubuntu 14.04, and
// CentOS 7 images on DigitalOcean.
func (c *Client) UserData() (string, error) {
	var userdata string
	err := c.doGet("user-data", func(r io.Reader) error {
		userdataraw, err := ioutil.ReadAll(r)
		userdata = string(userdataraw)
		return err
	})
	return userdata, err
}

// VendorData provided data that can be used to configure Droplets
// upon their creation. This is similar to user data, but it is
// provided by DigitalOcean instead of the user.
func (c *Client) VendorData() (string, error) {
	var vendordata string
	err := c.doGet("vendor-data", func(r io.Reader) error {
		vendordataraw, err := ioutil.ReadAll(r)
		vendordata = string(vendordataraw)
		return err
	})
	return vendordata, err
}

// Region returns the region code of where the Droplet resides.
func (c *Client) Region() (string, error) {
	var region string
	err := c.doGet("region", func(r io.Reader) error {
		regionraw, err := ioutil.ReadAll(r)
		region = string(regionraw)
		return err
	})
	return region, err
}

// PublicKeys returns the public SSH key(s) that were added to
// the Droplet's root user's authorized_keys file during Droplet
// creation.
func (c *Client) PublicKeys() ([]string, error) {
	var keys []string
	err := c.doGet("public-keys", func(r io.Reader) error {
		scan := bufio.NewScanner(r)
		for scan.Scan() {
			keys = append(keys, scan.Text())
		}
		return scan.Err()
	})
	return keys, err
}

// Nameservers returns the nameserver entries that are added
// to a Droplet's /etc/resolv.conf file during creation.
func (c *Client) Nameservers() ([]string, error) {
	var ns []string
	err := c.doGet("dns/nameservers", func(r io.Reader) error {
		scan := bufio.NewScanner(r)
		for scan.Scan() {
			ns = append(ns, scan.Text())
		}
		return scan.Err()
	})
	return ns, err
}

// FloatingIPv4Active returns true if an IPv4 Floating IP
// Address is assigned to the Droplet.
func (c *Client) FloatingIPv4Active() (bool, error) {
	var active bool
	err := c.doGet("floating_ip/ipv4/active", func(r io.Reader) error {
		activeraw, err := ioutil.ReadAll(r)
		if string(activeraw) == "true" {
			active = true
		}
		return err
	})
	return active, err
}

func (c *Client) doGet(resource string, decoder func(r io.Reader) error) error {
	return c.doGetURL(c.resolve(defaultPath, resource), decoder)
}

func (c *Client) doGetURL(url string, decoder func(r io.Reader) error) error {
	resp, err := c.client.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return c.makeError(resp)
	}
	return decoder(resp.Body)
}

func (c *Client) makeError(resp *http.Response) error {
	body, _ := ioutil.ReadAll(io.LimitReader(resp.Body, maxErrMsgLen))
	if len(body) >= maxErrMsgLen {
		body = append(body[:maxErrMsgLen], []byte("... (elided)")...)
	} else if len(body) == 0 {
		body = []byte(resp.Status)
	}
	return fmt.Errorf("unexpected response from metadata API, status %d: %s",
		resp.StatusCode, string(body))
}

func (c *Client) resolve(basePath string, resource ...string) string {
	dupe := *c.baseURL
	dupe.Path = path.Join(append([]string{basePath}, resource...)...)
	return dupe.String()
}

package profitbricks

import (
	"fmt"
	"os"
	"strconv"
	"testing"
)

// bad_status is the return string for bad status errors
func bad_status(wanted, got int) string {
	return " StatusCode is " + strconv.Itoa(got) + " wanted " + strconv.Itoa(wanted)
}

// Set Username and Password here for Testing.
/*var username = "jclouds@stackpointcloud.com"
var passwd = os.Getenv("PB_PASSWORD")*/
var username = os.Getenv("PROFITBRICKS_USERNAME")
var passwd = os.Getenv("PROFITBRICKS_PASSWORD")
var endpoint = os.Getenv("PROFITBRICKS_API_URL")
var location = "us/las"
var image = getImageId(location, "ubuntu-16.04", "HDD")

func TestSetAuth(t *testing.T) {
	fmt.Println("Current Username ", Username)
	SetAuth(username, passwd)
	fmt.Println("Applied Username ", Username)
}

func TestSetEndpoint(t *testing.T) {
	SetEndpoint(endpoint)
	fmt.Println("Endpoint is ", Endpoint)
}

func TestImage(t *testing.T) {
	fmt.Println("Image ID is:", image)
}

func TestMain(m *testing.M) {
	r := m.Run()
	SetAuth(os.Getenv("PROFITBRICKS_USERNAME"), os.Getenv("PROFITBRICKS_PASSWORD"))
	os.Exit(r)
}

// Setup creds for single running tests
func setupTestEnv() {
	SetAuth(os.Getenv("PROFITBRICKS_USERNAME"), os.Getenv("PROFITBRICKS_PASSWORD"))
	SetEndpoint(os.Getenv("PROFITBRICKS_API_URL"))
}

func TestSetUserAgent(t *testing.T) {
	SetUserAgent("blah")

	if AgentHeader != "blah" {
		t.Errorf("AgentHeader not equal %s", "blah")
	}
}
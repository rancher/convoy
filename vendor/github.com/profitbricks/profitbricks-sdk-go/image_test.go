// image_test.go

package profitbricks

import (
	"fmt"
	"testing"
)

var imgid string

func TestListImages(t *testing.T) {
	setupTestEnv()
	want := 200
	resp := ListImages()

	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
		t.Errorf(resp.Response)
	}
}

func TestGetImage(t *testing.T) {
	want := 200
	resp := GetImage(imgid)

	if resp.StatusCode != want {
		if resp.StatusCode == 403 {
			fmt.Println(bad_status(want, resp.StatusCode))
			fmt.Println("This error might be due to user's permission level ")
		}
	}
}

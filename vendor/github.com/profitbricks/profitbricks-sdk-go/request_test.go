package profitbricks

import (
	"testing"
)

func TestGetRequestStatus(t *testing.T) {
	setupTestEnv()
	want := 200
	resp := GetRequestStatus("https://api.profitbricks.com/rest/v2/requests/2b31cc27-a604-4751-afc4-497b481e353d/status")
	if resp.StatusCode != want {
		t.Errorf(bad_status(want, resp.StatusCode))
	}

}

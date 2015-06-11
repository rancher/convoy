package main

import (
	"fmt"
	"github.com/rancherio/volmgr/util"
	"io"
	"io/ioutil"
	"net/http"
)

func (c *Client) call(method, path string, data interface{}, headers map[string][]string) (io.ReadCloser, int, error) {
	params, err := util.EncodeData(data)
	if err != nil {
		return nil, -1, err
	}

	if data != nil {
		if headers == nil {
			headers = make(map[string][]string)
		}
		headers["Context-Type"] = []string{"application/json"}
	}

	body, _, statusCode, err := c.clientRequest(method, path, params, headers)

	return body, statusCode, err
}

func (c *Client) HTTPClient() *http.Client {
	return &http.Client{Transport: c.transport}
}

func getRequestPath(path string) string {
	return fmt.Sprintf("/v1%s", path)
}

func (c *Client) clientRequest(method, path string, in io.Reader, headers map[string][]string) (io.ReadCloser, string, int, error) {
	req, err := http.NewRequest(method, getRequestPath(path), in)
	if err != nil {
		return nil, "", -1, err
	}
	req.Header.Set("User-Agent", "Rancher-Volmgr-Client/"+API_VERSION)
	req.URL.Host = c.addr
	req.URL.Scheme = c.scheme

	resp, err := c.HTTPClient().Do(req)
	statusCode := -1
	if resp != nil {
		statusCode = resp.StatusCode
	}
	if err != nil {
		return nil, "", statusCode, err
	}
	if statusCode < 200 || statusCode >= 400 {
		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			return nil, "", statusCode, err
		}
		if len(body) == 0 {
			return nil, "", statusCode, fmt.Errorf("Incompatable version")
		}
		return nil, "", statusCode, fmt.Errorf("Error response from server, %v", string(body))
	}
	return resp.Body, resp.Header.Get("Context-Type"), statusCode, nil
}

func sendRequest(method, request string, data interface{}) error {
	log.Debugf("Sending request %v %v", method, request)
	if data != nil {
		log.Debugf("With data %+v", data)
	}
	rc, _, err := client.call(method, request, data, nil)
	if err != nil {
		return err
	}

	defer rc.Close()

	b, err := ioutil.ReadAll(rc)
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

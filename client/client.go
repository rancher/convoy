package client

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/rancher/convoy/api"
	"github.com/rancher/convoy/util"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"time"
)

type convoyClient struct {
	addr      string
	scheme    string
	transport *http.Transport
}

var (
	verboseFlag = "verbose"

	log    = logrus.WithFields(logrus.Fields{"pkg": "client"})
	client convoyClient
)

func (c *convoyClient) call(method, path string, data interface{}, headers map[string][]string) (io.ReadCloser, int, error) {
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

func (c *convoyClient) httpClient() *http.Client {
	return &http.Client{Transport: c.transport}
}

func getRequestPath(path string) string {
	return fmt.Sprintf("/v1%s", path)
}

func (c *convoyClient) clientRequest(method, path string, in io.Reader, headers map[string][]string) (io.ReadCloser, string, int, error) {
	req, err := http.NewRequest(method, getRequestPath(path), in)
	if err != nil {
		return nil, "", -1, err
	}
	req.Header.Set("User-Agent", "Convoy-Client/"+api.API_VERSION)
	req.URL.Host = c.addr
	req.URL.Scheme = c.scheme

	resp, err := c.httpClient().Do(req)
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

func sendRequest(method, request string, data interface{}) (io.ReadCloser, error) {
	log.Debugf("Sending request %v %v", method, request)
	if data != nil {
		log.Debugf("With data %+v", data)
	}
	rc, _, err := client.call(method, request, data, nil)
	if err != nil {
		return nil, err
	}
	return rc, nil
}

func sendRequestAndPrint(method, request string, data interface{}) error {
	rc, err := sendRequest(method, request, data)
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

func cmdNotFound(c *cli.Context, command string) {
	panic(fmt.Errorf("Unrecognized command: %s", command))
}

// NewCli would generate Convoy CLI
func NewCli(version string) *cli.App {
	app := cli.NewApp()
	app.Name = "convoy"
	app.Version = version
	app.Author = "Sheng Yang <sheng.yang@rancher.com>"
	app.Usage = "A volume manager capable of snapshot and delta backup"
	app.Flags = []cli.Flag{
		cli.StringFlag{
			Name:  "socket, s",
			Value: "/var/run/convoy/convoy.sock",
			Usage: "Specify unix domain socket for communication between server and client",
		},
		cli.BoolFlag{
			Name:  "debug, d",
			Usage: "Enable debug level log with client or not",
		},
		cli.BoolFlag{
			Name:  "verbose",
			Usage: "Verbose level output for client, for create volume/snapshot etc",
		},
	}
	app.CommandNotFound = cmdNotFound
	app.Before = initClient
	app.Commands = []cli.Command{
		daemonCmd,
		infoCmd,
		volumeCreateCmd,
		volumeDeleteCmd,
		volumeMountCmd,
		volumeUmountCmd,
		volumeListCmd,
		volumeInspectCmd,
		snapshotCmd,
		backupCmd,
	}
	return app
}

func initClient(c *cli.Context) error {
	sockFile := c.GlobalString("socket")
	if sockFile == "" {
		return fmt.Errorf("Require unix domain socket location")
	}
	logrus.SetOutput(os.Stderr)
	debug := c.GlobalBool("debug")
	if debug {
		logrus.SetLevel(logrus.DebugLevel)
	}
	client.addr = sockFile
	client.scheme = "http"
	client.transport = &http.Transport{
		DisableCompression: true,
		Dial: func(_, _ string) (net.Conn, error) {
			return net.DialTimeout("unix", sockFile, 10*time.Second)
		},
	}
	return nil
}

func getName(c *cli.Context, key string, required bool) (string, error) {
	var err error
	var name string
	if key == "" {
		name = c.Args().First()
	} else {
		name, err = util.GetFlag(c, key, required, err)
		if err != nil {
			return "", err
		}
	}
	if name == "" && !required {
		return "", nil
	}

	if err := util.CheckName(name); err != nil {
		return "", err
	}
	return name, nil
}

func getNames(c *cli.Context) ([]string, error) {
	names := c.Args()
	for _, name := range names {
		if err := util.CheckName(name); err != nil {
			return nil, err
		}
	}
	return names, nil
}

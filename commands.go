package main

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/gorilla/mux"
	"github.com/rancherio/volmgr/api"
	"github.com/rancherio/volmgr/drivers"
	"github.com/rancherio/volmgr/util"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	. "github.com/rancherio/volmgr/logging"
)

func getConfigFileName(root string) string {
	return filepath.Join(root, CONFIGFILE)
}

func getCfgName() string {
	return CONFIGFILE
}

func cmdStartServer(c *cli.Context) {
	if err := startServer(c); err != nil {
		panic(err)
	}
}

func serverEnvironmentSetup(c *cli.Context) error {
	root := c.String("root")
	if root == "" {
		return fmt.Errorf("Have to specific root directory")
	}
	if err := util.MkdirIfNotExists(root); err != nil {
		return fmt.Errorf("Invalid root directory:", err)
	}

	lock = filepath.Join(root, LOCKFILE)
	if err := util.LockFile(lock); err != nil {
		return fmt.Errorf("Failed to lock the file", err.Error())
	}

	logName := c.String("log")
	if logName != "" {
		logFile, err := os.OpenFile(logName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			return err
		}
		logrus.SetFormatter(&logrus.JSONFormatter{})
		logrus.SetOutput(logFile)
	} else {
		logrus.SetOutput(os.Stderr)
	}

	return nil
}

func (s *Server) cleanup() {
	/* cleanup doesn't works with mounted volume
	if err := s.StorageDriver.Shutdown(); err != nil {
		log.Error("fail to shutdown driver: ", err.Error())
	}
	*/
}

func environmentCleanup() {
	log.Debug("Cleaning up environment...")
	if lock != "" {
		util.UnlockFile(lock)
	}
	if logFile != nil {
		logFile.Close()
	}
	if r := recover(); r != nil {
		api.ResponseLogAndError(r)
		os.Exit(1)
	}
}

type RequestHandler func(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error

func makeHandlerFunc(method string, route string, version string, f RequestHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Debugf("Calling: %v, %v", method, route)
		log.Debugf("Request: %v, %v", r.Method, r.RequestURI)

		if strings.Contains(r.Header.Get("User-Agent"), "Rancher-Volmgr-Client/") {
			userAgent := strings.Split(r.Header.Get("User-Agent"), "/")
			if len(userAgent) == 2 && userAgent[1] != version {
				http.Error(w, fmt.Errorf("client version %v doesn't match with server %v", userAgent[1], version).Error(), http.StatusNotFound)
				return
			}
		}
		if err := f(version, w, r, mux.Vars(r)); err != nil {
			logrus.Errorf("Handler for %s %s returned error: %s", method, route, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	}
}

func createRouter(s *Server) *mux.Router {
	router := mux.NewRouter()
	m := map[string]map[string]RequestHandler{
		"GET": {
			"/info":                                                            s.doInfo,
			"/volumes/":                                                        s.doVolumeList,
			"/volumes/{volume}/":                                               s.doVolumeList,
			"/volumes/{volume}/snapshots/{snapshot}/":                          s.doVolumeList,
			"/blockstores/{blockstore}/volumes/{volume}/":                      s.doBlockStoreListVolume,
			"/blockstores/{blockstore}/volumes/{volume}/snapshots/{snapshot}/": s.doBlockStoreListVolume,
		},
		"POST": {
			"/volumes/create":                                                         s.doVolumeCreate,
			"/volumes/{volume}/mount":                                                 s.doVolumeMount,
			"/volumes/{volume}/umount":                                                s.doVolumeUmount,
			"/volumes/{volume}/snapshots/create":                                      s.doSnapshotCreate,
			"/blockstores/register":                                                   s.doBlockStoreRegister,
			"/blockstores/{blockstore}/volumes/{volume}/add":                          s.doBlockStoreAddVolume,
			"/blockstores/{blockstore}/volumes/{volume}/snapshots/{snapshot}/backup":  s.doSnapshotBackup,
			"/blockstores/{blockstore}/volumes/{volume}/snapshots/{snapshot}/restore": s.doSnapshotRestore,
			"/blockstores/{blockstore}/images/add":                                    s.doBlockStoreAddImage,
			"/blockstores/{blockstore}/images/{image}/activate":                       s.doBlockStoreActivateImage,
			"/blockstores/{blockstore}/images/{image}/deactivate":                     s.doBlockStoreDeactivateImage,
		},
		"DELETE": {
			"/volumes/{volume}/":                                               s.doVolumeDelete,
			"/volumes/{volume}/snapshots/{snapshot}/":                          s.doSnapshotDelete,
			"/blockstores/{blockstore}/":                                       s.doBlockStoreDeregister,
			"/blockstores/{blockstore}/volumes/{volume}/":                      s.doBlockStoreRemoveVolume,
			"/blockstores/{blockstore}/volumes/{volume}/snapshots/{snapshot}/": s.doSnapshotRemove,
			"/blockstores/{blockstore}/images/{image}/":                        s.doBlockStoreRemoveImage,
		},
	}
	for method, routes := range m {
		for route, f := range routes {
			log.Debugf("Registering %s, %s", method, route)
			handler := makeHandlerFunc(method, route, API_VERSION, f)
			router.Path("/v{version:[0-9.]+}" + route).Methods(method).HandlerFunc(handler)
			router.Path(route).Methods(method).HandlerFunc(handler)
		}
	}
	return router
}

func writeResponseOutput(w http.ResponseWriter, v interface{}) error {
	output, err := api.ResponseOutput(v)
	if err != nil {
		return err
	}
	_, err = w.Write(output)
	return err
}

func (s *Server) doInfo(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	driver := s.StorageDriver
	data, err := driver.Info()
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	if err != nil {
		return err
	}
	return nil
}

func (s *Server) notImplemented(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error {
	info := fmt.Sprintf("not implmeneted: %v %v %v", r.Method, r.RequestURI, objs)
	return fmt.Errorf(info)
}

func startServer(c *cli.Context) error {
	var err error
	if err = serverEnvironmentSetup(c); err != nil {
		return err
	}
	defer environmentCleanup()

	root := c.String("root")
	var server *Server
	if !util.ConfigExists(root, getCfgName()) {
		server, err = initServer(c)
		if err != nil {
			return err
		}
	} else {
		server, err = loadServerConfig(c)
		if err != nil {
			return err
		}
	}
	defer server.cleanup()

	server.Router = createRouter(server)

	l, err := net.Listen("unix", sockFile)
	if err != nil {
		fmt.Println("listen err", err)
		return err
	}
	defer l.Close()

	sigs := make(chan os.Signal, 1)
	done := make(chan bool, 1)
	signal.Notify(sigs, os.Interrupt, os.Kill, syscall.SIGTERM)
	go func() {
		sig := <-sigs
		fmt.Printf("Caught signal %s: shutting down.\n", sig)
		done <- true
	}()

	go func() {
		err = http.Serve(l, server.Router)
		if err != nil {
			log.Error("http server error", err.Error())
		}
		done <- true
	}()

	<-done
	return nil
}

func initServer(c *cli.Context) (*Server, error) {
	root := c.String("root")
	driverName := c.String("driver")
	driverOpts := util.SliceToMap(c.StringSlice("driver-opts"))
	imagesDir := c.String("images-dir")
	if root == "" || driverName == "" || driverOpts == nil || imagesDir == "" {
		return nil, fmt.Errorf("Missing or invalid parameters")
	}

	log.Debug("Config root is ", root)

	if util.ConfigExists(root, getCfgName()) {
		return nil, fmt.Errorf("Configuration file already existed. Don't need to initialize.")
	}

	if err := util.MkdirIfNotExists(imagesDir); err != nil {
		return nil, err
	}
	log.Debug("Images would be stored at ", imagesDir)

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_PREPARE,
		LOG_FIELD_EVENT:  LOG_EVENT_INIT,
		LOG_FIELD_DRIVER: driverName,
		"root":           root,
		"driverOpts":     driverOpts,
	}).Debug()
	driver, err := drivers.GetDriver(driverName, root, driverOpts)
	if err != nil {
		return nil, err
	}

	log.WithFields(logrus.Fields{
		LOG_FIELD_REASON: LOG_REASON_COMPLETE,
		LOG_FIELD_EVENT:  LOG_EVENT_INIT,
		LOG_FIELD_DRIVER: driverName,
	}).Debug()

	config := Config{
		Root:      root,
		Driver:    driverName,
		ImagesDir: imagesDir,
	}
	server := &Server{
		Config:        config,
		StorageDriver: driver,
	}
	err = util.SaveConfig(root, getCfgName(), &config)
	return server, err
}

func loadGlobalConfig(c *cli.Context) (*Config, drivers.Driver, error) {
	config := Config{}
	root := c.String("root")
	if root == "" {
		return nil, nil, genRequiredMissingError("root")
	}
	err := util.LoadConfig(root, getCfgName(), &config)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to load config:", err.Error())
	}

	driver, err := drivers.GetDriver(config.Driver, config.Root, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("Failed to load driver:", err.Error())
	}
	return &config, driver, nil
}

func loadServerConfig(c *cli.Context) (*Server, error) {
	config := Config{}
	root := c.String("root")
	if root == "" {
		return nil, genRequiredMissingError("root")
	}
	err := util.LoadConfig(root, getCfgName(), &config)
	if err != nil {
		return nil, fmt.Errorf("Failed to load config:", err.Error())
	}

	driver, err := drivers.GetDriver(config.Driver, config.Root, nil)
	if err != nil {
		return nil, fmt.Errorf("Failed to load driver:", err.Error())
	}

	server := &Server{
		Config:        config,
		StorageDriver: driver,
	}
	return server, nil
}

func genRequiredMissingError(name string) error {
	return fmt.Errorf("Cannot find valid required parameter:", name)
}

func getLowerCaseFlag(c *cli.Context, name string, required bool, err error) (string, error) {
	if err != nil {
		return "", err
	}
	result := strings.ToLower(c.String(name))
	if required && result == "" {
		err = genRequiredMissingError(name)
	}
	return result, err
}

func getLowerCaseHTTPFlag(value, name string, required bool, err error) (string, error) {
	if err != nil {
		return "", err
	}
	result := strings.ToLower(value)
	if required && result == "" {
		err = genRequiredMissingError(name)
	}
	return result, err
}

func cmdInfo(c *cli.Context) {
	if err := doInfo(c); err != nil {
		panic(err)
	}
}

func doInfo(c *cli.Context) error {
	rc, _, err := client.call("GET", "/info", nil, nil)
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

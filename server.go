package main

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/gorilla/mux"
	"github.com/rancher/rancher-volume/api"
	"github.com/rancher/rancher-volume/drivers"
	"github.com/rancher/rancher-volume/util"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"

	. "github.com/rancher/rancher-volume/logging"
)

func createRouter(s *Server) *mux.Router {
	router := mux.NewRouter()
	m := map[string]map[string]RequestHandler{
		"GET": {
			"/info":                                                                     s.doInfo,
			"/volumes/":                                                                 s.doVolumeList,
			"/volumes/uuid":                                                             s.doVolumeRequestUUID,
			"/volumes/{" + KEY_VOLUME_UUID + "}/":                                       s.doVolumeList,
			"/volumes/{" + KEY_VOLUME_UUID + "}/snapshots/{" + KEY_SNAPSHOT_UUID + "}/": s.doVolumeList,
			"/objectstores/":                                                                                            s.doObjectStoreList,
			"/objectstores/{objectstore-uuid}/":                                                                         s.doObjectStoreList,
			"/objectstores/{objectstore-uuid}/volumes/{" + KEY_VOLUME_UUID + "}/":                                       s.doObjectStoreListVolume,
			"/objectstores/{objectstore-uuid}/volumes/{" + KEY_VOLUME_UUID + "}/snapshots/{" + KEY_SNAPSHOT_UUID + "}/": s.doObjectStoreListVolume,
		},
		"POST": {
			"/volumes/create":                                                                                                  s.doVolumeCreate,
			"/volumes/{" + KEY_VOLUME_UUID + "}/mount":                                                                         s.doVolumeMount,
			"/volumes/{" + KEY_VOLUME_UUID + "}/umount":                                                                        s.doVolumeUmount,
			"/volumes/{" + KEY_VOLUME_UUID + "}/snapshots/create":                                                              s.doSnapshotCreate,
			"/objectstores/register":                                                                                           s.doObjectStoreRegister,
			"/objectstores/{objectstore-uuid}/volumes/{" + KEY_VOLUME_UUID + "}/add":                                           s.doObjectStoreAddVolume,
			"/objectstores/{objectstore-uuid}/volumes/{" + KEY_VOLUME_UUID + "}/snapshots/{" + KEY_SNAPSHOT_UUID + "}/backup":  s.doSnapshotBackup,
			"/objectstores/{objectstore-uuid}/volumes/{" + KEY_VOLUME_UUID + "}/snapshots/{" + KEY_SNAPSHOT_UUID + "}/restore": s.doSnapshotRestore,
			"/objectstores/{objectstore-uuid}/images/add":                                                                      s.doObjectStoreAddImage,
			"/objectstores/{objectstore-uuid}/images/{image-uuid}/activate":                                                    s.doObjectStoreActivateImage,
			"/objectstores/{objectstore-uuid}/images/{image-uuid}/deactivate":                                                  s.doObjectStoreDeactivateImage,
		},
		"DELETE": {
			"/volumes/{" + KEY_VOLUME_UUID + "}/":                                                                       s.doVolumeDelete,
			"/snapshots/{" + KEY_SNAPSHOT_UUID + "}/":                                                                   s.doSnapshotDelete,
			"/objectstores/{objectstore-uuid}/":                                                                         s.doObjectStoreDeregister,
			"/objectstores/{objectstore-uuid}/volumes/{" + KEY_VOLUME_UUID + "}/":                                       s.doObjectStoreRemoveVolume,
			"/objectstores/{objectstore-uuid}/volumes/{" + KEY_VOLUME_UUID + "}/snapshots/{" + KEY_SNAPSHOT_UUID + "}/": s.doSnapshotRemove,
			"/objectstores/{objectstore-uuid}/images/{image-uuid}/":                                                     s.doObjectStoreRemoveImage,
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
	router.NotFoundHandler = s

	pluginMap := map[string]map[string]http.HandlerFunc{
		"POST": {
			"/Plugin.Activate":      s.dockerActivate,
			"/VolumeDriver.Create":  s.dockerCreateVolume,
			"/VolumeDriver.Remove":  s.dockerRemoveVolume,
			"/VolumeDriver.Mount":   s.dockerMountVolume,
			"/VolumeDriver.Unmount": s.dockerUnmountVolume,
			"/VolumeDriver.Path":    s.dockerVolumePath,
		},
	}
	for method, routes := range pluginMap {
		for route, f := range routes {
			log.Debugf("Registering plugin handler %s, %s", method, route)
			router.Path(route).Methods(method).HandlerFunc(f)
		}
	}
	return router
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info := fmt.Sprintf("Handler not found: %v %v", r.Method, r.RequestURI)
	log.Errorf(info)
	w.Write([]byte(info))
}

type RequestHandler func(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error

func makeHandlerFunc(method string, route string, version string, f RequestHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		log.Debugf("Calling: %v, %v, request: %v, %v", method, route, r.Method, r.RequestURI)

		if strings.Contains(r.Header.Get("User-Agent"), "Rancher-Volume-Client/") {
			userAgent := strings.Split(r.Header.Get("User-Agent"), "/")
			if len(userAgent) == 2 && userAgent[1] != version {
				http.Error(w, fmt.Errorf("client version %v doesn't match with server %v", userAgent[1], version).Error(), http.StatusNotFound)
				return
			}
		}
		if err := f(version, w, r, mux.Vars(r)); err != nil {
			log.Errorf("Handler for %s %s returned error: %s", method, route, err)
			http.Error(w, err.Error(), http.StatusBadRequest)
		}
	}
}

func loadServerConfig(c *cli.Context) (*Server, error) {
	config := Config{}
	root := c.String("root")
	if root == "" {
		return nil, genRequiredMissingError("root")
	}
	log.Debug("Ignore command line opts, loading server config from ", root)
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

func (s *Server) updateIndex() error {
	volumeUUIDs := util.ListConfigIDs(s.Root, VOLUME_CFG_PREFIX, CFG_POSTFIX)
	for _, uuid := range volumeUUIDs {
		volume := s.loadVolume(uuid)
		if volume == nil {
			return fmt.Errorf("Volume list changed for volume %v, something is wrong", uuid)
		}
		if volume.Name != "" {
			if err := util.AddToIndex(volume.Name, volume.UUID, s.NameVolumeMap); err != nil {
				return err
			}
		}
		for snapshotUUID := range volume.Snapshots {
			if err := util.AddToIndex(snapshotUUID, uuid, s.SnapshotVolumeIndex); err != nil {
				return err
			}
		}
	}
	log.Debugf("Current volume name list: %v", s.NameVolumeMap)
	log.Debugf("Current snapshot uuid list: %v", s.SnapshotVolumeIndex)

	return nil
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

func writeResponseOutput(w http.ResponseWriter, v interface{}) error {
	output, err := api.ResponseOutput(v)
	if err != nil {
		return err
	}
	log.Debugln("Response: ", string(output))
	_, err = w.Write(output)
	return err
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

func cmdStartServer(c *cli.Context) {
	if err := startServer(c); err != nil {
		panic(err)
	}
}

func (s *Server) CheckEnvironment() error {
	if err := drivers.CheckEnvironment(s.StorageDriver); err != nil {
		return err
	}
	return nil
}

func (s *Server) finishInitialization() {
	s.NameVolumeMap = make(map[string]string)
	s.SnapshotVolumeIndex = make(map[string]string)
	s.GlobalLock = &sync.RWMutex{}

	s.updateIndex()
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
	server.finishInitialization()
	defer server.cleanup()

	if err := server.CheckEnvironment(); err != nil {
		return err
	}

	server.Router = createRouter(server)

	if err := util.MkdirIfNotExists(filepath.Dir(sockFile)); err != nil {
		return err
	}

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
	mountsDir := c.String("mounts-dir")
	defaultSize := c.String("default-volume-size")
	if root == "" || driverName == "" || driverOpts == nil || imagesDir == "" || mountsDir == "" || defaultSize == "" {
		return nil, fmt.Errorf("Missing or invalid parameters")
	}

	size, err := util.ParseSize(defaultSize)
	if err != nil {
		return nil, err
	}

	log.Debug("Config root is ", root)

	if util.ConfigExists(root, getCfgName()) {
		return nil, fmt.Errorf("Configuration file already existed. Don't need to initialize.")
	}

	if err := util.MkdirIfNotExists(imagesDir); err != nil {
		return nil, err
	}
	log.Debug("Images would be stored at ", imagesDir)

	if err := util.MkdirIfNotExists(mountsDir); err != nil {
		return nil, err
	}
	log.Debug("Default mounting directory would be ", mountsDir)

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
		Root:              root,
		Driver:            driverName,
		ImagesDir:         imagesDir,
		MountsDir:         mountsDir,
		DefaultVolumeSize: size,
	}
	server := &Server{
		Config:        config,
		StorageDriver: driver,
	}
	err = util.SaveConfig(root, getCfgName(), &config)
	return server, err
}

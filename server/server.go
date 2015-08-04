package server

import (
	"fmt"
	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/docker/docker/pkg/truncindex"
	"github.com/gorilla/mux"
	"github.com/rancher/rancher-volume/api"
	"github.com/rancher/rancher-volume/storagedriver"
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

type Server struct {
	Router              *mux.Router
	StorageDrivers      map[string]storagedriver.StorageDriver
	GlobalLock          *sync.RWMutex
	NameUUIDIndex       *util.Index
	SnapshotVolumeIndex *util.Index
	UUIDIndex           *truncindex.TruncIndex
	Config
}

const (
	KEY_VOLUME_UUID   = "volume-uuid"
	KEY_SNAPSHOT_UUID = "snapshot-uuid"

	VOLUME_CFG_PREFIX = "volume_"
	CFG_POSTFIX       = ".json"

	CONFIGFILE = "rancher-volume.cfg"
	LOCKFILE   = "lock"
)

var (
	lock    string
	logFile *os.File

	log = logrus.WithFields(logrus.Fields{"pkg": "server"})
)

type Config struct {
	Root          string
	DriverList    []string
	DefaultDriver string
}

func (c *Config) ConfigFile() (string, error) {
	if c.Root == "" {
		return "", fmt.Errorf("BUG: Invalid empty server config path")
	}
	return filepath.Join(c.Root, CONFIGFILE), nil
}

func createRouter(s *Server) *mux.Router {
	router := mux.NewRouter()
	m := map[string]map[string]RequestHandler{
		"GET": {
			"/info":            s.doInfo,
			"/uuid":            s.doRequestUUID,
			"/volumes/list":    s.doVolumeList,
			"/volumes/":        s.doVolumeInspect,
			"/snapshots/":      s.doSnapshotInspect,
			"/backups/list":    s.doBackupList,
			"/backups/inspect": s.doBackupInspect,
		},
		"POST": {
			"/volumes/create":   s.doVolumeCreate,
			"/volumes/mount":    s.doVolumeMount,
			"/volumes/umount":   s.doVolumeUmount,
			"/snapshots/create": s.doSnapshotCreate,
			"/backups/create":   s.doBackupCreate,
		},
		"DELETE": {
			"/volumes/":   s.doVolumeDelete,
			"/snapshots/": s.doSnapshotDelete,
			"/backups":    s.doBackupDelete,
		},
	}
	for method, routes := range m {
		for route, f := range routes {
			log.Debugf("Registering %s, %s", method, route)
			handler := makeHandlerFunc(method, route, api.API_VERSION, f)
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

func (s *Server) updateIndex() error {
	volumeUUIDs, err := util.ListConfigIDs(s.Root, VOLUME_CFG_PREFIX, CFG_POSTFIX)
	if err != nil {
		return err
	}
	for _, uuid := range volumeUUIDs {
		volume := s.loadVolume(uuid)
		if err := s.UUIDIndex.Add(uuid); err != nil {
			return err
		}
		if volume == nil {
			return fmt.Errorf("Volume list changed for volume %v, something is wrong", uuid)
		}
		if volume.Name != "" {
			if err := s.NameUUIDIndex.Add(volume.Name, volume.UUID); err != nil {
				return err
			}
		}
		for snapshotUUID, snapshot := range volume.Snapshots {
			if err := s.UUIDIndex.Add(snapshotUUID); err != nil {
				return err
			}
			if err := s.SnapshotVolumeIndex.Add(snapshotUUID, uuid); err != nil {
				return err
			}
			if snapshot.Name != "" {
				if err := s.NameUUIDIndex.Add(snapshot.Name, snapshot.UUID); err != nil {
					return err
				}
			}
		}
	}

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

func (s *Server) finializeInitialization() error {
	s.NameUUIDIndex = util.NewIndex()
	s.SnapshotVolumeIndex = util.NewIndex()
	s.UUIDIndex = truncindex.NewTruncIndex([]string{})
	s.GlobalLock = &sync.RWMutex{}

	s.updateIndex()
	return nil
}

func (s *Server) initDrivers(driverOpts map[string]string) error {
	for _, driverName := range s.DriverList {
		log.WithFields(logrus.Fields{
			LOG_FIELD_REASON: LOG_REASON_PREPARE,
			LOG_FIELD_EVENT:  LOG_EVENT_INIT,
			LOG_FIELD_DRIVER: driverName,
			"root":           s.Root,
			"driver_opts":    driverOpts,
		}).Debug()

		driver, err := storagedriver.GetDriver(driverName, s.Root, driverOpts)
		if err != nil {
			return err
		}

		log.WithFields(logrus.Fields{
			LOG_FIELD_REASON: LOG_REASON_COMPLETE,
			LOG_FIELD_EVENT:  LOG_EVENT_INIT,
			LOG_FIELD_DRIVER: driverName,
		}).Debug()
		s.StorageDrivers[driverName] = driver
	}
	return nil
}

func Start(sockFile string, c *cli.Context) error {
	var err error

	if err = serverEnvironmentSetup(c); err != nil {
		return err
	}
	defer environmentCleanup()

	root := c.String("root")
	server := &Server{
		StorageDrivers: make(map[string]storagedriver.StorageDriver),
	}
	config := &Config{
		Root: root,
	}
	exists, err := util.ObjectExists(config)
	if err != nil {
		return err
	}
	driverOpts := util.SliceToMap(c.StringSlice("driver-opts"))
	if exists {
		log.Debug("Found existing config. Ignoring command line opts, loading config from ", root)
		if err := util.ObjectLoad(config); err != nil {
			return err
		}
	} else {
		driverList := c.StringSlice("drivers")
		if len(driverList) == 0 || driverOpts == nil {
			return fmt.Errorf("Missing or invalid parameters")
		}
		log.Debug("Creating config at ", root)

		config.DriverList = driverList
		config.DefaultDriver = driverList[0]
	}
	server.Config = *config

	if err := server.initDrivers(driverOpts); err != nil {
		return err
	}
	if err := server.finializeInitialization(); err != nil {
		return err
	}
	if err := util.ObjectSave(config); err != nil {
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

func (s *Server) getDriver(driverName string) (storagedriver.StorageDriver, error) {
	driver, exists := s.StorageDrivers[driverName]
	if !exists {
		return nil, fmt.Errorf("Cannot find driver %s", driverName)
	}
	return driver, nil
}

func (s *Server) getVolumeOpsForVolume(volume *Volume) (storagedriver.VolumeOperations, error) {
	driver, err := s.getDriver(volume.DriverName)
	if err != nil {
		return nil, err
	}
	return driver.VolumeOps()
}

func (s *Server) getSnapshotOpsForVolume(volume *Volume) (storagedriver.SnapshotOperations, error) {
	driver, err := s.getDriver(volume.DriverName)
	if err != nil {
		return nil, err
	}
	return driver.SnapshotOps()
}

func (s *Server) getBackupOpsForVolume(volume *Volume) (storagedriver.BackupOperations, error) {
	driver, err := s.getDriver(volume.DriverName)
	if err != nil {
		return nil, err
	}
	return driver.BackupOps()
}

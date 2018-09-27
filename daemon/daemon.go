package daemon

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/gorilla/mux"
	"github.com/rancher/convoy/api"
	"github.com/rancher/convoy/util"

	. "github.com/rancher/convoy/convoydriver"
	. "github.com/rancher/convoy/logging"
)

type daemon struct {
	Router        *mux.Router
	ConvoyDrivers map[string]ConvoyDriver

	NameUUIDIndex       *util.Index
	SnapshotVolumeIndex *util.Index
	daemonConfig
}

const (
	VOLUME_CFG_PREFIX = "volume_"
	CFG_POSTFIX       = ".json"

	CONFIGFILE = "convoy.cfg"
	LOCKFILE   = "lock"
)

var (
	lockFile *os.File
	logFile  *os.File

	log = logrus.WithFields(logrus.Fields{"pkg": "daemon"})
)

type daemonConfig struct {
	Root                string
	DriverList          []string
	DefaultDriver       string
	MountNamespaceFD    string
	IgnoreDockerDelete  bool
	CreateOnDockerMount bool
	CmdTimeout          string
}

func (c *daemonConfig) ConfigFile() (string, error) {
	if c.Root == "" {
		return "", fmt.Errorf("BUG: Invalid empty daemon config path")
	}
	return filepath.Join(c.Root, CONFIGFILE), nil
}

func createRouter(s *daemon) *mux.Router {
	router := mux.NewRouter()
	m := map[string]map[string]requestHandler{
		"GET": {
			"/info":            s.doInfo,
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
			"/Plugin.Activate":           s.dockerActivate,
			"/VolumeDriver.Create":       s.dockerCreateVolume,
			"/VolumeDriver.Remove":       s.dockerRemoveVolume,
			"/VolumeDriver.Mount":        s.dockerMountVolume,
			"/VolumeDriver.Unmount":      s.dockerUnmountVolume,
			"/VolumeDriver.Path":         s.dockerVolumePath,
			"/VolumeDriver.Get":          s.dockerGetVolume,
			"/VolumeDriver.List":         s.dockerListVolume,
			"/VolumeDriver.Capabilities": s.dockerCapabilities,
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

func (s *daemon) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	info := fmt.Sprintf("Handler not found: %v %v", r.Method, r.RequestURI)
	log.Errorf(info)
	w.WriteHeader(http.StatusNotFound)
	w.Write([]byte(info))
}

type requestHandler func(version string, w http.ResponseWriter, r *http.Request, objs map[string]string) error

func makeHandlerFunc(method string, route string, version string, f requestHandler) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Don't record volume list API call since it may used for polling
		if route != "/volumes/list" {
			log.Debugf("Calling: %v, %v, request: %v, %v", method, route, r.Method, r.RequestURI)
		}

		if strings.Contains(r.Header.Get("User-Agent"), "Convoy-Client/") {
			userAgent := strings.Split(r.Header.Get("User-Agent"), "/")
			if len(userAgent) == 2 && userAgent[1] != version {
				http.Error(w, fmt.Errorf("client version %v doesn't match with server %v", userAgent[1], version).Error(), http.StatusNotFound)
				return
			}
		}
		if err := f(version, w, r, mux.Vars(r)); err != nil {
			statusCode := checkForStatusCode(err)
			if statusCode == 0 {
				log.Errorf("Handler for %s %s returned error: %s", method, route, err)
				statusCode = http.StatusBadRequest
			}
			http.Error(w, err.Error(), statusCode)
		}
	}
}

func (s *daemon) updateIndex() error {
	volumes := s.getVolumeList()
	for name := range volumes {
		if err := s.NameUUIDIndex.Add(name, "exists"); err != nil {
			return err
		}
		snapshots, err := s.listSnapshotDriverInfos(s.getVolume(name))
		if err == nil {
			for snapshotID := range snapshots {
				if err := s.SnapshotVolumeIndex.Add(snapshotID, name); err != nil {
					return err
				}
				if err := s.NameUUIDIndex.Add(snapshotID, "exists"); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func daemonEnvironmentSetup(c *cli.Context) error {
	var err error

	root := c.String("root")
	if root == "" {
		return fmt.Errorf("Have to specific root directory")
	}
	if err := util.MkdirIfNotExists(root); err != nil {
		return fmt.Errorf("Invalid root directory:", err)
	}

	lockPath := filepath.Join(root, LOCKFILE)
	if lockFile, err = util.LockFile(lockPath); err != nil {
		return fmt.Errorf("Failed to lock the file at %v: %v", lockPath, err.Error())
	}

	logrus.SetLevel(logrus.DebugLevel)
	logName := c.String("log")
	if logName != "" {
		logFile, err := os.OpenFile(logName, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			return err
		}
		logrus.SetFormatter(&logrus.JSONFormatter{})
		logrus.SetOutput(logFile)
	} else {
		logrus.SetOutput(os.Stdout)
	}

	return nil
}

func environmentCleanup() {
	log.Debug("Cleaning up environment...")
	if lockFile != nil {
		util.UnlockFile(lockFile)
	}
	if logFile != nil {
		logFile.Close()
	}
	if r := recover(); r != nil {
		api.ResponseLogAndError(r)
		os.Exit(1)
	}
}

func (s *daemon) finializeInitialization() error {
	s.NameUUIDIndex = util.NewIndex()
	s.SnapshotVolumeIndex = util.NewIndex()

	s.updateIndex()
	return nil
}

func (s *daemon) initDrivers(driverOpts map[string]string) error {
	for _, driverName := range s.DriverList {
		log.WithFields(logrus.Fields{
			LOG_FIELD_REASON: LOG_REASON_PREPARE,
			LOG_FIELD_EVENT:  LOG_EVENT_INIT,
			LOG_FIELD_DRIVER: driverName,
			"root":           s.Root,
			"driver_opts":    driverOpts,
		}).Debug()

		driver, err := GetDriver(driverName, s.Root, driverOpts)
		if err != nil {
			return err
		}

		log.WithFields(logrus.Fields{
			LOG_FIELD_REASON: LOG_REASON_COMPLETE,
			LOG_FIELD_EVENT:  LOG_EVENT_INIT,
			LOG_FIELD_DRIVER: driverName,
		}).Debug()
		s.ConvoyDrivers[driverName] = driver
	}
	return nil
}

// Start the daemon
func Start(sockFile string, c *cli.Context) error {
	var err error

	if err = daemonEnvironmentSetup(c); err != nil {
		return err
	}
	defer environmentCleanup()

	root := c.String("root")
	s := &daemon{
		ConvoyDrivers: make(map[string]ConvoyDriver),
	}
	config := &daemonConfig{
		Root: root,
	}

	ignoreCfgFile := c.Bool("ignore-config-file")
	exists := false
	if !ignoreCfgFile {
		exists, err = util.ObjectExists(config)
		if err != nil {
			return err
		}
	}

	if exists {
		log.Debug("Found existing config. Ignoring command line opts, loading config from ", root)
		if err := util.ObjectLoad(config); err != nil {
			return err
		}
	} else {
		fd := c.String("mnt-ns")
		if fd != "" {
			if _, err := os.Stat(fd); err != nil {
				return fmt.Errorf("Cannot find mount namespace fd %v", fd)
			}
			config.MountNamespaceFD = fd
		}

		driverList := c.StringSlice("drivers")
		if len(driverList) == 0 {
			return fmt.Errorf("Missing or invalid parameters")
		}
		log.Debug("Creating config at ", root)

		config.DriverList = driverList
		config.DefaultDriver = driverList[0]
		config.IgnoreDockerDelete = c.Bool("ignore-docker-delete")
		config.CreateOnDockerMount = c.Bool("create-on-docker-mount")
		config.CmdTimeout = c.String("cmd-timeout")
	}

	s.daemonConfig = *config

	if err := util.InitMountNamespace(s.MountNamespaceFD); err != nil {
		return err
	}

	util.InitTimeout(config.CmdTimeout)

	// driverOpts would be ignored by Convoy Drivers if config already exists
	driverOpts := util.SliceToMap(c.StringSlice("driver-opts"))
	if err := s.initDrivers(driverOpts); err != nil {
		return err
	}
	if err := s.finializeInitialization(); err != nil {
		return err
	}
	if err := util.ObjectSave(config); err != nil {
		return err
	}

	s.Router = createRouter(s)

	if err := util.MkdirIfNotExists(filepath.Dir(sockFile)); err != nil {
		return err
	}
	// This should be safe because lock file prevent starting daemon twice
	if _, err := os.Stat(sockFile); err == nil {
		log.Warnf("Remove previous sockfile at %v", sockFile)
		if err := os.Remove(sockFile); err != nil {
			return err
		}
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
		err = http.Serve(l, s.Router)
		if err != nil {
			log.Error("http server error", err.Error())
		}
		done <- true
	}()

	<-done
	return nil
}

func (s *daemon) getDriver(driverName string) (ConvoyDriver, error) {
	driver, exists := s.ConvoyDrivers[driverName]
	if !exists {
		return nil, fmt.Errorf("Cannot find driver %s", driverName)
	}
	return driver, nil
}

func (s *daemon) getVolumeOpsForVolume(volume *Volume) (VolumeOperations, error) {
	driver, err := s.getDriver(volume.DriverName)
	if err != nil {
		return nil, err
	}
	return driver.VolumeOps()
}

func (s *daemon) getSnapshotOpsForVolume(volume *Volume) (SnapshotOperations, error) {
	driver, err := s.getDriver(volume.DriverName)
	if err != nil {
		return nil, err
	}
	return driver.SnapshotOps()
}

func (s *daemon) getBackupOpsForVolume(volume *Volume) (BackupOperations, error) {
	driver, err := s.getDriver(volume.DriverName)
	if err != nil {
		return nil, err
	}
	return driver.BackupOps()
}

type APIError struct {
	error      string
	statusCode int
}

func (e APIError) Error() string {
	return e.error
}

func checkForStatusCode(err error) int {
	if apiError, ok := err.(APIError); ok {
		return apiError.statusCode
	}
	return 0
}

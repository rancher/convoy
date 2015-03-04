package main

import (
	"flag"
	"fmt"
	log "github.com/Sirupsen/logrus"
	"github.com/alecthomas/kingpin"
	"github.com/yasker/volmgr/drivers"
	"github.com/yasker/volmgr/utils"
	"os"
	"path/filepath"
)

var (
	flagApp   = kingpin.New("volmgr", "A volume manager capable of snapshot and delta backup")
	flagDebug = flagApp.Flag("debug", "Enable debug mode.").Default("true").Bool()

	flagInitialize           = flagApp.Command("init", "initialize volmgr")
	flagInitializeDriver     = flagInitialize.Flag("driver", "Driver for volume manager, only support \"devicemapper\" currently").Default("devicemapper").String()
	flagInitializeDriverOpts = flagInitialize.Flag("driver-opts", "options for driver").StringMap()

	flagVolume           = flagApp.Command("volume", "volume related operations")
	flagVolumeCreate     = flagVolume.Command("create", "create a new volume")
	flagVolumeCreateSize = flagVolumeCreate.Flag("size", "size of volume").Required().Uint64()
	flagVolumeDelete     = flagVolume.Command("delete", "delete a volume with all of it's snapshots")
	flagVolumeDeleteUUID = flagVolumeDelete.Arg("uuid", "uuid of volume").Required().String()
	flagVolumeUpdate     = flagVolume.Command("update", "update info about volume")
	flagVolumeUpdateUUID = flagVolumeUpdate.Arg("uuid", "uuid of volume").Required().String()
	flagVolumeUpdateSize = flagVolumeUpdate.Flag("size", "size of volume").Required().Uint64()
	flagVolumeList       = flagVolume.Command("list", "list all managed volumes")

	flagInfo = flagApp.Command("info", "information about volmgr")
)

const (
	LOCKFILE   = "lock"
	CONFIGFILE = "volmgr.cfg"
	ROOTDIR    = "/var/lib/volmgr/"
)

type Config struct {
	Root   string
	Driver string
}

type Manager struct {
	root   string
	driver drivers.Driver
}

func main() {
	log.SetOutput(os.Stderr)

	if len(os.Args) == 1 {
		fmt.Println("Use --help to see command list")
		os.Exit(-1)
	}

	command := kingpin.MustParse(flagApp.Parse(os.Args[1:]))
	if *flagDebug {
		log.SetLevel(log.DebugLevel)
	} else {
		log.SetLevel(log.InfoLevel)
	}
	configFile := filepath.Join(ROOTDIR, CONFIGFILE)

	if command == flagInitialize.FullCommand() {
		if _, err := os.Stat(configFile); err == nil {
			log.Errorf("Configuration file %v existed. Don't need to initialize.", configFile)
			os.Exit(-1)
		}

		err := doInitialize(ROOTDIR, *flagInitializeDriver, *flagInitializeDriverOpts)
		if err != nil {
			log.Errorln("Failed to initialize volmgr.", err)
			os.Exit(-1)
		}
		os.Exit(0)
	}

	config := Config{}
	err := utils.LoadConfig(configFile, &config)
	if err != nil {
		log.Errorln("Failed to load config.", err)
		os.Exit(-1)
	}

	switch command {
	case flagVolumeCreate.FullCommand():
		doVolumeCreate(&config, *flagVolumeCreateSize)
	case flagVolumeDelete.FullCommand():
		doVolumeDelete(&config, *flagVolumeDeleteUUID)
	case flagVolumeUpdate.FullCommand():
		doVolumeUpdate(&config, *flagVolumeUpdateUUID, *flagVolumeUpdateSize)
	case flagVolumeList.FullCommand():
		doVolumeList(&config)
	case flagInfo.FullCommand():
		err = doInfo(&config)
		if err != nil {
			log.Errorln("Failed to load complete info.", err)
			os.Exit(-1)
		}
	default:
		log.Errorln("Unrecognized command")
		os.Exit(-1)
	}
}

func oldMain() {
	exportArg := flag.Bool("export", false, "Export blocks according to metadata")
	dataArg := flag.String("d", "", "Data device location")
	metaArg := flag.String("m", "", "Metadata device location")
	blockDirArg := flag.String("b", "", "Blocks directory")

	restoreArg := flag.Bool("restore", false, "Restore blocks according to metadata")
	devArg := flag.Int("dev", -1, "Device ID(int) for restore")
	outputFileArg := flag.String("o", "", "Output volume filename")
	volumeSizeArg := flag.Int64("s", -1, "Size of output volume")

	flag.Parse()

	if *exportArg == *restoreArg {
		fmt.Println("Must specify either -export or -restore!")
		os.Exit(1)
	}

	if *exportArg && (*dataArg == "" || *metaArg == "" || *blockDirArg == "") {
		fmt.Println("Not enough parameters for export!")
		os.Exit(1)
	}

	if *restoreArg && (*metaArg == "" || *blockDirArg == "" || *devArg == -1 || *outputFileArg == "" || *volumeSizeArg == -1) {
		fmt.Println("Not enough parameters for restore!")
		os.Exit(1)
	}

	if *exportArg {
		if st, err := os.Stat(*dataArg); os.IsNotExist(err) || st.IsDir() {
			fmt.Println("Data device doesn't existed!")
			os.Exit(1)
		}

		if st, err := os.Stat(*metaArg); os.IsNotExist(err) || st.IsDir() {
			fmt.Println("Meta device doesn't existed!")
			os.Exit(1)
		}

		if st, err := os.Stat(*blockDirArg); os.IsNotExist(err) || !st.IsDir() {
			fmt.Println("Blocks directory doesn't existed!")
			os.Exit(1)
		}

		err := processExport(*dataArg, *metaArg, *blockDirArg)
		if err != nil {
			os.Exit(1)
		}
	} else if *restoreArg {
		if st, err := os.Stat(*metaArg); os.IsNotExist(err) || st.IsDir() {
			fmt.Println("Meta device doesn't existed!")
			os.Exit(1)
		}

		if st, err := os.Stat(*blockDirArg); os.IsNotExist(err) || !st.IsDir() {
			fmt.Println("Blocks directory doesn't existed!")
			os.Exit(1)
		}
		err := processRestore(*metaArg, *blockDirArg, *outputFileArg, uint32(*devArg), uint64(*volumeSizeArg))
		if err != nil {
			os.Exit(1)
		}
	}
}

func processExport(dataFileName, metaFileName, outputDirName string) error {
	metadata, err := getMetadataFromFile(metaFileName)
	if err != nil {
		fmt.Println("Failed to get meta data from file", metaFileName)
		return err
	}

	err = export(metadata, dataFileName, outputDirName)
	return err
}

func processRestore(metaFileName, blockDirName, outputFileName string, devId uint32, volumeSize uint64) error {
	metadata, err := getMetadataFromFile(metaFileName)
	if err != nil {
		fmt.Println("Failed to get meta data from file", metaFileName)
		return err
	}

	err = restore(metadata, blockDirName, outputFileName, devId, volumeSize)
	return err
}

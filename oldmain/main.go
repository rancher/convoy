package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
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

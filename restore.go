package main

import (
	"errors"
	"fmt"
	"os"
)

func fillBlocks(f *os.File, blockSize uint32, blockCount uint64, origin *os.File) error {
	blockLen := blockSize * SECTOR_SIZE
	content := make([]byte, blockLen)
	if origin != nil {
		l, err := origin.Read(content)
		if err != nil || uint32(l) != blockLen {
			return err
		}
	}
	for i := uint64(0); i < blockCount; i++ {
		_, err := f.Write(content)
		if err != nil {
			return err
		}
	}
	return nil
}

func restore(metadata Metadata, blockDirName, outputFileName string, devId uint32, volumeSize uint64) error {
	blockSize := metadata.DataBlockSize
	if volumeSize%(uint64(blockSize)*SECTOR_SIZE) != 0 {
		return errors.New("Volume size is not aligned with block size")
	}
	dev := metadata.Devices[devId]
	if dev == nil {
		return errors.New("cannot find the specific device ID")
	}
	outputFile, err := os.Create(outputFileName)
	if err != nil {
		fmt.Println("Failed to create the output volume file", outputFileName)
		return err
	}
	defer outputFile.Close()
	blockNr := volumeSize / (uint64(blockSize) * SECTOR_SIZE)
	fmt.Println("block nr", blockNr)
	currentBlock := uint64(0)
	for _, mapping := range dev.Mappings {
		fmt.Println("current block", currentBlock)
		if mapping.OriginBegin > currentBlock {
			err = fillBlocks(outputFile, blockSize, mapping.OriginBegin-currentBlock, nil)
			if err != nil {
				return err
			}
			currentBlock = mapping.OriginBegin
		}
		dataBlock := mapping.DataBegin
		for k := uint64(0); k < mapping.Length; k++ {
			blockFileName, err := getBlockFileName(blockDirName, dataBlock+k, mapping.Time)
			if err != nil {
				return err
			}
			blockFile, err := os.Open(blockFileName)
			if err != nil {
				return err
			}
			err = fillBlocks(outputFile, blockSize, 1, blockFile)
			blockFile.Close()
			if err != nil {
				return err
			}
			currentBlock++
		}
	}
	fmt.Println("current block", currentBlock)
	if currentBlock < blockNr-1 {
		err = fillBlocks(outputFile, blockSize, blockNr-1-currentBlock, nil)
		if err != nil {
			return err
		}
	}
	return nil
}

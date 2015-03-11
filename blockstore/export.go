package main

import (
	//"crypto/sha1"
	//"encoding/hex"
	"fmt"
	"os"
	//"path/filepath"
	"strconv"
)

const SECTOR_SIZE = 512

func export(metaData Metadata, dataFileName, outputDirname string) error {
	blockSize := int(metaData.DataBlockSize * SECTOR_SIZE) //in byte
	blockNr := int(metaData.NrDataBlock)

	dataFile, err := os.Open(dataFileName)
	if err != nil {
		fmt.Println("Failed to open data device", dataFileName)
		return err
	}
	defer dataFile.Close()

	for _, dev := range metaData.Devices {
		blockCount := dev.MappedBlocks
		for _, mapping := range dev.Mappings {
			pos := uint64(blockSize) * mapping.DataBegin
			_, err := dataFile.Seek(int64(pos), 0)
			if err != nil {
				fmt.Println("Failed to seek data device", dataFileName)
				return err
			}
			for i := uint64(0); i < mapping.Length; i++ {
				outputFileNameBase := outputDirname + "/" +
					getBlockFileNamePrefix(mapping.DataBegin+i, mapping.Time)
				outputTmpFileName := outputFileNameBase + "tmp.blk"
				outputFile, err := os.Create(outputTmpFileName)
				if err != nil {
					fmt.Println("Failed to output device", outputTmpFileName)
					return err

				}

				block := make([]byte, blockSize, blockSize)
				n, err := dataFile.Read(block)
				if err != nil || n != blockSize {
					fmt.Printf("Failed to read data device %v at location %v, err %v",
						dataFileName, uint64(blockSize)*(mapping.DataBegin+i), err)
					outputFile.Close()
					return err

				}
				//checksum := sha1.Sum(block)
				_, err = outputFile.Write(block)
				if err != nil {
					fmt.Printf("Failed to write data block %v, err %v",
						uint64(blockSize)*(mapping.DataBegin+i), err)
					outputFile.Close()
					return err

				}
				// I don't want to use defer because I cannot wait for
				// function exit to close all these file.
				outputFile.Close()
				//os.Rename(outputTmpFileName, outputFileNameBase+hex.EncodeToString(checksum[:])+".blk")

				blockCount--
			}
		}
		if blockCount != 0 {
			fmt.Printf("Wrong block count! Remaining %v\n", blockNr)
		}
	}
	return nil
}

func getBlockFileNamePrefix(dataBlock uint64, time uint32) string {
	return strconv.FormatUint(dataBlock, 10) + "_" + strconv.FormatUint(uint64(time), 10) + "-"
}

func getBlockFileName(blockDirName string, dataBlock uint64, time uint32) (string, error) {
	name := blockDirName + "/" + getBlockFileNamePrefix(dataBlock, time) + "tmp.blk"
	_, err := os.Stat(name)
	if os.IsExist(err) {
		return "", err
	}
	return name, nil

}

package main

import (
	"encoding/xml"
	//"math/rand"
	"os"
	"testing"
)

const dataFileName = "/tmp/data_volume"
const metaFileName = "/tmp/meta_volume"
const blockDirName = "/tmp/output/"
const volumeOutputName = "/tmp/restore_volume"

// Total size of test file is 2G
const sectorSize = 512
const blockSize = 1024

const blockNr = 1024

//const blockNr = 128

const deviceBase = 10
const deviceMax = 10
const mappingBase = 10
const mappingMax = 100

const ascii_start = 32
const ascii_array_len = 95

/*
	Here we would try to generate pesudo data and meta volume, and see if exported blocks are what we expected. We would generate data in a certain way, so we can easily map the contain with the location. The generator would loop ASCII 32 ~ 126 in data file. So for any location x, we would know the character should be ASCII 32 + x mod 95, which would be easy for us to verify the result
*/
func generateDataFile(t *testing.T, dataFileName string, size int64) {
	file, err := os.Create(dataFileName)
	if err != nil {
		t.Fatal("Fail to create tmp data file")
	}
	defer file.Close()

	content := make([]byte, ascii_array_len, ascii_array_len)
	for i := byte(0); i < ascii_array_len; i++ {
		content[i] = i + ascii_start
	}
	for i := int64(0); i < size; i += ascii_array_len {
		_, err = file.Write(content)
		if err != nil {
			t.Fatal("Fail to complete creating tmp data file")
		}
	}
}

func generateMetaFile(t *testing.T, metaFileName string, metaDataXML SuperblockXML) {
	file, err := os.Create(metaFileName)
	if err != nil {
		t.Fatal("Fail to create metadata file")
	}
	defer file.Close()

	output, err := xml.MarshalIndent(metaDataXML, " ", "	")
	if err != nil {
		t.Fatal("Fail to generate XML!")
	}

	_, err = file.Write(output)
	if err != nil {
		t.Fatal("Fail to write to metadata XML!")
	}
}

func generateMetaData(t *testing.T, blockSize, blockNr int) SuperblockXML {
	/*
		metadata := SuperblockXML{}
		metadata.UUID = "Test-superblock-uuid"
		metadata.DataBlockSize = blockSize
		metadata.NrDataBlock = blockNr

		deviceNr := deviceBase + rand.Intn(deviceMax)
		for i := 0; i < deviceNr; i++ {
			dev = DeviceXML{}
			dev.DevID = i
			dev.Transaction = 0
			dev.CreationTime = i
			dev.SnapTime = 0

			currentBlock = 0
			mappingNr = mappingBase + rand.Intn(mappingMax)
			lastIsRange := false
			for j := 0; j < mappingNr; j++ {
				isRange := true
				if lastIsRange {
					isRange = false
				} else {
					rangeDice := rand.Intn(2)
					if rangeDice == 0 {
						isRange = false
					}
				}
			}
			dev.MappingBlocks = blockCount
		}
	*/
	data := `
		<superblock uuid="uuid-superblock" time="2" transaction="1" data_block_size="1024" nr_data_blocks="1024">
		<device dev_id="100" mapped_blocks="102" transaction="0" creation_time="0" snap_time="2">
		<single_mapping origin_block="0" data_block="1" time="2"/>
		<range_mapping origin_begin="1" data_begin="71" length="15" time="0"/>
		<single_mapping origin_block="16" data_block="0" time="1"/>
		<range_mapping origin_begin="17" data_begin="108" length="4" time="1"/>
		<single_mapping origin_block="21" data_block="86" time="1"/>
		<range_mapping origin_begin="22" data_begin="112" length="3" time="1"/>
		<single_mapping origin_block="25" data_block="4" time="1"/>
		<range_mapping origin_begin="26" data_begin="116" length="7" time="1"/>
		<single_mapping origin_block="256" data_block="2" time="0"/>
		<single_mapping origin_block="768" data_block="3" time="0"/>
		<single_mapping origin_block="1024" data_block="123" time="2"/>
		<range_mapping origin_begin="1025" data_begin="5" length="63" time="0"/>
		<single_mapping origin_block="1280" data_block="68" time="0"/>
		<single_mapping origin_block="1792" data_block="69" time="0"/>
		<single_mapping origin_block="2047" data_block="70" time="0"/>
		</device>
		<device dev_id="101" mapped_blocks="102" transaction="1" creation_time="1" snap_time="1">
		<single_mapping origin_block="0" data_block="87" time="1"/>
		<range_mapping origin_begin="1" data_begin="71" length="15" time="0"/>
		<range_mapping origin_begin="16" data_begin="88" length="9" time="1"/>
		<range_mapping origin_begin="25" data_begin="98" length="8" time="1"/>
		<single_mapping origin_block="256" data_block="106" time="1"/>
		<single_mapping origin_block="768" data_block="3" time="0"/>
		<single_mapping origin_block="1024" data_block="97" time="1"/>
		<range_mapping origin_begin="1025" data_begin="5" length="63" time="0"/>
		<single_mapping origin_block="1280" data_block="68" time="0"/>
		<single_mapping origin_block="1792" data_block="69" time="0"/>
		<single_mapping origin_block="2047" data_block="70" time="0"/>
		</device>
		</superblock>
		`
	result := SuperblockXML{}

	err := xml.Unmarshal([]byte(data), &result)

	if err != nil {
		t.Fatal("Fail to unmarshal metadata")
	}
	return result
}

func verifyProcessOutput(t *testing.T, metaData Metadata, blockDirName string) {
	blockSize := metaData.DataBlockSize
	for _, dev := range metaData.Devices {
		for _, mapping := range dev.Mappings {
			dataBlock := mapping.DataBegin
			for i := uint64(0); i < mapping.Length; i++ {
				blockPos := dataBlock + i
				location := uint64(blockSize) * blockPos * SECTOR_SIZE
				expectedByte := byte(location%ascii_array_len + ascii_start)
				blockFileName, err := getBlockFileName(blockDirName, blockPos, mapping.Time)
				if err != nil {
					t.Fatal("Failed to find the block file for block/time", blockPos, mapping.Time)
				}
				file, err := os.Open(blockFileName)
				if err != nil {
					t.Fatal("Failed to open the block file", blockFileName, err)
				}
				defer file.Close()
				valueBytes := make([]byte, 1, 1)
				l, err := file.Read(valueBytes)
				if err != nil || l != 1 {
					t.Fatal("Failed to read the block file")
				}
				if expectedByte != valueBytes[0] {
					t.Fatal("Expected byte doesn't match the value in the file", blockFileName, expectedByte, valueBytes[0])
				}
			}
		}
	}
}

func testExport(t *testing.T, metadata Metadata) {
	processExport(dataFileName, metaFileName, blockDirName)

	verifyProcessOutput(t, metadata, blockDirName)
}

func testRestore(t *testing.T, metadata Metadata) {
	restore(metadata, blockDirName, volumeOutputName, 100, 1073741824)
}

func TestAll(t *testing.T) {
	generateDataFile(t, dataFileName, sectorSize*blockSize*blockNr)
	metaDataXML := generateMetaData(t, blockSize, blockNr)
	generateMetaFile(t, metaFileName, metaDataXML)

	err := os.MkdirAll(blockDirName, os.ModeDir|os.ModePerm)
	if err != nil {
		t.Fatal("Fail to create tmp directory for output")
	}

	metadata, err := getMetadataFromFile(metaFileName)
	if err != nil {
		t.Fatal("Failed to parse the metadata file")
	}

	testExport(t, metadata)
	testRestore(t, metadata)
}

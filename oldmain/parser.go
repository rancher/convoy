package main

import (
	"encoding/xml"
	"fmt"
	"math"
	"os"
)

type SingleMappingXML struct {
	OriginBlock uint64 `xml:"origin_block,attr"`
	DataBlock   uint64 `xml:"data_block,attr"`
	Time        uint32 `xml:"time,attr"`
}

type RangeMappingXML struct {
	OriginBegin uint64 `xml:"origin_begin,attr"`
	DataBegin   uint64 `xml:"data_begin,attr"`
	Length      uint64 `xml:"length,attr"`
	Time        uint32 `xml:"time,attr"`
}

type Mapping struct {
	RangeMappingXML
}

type DeviceBase struct {
	DevID        uint32 `xml:"dev_id,attr"`
	MappedBlocks uint64 `xml:"mapped_blocks,attr"`
	Transaction  uint64 `xml:"transaction,attr"`
	CreationTime uint64 `xml:"creation_time,attr"`
	SnapTime     uint64 `xml:"snap_time,attr"`
}

type DeviceXML struct {
	DeviceBase
	SingleMappingXML []SingleMappingXML `xml:"single_mapping"`
	RangeMappingXML  []RangeMappingXML  `xml:"range_mapping"`
}

type Device struct {
	DeviceBase
	Mappings []Mapping
}

type SuperblockBase struct {
	UUID          string `xml:"uuid,attr"`
	Time          uint64 `xml:"time,attr"`
	Transaction   uint64 `xml:"transaction,attr"`
	DataBlockSize uint32 `xml:"data_block_size,attr"`
	NrDataBlock   uint64 `xml:"nr_data_blocks,attr"`
}

type SuperblockXML struct {
	SuperblockBase
	DeviceXML []DeviceXML `xml:"device"`
}

type Metadata struct {
	SuperblockBase
	Devices map[uint32]*Device
}

func (m *Metadata) Parser(data []byte) error {
	result := SuperblockXML{}

	err := xml.Unmarshal(data, &result)

	if err != nil {
		return err
	}

	m.SuperblockBase = result.SuperblockBase
	m.Devices = make(map[uint32]*Device)
	for i := 0; i < len(result.DeviceXML); i++ {
		dev := result.DeviceXML[i]

		mdev := new(Device)
		mdev.DeviceBase = dev.DeviceBase
		mdev.Mappings = make([]Mapping, len(dev.SingleMappingXML)+len(dev.RangeMappingXML))
		// As stopper
		fs := SingleMappingXML{math.MaxUint64, 0, 0}
		fr := RangeMappingXML{math.MaxUint64, 0, 0, 0}
		dev.SingleMappingXML = append(dev.SingleMappingXML, fs)
		dev.RangeMappingXML = append(dev.RangeMappingXML, fr)
		for ps, pr := 0, 0; ps < len(dev.SingleMappingXML)-1 || pr < len(dev.RangeMappingXML)-1; {
			if dev.SingleMappingXML[ps].OriginBlock < dev.RangeMappingXML[pr].OriginBegin {
				mdev.Mappings[ps+pr].OriginBegin = dev.SingleMappingXML[ps].OriginBlock
				mdev.Mappings[ps+pr].DataBegin = dev.SingleMappingXML[ps].DataBlock
				mdev.Mappings[ps+pr].Length = 1
				mdev.Mappings[ps+pr].Time = dev.SingleMappingXML[ps].Time
				ps++
			} else {
				mdev.Mappings[ps+pr].RangeMappingXML = dev.RangeMappingXML[pr]
				pr++
			}
		}
		m.Devices[dev.DevID] = mdev
	}

	return nil
}

func getMetadataFromFile(metaFileName string) (Metadata, error) {
	metaFile, err := os.Open(metaFileName)
	if err != nil {
		fmt.Println("Failed to open file ", metaFileName)
		return Metadata{}, err
	}
	defer metaFile.Close()

	stat, err := metaFile.Stat()
	if err != nil {
		fmt.Println("Failed to get stat of file ", metaFileName)
		return Metadata{}, err
	}

	bs := make([]byte, stat.Size())
	_, err = metaFile.Read(bs)
	if err != nil {
		fmt.Println("Failed to read file ", metaFileName)
		return Metadata{}, err
	}

	metadata := Metadata{}
	err = metadata.Parser(bs)
	if err != nil {
		fmt.Println("Failed to parse file ", metaFileName)
		return Metadata{}, err
	}

	return metadata, nil
}

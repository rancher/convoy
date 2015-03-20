package metadata

import (
	"bytes"
	"encoding/xml"
)

func DeviceMapperThinDeltaParser(data []byte, blockSize uint32, mapping *Mappings) error {
	type DeltaRange struct {
		Begin          uint64 `xml:"begin,attr"`
		DataBegin      uint64 `xml:"data_begin,attr"`
		LeftDataBegin  uint64 `xml:"left_data_begin,attr"`
		RightDataBegin uint64 `xml:"right_data_begin,attr"`
		Length         uint64 `xml:"length,attr"`
	}

	type ItemXml struct {
		DeltaRange []DeltaRange `xml:"range"`
	}

	type ThinDelta struct {
		Sames ItemXml `xml:"same"`
		Diffs ItemXml `xml:"different"`
	}

	wrapStart := []byte("<wrap>")
	wrapEnd := []byte("</wrap>")
	wrapData := bytes.Join([][]byte{wrapStart, data, wrapEnd}, []byte(" "))
	delta := &ThinDelta{}
	if err := xml.Unmarshal(wrapData, delta); err != nil {
		return err
	}

	var needProcess ItemXml
	//processDiff := true
	if len(delta.Diffs.DeltaRange) != 0 {
		needProcess = delta.Diffs
	} else {
		needProcess = delta.Sames
		//processDiff = false
	}

	mapping.Mappings = make([]Mapping, len(needProcess.DeltaRange))
	for i, d := range needProcess.DeltaRange {
		var m Mapping
		m.Offset = d.Begin * uint64(blockSize)
		m.Size = d.Length * uint64(blockSize)
		mapping.Mappings[i] = m
	}
	mapping.BlockSize = blockSize

	return nil
}

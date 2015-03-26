package metadata

import (
	"bytes"
	"encoding/xml"
)

func DeviceMapperThinDeltaParser(data []byte, blockSize int64, mapping *Mappings) error {
	type DeltaRange struct {
		Begin          int64 `xml:"begin,attr"`
		DataBegin      int64 `xml:"data_begin,attr"`
		LeftDataBegin  int64 `xml:"left_data_begin,attr"`
		RightDataBegin int64 `xml:"right_data_begin,attr"`
		Length         int64 `xml:"length,attr"`
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
		m.Offset = d.Begin * blockSize
		m.Size = d.Length * blockSize
		mapping.Mappings[i] = m
	}
	mapping.BlockSize = blockSize

	return nil
}

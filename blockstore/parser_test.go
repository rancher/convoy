package main

import (
	"testing"
)

func TestParser(t *testing.T) {
	data := `
		<superblock uuid="uuid-superblock" time="2" transaction="1" data_block_size="1024" nr_data_blocks="20480">
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
	m := Metadata{}
	err := m.Parser([]byte(data))

	if err != nil {
		t.Fatal(err)
	}

	if m.UUID != "uuid-superblock" {
		t.Fatal("Not expected", m.UUID)
	}
	if m.Time != 2 {
		t.Fatal("Not expected", m.Time)
	}
	if m.Transaction != 1 {
		t.Fatal("Not expected", m.Transaction)
	}
	if m.DataBlockSize != 1024 {
		t.Fatal("Not expected", m.DataBlockSize)
	}
	if m.NrDataBlock != 20480 {
		t.Fatal("Not expected", m.NrDataBlock)
	}

	if len(m.Devices) != 2 {
		t.Fatal("Not expected", len(m.Devices))
	}
	if m.Devices[100].DevID != 100 {
		t.Fatal("Not expected", m.Devices[100].DevID)
	}

	if m.Devices[100].Mappings[0].OriginBegin != 0 {
		t.Fatal("Not expected", m.Devices[100].Mappings[0].OriginBegin)
	}
	if m.Devices[100].Mappings[0].DataBegin != 1 {
		t.Fatal("Not expected", m.Devices[100].Mappings[0].DataBegin)
	}
	if m.Devices[100].Mappings[0].Length != 1 {
		t.Fatal("Not expected", m.Devices[100].Mappings[0].Length)
	}
	if m.Devices[100].Mappings[0].Time != 2 {
		t.Fatal("Not expected", m.Devices[100].Mappings[0].Time)
	}

	if m.Devices[100].Mappings[1].OriginBegin != 1 {
		t.Fatal("Not expected", m.Devices[100].Mappings[1].OriginBegin)
	}
	if m.Devices[100].Mappings[1].DataBegin != 71 {
		t.Fatal("Not expected", m.Devices[100].Mappings[1].DataBegin)
	}
	if m.Devices[100].Mappings[1].Length != 15 {
		t.Fatal("Not expected", m.Devices[100].Mappings[1].Length)
	}
	if m.Devices[100].Mappings[1].Time != 0 {
		t.Fatal("Not expected", m.Devices[100].Mappings[1].Time)
	}

	if m.Devices[101].DevID != 101 {
		t.Fatal("Not expected", m.Devices[101].DevID)
	}
	if m.Devices[101].MappedBlocks != 102 {
		t.Fatal("Not expected", m.Devices[101].MappedBlocks)
	}
	if m.Devices[101].Transaction != 1 {
		t.Fatal("Not expected", m.Devices[101].Transaction)
	}
	if m.Devices[101].CreationTime != 1 {
		t.Fatal("Not expected", m.Devices[101].CreationTime)
	}
	if m.Devices[101].SnapTime != 1 {
		t.Fatal("Not expected", m.Devices[101].SnapTime)
	}
	if m.Devices[101].Mappings[9].OriginBegin != 1792 {
		t.Fatal("Not expected", m.Devices[100].Mappings[0].Time)
	}
	if m.Devices[101].Mappings[10].OriginBegin != 2047 {
		t.Fatal("Not expected", m.Devices[100].Mappings[1].DataBegin)
	}
}

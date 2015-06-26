package metadata

import (
	"testing"

	. "gopkg.in/check.v1"
)

const (
	thinDeltaOutputSame = `
		<superblock uuid="" time="0" transaction="0" data_block_size="4096" nr_data_blocks="51200">
			<diff left="1" right="2">
				<same begin="0" length="1"/>
				<same begin="1" length="1"/>
				<same begin="4" length="3"/>
				<same begin="8" length="1"/>
				<same begin="12" length="1"/>
			</diff>
		</superblock>`
	thinDeltaOutputDiff = `
		<superblock uuid="" time="0" transaction="0" data_block_size="4096" nr_data_blocks="51200">
			<diff left="1" right="2">
				<different begin="0" length="1"/>
				<different begin="4" length="1"/>
			</diff>
		</superblock>`
	thinDeltaOutputMix = `
		<superblock uuid="" time="0" transaction="0" data_block_size="4096" nr_data_blocks="51200">
			<diff left="1" right="2">
				<same begin="0" length="1"/>
				<left_only begin="2" length="1"/>
				<different begin="4" length="1"/>
				<different begin="6" length="1"/>
				<right_only begin="8" length="2"/>
			</diff>
		</superblock>`
	blockSize = 2097152
)

func Test(t *testing.T) {
	TestingT(t)
}

type TestSuite struct{}

var _ = Suite(&TestSuite{})

func (s *TestSuite) TestThinDelta(c *C) {
	mSame := Mappings{
		Mappings: []Mapping{
			{Offset: 0, Size: 1 * blockSize},
			{Offset: 1 * blockSize, Size: 1 * blockSize},
			{Offset: 4 * blockSize, Size: 3 * blockSize},
			{Offset: 8 * blockSize, Size: 1 * blockSize},
			{Offset: 12 * blockSize, Size: 1 * blockSize},
		},
		BlockSize: blockSize,
	}
	mDiff := Mappings{
		Mappings: []Mapping{
			{Offset: 0, Size: 1 * blockSize},
			{Offset: 4 * blockSize, Size: 1 * blockSize},
		},
		BlockSize: blockSize,
	}
	mMix := Mappings{
		Mappings: []Mapping{
			{Offset: 2 * blockSize, Size: 1 * blockSize},
			{Offset: 4 * blockSize, Size: 1 * blockSize},
			{Offset: 6 * blockSize, Size: 1 * blockSize},
			{Offset: 8 * blockSize, Size: 2 * blockSize},
		},
		BlockSize: blockSize,
	}

	var (
		m   *Mappings
		err error
	)

	m, err = DeviceMapperThinDeltaParser([]byte(thinDeltaOutputSame), blockSize, true)
	c.Assert(err, IsNil)
	c.Assert(*m, DeepEquals, mSame)

	m, err = DeviceMapperThinDeltaParser([]byte(thinDeltaOutputMix), blockSize, false)
	c.Assert(err, IsNil)
	c.Assert(*m, DeepEquals, mMix)

	m, err = DeviceMapperThinDeltaParser([]byte(thinDeltaOutputDiff), blockSize, false)
	c.Assert(err, IsNil)
	c.Assert(*m, DeepEquals, mDiff)
}

package util

import (
	"code.google.com/p/go-uuid/uuid"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	. "gopkg.in/check.v1"
)

func Test(t *testing.T) { TestingT(t) }

type TestSuite struct {
	imageFile string
}

var _ = Suite(&TestSuite{})

const (
	testRoot  = "/tmp/util"
	emptyFile = "/tmp/util/empty"
	testImage = "test.img"
	imageSize = 1 << 27
)

func (s *TestSuite) SetUpSuite(c *C) {
	err := exec.Command("mkdir", "-p", testRoot).Run()
	c.Assert(err, IsNil)

	s.imageFile = filepath.Join(testRoot, testImage)
	err = exec.Command("dd", "if=/dev/zero", "of="+s.imageFile, "bs=4096", "count="+strconv.Itoa(imageSize/4096)).Run()
	c.Assert(err, IsNil)

	err = exec.Command("mkfs.ext4", "-F", s.imageFile).Run()
	c.Assert(err, IsNil)

	err = exec.Command("touch", emptyFile).Run()
	c.Assert(err, IsNil)
}

func (s *TestSuite) TearDownSuite(c *C) {
	err := exec.Command("rm", "-rf", testRoot).Run()
	c.Assert(err, IsNil)
}

func (s *TestSuite) TestExtractUUIDs(c *C) {
	prefix := "prefix_"
	suffix := ".suffix"
	counts := 10
	uuids := make([]string, counts)
	names := make([]string, counts)
	for i := 0; i < counts; i++ {
		uuids[i] = uuid.New()
		names[i] = prefix + uuids[i] + suffix
	}

	result, err := ExtractUUIDs(names, "prefix_", ".suffix")
	c.Assert(err, Equals, nil)
	for i := 0; i < counts; i++ {
		c.Assert(result[i], Equals, uuids[i])
	}

	names[0] = "/" + names[0]
	result, err = ExtractUUIDs(names, "prefix_", ".suffix")
	c.Assert(err, Equals, nil)
	c.Assert(result[0], Equals, uuids[0])

	names[0] = "prefix_dd_xx.suffix"
	result, err = ExtractUUIDs(names, "prefix_", ".suffix")
	c.Assert(err, ErrorMatches, "Invalid name.*")
}

func (s *TestSuite) TestListConfigIDs(c *C) {
	tmpdir, err := ioutil.TempDir("/tmp", "convoy")
	c.Assert(err, IsNil)
	defer os.RemoveAll(tmpdir)

	prefix := "prefix_"
	suffix := "_suffix.cfg"
	ids, err := ListConfigIDs(tmpdir, prefix, suffix)
	c.Assert(err, Equals, nil)
	c.Assert(ids, HasLen, 0)

	counts := 10
	uuids := make(map[string]bool)
	for i := 0; i < counts; i++ {
		id := uuid.New()
		uuids[id] = true
		err := exec.Command("touch", filepath.Join(tmpdir, prefix+id+suffix)).Run()
		c.Assert(err, IsNil)
	}
	uuidList, err := ListConfigIDs(tmpdir, prefix, suffix)
	c.Assert(err, Equals, nil)
	c.Assert(uuidList, HasLen, counts)
	for i := 0; i < counts; i++ {
		uuids[uuidList[i]] = false
	}
	for _, notCovered := range uuids {
		c.Assert(notCovered, Equals, false)
	}
}

func (s *TestSuite) TestLockFile(c *C) {
	file := "/tmp/t.lock"
	err := LockFile(file)
	c.Assert(err, IsNil)

	err = LockFile(file)
	c.Assert(err, Not(IsNil))
	c.Assert(err, ErrorMatches, "resource temporarily unavailable")

	err = LockFile(file)
	c.Assert(err, Not(IsNil))
	c.Assert(err, ErrorMatches, "resource temporarily unavailable")

	err = UnlockFile(file)
	c.Assert(err, IsNil)

	err = LockFile(file)
	c.Assert(err, IsNil)

	err = UnlockFile(file)
	c.Assert(err, IsNil)
}

func (s *TestSuite) TestSliceToMap(c *C) {
	legalMap := []string{
		"a=1",
		"b=2",
	}
	m := SliceToMap(legalMap)
	c.Assert(m["a"], Equals, "1")
	c.Assert(m["b"], Equals, "2")

	illegalMap := []string{
		"a=1",
		"bcd",
	}
	m = SliceToMap(illegalMap)
	c.Assert(m, IsNil)
}

func (s *TestSuite) TestChecksum(c *C) {
	checksum, err := GetFileChecksum(emptyFile)
	c.Assert(err, IsNil)
	c.Assert(checksum, Equals,
		"cf83e1357eefb8bdf1542850d66d8007d620e4050b5715dc83f4a921d36ce9ce47d0d13c5d85f2b0ff8318d2877eec2f63b931bd47417a81a538327af927da3e")
}

func (s *TestSuite) TestLoopDevice(c *C) {
	dev, err := AttachLoopbackDevice(s.imageFile, true)
	c.Assert(err, IsNil)

	err = DetachLoopbackDevice("/tmp", dev)
	c.Assert(err, Not(IsNil))

	err = DetachLoopbackDevice(s.imageFile, dev)
	c.Assert(err, IsNil)

	_, err = AttachLoopbackDevice("/tmp", true)
	c.Assert(err, Not(IsNil))

	err = DetachLoopbackDevice("/tmp", "/dev/loop0")
	c.Assert(err, Not(IsNil))
}

func (s *TestSuite) TestValidateUUID(c *C) {
	c.Assert(ValidateUUID(""), Equals, false)
	c.Assert(ValidateUUID("123"), Equals, false)
	c.Assert(ValidateUUID("asdf"), Equals, false)
	c.Assert(ValidateUUID("f997529d-904f-4fbc-8ba2-6d296b74470a"), Equals, true)
	c.Assert(ValidateUUID("00000000-0000-0000-0000-000000000000"), Equals, true)
}

func (s *TestSuite) TestValidateName(c *C) {
	c.Assert(ValidateName(""), Equals, false)
	c.Assert(ValidateName("_09123a."), Equals, true)
	c.Assert(ValidateName("ubuntu14.04_v1"), Equals, true)
	c.Assert(ValidateName("123/456.a"), Equals, false)
	c.Assert(ValidateName("a.\t"), Equals, false)
	c.Assert(ValidateName("ubuntu14.04_v1 "), Equals, false)
}

func (s *TestSuite) TestParseSize(c *C) {
	var (
		value int64
		err   error
	)
	value, err = ParseSize("1024")
	c.Assert(value, Equals, int64(1024))
	c.Assert(err, IsNil)

	value, err = ParseSize("100k")
	c.Assert(value, Equals, int64(102400))
	c.Assert(err, IsNil)

	value, err = ParseSize("100m")
	c.Assert(value, Equals, int64(104857600))
	c.Assert(err, IsNil)

	value, err = ParseSize("100g")
	c.Assert(value, Equals, int64(107374182400))
	c.Assert(err, IsNil)

	value, err = ParseSize("100K")
	c.Assert(value, Equals, int64(102400))

	value, err = ParseSize("0")
	c.Assert(value, Equals, int64(0))
	c.Assert(err, IsNil)

	value, err = ParseSize("0k")
	c.Assert(value, Equals, int64(0))
	c.Assert(err, IsNil)

	value, err = ParseSize("")
	c.Assert(value, Equals, int64(0))
	c.Assert(err, IsNil)

	value, err = ParseSize("m")
	c.Assert(value, Equals, int64(0))
	c.Assert(err, ErrorMatches, "strconv.ParseInt: parsing .*: invalid syntax")

	value, err = ParseSize(".m")
	c.Assert(value, Equals, int64(0))
	c.Assert(err, ErrorMatches, "strconv.ParseInt: parsing .*: invalid syntax")
}

func (s *TestSuite) TestIndex(c *C) {
	var err error
	index := NewIndex()
	err = index.Add("key1", "value1")
	c.Assert(err, IsNil)

	err = index.Add("key1", "value2")
	c.Assert(err, ErrorMatches, "BUG: Conflict when updating index.*")

	err = index.Add("", "value")
	c.Assert(err, ErrorMatches, "BUG: Invalid empty index key")

	err = index.Add("key", "")
	c.Assert(err, ErrorMatches, "BUG: Invalid empty index value")

	value := index.Get("key1")
	c.Assert(value, Equals, "value1")

	value = index.Get("keyx")
	c.Assert(value, Equals, "")

	err = index.Delete("")
	c.Assert(err, ErrorMatches, "BUG: Invalid empty index key")

	err = index.Delete("keyx")
	c.Assert(err, ErrorMatches, "BUG: About to remove non-existed key.*")

	err = index.Delete("key1")
	c.Assert(err, IsNil)
}

func (s *TestSuite) TestCompress(c *C) {
	var err error
	data := []byte("Some random string")
	checksum := GetChecksum(data)

	compressed, err := CompressData(data)
	c.Assert(err, IsNil)

	decompressed, err := DecompressAndVerify(compressed, checksum)
	c.Assert(err, IsNil)

	result, err := ioutil.ReadAll(decompressed)
	c.Assert(err, IsNil)

	c.Assert(result, DeepEquals, data)
}

func (s *TestSuite) TestCompressDir(c *C) {
	var err error

	tmpdir, err := ioutil.TempDir("/tmp", "convoy")
	c.Assert(err, IsNil)
	defer os.RemoveAll(tmpdir)

	path := filepath.Join(tmpdir, "path")
	err = os.Mkdir(path, os.ModeDir|0700)
	c.Assert(err, IsNil)

	filename1 := filepath.Join(path, "file1")
	data1 := []byte("Some random string for file1")
	file1, err := os.Create(filename1)
	c.Assert(err, IsNil)
	_, err = file1.Write(data1)
	c.Assert(err, IsNil)
	err = file1.Close()
	c.Assert(err, IsNil)

	filename2 := filepath.Join(path, "file1")
	data2 := []byte("Some random string for file1")
	file2, err := os.Create(filename2)
	c.Assert(err, IsNil)
	_, err = file2.Write(data2)
	c.Assert(err, IsNil)
	err = file2.Close()
	c.Assert(err, IsNil)

	tarFile := filepath.Join(tmpdir, "test.tar.gz")
	err = CompressDir(path, tarFile)
	c.Assert(err, IsNil)
	err = os.RemoveAll(path)
	c.Assert(err, IsNil)
	err = DecompressDir(tarFile, path)
	c.Assert(err, IsNil)

	file1, err = os.Open(filename1)
	c.Assert(err, IsNil)
	data, err := ioutil.ReadAll(file1)
	c.Assert(err, IsNil)
	c.Assert(data, DeepEquals, data1)
	err = file1.Close()
	c.Assert(err, IsNil)

	file2, err = os.Open(filename2)
	c.Assert(err, IsNil)
	data, err = ioutil.ReadAll(file2)
	c.Assert(err, IsNil)
	c.Assert(data, DeepEquals, data2)
	err = file2.Close()
	c.Assert(err, IsNil)
}

/*
func (s *TestSuite) TestNBDDeviceFind(c *C) {

	dev, err := NBDFindFreeDevice()
	c.Assert(err, IsNil)
	c.Assert(strings.Contains(dev, "/dev/nbd"), Equals, true)
}
*/

package util

import (
	"code.google.com/p/go-uuid/uuid"

	. "gopkg.in/check.v1"
)

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

func (s *TestSuite) TestValidateUUID(c *C) {
	c.Assert(ValidateUUID(""), Equals, false)
	c.Assert(ValidateUUID("123"), Equals, false)
	c.Assert(ValidateUUID("asdf"), Equals, false)
	c.Assert(ValidateUUID("f997529d-904f-4fbc-8ba2-6d296b74470a"), Equals, true)
	c.Assert(ValidateUUID("00000000-0000-0000-0000-000000000000"), Equals, true)
}

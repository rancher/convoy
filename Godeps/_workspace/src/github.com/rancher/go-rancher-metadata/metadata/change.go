package metadata

import (
	"time"

	"github.com/Sirupsen/logrus"
)

func (m *Client) OnChange(intervalSeconds int, do func(string)) {
	interval := time.Duration(intervalSeconds)
	version := "init"

	for {
		newVersion, err := m.GetVersion()
		if err != nil {
			logrus.Errorf("Error reading metadata version: %v", err)
			time.Sleep(interval * time.Second)
		} else if version == newVersion {
			logrus.Debug("No changes in metadata version")
			time.Sleep(interval * time.Second)
		} else {
			logrus.Debugf("Metadata Version has been changed. Old version: %s. New version: %s.", version, newVersion)
			version = newVersion
			do(newVersion)
		}
	}
}

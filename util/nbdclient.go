package util

import (
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

func NBDGetDeviceList() ([]string, error) {
	out, err := Execute("find", []string{"/dev",
		"-maxdepth", "1",
		"-name", "nbd*",
		"-printf", "%p "})
	if err != nil {
		return nil, fmt.Errorf("Error when finding NBD devices: %v", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("Cannot find NBD devices, check if nbd kernel module is loaded")
	}
	devs := strings.Split(strings.TrimSpace(string(out)), " ")
	return devs, nil
}

func NBDFindFreeDevice() (string, error) {
	devs, err := NBDGetDeviceList()
	if err != nil {
		return "", err
	}
	for _, dev := range devs {
		// nbd-client would return nothing and error code 1 if the
		// device is available to use
		out, err := exec.Command("nbd-client", "-c", dev).CombinedOutput()
		if len(out) != 0 || err == nil {
			continue
		}
		fmt.Println(err)
		if msg, ok := err.(*exec.ExitError); ok {
			errCode := msg.Sys().(syscall.WaitStatus).ExitStatus()
			if errCode == 1 {
				return dev, nil
			}
		}
	}
	return "", fmt.Errorf("Cannot find a unused NBD device!")
}

func NBDConnect(ip string) (string, error) {
	dev, err := NBDFindFreeDevice()
	if err != nil {
		return "", err
	}
	log.Debugf("Found unused NBD device %v", dev)
	if _, err := Execute("nbd-client", []string{
		"-b", "4096",
		"-N", "disk",
		ip,
		dev,
	}); err != nil {
		return "", err
	}
	return dev, nil
}

func NBDDisconnect(device string) error {
	if _, err := Execute("nbd-client", []string{"-d", device}); err != nil {
		return err
	}
	return nil
}

package fs

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
)

var supportedFsTypes = []string{"btrfs", "ext2", "ext3", "ext4", "minix", "xfs"}

func TestDeviceFormatter(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("TestDeviceFormatter skipped because OS is not 'linux'")
	}
	if !hasRoot() {
		t.Skip("TestDeviceFormatter skipped because sudo privileges are not currently available")
	}

	for _, targetFSType := range supportedFsTypes {
		if _, err := exec.LookPath(fmt.Sprintf("mkfs.%v", targetFSType)); err != nil { // Verify existence of mkfs.X in $PATH or skip.
			t.Logf("Skipping formatter test for fsType=%[1]s because 'mkfs.%[1]s' binary was not found in $PATH (err=%v)", targetFSType, err)
			continue
		}

		func() {
			// Create fake device.
			fakeDevicePath := fmt.Sprintf("/tmp/TestDeviceFormatter-%v.img", targetFSType)
			if err := makeFakeDevice(fakeDevicePath); err != nil {
				t.Fatal(err)
			}

			// Cleanup and remove it afterwards.
			defer func() {
				if err := os.RemoveAll(fakeDevicePath); err != nil {
					t.Error(err.Error())
				}
			}()

			// Unformatted case.
			if fsType, err := Detect(fakeDevicePath); err == nil {
				t.Fatalf("Expected non-nil error from Detect for unformatted device, but got fsType=%v err=%s", fsType, err)
			}

			// Format it.
			if err := FormatDevice(fakeDevicePath, targetFSType); err != nil {
				t.Fatalf("Unexpected error formatting device=%v with fsType=%v: %s", fakeDevicePath, targetFSType, err)
			}
		}()
	}
}

func TestFSDetectorResize(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("TestFSDetectorResize skipped because OS is not 'linux'")
	}
	if !hasRoot() {
		t.Skip("TestFSDetectorResize skipped because sudo privileges are not currently available")
	}

	// Non-existent test case.
	if fsType, err := Detect("/TestFSDetectorResize/foo/bar/baz"); err != ErrNoFilesystemDetected {
		t.Errorf("Expected ErrNoFilesystemDetected error from Detect, but got result fsType=%v err=%s", fsType, err)
	}

	// Non-existent test case.
	if err := Resize("/TestFSDetectorResize/foo/bar/baz"); err == nil {
		t.Error("Expected error from Resize, but got nothing")
	}

	for _, targetFSType := range supportedFsTypes {
		if _, err := exec.LookPath(fmt.Sprintf("mkfs.%v", targetFSType)); err != nil { // Verify existence of mkfs.X in $PATH or skip.
			t.Logf("Skipping resize test for fsType=%[1]s because 'mkfs.%[1]s' binary was not found in $PATH (err=%v)", targetFSType, err)
			continue
		}

		func() {
			// Create fake device.
			fakeDevicePath := fmt.Sprintf("/tmp/TestFSDetectorResize-%v.img", targetFSType)
			if err := makeFakeDevice(fakeDevicePath); err != nil {
				t.Fatalf("Unexpected error making fake device for fsType=%v: %s", targetFSType, err)
			}

			// Cleanup and remove it afterwards.
			defer func() {
				if err := os.RemoveAll(fakeDevicePath); err != nil {
					t.Errorf("Unexpected error cleaning up fake device path=%v: %s", fakeDevicePath, err)
				}
			}()

			// Unformatted case.
			if fsType, err := Detect(fakeDevicePath); err != ErrNoFilesystemDetected {
				t.Fatalf("Expected ErrNoFilesystemDetected from Detect() for unformatted device and fsType=%v, but err=%s", fsType, err)
			}

			// Unformatted case.
			if err := Resize(fakeDevicePath); err == nil {
				t.Fatal("Expected error from Resize() for unformatted device and fsType=%v, but err=%s", targetFSType, err)
			}

			// Format it.
			if err := FormatDevice(fakeDevicePath, targetFSType); err != nil {
				t.Fatalf("Unexpected error formatting device=%v: %s", fakeDevicePath, err)
			}

			// Formatted case.
			{
				fsType, err := Detect(fakeDevicePath)
				if err != nil {
					t.Errorf("Unexpected error from Detect() for fsType=%v: %s", targetFSType, err)
				}
				if expected, actual := targetFSType, fsType; actual != expected {
					t.Errorf("Incorrect result from Detect(), expected fsType=%q but actual=%q", expected, actual)
				}

				err = Resize(fakeDevicePath)
				if strings.HasPrefix(targetFSType, "ext") {
					if err != nil {
						t.Errorf("Unexpected error from Resize() for fsType=%v: %s", targetFSType, err)
					}
				} else if err == nil {
					t.Errorf("Expected error from Resize() for fsType=%v, but err=%s", targetFSType, err)
				}
			}
		}()
	}
}

func makeFakeDevice(path string) error {
	size := 100 // NB: btrfs requires a minimum size of 100MB.
	if output, err := exec.Command("dd", "if=/dev/zero", fmt.Sprintf("of=%v", path), "bs=1M", fmt.Sprintf("count=%v", size)).CombinedOutput(); err != nil {
		return fmt.Errorf("fake device setup failed: dd command output=%v err=%s", string(bytes.Trim(output, "\r\n \t")), err)
	}
	return nil
}

func hasRoot() bool {
	cmd, err := sudoCmd("true")
	if err != nil {
		panic(err)
	}
	if len(cmd.Args) == 1 {
		// Already root.
		return true
	}
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

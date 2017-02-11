package fs

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"testing"
)

var supportedFsTypes = []string{"btrfs", "ext2", "ext3", "ext4", "minix", "xfs"}

func TestDeviceFormatter(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("TestDeviceFormatter skipped because OS is not 'linux'")
	}
	if !hasPasswordlessSudo() {
		t.Skip("TestDeviceFormatter skipped because password-less sudo is required and not presently available")
	}

	for _, targetFsType := range supportedFsTypes {
		func() {
			// Create fake device.
			fakeDevicePath := fmt.Sprintf("/tmp/TestDeviceFormatter-%s.img", targetFsType)
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
				t.Fatalf("Expected non-nil error from Detect for unformatted device, but got fsType=%s err=%+v", fsType, err)
			}

			// Format it.
			if err := FormatDevice(fakeDevicePath, targetFsType); err != nil {
				t.Fatalf("Unexpected error formatting device=%s: %s", fakeDevicePath, err)
			}
		}()
	}
}

func TestFSDetectorResize(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("TestFSDetectorResize skipped because OS is not 'linux'")
	}
	if !hasPasswordlessSudo() {
		t.Skip("TestFSDetectorResize skipped because password-less sudo is required and not presently available")
	}

	// Non-existent test case.
	if fsType, err := Detect("/TestFSDetectorResize/foo/bar/baz"); err != ErrNoFilesystemDetected {
		t.Errorf("Expected ErrNoFilesystemDetected error from Detect, but got result fsType=%s err=%+v", fsType, err)
	}

	// Non-existent test case.
	if err := Resize("/TestFSDetectorResize/foo/bar/baz"); err == nil  {
		t.Error("Expected error from Resize, but got nothing")
	}

	for _, targetFsType := range supportedFsTypes {
		func() {
			// Create fake device.
			fakeDevicePath := fmt.Sprintf("/tmp/TestFSDetectorResize-%s.img", targetFsType)
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
			if fsType, err := Detect(fakeDevicePath); err != ErrNoFilesystemDetected {
				t.Fatalf("Expected ErrNoFilesystemDetected from Detect for unformatted device, but got fsType=%s err=%+v", fsType, err)
			}

			// Unformatted case.
			if err := Resize(fakeDevicePath); err == nil {
				t.Fatal("Expected error from Resize for unformatted device, but got nothing")
			}

			// Format it.
			if err := FormatDevice(fakeDevicePath, targetFsType); err != nil {
				t.Fatalf("Unexpected error formatting device=%s: %s", fakeDevicePath, err)
			}

			// Formatted case.
			{
				fsType, err := Detect(fakeDevicePath)
				if err != nil {
					t.Errorf("Unexpected error from Detect: %s", err)
				}
				if expected, actual := targetFsType, fsType; actual != expected {
					t.Errorf("Wrong result from Detect, expected fsType=%q but actual=%q", expected, actual)
				}

				err = Resize(fakeDevicePath)
				if err != nil {
					t.Errorf("Unexpected error in resize: %s", err)
				}
			}
		}()
	}
}


func makeFakeDevice(path string) error {
	size := 100 // NB: btrfs requires a minimum size of 100MB.
	if output, err := exec.Command("dd", "if=/dev/zero", fmt.Sprintf("of=%s", path), "bs=1M", fmt.Sprintf("count=%v", size)).CombinedOutput(); err != nil {
		return fmt.Errorf("fake device setup failed: dd command output=%s err=%s", string(bytes.Trim(output, "\r\n \t")), err)
	}
	return nil
}

func hasPasswordlessSudo() bool {
	cmd := exec.Command("sudo", "-n", "true")
	if err := cmd.Run(); err != nil {
		return false
	}
	return true
}

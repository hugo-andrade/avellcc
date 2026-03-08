// Package hidraw provides low-level HID transport via Linux hidraw ioctls.
// No cgo required — uses golang.org/x/sys/unix for syscalls.
package hidraw

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"unsafe"

	"golang.org/x/sys/unix"
)

// ioctl direction bits
const (
	iocWrite     = 1
	iocRead      = 2
	iocReadWrite = 3
)

func hidiocsfeature(length int) uintptr {
	return uintptr(iocReadWrite)<<30 | uintptr('H')<<8 | 0x06 | uintptr(length)<<16
}

func hidiocgfeature(length int) uintptr {
	return uintptr(iocReadWrite)<<30 | uintptr('H')<<8 | 0x07 | uintptr(length)<<16
}

// FindHidraw locates the /dev/hidrawN path for a given USB VID:PID.
func FindHidraw(vendorID, productID uint16) (string, error) {
	matches, err := filepath.Glob("/sys/class/hidraw/hidraw*/device/uevent")
	if err != nil {
		return "", err
	}
	sort.Strings(matches)

	for _, path := range matches {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		for _, line := range strings.Split(string(data), "\n") {
			if !strings.HasPrefix(line, "HID_ID=") {
				continue
			}
			// Format: HID_ID=BUS:VID:PID (each field variable-width hex)
			parts := strings.Split(strings.TrimPrefix(line, "HID_ID="), ":")
			if len(parts) < 3 {
				continue
			}
			vid, err1 := strconv.ParseUint(parts[1], 16, 32)
			pid, err2 := strconv.ParseUint(parts[2], 16, 32)
			if err1 != nil || err2 != nil {
				continue
			}
			if uint16(vid) == vendorID && uint16(pid) == productID {
				rel := strings.TrimPrefix(path, "/sys/class/hidraw/")
				name := strings.SplitN(rel, "/", 2)[0]
				return "/dev/" + name, nil
			}
		}
	}
	return "", fmt.Errorf("hidraw device %04x:%04x not found", vendorID, productID)
}

// HidrawDevice provides direct hidraw access for HID feature reports.
type HidrawDevice struct {
	Path string
	fd   int
}

// Open opens the hidraw device for reading and writing.
func (d *HidrawDevice) Open() error {
	fd, err := unix.Open(d.Path, unix.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open %s: %w", d.Path, err)
	}
	d.fd = fd
	return nil
}

// Close closes the hidraw device.
func (d *HidrawDevice) Close() error {
	if d.fd > 0 {
		err := unix.Close(d.fd)
		d.fd = -1
		return err
	}
	return nil
}

// SendFeatureReport sends a SET_FEATURE report. buf[0] must be the report ID.
func (d *HidrawDevice) SendFeatureReport(buf []byte) error {
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		uintptr(d.fd),
		hidiocsfeature(len(buf)),
		uintptr(unsafe.Pointer(&buf[0])),
	)
	if errno != 0 {
		return fmt.Errorf("HIDIOCSFEATURE: %w", errno)
	}
	return nil
}

// GetFeatureReport reads a GET_FEATURE report. reportID is placed in buf[0].
func (d *HidrawDevice) GetFeatureReport(reportID byte, length int) ([]byte, error) {
	buf := make([]byte, length)
	buf[0] = reportID
	_, _, errno := unix.Syscall(
		unix.SYS_IOCTL,
		uintptr(d.fd),
		hidiocgfeature(length),
		uintptr(unsafe.Pointer(&buf[0])),
	)
	if errno != 0 {
		return nil, fmt.Errorf("HIDIOCGFEATURE: %w", errno)
	}
	return buf, nil
}

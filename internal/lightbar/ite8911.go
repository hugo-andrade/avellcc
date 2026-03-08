// Package lightbar implements the ITE 8911 lightbar controller (X58 protocol).
package lightbar

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/hugo-andrade/avellcc/internal/hidraw"
)

const (
	VID = 0x048D
	PID = 0x8911

	ReportIDInfo = 0x5A
	ReportIDCtrl = 0xCD

	DescriptorReportPayloadSize = 16
	DescriptorReportSize        = 1 + DescriptorReportPayloadSize
	WindowsFrameSize            = 0x40

	X58Command = 0xE2
)

// X58 effect codes.
var X58EffectCodes = map[string]byte{
	"static":       0x05,
	"breathe":      0x06,
	"wave":         0x07,
	"change-color": 0x08,
	"granular":     0x09,
	"color-wave":   0x0A,
}

// X58EffectNames maps codes back to names.
var X58EffectNames = map[byte]string{
	0x05: "static",
	0x06: "breathe",
	0x07: "wave",
	0x08: "change-color",
	0x09: "granular",
	0x0A: "color-wave",
}

// X58 color IDs.
var X58ColorIDs = map[string]byte{
	"red":    0x01,
	"yellow": 0x02,
	"lime":   0x03,
	"green":  0x04,
	"cyan":   0x05,
	"blue":   0x06,
	"purple": 0x07,
}

// X58ColorNames maps IDs back to names.
var X58ColorNames = map[byte]string{
	0x01: "red",
	0x02: "yellow",
	0x03: "lime",
	0x04: "green",
	0x05: "cyan",
	0x06: "blue",
	0x07: "purple",
}

// X58ColorHex maps color IDs to hex strings.
var X58ColorHex = map[byte]string{
	0x01: "#ff0000",
	0x02: "#ffff00",
	0x03: "#80ff00",
	0x04: "#00ff00",
	0x05: "#00ffff",
	0x06: "#0000ff",
	0x07: "#8000ff",
}

// Defaults
const (
	X58DefaultEffect     = "static"
	X58DefaultEffectCode = 0x05
	X58DefaultColor      = "red"
	X58DefaultColorID    = 0x01
	X58DefaultBrightness = 4
	X58DefaultSpeed      = 3
)

// ITE8911 drives the ITE 8911 lightbar controller.
type ITE8911 struct {
	dev     *hidraw.HidrawDevice
	ownsDev bool
}

// NewITE8911 creates a new controller. If dev is nil, it auto-discovers.
func NewITE8911(dev *hidraw.HidrawDevice) *ITE8911 {
	return &ITE8911{dev: dev, ownsDev: dev == nil}
}

// Path returns the device path.
func (c *ITE8911) Path() string {
	if c.dev != nil {
		return c.dev.Path
	}
	return ""
}

// Open opens the hidraw device.
func (c *ITE8911) Open() error {
	if c.dev == nil {
		path, err := hidraw.FindHidraw(VID, PID)
		if err != nil {
			return fmt.Errorf("ITE lightbar device (%04x:%04x) not found: %w", VID, PID, err)
		}
		c.dev = &hidraw.HidrawDevice{Path: path}
		c.ownsDev = true
	}
	return c.dev.Open()
}

// Close closes the device if owned.
func (c *ITE8911) Close() error {
	if c.dev != nil && c.ownsDev {
		return c.dev.Close()
	}
	return nil
}

// DescriptorPath returns the sysfs path to the HID report descriptor.
func (c *ITE8911) DescriptorPath() string {
	if c.dev == nil {
		return ""
	}
	return filepath.Join("/sys/class/hidraw", filepath.Base(c.dev.Path), "device/report_descriptor")
}

// ReadReportDescriptor reads the raw HID report descriptor.
func (c *ITE8911) ReadReportDescriptor() ([]byte, error) {
	return os.ReadFile(c.DescriptorPath())
}

// GetFeature reads a HID feature report.
func (c *ITE8911) GetFeature(reportID byte, length int) ([]byte, error) {
	return c.dev.GetFeatureReport(reportID, length)
}

// SendFeature sends a HID feature report with zero-padded payload.
func (c *ITE8911) SendFeature(reportID byte, payload []byte, totalSize int) error {
	if totalSize < 2 {
		return fmt.Errorf("feature length too small: %d", totalSize)
	}
	maxPayload := totalSize - 1
	if len(payload) > maxPayload {
		return fmt.Errorf("payload too large: %d bytes (max %d)", len(payload), maxPayload)
	}
	buf := make([]byte, totalSize)
	buf[0] = reportID
	copy(buf[1:], payload)
	return c.dev.SendFeatureReport(buf)
}

// SendCommand sends a command on report 0xCD.
func (c *ITE8911) SendCommand(command byte, data []byte, totalSize int) error {
	payload := make([]byte, 1+len(data))
	payload[0] = command
	copy(payload[1:], data)
	return c.SendFeature(ReportIDCtrl, payload, totalSize)
}

// X58SetEffect sets the lightbar effect.
func (c *ITE8911) X58SetEffect(effectCode byte) error {
	return c.SendCommand(X58Command, []byte{effectCode}, WindowsFrameSize)
}

// X58SetSpeed sets the animation speed.
func (c *ITE8911) X58SetSpeed(speed byte) error {
	return c.SendCommand(X58Command, []byte{0x01, speed}, WindowsFrameSize)
}

// X58SetBrightness sets brightness (0-4 user range).
func (c *ITE8911) X58SetBrightness(level int) error {
	if level < 0 || level > 4 {
		return fmt.Errorf("X58 brightness range is 0-4, got %d", level)
	}
	return c.SendCommand(X58Command, []byte{0x02, byte(level + 1)}, WindowsFrameSize)
}

// X58SetColorID sets the color by ID.
func (c *ITE8911) X58SetColorID(colorID byte) error {
	return c.SendCommand(X58Command, []byte{0x03, colorID}, WindowsFrameSize)
}

// X58Apply applies settings in the Windows app order: effect → color → brightness → speed.
func (c *ITE8911) X58Apply(effectCode *byte, colorID *byte, brightness *int, speed *byte) error {
	if effectCode != nil {
		if err := c.X58SetEffect(*effectCode); err != nil {
			return err
		}
	}
	if colorID != nil {
		if err := c.X58SetColorID(*colorID); err != nil {
			return err
		}
	}
	if brightness != nil {
		if err := c.X58SetBrightness(*brightness); err != nil {
			return err
		}
	}
	if speed != nil {
		if err := c.X58SetSpeed(*speed); err != nil {
			return err
		}
	}
	return nil
}

// X58Off turns off the lightbar.
func (c *ITE8911) X58Off() error {
	effectCode := byte(X58DefaultEffectCode)
	colorID := byte(X58DefaultColorID)
	brightness := 0
	speed := byte(X58DefaultSpeed)
	return c.X58Apply(&effectCode, &colorID, &brightness, &speed)
}

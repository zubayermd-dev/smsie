//go:build !nouac && linux

package calling

import (
	"path/filepath"
	"strings"
)

func ResolveUSBIdentityFromPort(target ModemTarget) (USBIdentity, error) {
	usbPath, err := resolveUSBDevicePathFromTTYPort(target.PortName, "/sys/class/tty")
	if err != nil {
		return USBIdentity{}, err
	}

	vid, err := readSysValue(filepath.Join(usbPath, "idVendor"))
	if err != nil {
		return USBIdentity{}, err
	}
	pid, err := readSysValue(filepath.Join(usbPath, "idProduct"))
	if err != nil {
		return USBIdentity{}, err
	}
	serial, _ := readSysValue(filepath.Join(usbPath, "serial"))

	return USBIdentity{VID: strings.ToUpper(vid), PID: strings.ToUpper(pid), Serial: serial}, nil
}

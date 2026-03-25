package calling

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func resolveUSBDevicePathFromTTYPort(portName, ttyClassRoot string) (string, error) {
	port := strings.TrimSpace(portName)
	if port == "" {
		return "", fmt.Errorf("empty port name")
	}

	base := filepath.Base(port)
	if !strings.HasPrefix(base, "tty") {
		base = "tty" + base
	}

	ttyPath := filepath.Join(ttyClassRoot, base, "device")
	resolved, err := filepath.EvalSymlinks(ttyPath)
	if err != nil {
		return "", fmt.Errorf("resolve tty path failed: %w", err)
	}

	usbPath, err := findUSBDevicePath(resolved)
	if err != nil {
		return "", err
	}
	return usbPath, nil
}

func resolveALSACardHintsFromTTYPort(portName, ttyClassRoot, soundClassRoot string) []string {
	usbPath, err := resolveUSBDevicePathFromTTYPort(portName, ttyClassRoot)
	if err != nil {
		return nil
	}
	return resolveALSACardHintsFromUSBPath(usbPath, soundClassRoot)
}

func resolveALSACardHintsFromUSBPath(usbPath, soundClassRoot string) []string {
	entries, err := os.ReadDir(soundClassRoot)
	if err != nil {
		return nil
	}

	hints := map[string]struct{}{}
	add := func(v string) {
		x := strings.TrimSpace(strings.ToLower(v))
		if x == "" {
			return
		}
		hints[x] = struct{}{}
	}

	for _, entry := range entries {
		cardName := strings.TrimSpace(entry.Name())
		if !strings.HasPrefix(cardName, "card") {
			continue
		}

		cardPath := filepath.Join(soundClassRoot, cardName)
		devicePath := filepath.Join(cardPath, "device")
		resolvedDevicePath, err := filepath.EvalSymlinks(devicePath)
		if err != nil {
			continue
		}
		cardUSBPath, err := findUSBDevicePath(resolvedDevicePath)
		if err != nil || filepath.Clean(cardUSBPath) != filepath.Clean(usbPath) {
			continue
		}

		cardNumber := strings.TrimPrefix(cardName, "card")
		cardID, _ := readSysValue(filepath.Join(cardPath, "id"))
		pcmDevices := listALSAPCMDevices(soundClassRoot, cardNumber)

		add(cardName)
		if cardID != "" {
			add(cardID)
			add("card=" + cardID)
		}
		if cardNumber != "" {
			add("hw:" + cardNumber)
			add("plughw:" + cardNumber)
		}
		for _, dev := range pcmDevices {
			if cardNumber != "" {
				add("hw:" + cardNumber + "," + dev)
				add("plughw:" + cardNumber + "," + dev)
			}
			if cardID != "" {
				add("card=" + cardID + ",dev=" + dev)
			}
		}
	}

	if len(hints) == 0 {
		return nil
	}

	result := make([]string, 0, len(hints))
	for hint := range hints {
		result = append(result, hint)
	}
	sort.Strings(result)
	return result
}

func listALSAPCMDevices(soundClassRoot, cardNumber string) []string {
	if cardNumber == "" {
		return nil
	}

	entries, err := os.ReadDir(soundClassRoot)
	if err != nil {
		return nil
	}

	prefix := "pcmC" + cardNumber + "D"
	devices := map[string]struct{}{}
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasPrefix(name, prefix) {
			continue
		}

		rest := name[len(prefix):]
		end := 0
		for end < len(rest) && rest[end] >= '0' && rest[end] <= '9' {
			end++
		}
		if end == 0 {
			continue
		}

		devices[rest[:end]] = struct{}{}
	}

	if len(devices) == 0 {
		return nil
	}

	result := make([]string, 0, len(devices))
	for dev := range devices {
		result = append(result, dev)
	}
	sort.Strings(result)
	return result
}

func findUSBDevicePath(start string) (string, error) {
	cur := start
	for i := 0; i < 10; i++ {
		if _, err := os.Stat(filepath.Join(cur, "idVendor")); err == nil {
			return cur, nil
		}
		next := filepath.Dir(cur)
		if next == cur {
			break
		}
		cur = next
	}
	return "", fmt.Errorf("usb device root not found from %s", start)
}

func readSysValue(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read %s failed: %w", path, err)
	}
	return strings.TrimSpace(string(b)), nil
}

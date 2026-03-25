//go:build !nouac && linux

package calling

func ResolveALSACardHintsFromPort(target ModemTarget) []string {
	return resolveALSACardHintsFromTTYPort(target.PortName, "/sys/class/tty", "/sys/class/sound")
}

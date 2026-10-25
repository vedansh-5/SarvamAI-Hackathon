package macset

import (
	"context"
	"fmt"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
)

const (
	Volume = "volume"
	Mute   = "mute"
	Dark   = "dark mode"
	WiFi   = "wifi"
)

type Runner interface {
	Run(ctx context.Context, name string, args ...string) (string, error)
}

type Exec struct{}

func (Exec) Run(ctx context.Context, name string, args ...string) (string, error) {
	out, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("%s: %w (%s)", name, err, strings.TrimSpace(string(out)))
	}
	return strings.TrimSpace(string(out)), nil
}

type Mac struct {
	Runner Runner
}

func (m Mac) runner() Runner {
	if m.Runner != nil {
		return m.Runner
	}
	return Exec{}
}

func (m Mac) script(ctx context.Context, line string) (string, error) {
	return m.runner().Run(ctx, "/usr/bin/osascript", "-e", line)
}

var wifiPort = regexp.MustCompile(`(?s)Hardware Port: (?:Wi-Fi|AirPort)\s*\nDevice: (\S+)`)

func (m Mac) wifiDevice(ctx context.Context) (string, error) {
	out, err := m.runner().Run(ctx, "/usr/sbin/networksetup", "-listallhardwareports")
	if err != nil {
		return "", err
	}
	found := wifiPort.FindStringSubmatch(out)
	if found == nil {
		return "", fmt.Errorf("macset: this Mac has no Wi-Fi")
	}
	return found[1], nil
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func (m Mac) Get(ctx context.Context, setting string) (string, error) {
	switch setting {
	case Volume:
		return m.script(ctx, "output volume of (get volume settings)")
	case Mute:
		out, err := m.script(ctx, "output muted of (get volume settings)")
		return onOff(out == "true"), err
	case Dark:
		out, err := m.script(ctx, `tell application "System Events" to tell appearance preferences to get dark mode`)
		return onOff(out == "true"), err
	case WiFi:
		device, err := m.wifiDevice(ctx)
		if err != nil {
			return "", err
		}
		out, err := m.runner().Run(ctx, "/usr/sbin/networksetup", "-getairportpower", device)
		return onOff(strings.HasSuffix(strings.ToLower(out), ": on")), err
	}
	return "", fmt.Errorf("macset: %q is not a setting Vspk changes directly", setting)
}

func (m Mac) Set(ctx context.Context, setting, value string) error {
	switch setting {
	case Volume:
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 || n > 100 {
			return fmt.Errorf("macset: the volume goes from 0 to 100, not %q", value)
		}
		_, err = m.script(ctx, fmt.Sprintf("set volume output volume %d", n))
		return err
	case Mute:
		_, err := m.script(ctx, fmt.Sprintf("set volume output muted %t", value == "on"))
		return err
	case Dark:
		_, err := m.script(ctx, fmt.Sprintf(`tell application "System Events" to tell appearance preferences to set dark mode to %t`, value == "on"))
		return err
	case WiFi:
		device, err := m.wifiDevice(ctx)
		if err != nil {
			return err
		}
		_, err = m.runner().Run(ctx, "/usr/sbin/networksetup", "-setairportpower", device, value)
		return err
	}
	return fmt.Errorf("macset: %q is not a setting Vspk changes directly", setting)
}

var (
	isVolume = regexp.MustCompile(`(?i)\b(volume|sound|loud|louder|quieter|audio)\b`)
	isMute   = regexp.MustCompile(`(?i)\b(mute|muted|unmute|unmuted|silence|silent)\b`)
	isDark   = regexp.MustCompile(`(?i)\b(dark|light|appearance|theme)\b`)
	isWiFi   = regexp.MustCompile(`(?i)\b(wi-?fi|wireless|airport)\b`)

	saysOff  = regexp.MustCompile(`(?i)\b(off|disable|disabled|false|no|stop|unmute|unmuted)\b`)
	saysOn   = regexp.MustCompile(`(?i)\b(on|enable|enabled|true|yes|start)\b`)
	aNumber  = regexp.MustCompile(`\d{1,3}`)
	goesUp   = regexp.MustCompile(`(?i)\b(up|louder|raise|increase|higher|more)\b`)
	goesDown = regexp.MustCompile(`(?i)\b(down|quieter|lower|decrease|reduce|less|softer)\b`)
)

const VolumeStep = 10

func Which(setting string) (string, bool) {
	switch {
	case isMute.MatchString(setting):
		return Mute, true
	case isVolume.MatchString(setting):
		return Volume, true
	case isWiFi.MatchString(setting):
		return WiFi, true
	case isDark.MatchString(setting):
		return Dark, true
	}
	return "", false
}

func clamp(n int) int {
	return max(0, min(100, n))
}

func Wanted(setting, said, was string) (string, error) {
	said = strings.TrimSpace(said)
	toggle := func() string {
		if was == "on" {
			return "off"
		}
		return "on"
	}

	switch setting {
	case Volume:
		now, _ := strconv.Atoi(was)
		lowered := strings.ToLower(said)
		switch {
		case aNumber.MatchString(said):
			n, _ := strconv.Atoi(aNumber.FindString(said))
			if n > 100 {
				return "", fmt.Errorf("the volume goes from 0 to 100, not %d", n)
			}
			return strconv.Itoa(n), nil
		case strings.Contains(lowered, "max") || strings.Contains(lowered, "full"):
			return "100", nil
		case strings.Contains(lowered, "half"):
			return "50", nil
		case goesUp.MatchString(said):
			return strconv.Itoa(clamp(now + VolumeStep)), nil
		case goesDown.MatchString(said):
			return strconv.Itoa(clamp(now - VolumeStep)), nil
		}
		return "", fmt.Errorf("could not tell what volume %q means; say a number from 0 to 100, or up or down", said)

	case Mute:
		switch {
		case saysOff.MatchString(said):
			return "off", nil
		case saysOn.MatchString(said) || isMute.MatchString(said):
			return "on", nil
		}
		return toggle(), nil

	case Dark:
		lowered := strings.ToLower(said)
		switch {
		case strings.Contains(lowered, "light") || saysOff.MatchString(said):
			return "off", nil
		case strings.Contains(lowered, "dark") || saysOn.MatchString(said):
			return "on", nil
		}
		return toggle(), nil

	case WiFi:
		switch {
		case saysOff.MatchString(said):
			return "off", nil
		case saysOn.MatchString(said):
			return "on", nil
		}
		return toggle(), nil
	}
	return "", fmt.Errorf("%q is not a setting Vspk changes directly", setting)
}

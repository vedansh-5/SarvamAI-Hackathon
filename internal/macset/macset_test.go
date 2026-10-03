package macset

import (
	"context"
	"strings"
	"testing"
)

type fakeMac struct {
	volume string
	muted  bool
	dark   bool
	wifi   bool
	ran    []string
}

func (f *fakeMac) Run(_ context.Context, name string, args ...string) (string, error) {
	line := name + " " + strings.Join(args, " ")
	f.ran = append(f.ran, line)
	switch {
	case strings.Contains(line, "-listallhardwareports"):
		return "Hardware Port: Ethernet\nDevice: en5\n\nHardware Port: Wi-Fi\nDevice: en0\nEthernet Address: aa:bb", nil
	case strings.Contains(line, "-getairportpower en0"):
		if f.wifi {
			return "Wi-Fi Power (en0): On", nil
		}
		return "Wi-Fi Power (en0): Off", nil
	case strings.Contains(line, "-setairportpower en0 on"):
		f.wifi = true
	case strings.Contains(line, "-setairportpower en0 off"):
		f.wifi = false
	case strings.Contains(line, "set volume output volume "):
		f.volume = line[strings.LastIndex(line, " ")+1:]
	case strings.Contains(line, "output volume of"):
		return f.volume, nil
	case strings.Contains(line, "set volume output muted true"):
		f.muted = true
	case strings.Contains(line, "set volume output muted false"):
		f.muted = false
	case strings.Contains(line, "output muted of"):
		if f.muted {
			return "true", nil
		}
		return "false", nil
	case strings.Contains(line, "set dark mode to true"):
		f.dark = true
	case strings.Contains(line, "set dark mode to false"):
		f.dark = false
	case strings.Contains(line, "get dark mode"):
		if f.dark {
			return "true", nil
		}
		return "false", nil
	}
	return "", nil
}

func TestEachSettingIsReadAndSet(t *testing.T) {
	f := &fakeMac{volume: "30", wifi: true}
	m := Mac{Runner: f}
	ctx := context.Background()

	for _, tc := range []struct{ setting, was, to string }{
		{Volume, "30", "55"},
		{Mute, "off", "on"},
		{Dark, "off", "on"},
		{WiFi, "on", "off"},
	} {
		if got, err := m.Get(ctx, tc.setting); err != nil || got != tc.was {
			t.Errorf("%s: read %q, %v; want %q", tc.setting, got, err, tc.was)
		}
		if err := m.Set(ctx, tc.setting, tc.to); err != nil {
			t.Errorf("%s: Set: %v", tc.setting, err)
		}
		if got, _ := m.Get(ctx, tc.setting); got != tc.to {
			t.Errorf("%s: read back %q after setting %q", tc.setting, got, tc.to)
		}
	}
}

func TestAVolumeOutOfRangeIsNeverSent(t *testing.T) {
	f := &fakeMac{volume: "30"}
	if err := (Mac{Runner: f}).Set(context.Background(), Volume, "140"); err == nil {
		t.Error("a volume of 140 was accepted")
	}
	if len(f.ran) != 0 {
		t.Errorf("ran %q for a value that was refused", f.ran)
	}
}

func TestWhatWasSaidBecomesAValue(t *testing.T) {
	for _, tc := range []struct{ setting, said, was, want string }{
		{Volume, "50%", "30", "50"},
		{Volume, "turn the volume up", "30", "40"},
		{Volume, "quieter", "5", "0"},
		{Volume, "max", "30", "100"},
		{Mute, "mute the sound", "off", "on"},
		{Mute, "unmute", "on", "off"},
		{Dark, "switch to dark mode", "off", "on"},
		{Dark, "light mode please", "on", "off"},
		{Dark, "toggle the appearance", "on", "off"},
		{WiFi, "turn off wifi", "on", "off"},
		{WiFi, "turn wifi on", "off", "on"},
	} {
		got, err := Wanted(tc.setting, tc.said, tc.was)
		if err != nil || got != tc.want {
			t.Errorf("%s %q (was %s): got %q, %v; want %q", tc.setting, tc.said, tc.was, got, err, tc.want)
		}
	}
	if _, err := Wanted(Volume, "set the volume nicely", "30"); err == nil {
		t.Error("a volume with no number and no direction was given a value")
	}
}

func TestOnlyTheFourSettingsAreKnown(t *testing.T) {
	for said, want := range map[string]string{
		"volume": Volume, "mute": Mute, "dark mode": Dark, "wi-fi": WiFi, "wifi": WiFi, "the sound": Volume,
	} {
		if got, ok := Which(said); !ok || got != want {
			t.Errorf("%q: got %q, %v; want %q", said, got, ok, want)
		}
	}
	for _, said := range []string{"bluetooth", "wallpaper", "brightness", "night shift"} {
		if got, ok := Which(said); ok {
			t.Errorf("%q was taken for %q", said, got)
		}
	}
}

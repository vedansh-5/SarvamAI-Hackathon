package engine

import (
	"context"
	"fmt"
	"strings"

	"github.com/vedansh/vans/internal/macset"
)

type MacSettings interface {
	Get(ctx context.Context, setting string) (string, error)
	Set(ctx context.Context, setting, value string) error
}

type SettingPlan struct {
	Setting string
	To      string
	Was     string
	Preview string
}

const whereSettingsLive = "this Mac"

func (e *Engine) canSet() bool { return e.Settings != nil }

func spoken(setting, value string) string {
	switch setting {
	case macset.Volume:
		return value + "%"
	case macset.Dark:
		if value == "on" {
			return "dark"
		}
		return "light"
	}
	return value
}

func nameOf(setting string) string {
	switch setting {
	case macset.Volume:
		return "the volume"
	case macset.Mute:
		return "mute"
	case macset.Dark:
		return "the appearance"
	case macset.WiFi:
		return "Wi-Fi"
	}
	return setting
}

func (e *Engine) planSetting(ctx context.Context, instruction, settingSlot, valueSlot string) (*Plan, error) {
	setting, ok := macset.Which(settingSlot)
	if !ok {
		setting, ok = macset.Which(instruction)
	}
	if !ok {
		return nil, &UnroutableError{
			Instruction: instruction,
			Why:         "Vspk changes the volume, mute, dark or light appearance and Wi-Fi directly; for anything else ask how, and it will show you in System Settings",
			Can:         e.capabilities(),
		}
	}

	was, err := e.Settings.Get(ctx, setting)
	if err != nil {
		return nil, fmt.Errorf("engine: could not read %s: %w", nameOf(setting), err)
	}

	said := strings.TrimSpace(valueSlot)
	if said == "" {
		said = instruction
	}
	to, err := macset.Wanted(setting, said+" "+instruction, was)
	if err != nil {
		return saying("One thing first", strings.ToUpper(err.Error()[:1])+err.Error()[1:]+"."), nil
	}
	if to == was && setting == macset.Dark {
		plan := saying("Already so", fmt.Sprintf("Your Mac is already %s. Vspk is %s now too.", spoken(setting, was), spoken(setting, was)))
		plan.Panel = spoken(setting, was)
		return plan, nil
	}
	if to == was {
		return saying("Already so", fmt.Sprintf("%s is already %s.", capitalised(nameOf(setting)), spoken(setting, was))), nil
	}

	preview := fmt.Sprintf("Set %s to %s. It is %s now.", nameOf(setting), spoken(setting, to), spoken(setting, was))
	if setting == macset.WiFi && to == "off" {
		preview += "\n\nVANS needs the network to work things out, so it can do very little until Wi-Fi is back on."
	}
	preview += "\n\nIt can be put back from History."

	plan := &Plan{
		Task:        Task{Intent: strings.TrimSpace(instruction), Title: whereSettingsLive},
		Destination: whereSettingsLive,
		Setting:     &SettingPlan{Setting: setting, To: to, Was: was, Preview: preview},
	}
	if setting == macset.Dark {
		plan.Panel = spoken(setting, to)
	}
	return plan, nil
}

func capitalised(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (e *Engine) runSetting(ctx context.Context, plan *Plan) (Receipt, error) {
	s := plan.Setting
	if !e.canSet() {
		return Receipt{}, fmt.Errorf("engine: this build cannot change settings")
	}
	if err := e.Settings.Set(ctx, s.Setting, s.To); err != nil {
		return Receipt{}, fmt.Errorf("engine: could not set %s: %w", nameOf(s.Setting), err)
	}
	now, err := e.Settings.Get(ctx, s.Setting)
	did := []string{fmt.Sprintf("Set %s to %s", nameOf(s.Setting), spoken(s.Setting, s.To))}
	e.logf("set %s to %s (it was %s)", nameOf(s.Setting), s.To, s.Was)
	return Receipt{Title: whereSettingsLive, Verified: err == nil && now == s.To, Did: did}, nil
}

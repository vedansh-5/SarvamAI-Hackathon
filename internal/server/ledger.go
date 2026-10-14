package server

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
)

type Memory struct {
	mu      sync.Mutex
	testers map[string]Tester
	counts  map[string]int
	day     string
}

func NewMemory(testers []Tester) *Memory {
	m := &Memory{testers: map[string]Tester{}, counts: map[string]int{}}
	for _, t := range testers {
		m.testers[t.Code] = t
	}
	return m
}

func (m *Memory) Tester(_ context.Context, code string) (Tester, bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.testers[code]
	return t, ok, nil
}

func (m *Memory) Spend(_ context.Context, code, day string) (int, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if day != m.day {
		m.day, m.counts = day, map[string]int{}
	}
	m.counts[code]++
	m.counts[""]++
	return m.counts[code], m.counts[""], nil
}

func ParseTesters(spelled string) ([]Tester, error) {
	var out []Tester
	seen := map[string]bool{}
	for _, entry := range strings.Split(spelled, ";") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, ":")
		if len(parts) != 3 {
			return nil, fmt.Errorf("server: tester %q should read name:code:daily", entry)
		}
		name, code := strings.TrimSpace(parts[0]), strings.TrimSpace(parts[1])
		daily, err := strconv.Atoi(strings.TrimSpace(parts[2]))
		if name == "" || len(code) < 12 || err != nil || daily < 0 {
			return nil, fmt.Errorf("server: tester %q needs a name, a code of 12 or more characters and a daily number", name)
		}
		if seen[code] {
			return nil, fmt.Errorf("server: two testers share a code")
		}
		seen[code] = true
		out = append(out, Tester{Name: name, Code: code, Daily: daily})
	}
	return out, nil
}

package server

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Memory struct {
	mu       sync.Mutex
	testers  map[string]Tester
	counts   map[string]int
	day      string
	feedback []Feedback
	activity []Activity
	learned  map[string]string
	sentAt   map[string]time.Time
}

var (
	ErrNoTester   = errors.New("server: no tester has that code")
	ErrNoFeedback = errors.New("server: no feedback has that id")
)

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

func (m *Memory) Keep(_ context.Context, f Feedback) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	f.ID = strconv.Itoa(len(m.feedback) + 1)
	m.feedback = append(m.feedback, f)
	return nil
}

func (m *Memory) KeepLearned(_ context.Context, from string, at time.Time, habits string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.learned == nil {
		m.learned, m.sentAt = map[string]string{}, map[string]time.Time{}
	}
	m.learned[from] = habits
	m.sentAt[from] = at
	return nil
}

func (m *Memory) Habits(_ context.Context, from string) (Habits, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return habitsFrom(m.sentAt[from], m.learned[from])
}

func (m *Memory) Learned(from string) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.learned[from]
}

func (m *Memory) Kept() []Feedback {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Feedback(nil), m.feedback...)
}

func (m *Memory) Record(_ context.Context, a Activity) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.activity = append(m.activity, a)
	return nil
}

func (m *Memory) Activity(_ context.Context, day string) ([]Activity, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []Activity
	for i := len(m.activity) - 1; i >= 0; i-- {
		if Day(m.activity[i].At) == day {
			out = append(out, m.activity[i])
		}
	}
	return out, nil
}

func (m *Memory) Testers(_ context.Context) ([]Tester, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Tester, 0, len(m.testers))
	for _, t := range m.testers {
		out = append(out, t)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

func (m *Memory) AddTester(_ context.Context, t Tester) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, taken := m.testers[t.Code]; taken {
		return ErrCodeTaken
	}
	m.testers[t.Code] = t
	return nil
}

func (m *Memory) change(code string, f func(*Tester)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.testers[code]
	if !ok {
		return ErrNoTester
	}
	f(&t)
	m.testers[code] = t
	return nil
}

func (m *Memory) SetOff(_ context.Context, code string, off bool) error {
	return m.change(code, func(t *Tester) { t.Off = off })
}

func (m *Memory) SetDaily(_ context.Context, code string, daily int) error {
	return m.change(code, func(t *Tester) { t.Daily = daily })
}

func (m *Memory) Used(_ context.Context, day string) (map[string]int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := map[string]int{}
	if day == m.day {
		for k, v := range m.counts {
			if k == "" {
				k = everyoneKey
			}
			out[k] = v
		}
	}
	return out, nil
}

func (m *Memory) Feedback(_ context.Context) ([]Feedback, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]Feedback, 0, len(m.feedback))
	for i := len(m.feedback) - 1; i >= 0; i-- {
		out = append(out, m.feedback[i])
	}
	return out, nil
}

func (m *Memory) Handled(_ context.Context, id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.feedback {
		if m.feedback[i].ID == id {
			m.feedback[i].Handled = true
			return nil
		}
	}
	return ErrNoFeedback
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

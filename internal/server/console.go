package server

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/vedansh/vans/internal/synth"
)

const (
	EnvConsoleKey  = "VANS_CONSOLE_KEY"
	EnvConsoleGate = "VANS_CONSOLE_GATE"
	GateHeader     = "X-Vans-Console-Gate"
	ConsolePath    = "/console"
	consoleCookie  = "vans_console"
	consoleDays    = 14
	activityDays   = 7
	testerDays     = 30
	testerRows     = 50
)

type Books interface {
	Testers(ctx context.Context) ([]Tester, error)
	AddTester(ctx context.Context, t Tester) error
	SetDaily(ctx context.Context, code string, daily int) error
	SetOff(ctx context.Context, code string, off bool) error
	Used(ctx context.Context, day string) (map[string]int, error)
	Activity(ctx context.Context, day string) ([]Activity, error)
	Feedback(ctx context.Context) ([]Feedback, error)
	Handled(ctx context.Context, id string) error
	Habits(ctx context.Context, from string) (Habits, error)
}

type Habits struct {
	At   time.Time
	List []synth.LearnedHabit
}

func habitsFrom(at time.Time, raw string) (Habits, error) {
	h := Habits{At: at}
	if raw == "" {
		return h, nil
	}
	if err := json.Unmarshal([]byte(raw), &h.List); err != nil {
		return Habits{}, fmt.Errorf("server: the habits kept for this tester are not readable: %w", err)
	}
	return h, nil
}

type Console struct {
	Books  Books
	Key    string
	Gate   string
	Limits map[string]int
	Now    func() time.Time
}

func LimitsFromEnv() map[string]int {
	limits := map[string]int{}
	for _, name := range strings.Split(os.Getenv(synth.EnvProviders), ",") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimSpace(os.Getenv("VANS_PROVIDER_" + strings.ToUpper(name) + "_DAILY"))); err == nil && n > 0 {
			limits[name] = n
		}
	}
	return limits
}

//go:embed console.html
var consolePage string

var consoleTemplate = template.Must(template.New("console").Funcs(template.FuncMap{
	"when": func(t time.Time) string {
		if t.IsZero() {
			return "never"
		}
		return t.In(india).Format("2 Jan 15:04")
	},
	"took": func(d time.Duration) string {
		if d <= 0 {
			return "-"
		}
		if d < time.Second {
			return fmt.Sprintf("%d ms", d.Milliseconds())
		}
		return fmt.Sprintf("%.1f s", d.Seconds())
	},
	"words":    func(s string) string { return strings.ReplaceAll(s, "_", " ") },
	"pct":      func(n, of int) int { return percent(n, of) },
	"subtract": func(a, b int) int { return a - b },
	"mod":      func(a, b int) int { return a % b },
}).Parse(consolePage))

var india = time.FixedZone("IST", 5*3600+1800)

func percent(n, of int) int {
	if of <= 0 {
		return 0
	}
	return n * 100 / of
}

func (c *Console) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

func (c *Console) token() string {
	sum := sha256.Sum256([]byte("vans-console:" + c.Key))
	return hex.EncodeToString(sum[:])
}

func (c *Console) base() string {
	if c.Gate != "" {
		return ""
	}
	return ConsolePath
}

func (c *Console) cookiePath() string {
	if c.Gate != "" {
		return "/"
	}
	return ConsolePath
}

func (c *Console) signedIn(r *http.Request) bool {
	cookie, err := r.Cookie(consoleCookie)
	return err == nil && subtle.ConstantTimeCompare([]byte(cookie.Value), []byte(c.token())) == 1
}

func (c *Console) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; img-src data:; form-action 'self'; frame-ancestors 'none'")

	if c.Gate != "" && subtle.ConstantTimeCompare([]byte(r.Header.Get(GateHeader)), []byte(c.Gate)) != 1 {
		http.NotFound(w, r)
		return
	}
	page := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, ConsolePath), "/")
	if page == "login" && r.Method == http.MethodPost {
		c.login(w, r)
		return
	}
	if !c.signedIn(r) {
		c.render(w, http.StatusUnauthorized, view{Page: "login"})
		return
	}
	if r.Method == http.MethodPost {
		c.act(w, r, page)
		return
	}
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	switch page {
	case "", "overview":
		c.show(w, r, c.overview)
	case "testers":
		c.show(w, r, func(ctx context.Context, v *view) error { return c.testers(ctx, v) })
	case "activity":
		c.show(w, r, func(ctx context.Context, v *view) error { return c.activity(ctx, v, r) })
	case "feedback":
		c.show(w, r, c.feedback)
	default:
		name, ok := strings.CutPrefix(page, "testers/")
		if !ok || name == "" {
			http.NotFound(w, r)
			return
		}
		c.show(w, r, func(ctx context.Context, v *view) error { return c.tester(ctx, v, name) })
	}
}

func (c *Console) login(w http.ResponseWriter, r *http.Request) {
	given := strings.TrimSpace(r.PostFormValue("key"))
	if c.Key == "" || subtle.ConstantTimeCompare([]byte(given), []byte(c.Key)) != 1 {
		c.render(w, http.StatusUnauthorized, view{Page: "login", Error: "That is not the console key."})
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: consoleCookie, Value: c.token(), Path: c.cookiePath(), MaxAge: 30 * 24 * 3600,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
	http.Redirect(w, r, c.base()+"/", http.StatusSeeOther)
}

func (c *Console) act(w http.ResponseWriter, r *http.Request, page string) {
	ctx := r.Context()
	back := c.base() + "/testers"
	var err error
	switch page {
	case "logout":
		http.SetCookie(w, &http.Cookie{Name: consoleCookie, Value: "", Path: c.cookiePath(), MaxAge: -1, HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode})
		http.Redirect(w, r, c.base()+"/", http.StatusSeeOther)
		return
	case "testers/add":
		name := strings.TrimSpace(r.PostFormValue("name"))
		daily, convErr := strconv.Atoi(strings.TrimSpace(r.PostFormValue("daily")))
		if name == "" || convErr != nil || daily < 0 {
			c.show(w, r, func(ctx context.Context, v *view) error {
				v.Error = "A tester needs a name and a daily allowance of 0 or more."
				return c.testers(ctx, v)
			})
			return
		}
		made := Tester{Name: name, Code: NewCode(), Daily: daily}
		if err = c.Books.AddTester(ctx, made); err == nil {
			c.show(w, r, func(ctx context.Context, v *view) error {
				v.Made = &made
				return c.testers(ctx, v)
			})
			return
		}
	case "testers/daily":
		var daily int
		if daily, err = strconv.Atoi(strings.TrimSpace(r.PostFormValue("daily"))); err == nil {
			err = c.Books.SetDaily(ctx, r.PostFormValue("code"), daily)
		}
	case "testers/off", "testers/on":
		err = c.Books.SetOff(ctx, r.PostFormValue("code"), page == "testers/off")
	case "feedback/handled":
		back = c.base() + "/feedback"
		err = c.Books.Handled(ctx, r.PostFormValue("id"))
	default:
		http.NotFound(w, r)
		return
	}
	if err != nil {
		c.render(w, http.StatusBadRequest, view{Page: "error", Error: err.Error()})
		return
	}
	if name := r.PostFormValue("back"); name != "" && strings.HasPrefix(page, "testers/") {
		back = c.base() + "/testers/" + url.PathEscape(name)
	}
	http.Redirect(w, r, back, http.StatusSeeOther)
}

type view struct {
	Base  string
	Page  string
	Error string
	Made  *Tester
	Get   string

	Days      []dayBar
	Today     int
	Finished  int
	Answered  int
	Median    time.Duration
	Providers []providerRow
	Apps      []countRow
	Failures  []countRow

	Testers []testerRow
	Rows    []Activity
	Filter  activityFilter
	Names   []string

	Feedback []Feedback
	Open     int

	Who    *testerRow
	Kinds  []countRow
	Habits Habits
}

type dayBar struct {
	Label  string
	Count  int
	Height int
	X      int
}

type providerRow struct {
	Name   string
	Count  int
	Median time.Duration
	Today  int
	Limit  int
}

type countRow struct {
	Name    string
	Count   int
	Width   int
	Example string
}

type testerRow struct {
	Tester
	Today    int
	LastSeen time.Time
	Week     int
}

type activityFilter struct {
	Tester string
	App    string
	Failed bool
	Days   int
}

func (c *Console) show(w http.ResponseWriter, r *http.Request, fill func(ctx context.Context, v *view) error) {
	page := strings.TrimPrefix(strings.TrimPrefix(r.URL.Path, ConsolePath), "/")
	if page == "" {
		page = "overview"
	}
	if i := strings.Index(page, "/"); i > 0 {
		page = page[:i]
	}
	v := view{Page: page}
	if err := fill(r.Context(), &v); err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, ErrNoTester) {
			status = http.StatusNotFound
		}
		c.render(w, status, view{Page: "error", Error: err.Error()})
		return
	}
	c.render(w, http.StatusOK, v)
}

func (c *Console) render(w http.ResponseWriter, status int, v view) {
	v.Base = c.base()
	v.Get = DownloadLink()
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_ = consoleTemplate.Execute(w, v)
}

func (c *Console) recent(ctx context.Context, days int) ([]Activity, error) {
	var all []Activity
	now := c.now()
	for i := 0; i < days; i++ {
		rows, err := c.Books.Activity(ctx, Day(now.AddDate(0, 0, -i)))
		if err != nil {
			return nil, err
		}
		all = append(all, rows...)
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].At.After(all[j].At) })
	return all, nil
}

func median(ds []time.Duration) time.Duration {
	if len(ds) == 0 {
		return 0
	}
	c := append([]time.Duration(nil), ds...)
	sort.Slice(c, func(i, j int) bool { return c[i] < c[j] })
	return c[len(c)/2]
}

func provider(seat string) string {
	name, _, _ := strings.Cut(seat, "/")
	return name
}

func (c *Console) overview(ctx context.Context, v *view) error {
	now := c.now()
	most := 1
	for i := consoleDays - 1; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		used, err := c.Books.Used(ctx, Day(day))
		if err != nil {
			return err
		}
		v.Days = append(v.Days, dayBar{Label: day.In(india).Format("2 Jan"), Count: used[everyoneKey]})
		most = max(most, used[everyoneKey])
	}
	for i := range v.Days {
		v.Days[i].Height = v.Days[i].Count * 120 / most
		v.Days[i].X = i * 40
	}
	v.Today = v.Days[len(v.Days)-1].Count

	week, err := c.recent(ctx, activityDays)
	if err != nil {
		return err
	}
	today := Day(now)
	times := map[string][]time.Duration{}
	counts, todays, apps, failures := map[string]int{}, map[string]int{}, map[string]int{}, map[string]int{}
	examples := map[string]string{}
	var all []time.Duration
	for _, a := range week {
		v.Answered++
		if a.Fault == "" {
			v.Finished++
			all = append(all, a.Took)
		} else {
			failures[a.Fault]++
			if examples[a.Fault] == "" && a.Words != "" {
				examples[a.Fault] = a.Words
			}
		}
		if a.By != "" {
			counts[a.By]++
			if a.Fault == "" {
				times[a.By] = append(times[a.By], a.Took)
			}
			if Day(a.At) == today {
				todays[a.By]++
			}
		}
		if a.App != "" {
			apps[a.App]++
		}
	}
	v.Median = median(all)
	for seat, n := range counts {
		v.Providers = append(v.Providers, providerRow{Name: seat, Count: n, Median: median(times[seat]), Today: todays[seat], Limit: c.Limits[provider(seat)]})
	}
	sort.Slice(v.Providers, func(i, j int) bool { return v.Providers[i].Count > v.Providers[j].Count })
	v.Apps = ranked(apps, nil, 8)
	v.Failures = ranked(failures, examples, 6)
	return nil
}

func ranked(counts map[string]int, examples map[string]string, keep int) []countRow {
	var rows []countRow
	most := 1
	for name, n := range counts {
		rows = append(rows, countRow{Name: name, Count: n, Example: examples[name]})
		most = max(most, n)
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Count != rows[j].Count {
			return rows[i].Count > rows[j].Count
		}
		return rows[i].Name < rows[j].Name
	})
	if len(rows) > keep {
		rows = rows[:keep]
	}
	for i := range rows {
		rows[i].Width = rows[i].Count * 100 / most
	}
	return rows
}

func (c *Console) testers(ctx context.Context, v *view) error {
	testers, err := c.Books.Testers(ctx)
	if err != nil {
		return err
	}
	used, err := c.Books.Used(ctx, Day(c.now()))
	if err != nil {
		return err
	}
	week, err := c.recent(ctx, activityDays)
	if err != nil {
		return err
	}
	seen, asked := map[string]time.Time{}, map[string]int{}
	for _, a := range week {
		asked[a.From]++
		if a.At.After(seen[a.From]) {
			seen[a.From] = a.At
		}
	}
	for _, t := range testers {
		v.Testers = append(v.Testers, testerRow{Tester: t, Today: used[t.Code], LastSeen: seen[t.Name], Week: asked[t.Name]})
	}
	return nil
}

func (c *Console) tester(ctx context.Context, v *view, name string) error {
	v.Page = "tester"
	testers, err := c.Books.Testers(ctx)
	if err != nil {
		return err
	}
	for _, t := range testers {
		if t.Name == name {
			v.Who = &testerRow{Tester: t}
			break
		}
	}
	if v.Who == nil {
		return fmt.Errorf("%w called %q", ErrNoTester, name)
	}

	now := c.now()
	most := 1
	for i := testerDays - 1; i >= 0; i-- {
		day := now.AddDate(0, 0, -i)
		used, err := c.Books.Used(ctx, Day(day))
		if err != nil {
			return err
		}
		n := used[v.Who.Code]
		v.Days = append(v.Days, dayBar{Label: day.In(india).Format("2 Jan"), Count: n})
		most = max(most, n)
	}
	for i := range v.Days {
		v.Days[i].Height = v.Days[i].Count * 120 / most
		v.Days[i].X = i * 36
	}
	v.Who.Today = v.Days[len(v.Days)-1].Count

	all, err := c.recent(ctx, testerDays)
	if err != nil {
		return err
	}
	apps, failures, kinds := map[string]int{}, map[string]int{}, map[string]int{}
	examples := map[string]string{}
	var times []time.Duration
	for _, a := range all {
		if a.From != name {
			continue
		}
		v.Rows = append(v.Rows, a)
		v.Answered++
		if a.At.After(v.Who.LastSeen) {
			v.Who.LastSeen = a.At
		}
		if now.Sub(a.At) < activityDays*24*time.Hour {
			v.Who.Week++
		}
		if a.Fault == "" {
			v.Finished++
			times = append(times, a.Took)
		} else {
			failures[a.Fault]++
			if examples[a.Fault] == "" && a.Words != "" {
				examples[a.Fault] = a.Words
			}
		}
		if a.App != "" {
			apps[a.App]++
		}
		if a.Kind != "" {
			kinds[a.Kind]++
		}
	}
	v.Median = median(times)
	v.Apps = ranked(apps, nil, 8)
	v.Failures = ranked(failures, examples, 6)
	v.Kinds = ranked(kinds, nil, 8)
	if len(v.Rows) > testerRows {
		v.Rows = v.Rows[:testerRows]
	}

	v.Habits, err = c.Books.Habits(ctx, name)
	return err
}

func (c *Console) activity(ctx context.Context, v *view, r *http.Request) error {
	q := r.URL.Query()
	f := activityFilter{Tester: q.Get("tester"), App: q.Get("app"), Failed: q.Get("failed") == "1", Days: 1}
	if n, err := strconv.Atoi(q.Get("days")); err == nil && n >= 1 && n <= activityDays {
		f.Days = n
	}
	v.Filter = f
	rows, err := c.recent(ctx, f.Days)
	if err != nil {
		return err
	}
	names := map[string]bool{}
	for _, a := range rows {
		names[a.From] = true
		if f.Tester != "" && a.From != f.Tester {
			continue
		}
		if f.App != "" && !strings.EqualFold(a.App, f.App) {
			continue
		}
		if f.Failed && a.Fault == "" {
			continue
		}
		v.Rows = append(v.Rows, a)
	}
	for name := range names {
		v.Names = append(v.Names, name)
	}
	sort.Strings(v.Names)
	if len(v.Rows) > 500 {
		v.Rows = v.Rows[:500]
	}
	return nil
}

func (c *Console) feedback(ctx context.Context, v *view) error {
	all, err := c.Books.Feedback(ctx)
	if err != nil {
		return err
	}
	v.Feedback = all
	for _, f := range all {
		if !f.Handled {
			v.Open++
		}
	}
	return nil
}

func ConsoleFor(ledger Ledger) http.Handler {
	key := strings.TrimSpace(os.Getenv(EnvConsoleKey))
	books, ok := ledger.(Books)
	if key == "" || !ok {
		return nil
	}
	return &Console{Books: books, Key: key, Gate: strings.TrimSpace(os.Getenv(EnvConsoleGate)), Limits: LimitsFromEnv()}
}

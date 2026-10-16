package eyes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
	"strings"
	"sync"
	"time"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

type Camera interface {
	Shot(ctx context.Context, app string) (uiact.Shot, error)
}

type Reader interface {
	SeeScreen(ctx context.Context, req synth.SeeRequest) (synth.SeeAnswer, error)
}

type Screen struct {
	Camera Camera
	Reader Reader
	Words  func(picture []byte) ([]uiact.Word, error)
	Menus  func(ctx context.Context, app string) ([]uiact.Element, error)
	Hands  func(ctx context.Context, p uiact.Program, t uiact.Tree) error
	Typist func(ctx context.Context, text string) error
	Raise  func(ctx context.Context, app string) error
	Log    func(format string, args ...any)

	mu      sync.Mutex
	app     string
	seen    synth.SeeAnswer
	looked  signature
	at      time.Time
	have    bool
	looking bool

	words     []uiact.Word
	read      signature
	haveWords bool

	menus   []uiact.Element
	menusAt time.Time
}

var ErrOnlyPoints = errors.New("eyes: a window that is only looked at can be pointed at, not acted on")

var (
	grid          = 96
	cellMoved     = 28
	cellsThatMean = 4
	Rest          = 1200 * time.Millisecond
)

const WindowArea = "AXWindow"

type signature []uint8

func sign(data []byte) (signature, error) {
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("eyes: the picture of the window could not be read: %w", err)
	}
	return signOf(img), nil
}

func signOf(img image.Image) signature {
	b := img.Bounds()
	out := make(signature, 0, grid*grid)
	for gy := 0; gy < grid; gy++ {
		for gx := 0; gx < grid; gx++ {
			x := b.Min.X + (gx*2+1)*b.Dx()/(grid*2)
			y := b.Min.Y + (gy*2+1)*b.Dy()/(grid*2)
			r, g, bl, _ := img.At(x, y).RGBA()
			out = append(out, uint8((299*(r>>8)+587*(g>>8)+114*(bl>>8))/1000))
		}
	}
	return out
}

func (a signature) differsFrom(b signature) bool {
	if len(a) != len(b) {
		return true
	}
	moved := 0
	for i := range a {
		d := int(a[i]) - int(b[i])
		if d < 0 {
			d = -d
		}
		if d > cellMoved {
			moved++
			if moved >= cellsThatMean {
				return true
			}
		}
	}
	return false
}

func (s *Screen) logf(format string, args ...any) {
	if s.Log != nil {
		s.Log(format, args...)
	}
}

var (
	wordsEnough  = 5
	menusLastFor = 10 * time.Second
)

const wordPaths = 10000

func (s *Screen) Capture(ctx context.Context, app string) (uiact.Tree, error) {
	if s.Camera == nil || (s.Reader == nil && s.Words == nil) {
		return uiact.Tree{}, fmt.Errorf("eyes: this build cannot look at a window")
	}
	shot, err := s.Camera.Shot(ctx, app)
	if err != nil {
		if errors.Is(err, uiact.ErrCannotSee) {
			return uiact.Tree{}, fmt.Errorf("%w. %s", err, uiact.CannotSeeFix)
		}
		return uiact.Tree{}, err
	}
	now, err := sign(shot.JPEG)
	if err != nil {
		return uiact.Tree{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	if s.app != app {
		s.app, s.have, s.haveWords, s.menus, s.menusAt = app, false, false, nil, time.Time{}
	}

	if s.Words != nil && (!s.haveWords || now.differsFrom(s.read)) {
		started := time.Now()
		picture := shot.Full
		if len(picture) == 0 {
			picture = shot.JPEG
		}
		if words, err := s.Words(picture); err == nil {
			s.words, s.read, s.haveWords = words, now, true
			s.logf("read %d piece(s) of text in %s in %s", len(words), app, time.Since(started).Round(time.Millisecond))
		} else {
			s.logf("could not read the words in %s: %v", app, err)
		}
	}

	if s.Menus != nil && time.Since(s.menusAt) > menusLastFor {
		if menus, err := s.Menus(ctx, app); err == nil {
			s.menus, s.menusAt = menus, time.Now()
		}
	}

	stale := !s.have || now.differsFrom(s.looked)
	if s.Reader != nil && stale && (!s.have || time.Since(s.at) >= Rest) {
		if len(s.words) >= wordsEnough {
			if !s.looking {
				s.looking = true
				go s.lookInTheBackground(context.WithoutCancel(ctx), app, shot, now)
			}
		} else {
			started := time.Now()
			seen, err := s.Reader.SeeScreen(ctx, synth.SeeRequest{App: app, JPEG: shot.JPEG})
			if err != nil {
				if !s.haveWords {
					return uiact.Tree{}, err
				}
				s.logf("could not look at %s: %v", app, err)
			} else {
				s.logf("looked at %s and found %d control(s) in %s", app, len(seen.Controls), time.Since(started).Round(10*time.Millisecond))
				s.seen, s.looked, s.at, s.have, stale = seen, now, time.Now(), true, false
			}
		}
	}

	var seen synth.SeeAnswer
	if s.have && !stale {
		seen = s.seen
	}
	return treeOf(app, seen, shot, s.words, s.menus), nil
}

func (s *Screen) lookInTheBackground(ctx context.Context, app string, shot uiact.Shot, sig signature) {
	started := time.Now()
	seen, err := s.Reader.SeeScreen(ctx, synth.SeeRequest{App: app, JPEG: shot.JPEG})
	s.mu.Lock()
	defer s.mu.Unlock()
	s.looking = false
	if err != nil {
		s.logf("could not look at %s: %v", app, err)
		return
	}
	if s.app != app {
		return
	}
	s.logf("looked at %s and found %d control(s) in %s", app, len(seen.Controls), time.Since(started).Round(10*time.Millisecond))
	s.seen, s.looked, s.at, s.have = seen, sig, time.Now(), true
}

var roles = map[string]string{
	"button": "AXButton", "field": "AXTextField", "link": "AXLink", "tab": "AXTab",
	"menu": "AXMenuItem", "checkbox": "AXCheckBox", "item": "AXRow",
}

func treeOf(app string, seen synth.SeeAnswer, shot uiact.Shot, words []uiact.Word, menus []uiact.Element) uiact.Tree {
	window := seen.Window
	if window == "" {
		window = app
	}
	tree := uiact.Tree{App: app, Window: window}
	tree.Elements = append(tree.Elements, uiact.Element{
		Role: WindowArea, Enabled: true,
		Position: []int{shot.X, shot.Y}, Size: []int{shot.W, shot.H}, Path: []int{},
	})
	tree.Elements = append(tree.Elements, menus...)

	first := len(tree.Elements)
	for i, w := range words {
		tree.Elements = append(tree.Elements, uiact.Element{
			Role: "AXStaticText", Title: w.Text, Enabled: true,
			Position: []int{shot.X + int(w.X*float64(shot.W)), shot.Y + int(w.Y*float64(shot.H))},
			Size:     []int{max(1, int(w.W*float64(shot.W))), max(1, int(w.H*float64(shot.H)))},
			Path:     []int{wordPaths + i},
		})
	}
	taken := map[int]bool{}

	for i, c := range seen.Controls {
		role, ok := roles[c.Kind]
		if !ok {
			role = "AXButton"
		}
		x, y := shot.X+c.X0*shot.W/1000, shot.Y+c.Y0*shot.H/1000
		w, h := max(1, (c.X1-c.X0)*shot.W/1000), max(1, (c.Y1-c.Y0)*shot.H/1000)
		if at := wordFor(tree.Elements, first, taken, c.Name, x, y, w, h); at >= 0 {
			taken[at] = true
			tree.Elements[at].Role = role
			tree.Elements[at].Value = c.Value
			continue
		}
		tree.Elements = append(tree.Elements, uiact.Element{
			Role: role, Title: c.Name, Value: c.Value, Enabled: true,
			Position: []int{x, y}, Size: []int{w, h},
			Path: []int{i}, Estimated: true,
		})
	}
	return tree
}

func wordFor(elements []uiact.Element, first int, taken map[int]bool, name string, x, y, w, h int) int {
	if strings.TrimSpace(name) == "" {
		return -1
	}
	best, bestGap := -1, 0
	for i := first; i < len(elements); i++ {
		el := elements[i]
		if taken[i] || el.Role != "AXStaticText" || !uiact.SameLabel(el.Title, name) {
			continue
		}
		cx, cy := el.Position[0]+el.Size[0]/2, el.Position[1]+el.Size[1]/2
		gap := max(0, x-cx, cx-(x+w)) + max(0, y-cy, cy-(y+h))
		if gap > 3*max(h, el.Size[1]) {
			continue
		}
		if best < 0 || gap < bestGap {
			best, bestGap = i, gap
		}
	}
	return best
}

func (s *Screen) Act(ctx context.Context, p uiact.Program, t uiact.Tree) error {
	if s.Hands == nil {
		return ErrOnlyPoints
	}
	return s.Hands(ctx, p, t)
}

func (s *Screen) Type(ctx context.Context, text string) error {
	if s.Typist == nil {
		return ErrOnlyPoints
	}
	return s.Typist(ctx, text)
}

func (s *Screen) Frontmost(context.Context) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.app, nil
}

func (s *Screen) BringToFront(ctx context.Context, app string) error {
	if s.Raise == nil {
		return nil
	}
	return s.Raise(ctx, app)
}

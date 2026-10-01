package eyes

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"image"
	"image/jpeg"
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
	Raise  func(ctx context.Context, app string) error
	Log    func(format string, args ...any)

	mu     sync.Mutex
	app    string
	seen   synth.SeeAnswer
	looked signature
	at     time.Time
	have   bool
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

func (s *Screen) Capture(ctx context.Context, app string) (uiact.Tree, error) {
	if s.Camera == nil || s.Reader == nil {
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

	fresh := !s.have || s.app != app
	if !fresh && now.differsFrom(s.looked) && time.Since(s.at) >= Rest {
		fresh = true
	}
	if fresh {
		started := time.Now()
		seen, err := s.Reader.SeeScreen(ctx, synth.SeeRequest{App: app, JPEG: shot.JPEG})
		if err != nil {
			return uiact.Tree{}, err
		}
		s.logf("looked at %s and found %d control(s) in %s", app, len(seen.Controls), time.Since(started).Round(10*time.Millisecond))
		s.app, s.seen, s.looked, s.at, s.have = app, seen, now, time.Now(), true
	}
	return treeOf(app, s.seen, shot), nil
}

var roles = map[string]string{
	"button": "AXButton", "field": "AXTextField", "link": "AXLink", "tab": "AXTab",
	"menu": "AXMenuItem", "checkbox": "AXCheckBox", "item": "AXRow",
}

func treeOf(app string, seen synth.SeeAnswer, shot uiact.Shot) uiact.Tree {
	window := seen.Window
	if window == "" {
		window = app
	}
	tree := uiact.Tree{App: app, Window: window}
	tree.Elements = append(tree.Elements, uiact.Element{
		Role: WindowArea, Enabled: true,
		Position: []int{shot.X, shot.Y}, Size: []int{shot.W, shot.H}, Path: []int{},
	})
	for i, c := range seen.Controls {
		role, ok := roles[c.Kind]
		if !ok {
			role = "AXButton"
		}
		tree.Elements = append(tree.Elements, uiact.Element{
			Role: role, Title: c.Name, Value: c.Value, Enabled: true,
			Position: []int{shot.X + c.X0*shot.W/1000, shot.Y + c.Y0*shot.H/1000},
			Size:     []int{max(1, (c.X1-c.X0)*shot.W/1000), max(1, (c.Y1-c.Y0)*shot.H/1000)},
			Path:     []int{i},
		})
	}
	return tree
}

func (s *Screen) Act(context.Context, uiact.Program, uiact.Tree) error { return ErrOnlyPoints }

func (s *Screen) Type(context.Context, string) error { return ErrOnlyPoints }

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

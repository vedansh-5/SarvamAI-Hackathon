package eyes

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/jpeg"
	"testing"
	"time"

	"github.com/vedansh/vans/internal/synth"
	"github.com/vedansh/vans/internal/uiact"
)

func picture(t *testing.T, marks ...image.Rectangle) []byte {
	t.Helper()
	img := image.NewGray(image.Rect(0, 0, 960, 600))
	for i := range img.Pix {
		img.Pix[i] = 230
	}
	for _, m := range marks {
		for y := m.Min.Y; y < m.Max.Y; y++ {
			for x := m.Min.X; x < m.Max.X; x++ {
				img.SetGray(x, y, color.Gray{Y: 20})
			}
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type camera struct {
	jpeg  []byte
	shots int
}

func (c *camera) Shot(context.Context, string) (uiact.Shot, error) {
	c.shots++
	return uiact.Shot{JPEG: c.jpeg, X: 100, Y: 50, W: 800, H: 500}, nil
}

type reader struct{ looks int }

func (r *reader) SeeScreen(context.Context, synth.SeeRequest) (synth.SeeAnswer, error) {
	r.looks++
	return synth.SeeAnswer{Window: "Editing", Controls: []synth.Seen{
		{Kind: "button", Name: "Export", X0: 900, Y0: 20, X1: 980, Y1: 60},
		{Kind: "field", Name: "Search", Value: "luts", X0: 100, Y0: 20, X1: 400, Y1: 60},
	}}, nil
}

func quick(t *testing.T) {
	t.Helper()
	was := Rest
	Rest = 0
	t.Cleanup(func() { Rest = was })
}

func TestAWindowIsReadFromItsPictureWithControlsWhereTheyAreOnScreen(t *testing.T) {
	quick(t)
	s := &Screen{Camera: &camera{jpeg: picture(t)}, Reader: &reader{}}

	tree, err := s.Capture(context.Background(), "Lightroom")
	if err != nil {
		t.Fatalf("Capture: %v", err)
	}
	if tree.App != "Lightroom" || tree.Window != "Editing" || len(tree.Elements) != 3 {
		t.Fatalf("tree = %+v", tree)
	}
	x, y, w, h, why := tree.RectFor(uiact.Selector{Role: "AXButton", Title: "Export"})
	if why != "" || x != 820 || y != 60 || w != 64 || h != 20 {
		t.Errorf("Export at (%d,%d %dx%d) %q, want its place on screen worked out from the picture", x, y, w, h, why)
	}
	if err := uiact.Validate(uiact.Program{App: "Lightroom", Steps: []uiact.Step{
		{Do: "type", Target: uiact.Selector{Role: "AXTextField", Title: "Search"}, Text: "x"},
	}}, tree); err != nil {
		t.Errorf("a field that was seen cannot be named in a step: %v", err)
	}
}

func TestThePictureIsOnlyReadAgainWhenTheWindowHasChanged(t *testing.T) {
	quick(t)
	cam := &camera{jpeg: picture(t)}
	r := &reader{}
	s := &Screen{Camera: cam, Reader: r}
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if _, err := s.Capture(ctx, "Lightroom"); err != nil {
			t.Fatal(err)
		}
	}
	if r.looks != 1 || cam.shots != 5 {
		t.Fatalf("looked %d time(s) over %d picture(s) of a window that never changed, want 1", r.looks, cam.shots)
	}

	cam.jpeg = picture(t, image.Rect(300, 200, 420, 240))
	if _, err := s.Capture(ctx, "Lightroom"); err != nil {
		t.Fatal(err)
	}
	if r.looks != 2 {
		t.Errorf("looked %d time(s) after a panel appeared, want 2", r.looks)
	}
}

func TestATextCursorBlinkingIsNotAChange(t *testing.T) {
	quick(t)
	cam := &camera{jpeg: picture(t)}
	r := &reader{}
	s := &Screen{Camera: cam, Reader: r}
	ctx := context.Background()
	_, _ = s.Capture(ctx, "Lightroom")

	cam.jpeg = picture(t, image.Rect(200, 300, 202, 316))
	_, _ = s.Capture(ctx, "Lightroom")
	if r.looks != 1 {
		t.Errorf("looked again for a two-pixel cursor; that would be a model call twice a second")
	}
}

func TestLookingIsRateLimited(t *testing.T) {
	was := Rest
	Rest = time.Hour
	t.Cleanup(func() { Rest = was })

	cam := &camera{jpeg: picture(t)}
	r := &reader{}
	s := &Screen{Camera: cam, Reader: r}
	ctx := context.Background()
	_, _ = s.Capture(ctx, "Lightroom")
	cam.jpeg = picture(t, image.Rect(300, 200, 420, 240))
	_, _ = s.Capture(ctx, "Lightroom")
	if r.looks != 1 {
		t.Errorf("looked %d time(s) inside the rest between looks", r.looks)
	}
}

func TestAWindowThatIsOnlySeenIsNeverActedOn(t *testing.T) {
	s := &Screen{}
	if err := s.Act(context.Background(), uiact.Program{}, uiact.Tree{}); err == nil {
		t.Fatal("acted through a picture")
	}
}

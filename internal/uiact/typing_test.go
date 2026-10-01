package uiact

import (
	"context"
	"errors"
	"testing"
	"time"
)

type field struct {
	value         string
	hiddenEmpty   bool
	unreadable    bool
	takesKeys     bool
	takesPaste    bool
	keyed, pasted int
}

func typingInto(t *testing.T, f *field) {
	t.Helper()
	val, post, paste, settle := valueReader, textPoster, paster, typingSettle
	valueReader = func(int, []int) (string, bool) {
		if f.unreadable || (f.hiddenEmpty && f.value == "") {
			return "", false
		}
		return f.value, true
	}
	textPoster = func(text string) {
		f.keyed++
		if f.takesKeys {
			f.value += text
		}
	}
	paster = func(_ context.Context, text string) bool {
		f.pasted++
		if f.takesPaste {
			f.value += text
		}
		return true
	}
	typingSettle = time.Millisecond
	t.Cleanup(func() { valueReader, textPoster, paster, typingSettle = val, post, paste, settle })
}

func hello() Act { return Act{Do: "type", Text: "hello", Path: []int{0, 1}} }

func TestKeystrokesThatLandAreAllThatIsSent(t *testing.T) {
	f := &field{takesKeys: true, hiddenEmpty: true}
	typingInto(t, f)
	if err := typeInto(context.Background(), 1, hello()); err != nil {
		t.Fatalf("typeInto: %v", err)
	}
	if f.value != "hello" || f.pasted != 0 {
		t.Errorf("field = %+v, want the text in once and no paste", f)
	}
}

func TestPasteIsTriedWhenTheFieldCanBeReadAndTheTextIsNotInIt(t *testing.T) {
	f := &field{takesPaste: true}
	typingInto(t, f)
	if err := typeInto(context.Background(), 1, hello()); err != nil || f.value != "hello" {
		t.Errorf("err=%v field=%+v, want the paste to have put it in", err, f)
	}
}

func TestTextThatNeverShowsUpInAReadableFieldIsAFailure(t *testing.T) {
	f := &field{}
	typingInto(t, f)
	if err := typeInto(context.Background(), 1, hello()); !errors.Is(err, ErrTextDidNotLand) {
		t.Fatalf("err = %v, want to be told the text did not land", err)
	}
}

func TestAFieldThatCannotBeReadIsNotTypedIntoTwice(t *testing.T) {
	f := &field{unreadable: true, takesKeys: true}
	typingInto(t, f)
	if err := typeInto(context.Background(), 1, hello()); err != nil {
		t.Fatalf("typeInto: %v", err)
	}
	if f.keyed != 1 || f.pasted != 0 {
		t.Errorf("tried %+v, want the keystrokes once and nothing on top of them", f)
	}
}

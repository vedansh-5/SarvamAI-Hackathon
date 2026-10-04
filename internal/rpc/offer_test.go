package rpc

import (
	"testing"

	"github.com/vedansh/vans/internal/engine"
)

func TestAnAnswerToTheOfferReachesTheRunAsWhatWasSaid(t *testing.T) {
	for _, c := range []struct {
		said AnswerGuideParams
		want engine.OfferReply
	}{
		{AnswerGuideParams{Yes: true}, engine.OfferTaken},
		{AnswerGuideParams{Yes: true, All: true}, engine.OfferTakenForAll},
		{AnswerGuideParams{}, engine.OfferDeclined},
		{AnswerGuideParams{All: true}, engine.OfferDeclined},
	} {
		reply := make(chan engine.OfferReply, 1)
		s := &Server{guideReply: reply}

		got, err := s.answerGuide(req(1, "answer_guide", c.said))
		if err != nil || !got.Heard {
			t.Fatalf("answerGuide(%+v) = %+v, %v", c.said, got, err)
		}
		if heard := <-reply; heard != c.want {
			t.Errorf("%+v reached the run as %v, want %v", c.said, heard, c.want)
		}
	}
}

func TestAnAnswerWithNoRunToHearItIsNotAnError(t *testing.T) {
	got, err := (&Server{}).answerGuide(req(1, "answer_guide", AnswerGuideParams{Yes: true, All: true}))
	if err != nil || got.Heard {
		t.Errorf("answerGuide with no run = %+v, %v, want it unheard and no error", got, err)
	}
}

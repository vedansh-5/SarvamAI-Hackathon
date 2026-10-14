package rpc

import (
	"context"
	"testing"

	"github.com/vedansh/vans/internal/synth"
)

type outbox struct{ sent []synth.Feedback }

func (o *outbox) SendFeedback(_ context.Context, f synth.Feedback) error {
	o.sent = append(o.sent, f)
	return nil
}

func TestFeedbackIsSentOnlyWhenThereIsAServer(t *testing.T) {
	without := run(t, nil,
		req(1, "can_send_feedback", nil),
		req(2, "send_feedback", synth.Feedback{Kind: synth.FeedbackNote, Text: "hello"}),
	)
	if got := decode[map[string]bool](t, without.ok(1)); got["ready"] {
		t.Error("a copy with no server says it can send feedback")
	}
	if e := without.failure(2); e.Code != CodeBadRequest {
		t.Errorf("code = %s", e.Code)
	}

	box := &outbox{}
	with := run(t, func(s *Server, _ *session) { s.Feedback = box },
		req(1, "can_send_feedback", nil),
		req(2, "send_feedback", synth.Feedback{Kind: synth.FeedbackFailedGuide, Text: "  Asked: turn on dark mode  "}),
		req(3, "send_feedback", synth.Feedback{Kind: synth.FeedbackNote, Text: "   "}),
	)
	if got := decode[map[string]bool](t, with.ok(1)); !got["ready"] {
		t.Error("a copy with a server says it cannot send feedback")
	}
	with.ok(2)
	if len(box.sent) != 1 || box.sent[0].Text != "Asked: turn on dark mode" || box.sent[0].Kind != synth.FeedbackFailedGuide {
		t.Errorf("sent = %+v", box.sent)
	}
	with.failure(3)
}

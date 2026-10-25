package engine

import (
	"testing"

	"github.com/vedansh/vans/internal/route"
)

func TestQuotedTextStaysQuotedInTheTaskTheModelIsGiven(t *testing.T) {
	r := route.Route{
		Kind:      route.KindAppAction,
		Slots:     map[string]string{"app": "whatsapp", "action": "send sent from Vspk to suhani"},
		Templates: map[string]string{"app": "whatsapp", "action": "send {s0} to suhani"},
		Literals:  route.Literals{"s0": "sent from Vspk"},
	}
	if got := theTask(r); got != `send "sent from Vspk" to suhani` {
		t.Errorf("task = %q; without the quotes \"sent\" reads as the message and the rest as the sentence", got)
	}
}

func TestATaskWithNothingQuotedIsLeftAsItWas(t *testing.T) {
	r := route.Route{
		Slots:     map[string]string{"action": "open the top 3 chats"},
		Templates: map[string]string{"action": "open the top {n0} chats"},
		Literals:  route.Literals{"n0": "3"},
	}
	if got := theTask(r); got != "open the top 3 chats" {
		t.Errorf("task = %q", got)
	}
	plain := route.Route{Slots: map[string]string{"action": "say hello"}}
	if got := theTask(plain); got != "say hello" {
		t.Errorf("task = %q", got)
	}
}

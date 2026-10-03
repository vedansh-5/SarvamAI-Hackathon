package synth

import "testing"

func TestAQuestionWrittenAsTheReasonIsStillAQuestion(t *testing.T) {
	for _, why := range []string{
		"Please provide the file path of the video you want to upload.",
		"Which video file should be uploaded?",
		"I need to know the departure city, destination and date.",
		"Let me know which contact you mean.",
	} {
		if questionIn(why) == nil {
			t.Errorf("%q is a question to the person and was thrown away", why)
		}
	}
	for _, why := range []string{
		"",
		"The extensions panel is open and python has been searched for.",
		"Clicking Search to run the query.",
		"Nothing on screen opens the settings.",
	} {
		if q := questionIn(why); q != nil {
			t.Errorf("%q was put to the person as a question", why)
		}
	}
}

func TestAnAnswerIsReadByTheKindItSaysItIs(t *testing.T) {
	for name, tc := range map[string]struct {
		body string
		want func(StepAnswer, error) bool
	}{
		"a step": {`{"next":"step","step":{"do":"click","target":{"role":"AXButton","title":"Send"},"describe":"Click Send"},"why":"send it"}`,
			func(a StepAnswer, err error) bool { return err == nil && a.Step.Do == "click" && !a.Done }},
		"done with a padded step": {`{"next":"done","step":{"do":"click","target":{"role":""}},"why":"the message is there"}`,
			func(a StepAnswer, err error) bool { return err == nil && a.Done && a.Step.Do == "" }},
		"stuck with a padded step": {`{"next":"stuck","step":{"do":"click","target":{"role":""}},"why":"nothing here books a table"}`,
			func(a StepAnswer, err error) bool { return err == nil && a.Stuck }},
		"a question in ask": {`{"next":"ask","step":null,"why":"need it","ask":{"question":"Which file?","options":[]}}`,
			func(a StepAnswer, err error) bool {
				return err == nil && a.Question != nil && a.Question.Ask == "Which file?"
			}},
		"a question left in why": {`{"next":"ask","step":null,"why":"The date of travel is not given"}`,
			func(a StepAnswer, err error) bool { return err == nil && a.Question != nil }},
		"a step that is missing": {`{"next":"step","step":null,"why":"click send"}`,
			func(a StepAnswer, err error) bool { return err != nil }},
		"the older shape": {`{"done":true,"why":"all there"}`,
			func(a StepAnswer, err error) bool { return err == nil && a.Done }},
	} {
		got, err := decodeStep(tc.body)
		if !tc.want(got, err) {
			t.Errorf("%s: got %+v, %v", name, got, err)
		}
	}
}

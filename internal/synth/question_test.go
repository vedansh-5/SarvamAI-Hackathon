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

package synth

import (
	"encoding/json"
	"errors"
	"reflect"
	"testing"

	"github.com/vedansh/vans/internal/answer"
	"github.com/vedansh/vans/internal/appact"
	"github.com/vedansh/vans/internal/extract"
	"github.com/vedansh/vans/internal/route"
	"github.com/vedansh/vans/internal/uiact"
	"github.com/vedansh/vans/internal/webact"
)

func filled(t *testing.T, v reflect.Value, path string, depth int) {
	t.Helper()
	if depth > 6 {
		return
	}
	switch v.Kind() {
	case reflect.String:
		v.SetString("x-" + path)
	case reflect.Bool:
		v.SetBool(true)
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		v.SetInt(3)
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		v.SetUint(3)
	case reflect.Float32, reflect.Float64:
		v.SetFloat(1.5)
	case reflect.Slice:
		s := reflect.MakeSlice(v.Type(), 1, 1)
		filled(t, s.Index(0), path+"[]", depth+1)
		v.Set(s)
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			filled(t, v.Index(i), path, depth+1)
		}
	case reflect.Map:
		m := reflect.MakeMap(v.Type())
		k := reflect.New(v.Type().Key()).Elem()
		filled(t, k, path+"{k}", depth+1)
		e := reflect.New(v.Type().Elem()).Elem()
		filled(t, e, path+"{v}", depth+1)
		m.SetMapIndex(k, e)
		v.Set(m)
	case reflect.Pointer:
		p := reflect.New(v.Type().Elem())
		filled(t, p.Elem(), path, depth+1)
		v.Set(p)
	case reflect.Struct:
		if v.Type().PkgPath() == "time" {
			return
		}
		for i := 0; i < v.NumField(); i++ {
			f := v.Type().Field(i)
			if !f.IsExported() {
				continue
			}
			filled(t, v.Field(i), path+"."+f.Name, depth+1)
		}
	case reflect.Interface, reflect.Func, reflect.Chan:
		t.Errorf("%s is a %s, which cannot travel to the server", path, v.Kind())
	}
}

func survives[T any](t *testing.T, name string) {
	t.Helper()
	var sent T
	filled(t, reflect.ValueOf(&sent).Elem(), name, 0)
	data, err := json.Marshal(sent)
	if err != nil {
		t.Fatalf("%s: encode: %v", name, err)
	}
	var got T
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("%s: decode: %v", name, err)
	}
	if !reflect.DeepEqual(sent, got) {
		t.Errorf("%s does not arrive as it was sent:\nsent %+v\n got %+v", name, sent, got)
	}
}

func TestEveryQuestionAndAnswerSurvivesTheTrip(t *testing.T) {
	survives[Request](t, "Request")
	survives[extract.Program](t, "extract.Program")
	survives[route.Request](t, "route.Request")
	survives[route.Decision](t, "route.Decision")
	survives[AnswerRequest](t, "AnswerRequest")
	survives[answer.Answer](t, "answer.Answer")
	survives[AppActRequest](t, "AppActRequest")
	survives[appact.Program](t, "appact.Program")
	survives[WebActRequest](t, "WebActRequest")
	survives[webact.Program](t, "webact.Program")
	survives[UIActRequest](t, "UIActRequest")
	survives[uiact.Program](t, "uiact.Program")
	survives[StepRequest](t, "StepRequest")
	survives[StepAnswer](t, "StepAnswer")
	survives[PlanRequest](t, "PlanRequest")
	survives[PlanAnswer](t, "PlanAnswer")
	survives[StyleRequest](t, "StyleRequest")
	survives[StyleAnswer](t, "StyleAnswer")
	survives[AppChoiceRequest](t, "AppChoiceRequest")
	survives[AppChoice](t, "AppChoice")
	survives[TextRequest](t, "TextRequest")
	survives[TextAnswer](t, "TextAnswer")
	survives[FilesRequest](t, "FilesRequest")
	survives[FilesAnswer](t, "FilesAnswer")
	survives[SeeRequest](t, "SeeRequest")
	survives[SeeAnswer](t, "SeeAnswer")
	survives[ReadRequest](t, "ReadRequest")
	survives[ReadAnswer](t, "ReadAnswer")
}

func TestTypedFailuresSurviveTheTrip(t *testing.T) {
	carried := func(err error) error {
		data, _ := json.Marshal(FaultOf(err))
		var f Fault
		_ = json.Unmarshal(data, &f)
		return f.Err()
	}

	var unwritable *UnwritableError
	if err := carried(&UnwritableError{App: "Notes", Why: "no such command"}); !errors.As(err, &unwritable) || unwritable.App != "Notes" || unwritable.Why != "no such command" {
		t.Errorf("unwritable arrived as %v", err)
	}
	var empty *NoStepError
	if err := carried(&NoStepError{Why: "nothing"}); !errors.As(err, &empty) || empty.Why != "nothing" {
		t.Errorf("no step arrived as %v", err)
	}
	if err := carried(&noAnswer{model: "m"}); !errors.Is(err, ErrNoAnswer) {
		t.Errorf("no answer arrived as %v", err)
	}
	if err := carried(ErrNotSetUp); !errors.Is(err, ErrNotSetUp) {
		t.Errorf("not set up arrived as %v", err)
	}
	if err := carried(errors.New("synth: secret prompt text leaked")); err.Error() == "synth: secret prompt text leaked" {
		t.Error("an untyped failure carried its text to the app")
	}
}

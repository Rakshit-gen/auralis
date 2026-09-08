package envelope

import "testing"

func TestNewAndParseRoundTrip(t *testing.T) {
	type payload struct {
		ShowID string `json:"show_id"`
	}
	env, err := New("content.show_published", 1, "content", "corr-1", "cause-1", payload{ShowID: "s-42"})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if env.CorrelationID != "corr-1" || env.CausationID != "cause-1" {
		t.Fatalf("tracing ids not carried: %+v", env)
	}

	parsed, err := Parse(env.Bytes())
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	var got payload
	if err := parsed.Decode(&got); err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if got.ShowID != "s-42" {
		t.Fatalf("payload lost: %+v", got)
	}
}

func TestValidateRejectsIncomplete(t *testing.T) {
	cases := map[string]Envelope{
		"no type":     {EventID: "11111111-1111-1111-1111-111111111111", EventVersion: 1},
		"bad version": {EventID: "11111111-1111-1111-1111-111111111111", EventType: "x", EventVersion: 0},
		"bad uuid":    {EventID: "not-a-uuid", EventType: "x", EventVersion: 1},
	}
	for name, e := range cases {
		if err := e.Validate(); err == nil {
			t.Errorf("%s: expected validation error", name)
		}
	}
}

func TestNewGeneratesCorrelationWhenEmpty(t *testing.T) {
	env, err := New("t", 1, "p", "", "", struct{}{})
	if err != nil {
		t.Fatal(err)
	}
	if env.CorrelationID == "" {
		t.Fatal("expected generated correlation id")
	}
}

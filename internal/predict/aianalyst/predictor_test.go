package aianalyst_test

import (
	"context"
	"testing"

	"github.com/iberkayC/case1back/internal/app"
	"github.com/iberkayC/case1back/internal/domain"
	"github.com/iberkayC/case1back/internal/external/openai"
	"github.com/iberkayC/case1back/internal/platform/apperror"
	"github.com/iberkayC/case1back/internal/predict/aianalyst"
)

// fakeCaller scripts a sequence of LLM responses for retry testing.
type fakeCaller struct {
	responses []string
	errs      []error
	calls     int
	lastReq   openai.ChatRequest
}

func (f *fakeCaller) Complete(_ context.Context, req openai.ChatRequest) (string, error) {
	f.lastReq = req
	defer func() { f.calls++ }()
	if f.calls < len(f.errs) && f.errs[f.calls] != nil {
		return "", f.errs[f.calls]
	}
	return f.responses[f.calls], nil
}

func teams() []domain.Team {
	return []domain.Team{
		{ID: 1, Name: "Alpha"}, {ID: 2, Name: "Bravo"},
		{ID: 3, Name: "Charlie"}, {ID: 4, Name: "Delta"},
	}
}

func input() app.PredictionInput {
	return app.PredictionInput{
		League: domain.League{ID: 1, Name: "Test", CurrentWeek: 5, TotalWeeks: 6, Version: 1},
		Teams:  teams(),
	}
}

const validJSON = `{
  "entries": [
    {"projected_position": 1, "team_id": 1, "team_name": "Alpha", "championship_pct": 50.0, "average_position": 1.4, "played": 6, "expected_won": 4.2, "expected_drawn": 1.0, "expected_lost": 0.8, "expected_goals_for": 13.5, "expected_goals_against": 6.5, "expected_goal_diff": 7.0, "expected_points": 13.6},
    {"projected_position": 2, "team_id": 2, "team_name": "Bravo", "championship_pct": 25.0, "average_position": 2.1, "played": 6, "expected_won": 3.2, "expected_drawn": 1.4, "expected_lost": 1.4, "expected_goals_for": 10.5, "expected_goals_against": 8.0, "expected_goal_diff": 2.5, "expected_points": 11.0},
    {"projected_position": 3, "team_id": 3, "team_name": "Charlie", "championship_pct": 15.0, "average_position": 2.8, "played": 6, "expected_won": 2.4, "expected_drawn": 1.3, "expected_lost": 2.3, "expected_goals_for": 8.5, "expected_goals_against": 9.0, "expected_goal_diff": -0.5, "expected_points": 8.5},
    {"projected_position": 4, "team_id": 4, "team_name": "Delta", "championship_pct": 10.0, "average_position": 3.7, "played": 6, "expected_won": 1.4, "expected_drawn": 1.1, "expected_lost": 3.5, "expected_goals_for": 6.0, "expected_goals_against": 14.0, "expected_goal_diff": -8.0, "expected_points": 5.3}
  ],
  "notes": "Alpha is the strongest side; Bravo a credible challenger."
}`

func TestPredictor_ParsesValidJSON(t *testing.T) {
	t.Parallel()

	caller := &fakeCaller{responses: []string{validJSON}}
	p := aianalyst.New(caller, "gpt-test")

	res, err := p.Predict(context.Background(), input())
	if err != nil {
		t.Fatalf("predict: %v", err)
	}
	if res.Strategy != domain.StrategyAIAnalyst {
		t.Errorf("expected ai_analyst strategy, got %q", res.Strategy)
	}
	if len(res.Entries) != 4 {
		t.Fatalf("expected 4 entries, got %d", len(res.Entries))
	}
	if res.Entries[0].TeamName != "Alpha" {
		t.Errorf("expected Alpha first, got %s", res.Entries[0].TeamName)
	}
	if res.Notes == "" {
		t.Error("expected notes")
	}
}

// PredictWithFeedback must include the validator's complaint in the user prompt.
func TestPredictor_RetryIncludesFeedback(t *testing.T) {
	t.Parallel()

	caller := &fakeCaller{responses: []string{validJSON}}
	p := aianalyst.New(caller, "gpt-test")

	_, err := p.PredictWithFeedback(context.Background(), input(), "pct sum was 90")
	if err != nil {
		t.Fatalf("predict with feedback: %v", err)
	}
	if caller.calls != 1 {
		t.Errorf("expected 1 call, got %d", caller.calls)
	}
	if len(caller.lastReq.Messages) < 2 {
		t.Fatal("expected user message")
	}
	user := caller.lastReq.Messages[len(caller.lastReq.Messages)-1].Content
	if !contains(user, "pct sum was 90") {
		t.Errorf("expected feedback in user prompt, got: %s", user)
	}
}

func TestPredictor_MalformedJSON_ReturnsAIPredictionInvalid(t *testing.T) {
	t.Parallel()

	caller := &fakeCaller{responses: []string{"not json"}}
	p := aianalyst.New(caller, "gpt-test")

	_, err := p.Predict(context.Background(), input())
	if err == nil {
		t.Fatal("expected error")
	}
	ae, ok := apperror.As(err)
	if !ok {
		t.Fatalf("expected *apperror.Error, got %T", err)
	}
	if ae.Status != 502 || ae.Code != apperror.CodeAIPredictionInvalid {
		t.Errorf("expected 502 ai_prediction_invalid, got %d %s", ae.Status, ae.Code)
	}
}

func TestPredictor_UnknownTeam_ReturnsAIPredictionInvalid(t *testing.T) {
	t.Parallel()

	bad := `{"entries":[{"projected_position":1,"team_id":999,"team_name":"Ghost","championship_pct":100,"average_position":1,"played":6,"expected_won":6,"expected_drawn":0,"expected_lost":0,"expected_goals_for":18,"expected_goals_against":0,"expected_goal_diff":18,"expected_points":18}],"notes":""}`
	caller := &fakeCaller{responses: []string{bad}}
	p := aianalyst.New(caller, "gpt-test")

	_, err := p.Predict(context.Background(), input())
	if err == nil {
		t.Fatal("expected error")
	}
	ae, _ := apperror.As(err)
	if ae == nil || ae.Code != apperror.CodeAIPredictionInvalid {
		t.Errorf("expected ai_prediction_invalid, got %v", err)
	}
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

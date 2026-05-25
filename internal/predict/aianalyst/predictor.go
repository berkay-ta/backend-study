// Package aianalyst implements a Predictor that delegates to an LLM.
package aianalyst

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/iberkayC/case1back/internal/app"
	"github.com/iberkayC/case1back/internal/domain"
	"github.com/iberkayC/case1back/internal/external/openai"
	"github.com/iberkayC/case1back/internal/platform/apperror"
)

// Caller is the minimum surface this predictor needs from the OpenAI client.
type Caller interface {
	Complete(ctx context.Context, req openai.ChatRequest) (string, error)
}

type Predictor struct {
	client Caller
	model  string
}

func New(client Caller, model string) *Predictor {
	return &Predictor{client: client, model: model}
}

func (p *Predictor) Strategy() domain.Strategy { return domain.StrategyAIAnalyst }

func (p *Predictor) Predict(ctx context.Context, in app.PredictionInput) (domain.PredictionResult, error) {
	return p.run(ctx, in, "")
}

// PredictWithFeedback re-runs with the validator's error message included in
// the user prompt; implements validating.Retryable.
func (p *Predictor) PredictWithFeedback(ctx context.Context, in app.PredictionInput, feedback string) (domain.PredictionResult, error) {
	return p.run(ctx, in, feedback)
}

func (p *Predictor) run(ctx context.Context, in app.PredictionInput, feedback string) (domain.PredictionResult, error) {
	userPrompt := buildUserPrompt(in, feedback)
	req := openai.ChatRequest{
		Model: p.model,
		Messages: []openai.Message{
			{Role: "system", Content: systemPrompt},
			{Role: "user", Content: userPrompt},
		},
		ResponseFormat: openai.ResponseFormat{
			Type:       "json_schema",
			JSONSchema: json.RawMessage(jsonSchema),
		},
		Temperature: 0.2,
	}

	raw, err := p.client.Complete(ctx, req)
	if err != nil {
		return domain.PredictionResult{}, apperror.BadGateway(apperror.CodeAIPredictionInvalid,
			"AI predictor call failed").WithCause(err)
	}

	var payload responsePayload
	if err := json.Unmarshal([]byte(raw), &payload); err != nil {
		return domain.PredictionResult{}, apperror.BadGateway(apperror.CodeAIPredictionInvalid,
			"AI predictor returned malformed JSON").WithCause(err)
	}

	teamByID := domain.TeamsByID(in.Teams)
	teamByName := make(map[string]domain.Team, len(in.Teams))
	for _, t := range in.Teams {
		teamByName[strings.ToLower(t.Name)] = t
	}

	entries := make([]domain.PredictionEntry, 0, len(payload.Entries))
	for _, e := range payload.Entries {
		team, ok := lookupTeam(teamByID, teamByName, e)
		if !ok {
			return domain.PredictionResult{}, apperror.BadGateway(apperror.CodeAIPredictionInvalid,
				fmt.Sprintf("AI predictor referenced unknown team: id=%d name=%q", e.TeamID, e.TeamName))
		}
		entries = append(entries, domain.PredictionEntry{
			ProjectedPosition:    e.ProjectedPosition,
			TeamID:               team.ID,
			TeamName:             team.Name,
			ChampionshipPct:      e.ChampionshipPct,
			AveragePosition:      e.AveragePosition,
			Played:               e.Played,
			ExpectedWon:          e.ExpectedWon,
			ExpectedDrawn:        e.ExpectedDrawn,
			ExpectedLost:         e.ExpectedLost,
			ExpectedGoalsFor:     e.ExpectedGoalsFor,
			ExpectedGoalsAgainst: e.ExpectedGoalsAgainst,
			ExpectedGoalDiff:     e.ExpectedGoalDiff,
			ExpectedPoints:       e.ExpectedPoints,
		})
	}
	domain.SortPredictionEntries(entries)

	return domain.PredictionResult{
		Strategy: domain.StrategyAIAnalyst,
		Entries:  entries,
		Notes:    strings.TrimSpace(payload.Notes),
	}, nil
}

func lookupTeam(byID map[int64]domain.Team, byName map[string]domain.Team, e entryPayload) (domain.Team, bool) {
	if e.TeamID != 0 {
		if t, ok := byID[e.TeamID]; ok {
			return t, true
		}
	}
	if e.TeamName != "" {
		if t, ok := byName[strings.ToLower(e.TeamName)]; ok {
			return t, true
		}
	}
	return domain.Team{}, false
}

const systemPrompt = `You are an experienced football analyst.
Estimate championship probabilities for a double round-robin football league,
where every team plays each other once at home and once away. Use 3 points for
a win, 1 for a draw, and 0 for a loss.
Use the played matches, remaining fixtures, team strengths and current standings.
Respond with ONLY a JSON object that conforms to the requested schema.
The championship_pct values must sum to exactly 100. expected_points must be non-negative.
Each entry must also be an estimated final-table row with projected_position,
average_position, played, expected W/D/L, expected goals, goal difference, and points.
Include a short 'notes' field (max 80 words) explaining your reasoning.

Hard constraints (any violation is rejected):
- Exactly one entry per team; use every team_id once.
- projected_position is a permutation of 1..N, ranked by championship_pct (ties broken by expected_points).
- championship_pct in [0,100]; the column sums to 100.
- played = total matches each team plays over the full season (= total weeks).
- expected_won + expected_drawn + expected_lost = played, all non-negative.
- expected_points = 3*expected_won + expected_drawn.
- expected_goal_diff = expected_goals_for - expected_goals_against; goals non-negative.`

const jsonSchema = `{
	"name": "championship_prediction",
	"strict": true,
	"schema": {
		"type": "object",
		"additionalProperties": false,
		"required": ["entries", "notes"],
		"properties": {
			"entries": {
				"type": "array",
				"items": {
					"type": "object",
					"additionalProperties": false,
					"required": [
						"projected_position", "team_id", "team_name", "championship_pct",
						"average_position", "played", "expected_won", "expected_drawn",
						"expected_lost", "expected_goals_for", "expected_goals_against",
						"expected_goal_diff", "expected_points"
					],
					"properties": {
						"projected_position": {"type": "integer"},
						"team_id": {"type": "integer"},
						"team_name": {"type": "string"},
						"championship_pct": {"type": "number"},
						"average_position": {"type": "number"},
						"played": {"type": "integer"},
						"expected_won": {"type": "number"},
						"expected_drawn": {"type": "number"},
						"expected_lost": {"type": "number"},
						"expected_goals_for": {"type": "number"},
						"expected_goals_against": {"type": "number"},
						"expected_goal_diff": {"type": "number"},
						"expected_points": {"type": "number"}
					}
				}
			},
			"notes": {"type": "string"}
		}
	}
}`

type responsePayload struct {
	Entries []entryPayload `json:"entries"`
	Notes   string         `json:"notes"`
}

type entryPayload struct {
	ProjectedPosition    int     `json:"projected_position"`
	TeamID               int64   `json:"team_id"`
	TeamName             string  `json:"team_name"`
	ChampionshipPct      float64 `json:"championship_pct"`
	AveragePosition      float64 `json:"average_position"`
	Played               int     `json:"played"`
	ExpectedWon          float64 `json:"expected_won"`
	ExpectedDrawn        float64 `json:"expected_drawn"`
	ExpectedLost         float64 `json:"expected_lost"`
	ExpectedGoalsFor     float64 `json:"expected_goals_for"`
	ExpectedGoalsAgainst float64 `json:"expected_goals_against"`
	ExpectedGoalDiff     float64 `json:"expected_goal_diff"`
	ExpectedPoints       float64 `json:"expected_points"`
}

func buildUserPrompt(in app.PredictionInput, feedback string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "League: %s (id=%d, week=%d/%d, version=%d)\n",
		in.League.Name, in.League.ID, in.League.CurrentWeek, in.League.TotalWeeks, in.League.Version)

	b.WriteString("\nTeams (id, name, strength 1-100):\n")
	for _, t := range in.Teams {
		fmt.Fprintf(&b, "- id=%d name=%q strength=%d\n", t.ID, t.Name, t.Strength)
	}

	b.WriteString("\nCurrent standings (computed live):\n")
	for _, r := range in.Snapshot {
		fmt.Fprintf(&b, "%d. %s  P=%d W=%d D=%d L=%d GF=%d GA=%d GD=%+d Pts=%d\n",
			r.Position, r.TeamName, r.Played, r.Won, r.Drawn, r.Lost,
			r.GoalsFor, r.GoalsAgainst, r.GoalDiff, r.Points)
	}

	if len(in.PlayedMatches) > 0 {
		b.WriteString("\nPlayed matches:\n")
		for _, m := range in.PlayedMatches {
			fmt.Fprintf(&b, "- W%d: team_id=%d vs team_id=%d  %d-%d\n",
				m.Week, m.HomeTeamID, m.AwayTeamID, *m.HomeGoals, *m.AwayGoals)
		}
	}

	if len(in.Remaining) > 0 {
		b.WriteString("\nRemaining fixtures:\n")
		for _, m := range in.Remaining {
			fmt.Fprintf(&b, "- W%d: team_id=%d vs team_id=%d\n", m.Week, m.HomeTeamID, m.AwayTeamID)
		}
	}

	b.WriteString("\nReturn JSON only. championship_pct must sum to 100 and entries must form the estimated final table.")
	if feedback != "" {
		fmt.Fprintf(&b, "\n\nIMPORTANT: previous attempt failed validation: %s\nFix it.", feedback)
	}
	return b.String()
}

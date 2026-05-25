// Package fakerepo provides hand-rolled in-memory implementations of the
// application's repository interfaces, used as test fakes (the
// "fakes vs. mocks" distinction: a small in-memory implementation of an
// interface, when we just need the behaviour).
package fakerepo

import (
	"context"
	"errors"
	"sort"
	"sync"
	"time"

	"github.com/iberkayC/case1back/internal/app"
	"github.com/iberkayC/case1back/internal/domain"
)

type Store struct {
	mu sync.Mutex

	leagues      map[int64]*domain.League
	teams        map[int64]*domain.Team
	matches      map[int64]*domain.Match
	snapshots    map[int64]*domain.StandingSnapshot
	runs         map[int64]*domain.PredictionRun
	idempotency  map[string]*app.IdempotencyRecord
	nextLeagueID int64
	nextTeamID   int64
	nextMatchID  int64
	nextSnapshot int64
	nextRun      int64
}

func New() *Store {
	return &Store{
		leagues:     map[int64]*domain.League{},
		teams:       map[int64]*domain.Team{},
		matches:     map[int64]*domain.Match{},
		snapshots:   map[int64]*domain.StandingSnapshot{},
		runs:        map[int64]*domain.PredictionRun{},
		idempotency: map[string]*app.IdempotencyRecord{},
	}
}

func (s *Store) Leagues() *LeagueRepo          { return &LeagueRepo{s: s} }
func (s *Store) Matches() *MatchRepo           { return &MatchRepo{s: s} }
func (s *Store) Snapshots() *SnapshotRepo      { return &SnapshotRepo{s: s} }
func (s *Store) Predictions() *PredictionRepo  { return &PredictionRepo{s: s} }
func (s *Store) Idempotency() *IdempotencyRepo { return &IdempotencyRepo{s: s} }

func (s *Store) WithTx(ctx context.Context, fn func(ctx context.Context) error) error {
	return fn(ctx)
}

type LeagueRepo struct{ s *Store }

func (r *LeagueRepo) Create(ctx context.Context, name string, totalWeeks int, teams []domain.Team) (domain.League, []domain.Team, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	r.s.nextLeagueID++
	id := r.s.nextLeagueID
	l := domain.League{
		ID: id, Name: name, CurrentWeek: 1, TotalWeeks: totalWeeks,
		Version: 0, CreatedAt: time.Now().UTC(),
	}
	r.s.leagues[id] = &l

	out := make([]domain.Team, len(teams))
	for i, t := range teams {
		r.s.nextTeamID++
		tID := r.s.nextTeamID
		stored := domain.Team{ID: tID, LeagueID: id, Name: t.Name, Strength: t.Strength}
		r.s.teams[tID] = &stored
		out[i] = stored
	}
	return l, out, nil
}

func (r *LeagueRepo) List(ctx context.Context) ([]domain.League, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	out := make([]domain.League, 0, len(r.s.leagues))
	for _, l := range r.s.leagues {
		out = append(out, *l)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func (r *LeagueRepo) Get(ctx context.Context, id int64) (domain.League, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	l, ok := r.s.leagues[id]
	if !ok {
		return domain.League{}, domain.ErrLeagueNotFound
	}
	return *l, nil
}

func (r *LeagueRepo) GetForUpdate(ctx context.Context, id int64) (domain.League, error) {
	return r.Get(ctx, id)
}

func (r *LeagueRepo) Delete(ctx context.Context, id int64) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if _, ok := r.s.leagues[id]; !ok {
		return domain.ErrLeagueNotFound
	}
	delete(r.s.leagues, id)
	for tID, t := range r.s.teams {
		if t.LeagueID == id {
			delete(r.s.teams, tID)
		}
	}
	for mID, m := range r.s.matches {
		if m.LeagueID == id {
			delete(r.s.matches, mID)
		}
	}
	for sID, s := range r.s.snapshots {
		if s.LeagueID == id {
			delete(r.s.snapshots, sID)
		}
	}
	for rID, run := range r.s.runs {
		if run.LeagueID == id {
			delete(r.s.runs, rID)
		}
	}
	return nil
}

func (r *LeagueRepo) BumpVersion(ctx context.Context, id int64) (int64, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	l, ok := r.s.leagues[id]
	if !ok {
		return 0, domain.ErrLeagueNotFound
	}
	l.Version++
	return l.Version, nil
}

func (r *LeagueRepo) SetCurrentWeek(ctx context.Context, id int64, week int) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	l, ok := r.s.leagues[id]
	if !ok {
		return domain.ErrLeagueNotFound
	}
	l.CurrentWeek = week
	return nil
}

func (r *LeagueRepo) GetTeams(ctx context.Context, leagueID int64) ([]domain.Team, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var out []domain.Team
	for _, t := range r.s.teams {
		if t.LeagueID == leagueID {
			out = append(out, *t)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

type MatchRepo struct{ s *Store }

func (r *MatchRepo) InsertFixtures(ctx context.Context, matches []domain.Match) ([]domain.Match, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	out := make([]domain.Match, len(matches))
	for i, m := range matches {
		r.s.nextMatchID++
		m.ID = r.s.nextMatchID
		stored := m
		r.s.matches[m.ID] = &stored
		out[i] = stored
	}
	return out, nil
}

func (r *MatchRepo) RecordResult(ctx context.Context, matchID int64, homeGoals, awayGoals int, edited bool) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	m, ok := r.s.matches[matchID]
	if !ok {
		return domain.ErrMatchNotFound
	}
	hg, ag := homeGoals, awayGoals
	m.HomeGoals = &hg
	m.AwayGoals = &ag
	now := time.Now().UTC()
	if edited {
		m.EditedAt = &now
	} else if m.PlayedAt == nil {
		m.PlayedAt = &now
	}
	return nil
}

func (r *MatchRepo) ClearAllResults(ctx context.Context, leagueID int64) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, m := range r.s.matches {
		if m.LeagueID == leagueID {
			m.HomeGoals = nil
			m.AwayGoals = nil
			m.PlayedAt = nil
			m.EditedAt = nil
		}
	}
	return nil
}

func (r *MatchRepo) Get(ctx context.Context, matchID int64) (domain.Match, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	m, ok := r.s.matches[matchID]
	if !ok {
		return domain.Match{}, domain.ErrMatchNotFound
	}
	return *m, nil
}

func (r *MatchRepo) List(ctx context.Context, leagueID int64) ([]domain.Match, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var out []domain.Match
	for _, m := range r.s.matches {
		if m.LeagueID == leagueID {
			out = append(out, *m)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Week != out[j].Week {
			return out[i].Week < out[j].Week
		}
		return out[i].ID < out[j].ID
	})
	return out, nil
}

func (r *MatchRepo) ListByWeek(ctx context.Context, leagueID int64, week int) ([]domain.Match, error) {
	all, _ := r.List(ctx, leagueID)
	out := all[:0]
	for _, m := range all {
		if m.Week == week {
			out = append(out, m)
		}
	}
	return out, nil
}

type SnapshotRepo struct{ s *Store }

func (r *SnapshotRepo) GetOrCreate(ctx context.Context, snap domain.StandingSnapshot) (domain.StandingSnapshot, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, existing := range r.s.snapshots {
		if existing.LeagueID == snap.LeagueID && existing.LeagueVersion == snap.LeagueVersion {
			return *existing, nil
		}
	}
	r.s.nextSnapshot++
	snap.ID = r.s.nextSnapshot
	snap.CreatedAt = time.Now().UTC()
	stored := snap
	r.s.snapshots[snap.ID] = &stored
	return snap, nil
}

func (r *SnapshotRepo) Get(ctx context.Context, snapshotID int64) (domain.StandingSnapshot, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	s, ok := r.s.snapshots[snapshotID]
	if !ok {
		return domain.StandingSnapshot{}, domain.ErrSnapshotNotFound
	}
	return *s, nil
}

type PredictionRepo struct{ s *Store }

func (r *PredictionRepo) Create(ctx context.Context, run domain.PredictionRun) (domain.PredictionRun, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	r.s.nextRun++
	run.ID = r.s.nextRun
	run.CreatedAt = time.Now().UTC()
	stored := run
	r.s.runs[run.ID] = &stored
	return run, nil
}

func (r *PredictionRepo) Get(ctx context.Context, runID int64) (domain.PredictionRun, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	run, ok := r.s.runs[runID]
	if !ok {
		return domain.PredictionRun{}, domain.ErrPredictionNotFound
	}
	return *run, nil
}

func (r *PredictionRepo) ListByLeague(ctx context.Context, leagueID int64) ([]domain.PredictionRun, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	var out []domain.PredictionRun
	for _, run := range r.s.runs {
		if run.LeagueID == leagueID {
			out = append(out, *run)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID > out[j].ID })
	return out, nil
}

func (r *PredictionRepo) MarkStaleBefore(ctx context.Context, leagueID int64, leagueVersion int64) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	for _, run := range r.s.runs {
		if run.LeagueID == leagueID && run.LeagueVersion < leagueVersion {
			run.Stale = true
		}
	}
	return nil
}

type IdempotencyRepo struct{ s *Store }

func (r *IdempotencyRepo) Reserve(ctx context.Context, key string, requestHash string) (app.IdempotencyRecord, bool, error) {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	if rec, ok := r.s.idempotency[key]; ok {
		cp := *rec
		cp.ResponseBody = append([]byte(nil), rec.ResponseBody...)
		return cp, false, nil
	}
	rec := app.IdempotencyRecord{
		Key:         key,
		RequestHash: requestHash,
		State:       app.IdempotencyInProgress,
	}
	r.s.idempotency[key] = &rec
	return rec, true, nil
}

func (r *IdempotencyRepo) Complete(ctx context.Context, key string, status int, body []byte) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	rec, ok := r.s.idempotency[key]
	if !ok {
		return errors.New("idempotency key not found")
	}
	rec.State = app.IdempotencyCompleted
	rec.ResponseStatus = status
	rec.ResponseBody = append([]byte(nil), body...)
	return nil
}

func (r *IdempotencyRepo) Abort(ctx context.Context, key string) error {
	r.s.mu.Lock()
	defer r.s.mu.Unlock()
	delete(r.s.idempotency, key)
	return nil
}

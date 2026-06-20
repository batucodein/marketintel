package simulation

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/batuhan/marketintel/internal/domain"
	"github.com/batuhan/marketintel/internal/platform/ai/prompts"
)

// TranscriptMessage is one line in a simulated conversation.
type TranscriptMessage struct {
	Who            string   `json:"who"` // "you" | "them" | "event"
	Day            int      `json:"day"`
	Subject        string   `json:"subject,omitempty"`
	Body           string   `json:"body"`
	Sentiment      string   `json:"sentiment,omitempty"`       // legacy 3-value
	SentimentLevel string   `json:"sentiment_level,omitempty"` // 5-level (matches production)
	Tags           []string `json:"tags,omitempty"`            // intent tags
}

// PendingDraft is the single outbound draft a lead is waiting for the user to
// approve/edit/refine (the cadence is sequential, so at most one at a time).
type PendingDraft struct {
	Kind    string `json:"kind"` // cold | followup | reply
	Subject string `json:"subject"`
	Body    string `json:"body"`
	Step    int    `json:"step"` // cadence step index this draft belongs to (-1 for cold/reply)
}

// Lead is a persisted simulated lead with its transcript + grade. For the
// interactive engine it also carries the live state machine fields.
type Lead struct {
	ID           uuid.UUID           `json:"id"`
	SimulationID uuid.UUID           `json:"simulation_id"`
	Persona      string              `json:"persona"`
	DisplayName  string              `json:"display_name"`
	BusinessID   *uuid.UUID          `json:"business_id"`
	Transcript   []TranscriptMessage `json:"transcript"`
	Outcome      string              `json:"outcome"`
	Grade        json.RawMessage     `json:"grade,omitempty"`
	CreatedAt    time.Time           `json:"created_at"`

	// Live interactive state (active | awaiting_approval | snoozed | done).
	State          string        `json:"state"`
	CurrentStep    int           `json:"current_step"`
	Touch          int           `json:"touch"`
	LastDirection  *string       `json:"last_direction,omitempty"`
	LastOutDay     int           `json:"last_out_day"`
	LastSentiment  *string       `json:"last_sentiment,omitempty"`
	NextDay        int           `json:"next_day"`
	SnoozeUntilDay *int          `json:"snooze_until_day,omitempty"`
	RepliedOnce    bool          `json:"replied_once"`
	PendingDraft   *PendingDraft `json:"pending_draft,omitempty"`
	ResumeTags     []string      `json:"-"`
	ResumeSentiment *string      `json:"-"`
	PromptCtx      *LeadPromptCtx `json:"-"` // cached AI-prompt context (not exposed)
}

// LeadPromptCtx caches the per-lead context the draft/reply/persona prompts need,
// computed once at creation so each step doesn't re-query.
type LeadPromptCtx struct {
	BusinessJSON  string `json:"business_json"`
	ShipmentCtx   string `json:"shipment_ctx"`
	LeadScoreJSON string `json:"lead_score_json"`
	ContactJSON   string `json:"contact_json"`
	PersonaBiz    string `json:"persona_biz"`
	HasCatalog    bool   `json:"has_catalog"`
	CatalogName   string `json:"catalog_name"`
	InitialSubj   string `json:"initial_subj"`
}

// Simulation is a persisted run.
type Simulation struct {
	ID              uuid.UUID             `json:"id"`
	UserID          uuid.UUID             `json:"user_id"`
	Name            string                `json:"name"`
	MarketID        *uuid.UUID            `json:"market_id"`
	SenderProfileID *uuid.UUID            `json:"sender_profile_id"`
	Steps           []domain.SequenceStep `json:"steps"`
	PersonaConfig   json.RawMessage       `json:"persona_config"`
	Status          string                `json:"status"`
	Summary         json.RawMessage       `json:"summary,omitempty"`
	Error           *string               `json:"error,omitempty"`
	CreatedAt       time.Time             `json:"created_at"`
	CompletedAt     *time.Time            `json:"completed_at"`
	Leads           []Lead                `json:"leads,omitempty"`

	// Interactive config / live state.
	Mode             string            `json:"mode"`
	VirtualDay       int               `json:"virtual_day"`
	Playbook         map[string]string `json:"playbook"`
	OnPositiveAction string            `json:"on_positive_action"`
	OnNegativeAction string            `json:"on_negative_action"`
	IncludeLessons   bool              `json:"include_lessons"`
	PendingCount     int               `json:"pending_count"` // derived: leads awaiting approval
}

type Repository interface {
	Create(ctx context.Context, s Simulation) (*Simulation, error)
	Finish(ctx context.Context, id uuid.UUID, status string, summary json.RawMessage, errMsg *string) error
	List(ctx context.Context, userID uuid.UUID) ([]Simulation, error)
	Get(ctx context.Context, userID, id uuid.UUID) (*Simulation, error)
	Delete(ctx context.Context, userID, id uuid.UUID) error
	AddLead(ctx context.Context, l Lead) error
	ListLeads(ctx context.Context, simID uuid.UUID) ([]Lead, error)

	// Interactive engine.
	InsertLead(ctx context.Context, l Lead) error            // full live-state insert (l.ID set)
	UpdateLead(ctx context.Context, l Lead) error            // persist a state transition
	GetLead(ctx context.Context, simID, leadID uuid.UUID) (*Lead, error)
	UpdateSim(ctx context.Context, id uuid.UUID, virtualDay int, status string, summary json.RawMessage) error
	SetPlaybook(ctx context.Context, id uuid.UUID, pb map[string]string) error

	// Agent chat thread.
	AddAssistantMessage(ctx context.Context, simID uuid.UUID, role, content string, action json.RawMessage, status string) (uuid.UUID, time.Time, error)
	ListAssistantMessages(ctx context.Context, simID uuid.UUID, limit int) ([]AssistantMessage, error)
	GetAssistantProposal(ctx context.Context, simID, msgID uuid.UUID) (json.RawMessage, string, error)
	SetAssistantStatus(ctx context.Context, simID, msgID uuid.UUID, status string) error
}

type repository struct{ pool *pgxpool.Pool }

func NewRepository(pool *pgxpool.Pool) Repository { return &repository{pool: pool} }

func (r *repository) Create(ctx context.Context, s Simulation) (*Simulation, error) {
	id := uuid.New()
	steps, _ := json.Marshal(s.Steps)
	pc := s.PersonaConfig
	if len(pc) == 0 {
		pc = json.RawMessage("{}")
	}
	pb, _ := json.Marshal(s.Playbook)
	if len(pb) == 0 || string(pb) == "null" {
		pb = json.RawMessage("{}")
	}
	status := s.Status
	if status == "" {
		status = "running"
	}
	mode := s.Mode
	if mode == "" {
		mode = "interactive"
	}
	_, err := r.pool.Exec(ctx,
		`INSERT INTO simulations
		   (id, user_id, name, market_id, sender_profile_id, steps, persona_config, status,
		    mode, playbook, on_positive_action, on_negative_action, include_lessons)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
		id, s.UserID, s.Name, s.MarketID, s.SenderProfileID, steps, pc, status,
		mode, pb, s.OnPositiveAction, s.OnNegativeAction, s.IncludeLessons,
	)
	if err != nil {
		return nil, fmt.Errorf("create simulation: %w", err)
	}
	return r.Get(ctx, s.UserID, id)
}

func (r *repository) Finish(ctx context.Context, id uuid.UUID, status string, summary json.RawMessage, errMsg *string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE simulations SET status=$1, summary=$2, error=$3, completed_at=now() WHERE id=$4`,
		status, summary, errMsg, id,
	)
	return err
}

func scanSim(row pgx.Row) (*Simulation, error) {
	var s Simulation
	var steps, summary, playbook []byte
	if err := row.Scan(&s.ID, &s.UserID, &s.Name, &s.MarketID, &s.SenderProfileID,
		&steps, &s.PersonaConfig, &s.Status, &summary, &s.Error, &s.CreatedAt, &s.CompletedAt,
		&s.Mode, &s.VirtualDay, &playbook, &s.OnPositiveAction, &s.OnNegativeAction, &s.IncludeLessons); err != nil {
		return nil, err
	}
	if len(steps) > 0 {
		_ = json.Unmarshal(steps, &s.Steps)
	}
	s.Summary = summary
	s.Playbook = map[string]string{}
	if len(playbook) > 0 {
		_ = json.Unmarshal(playbook, &s.Playbook)
	}
	return &s, nil
}

const simCols = `id, user_id, name, market_id, sender_profile_id, steps, persona_config, status, summary, error, created_at, completed_at, mode, virtual_day, playbook, on_positive_action, on_negative_action, include_lessons`

func (r *repository) List(ctx context.Context, userID uuid.UUID) ([]Simulation, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+simCols+` FROM simulations WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, fmt.Errorf("list simulations: %w", err)
	}
	defer rows.Close()
	var out []Simulation
	for rows.Next() {
		s, err := scanSim(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *s)
	}
	return out, nil
}

func (r *repository) Get(ctx context.Context, userID, id uuid.UUID) (*Simulation, error) {
	s, err := scanSim(r.pool.QueryRow(ctx, `SELECT `+simCols+` FROM simulations WHERE id=$1 AND user_id=$2`, id, userID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get simulation: %w", err)
	}
	leads, err := r.ListLeads(ctx, id)
	if err != nil {
		return nil, err
	}
	s.Leads = leads
	for _, l := range leads {
		if l.State == "awaiting_approval" {
			s.PendingCount++
		}
	}
	return s, nil
}

func (r *repository) Delete(ctx context.Context, userID, id uuid.UUID) error {
	tag, err := r.pool.Exec(ctx, `DELETE FROM simulations WHERE id=$1 AND user_id=$2`, id, userID)
	if err != nil {
		return fmt.Errorf("delete simulation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrNotFound
	}
	return nil
}

func (r *repository) AddLead(ctx context.Context, l Lead) error {
	transcript, _ := json.Marshal(l.Transcript)
	_, err := r.pool.Exec(ctx,
		`INSERT INTO simulation_leads (id, simulation_id, persona, display_name, business_id, transcript, outcome, grade)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
		uuid.New(), l.SimulationID, l.Persona, l.DisplayName, l.BusinessID, transcript, l.Outcome, nullJSON(l.Grade),
	)
	if err != nil {
		return fmt.Errorf("add simulation lead: %w", err)
	}
	return nil
}

const leadCols = `id, simulation_id, persona, display_name, business_id, transcript, outcome, grade, created_at,
	state, current_step, touch, last_direction, last_out_day, last_sentiment, next_day, snooze_until_day,
	replied_once, pending_draft, resume_tags, resume_sentiment, prompt_ctx`

func scanLead(row pgx.Row) (*Lead, error) {
	var l Lead
	var transcript, grade, pending, resumeTags, promptCtx []byte
	if err := row.Scan(&l.ID, &l.SimulationID, &l.Persona, &l.DisplayName, &l.BusinessID,
		&transcript, &l.Outcome, &grade, &l.CreatedAt,
		&l.State, &l.CurrentStep, &l.Touch, &l.LastDirection, &l.LastOutDay, &l.LastSentiment,
		&l.NextDay, &l.SnoozeUntilDay, &l.RepliedOnce, &pending, &resumeTags, &l.ResumeSentiment, &promptCtx); err != nil {
		return nil, err
	}
	if len(transcript) > 0 {
		_ = json.Unmarshal(transcript, &l.Transcript)
	}
	l.Grade = grade
	if len(pending) > 0 && string(pending) != "null" {
		var pd PendingDraft
		if json.Unmarshal(pending, &pd) == nil {
			l.PendingDraft = &pd
		}
	}
	if len(resumeTags) > 0 {
		_ = json.Unmarshal(resumeTags, &l.ResumeTags)
	}
	if len(promptCtx) > 0 && string(promptCtx) != "null" {
		var pc LeadPromptCtx
		if json.Unmarshal(promptCtx, &pc) == nil {
			l.PromptCtx = &pc
		}
	}
	return &l, nil
}

func (r *repository) ListLeads(ctx context.Context, simID uuid.UUID) ([]Lead, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT `+leadCols+` FROM simulation_leads WHERE simulation_id=$1 ORDER BY created_at`, simID)
	if err != nil {
		return nil, fmt.Errorf("list simulation leads: %w", err)
	}
	defer rows.Close()
	out := []Lead{}
	for rows.Next() {
		l, err := scanLead(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *l)
	}
	return out, nil
}

func (r *repository) GetLead(ctx context.Context, simID, leadID uuid.UUID) (*Lead, error) {
	l, err := scanLead(r.pool.QueryRow(ctx,
		`SELECT `+leadCols+` FROM simulation_leads WHERE id=$1 AND simulation_id=$2`, leadID, simID))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, domain.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get simulation lead: %w", err)
	}
	return l, nil
}

// InsertLead writes a lead with its full live state (l.ID must be set).
func (r *repository) InsertLead(ctx context.Context, l Lead) error {
	transcript, _ := json.Marshal(l.Transcript)
	resumeTags, _ := json.Marshal(l.ResumeTags)
	_, err := r.pool.Exec(ctx,
		`INSERT INTO simulation_leads
		   (id, simulation_id, persona, display_name, business_id, transcript, outcome, grade,
		    state, current_step, touch, last_direction, last_out_day, last_sentiment, next_day,
		    snooze_until_day, replied_once, pending_draft, resume_tags, resume_sentiment, prompt_ctx)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21)`,
		l.ID, l.SimulationID, l.Persona, l.DisplayName, l.BusinessID, transcript, l.Outcome, nullJSON(l.Grade),
		l.State, l.CurrentStep, l.Touch, l.LastDirection, l.LastOutDay, l.LastSentiment, l.NextDay,
		l.SnoozeUntilDay, l.RepliedOnce, pendingJSON(l.PendingDraft), nullJSON(resumeTags), l.ResumeSentiment,
		ctxJSON(l.PromptCtx),
	)
	if err != nil {
		return fmt.Errorf("insert simulation lead: %w", err)
	}
	return nil
}

// UpdateLead persists a state transition on an existing lead.
func (r *repository) UpdateLead(ctx context.Context, l Lead) error {
	transcript, _ := json.Marshal(l.Transcript)
	resumeTags, _ := json.Marshal(l.ResumeTags)
	_, err := r.pool.Exec(ctx,
		`UPDATE simulation_leads SET
		   transcript=$2, outcome=$3, grade=$4, state=$5, current_step=$6, touch=$7,
		   last_direction=$8, last_out_day=$9, last_sentiment=$10, next_day=$11, snooze_until_day=$12,
		   replied_once=$13, pending_draft=$14, resume_tags=$15, resume_sentiment=$16
		 WHERE id=$1`,
		l.ID, transcript, l.Outcome, nullJSON(l.Grade), l.State, l.CurrentStep, l.Touch,
		l.LastDirection, l.LastOutDay, l.LastSentiment, l.NextDay, l.SnoozeUntilDay,
		l.RepliedOnce, pendingJSON(l.PendingDraft), nullJSON(resumeTags), l.ResumeSentiment,
	)
	if err != nil {
		return fmt.Errorf("update simulation lead: %w", err)
	}
	return nil
}

func (r *repository) UpdateSim(ctx context.Context, id uuid.UUID, virtualDay int, status string, summary json.RawMessage) error {
	var completedAt any
	if status == "done" {
		completedAt = time.Now().UTC()
	}
	_, err := r.pool.Exec(ctx,
		`UPDATE simulations SET virtual_day=$2, status=$3, summary=COALESCE($4, summary),
		   completed_at=COALESCE($5, completed_at) WHERE id=$1`,
		id, virtualDay, status, nullJSON(summary), completedAt)
	return err
}

func (r *repository) SetPlaybook(ctx context.Context, id uuid.UUID, pb map[string]string) error {
	b, _ := json.Marshal(pb)
	if len(b) == 0 || string(b) == "null" {
		b = []byte("{}")
	}
	_, err := r.pool.Exec(ctx, `UPDATE simulations SET playbook=$2 WHERE id=$1`, id, b)
	return err
}

// --- agent chat thread (mirror of campaign_assistant_messages) ---

func (r *repository) AddAssistantMessage(ctx context.Context, simID uuid.UUID, role, content string, action json.RawMessage, status string) (uuid.UUID, time.Time, error) {
	var id uuid.UUID
	var created time.Time
	err := r.pool.QueryRow(ctx,
		`INSERT INTO simulation_assistant_messages (simulation_id, role, content, proposed_action, status)
		 VALUES ($1,$2,$3,$4,$5) RETURNING id, created_at`,
		simID, role, content, nullJSON(action), status,
	).Scan(&id, &created)
	return id, created, err
}

func (r *repository) ListAssistantMessages(ctx context.Context, simID uuid.UUID, limit int) ([]AssistantMessage, error) {
	rows, err := r.pool.Query(ctx,
		`SELECT id, role, content, proposed_action, status, created_at FROM (
		   SELECT id, role, content, proposed_action, status, created_at
		     FROM simulation_assistant_messages WHERE simulation_id=$1
		     ORDER BY created_at DESC LIMIT $2
		 ) recent ORDER BY created_at ASC`, simID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AssistantMessage{}
	for rows.Next() {
		var m AssistantMessage
		var action []byte
		if err := rows.Scan(&m.ID, &m.Role, &m.Content, &action, &m.Status, &m.CreatedAt); err != nil {
			return nil, err
		}
		if len(action) > 0 {
			var a prompts.AssistantAction
			if json.Unmarshal(action, &a) == nil {
				m.ProposedAction = &a
			}
		}
		out = append(out, m)
	}
	return out, nil
}

func (r *repository) GetAssistantProposal(ctx context.Context, simID, msgID uuid.UUID) (json.RawMessage, string, error) {
	var action []byte
	var status string
	err := r.pool.QueryRow(ctx,
		`SELECT proposed_action, status FROM simulation_assistant_messages
		  WHERE id=$1 AND simulation_id=$2 AND role='assistant'`, msgID, simID,
	).Scan(&action, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, "", domain.ErrNotFound
	}
	return action, status, err
}

func (r *repository) SetAssistantStatus(ctx context.Context, simID, msgID uuid.UUID, status string) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE simulation_assistant_messages SET status=$1 WHERE id=$2 AND simulation_id=$3`, status, msgID, simID)
	return err
}

func pendingJSON(p *PendingDraft) any {
	if p == nil {
		return nil
	}
	b, _ := json.Marshal(p)
	return b
}

func ctxJSON(c *LeadPromptCtx) any {
	if c == nil {
		return nil
	}
	b, _ := json.Marshal(c)
	return b
}

func nullJSON(b json.RawMessage) any {
	if len(b) == 0 {
		return nil
	}
	return []byte(b)
}

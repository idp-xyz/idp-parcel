// Package postgres 是 pilotgovernance 自有语义端口的 PostgreSQL 适配器。
//
// 显式 SQL、行模型与写入代数翻译都留在这里，不进领域对象。治理是产品级机制没有
// 租户维；三张表登记的全部是脱敏引用。
package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
)

// ErrCandidateSetAlreadyFixed 表示同标识的候选组已经固定过。编排先 Find 再 Save，
// 它只在并发竞态下出现——组不可扩张也不可改写，后到者重读已固定那一组即可。
var ErrCandidateSetAlreadyFixed = errors.New("pilot governance postgres: candidate version set already fixed")

// CandidateSets 实现 ports.CandidateSetStore。组不可扩张是结构性的：本类型没有任何
// UPDATE 语句，表侧也没有为改写留列语义。
type CandidateSets struct {
	db *bentopg.DB
}

func NewCandidateSets(db *bentopg.DB) (*CandidateSets, error) {
	if db == nil {
		return nil, fmt.Errorf("pilot governance postgres: db is nil")
	}
	return &CandidateSets{db: db}, nil
}

func (repository *CandidateSets) FindByID(
	ctx context.Context,
	id domain.CandidateVersionSetID,
) (domain.CandidateVersionSet, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.CandidateVersionSet{}, false, fmt.Errorf("find candidate set: %w", err)
	}

	var scope, parameters, rules string
	var formedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT scope, parameters, rules, formed_at
		   FROM pilot_governance.candidate_version_set
		  WHERE set_id = $1`,
		id.String(),
	).Scan(&scope, &parameters, &rules, &formedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CandidateVersionSet{}, false, nil
	}
	if err != nil {
		return domain.CandidateVersionSet{}, false, fmt.Errorf("find candidate set: %w", err)
	}

	scopeRef, err := domain.NewScopeVersionReference(scope)
	if err != nil {
		return domain.CandidateVersionSet{}, false, fmt.Errorf("find candidate set: %w", err)
	}
	parametersRef, err := domain.NewParameterSnapshotReference(parameters)
	if err != nil {
		return domain.CandidateVersionSet{}, false, fmt.Errorf("find candidate set: %w", err)
	}
	rulesRef, err := domain.NewRuleVersionsReference(rules)
	if err != nil {
		return domain.CandidateVersionSet{}, false, fmt.Errorf("find candidate set: %w", err)
	}
	set, err := domain.FixCandidateVersionSet(id, scopeRef, parametersRef, rulesRef, formedAt)
	if err != nil {
		return domain.CandidateVersionSet{}, false, fmt.Errorf("find candidate set: %w", err)
	}
	return set, true, nil
}

func (repository *CandidateSets) Save(ctx context.Context, set domain.CandidateVersionSet) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save candidate set: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO pilot_governance.candidate_version_set
			(set_id, scope, parameters, rules, formed_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT DO NOTHING`,
		set.ID().String(),
		set.Scope().String(),
		set.Parameters().String(),
		set.Rules().String(),
		set.FormedAt().UTC(),
	)
	if err != nil {
		return fmt.Errorf("save candidate set: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("%w", ErrCandidateSetAlreadyFixed)
	}
	return nil
}

// deviationRow 是 deviations jsonb 列的行模型，只在本包存在。
type deviationRow struct {
	Scope         string    `json:"scope"`
	Control       string    `json:"control"`
	Owner         string    `json:"owner"`
	CloseBy       time.Time `json:"closeBy"`
	ResidualRisk  string    `json:"residualRisk"`
	AcceptanceRef string    `json:"acceptanceRef"`
}

// ReviewDecisions 实现 ports.ReviewDecisionStore（写入代数同 ADR-0031）。
type ReviewDecisions struct {
	db *bentopg.DB
}

func NewReviewDecisions(db *bentopg.DB) (*ReviewDecisions, error) {
	if db == nil {
		return nil, fmt.Errorf("pilot governance postgres: db is nil")
	}
	return &ReviewDecisions{db: db}, nil
}

func (repository *ReviewDecisions) FindByKey(
	ctx context.Context,
	key ports.ReviewKey,
) (domain.StageReviewDecision, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.StageReviewDecision{}, false, fmt.Errorf("find stage review: %w", err)
	}

	var (
		stageName, scope, evidencePack, verdictName, decidedBy string
		disposition                                            *string
		deviationsJSON                                         []byte
		decidedAt, effectiveAt                                 time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT stage, scope, evidence_pack, verdict, disposition,
		        deviations, decided_by, decided_at, effective_at
		   FROM pilot_governance.stage_review
		  WHERE objective = $1
		    AND candidate_set_id = $2`,
		key.Objective,
		key.Candidates.String(),
	).Scan(&stageName, &scope, &evidencePack, &verdictName, &disposition,
		&deviationsJSON, &decidedBy, &decidedAt, &effectiveAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.StageReviewDecision{}, false, nil
	}
	if err != nil {
		return domain.StageReviewDecision{}, false, fmt.Errorf("find stage review: %w", err)
	}

	decision, err := rebuildDecision(key, decisionColumns{
		stage:        stageName,
		scope:        scope,
		evidencePack: evidencePack,
		verdict:      verdictName,
		disposition:  disposition,
		deviations:   deviationsJSON,
		decidedBy:    decidedBy,
		decidedAt:    decidedAt,
		effectiveAt:  effectiveAt,
	})
	if err != nil {
		return domain.StageReviewDecision{}, false, fmt.Errorf("find stage review: %w", err)
	}
	return decision, true, nil
}

// Save 写下一次评审决定。同（目标+候选组）已有记录时答`已有记录`——业务答案不是
// 错误（ADR-0031）；ON CONFLICT DO NOTHING 保事务可用，编排拿到它还要同事务读回
// 原决定作答。
func (repository *ReviewDecisions) Save(
	ctx context.Context,
	key ports.ReviewKey,
	decision domain.StageReviewDecision,
) (ports.ReviewSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ReviewSaveOutcomeInvalid, fmt.Errorf("save stage review: %w", err)
	}

	deviations := decision.Deviations()
	deviationRows := make([]deviationRow, 0, len(deviations))
	for _, deviation := range deviations {
		deviationRows = append(deviationRows, deviationRow{
			Scope:         deviation.Scope,
			Control:       deviation.Control,
			Owner:         deviation.Owner,
			CloseBy:       deviation.CloseBy.UTC(),
			ResidualRisk:  deviation.ResidualRisk,
			AcceptanceRef: deviation.AcceptanceRef,
		})
	}
	deviationsJSON, err := json.Marshal(deviationRows)
	if err != nil {
		return ports.ReviewSaveOutcomeInvalid, fmt.Errorf("save stage review: %w", err)
	}

	var disposition *string
	if value, noGo := decision.Disposition(); noGo {
		name := value.String()
		disposition = &name
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO pilot_governance.stage_review
			(objective, candidate_set_id, stage, scope, evidence_pack,
			 verdict, disposition, deviations, decided_by, decided_at, effective_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT DO NOTHING`,
		key.Objective,
		key.Candidates.String(),
		decision.Stage().String(),
		decision.Scope().String(),
		decision.EvidencePack(),
		decision.Verdict().String(),
		disposition,
		deviationsJSON,
		decision.DecidedBy(),
		decision.DecidedAt().UTC(),
		decision.EffectiveAt().UTC(),
	)
	if err != nil {
		return ports.ReviewSaveOutcomeInvalid, fmt.Errorf("save stage review: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ReviewAlreadyRecorded, nil
	}
	return ports.ReviewSaved, nil
}

type decisionColumns struct {
	stage        string
	scope        string
	evidencePack string
	verdict      string
	disposition  *string
	deviations   []byte
	decidedBy    string
	decidedAt    time.Time
	effectiveAt  time.Time
}

func rebuildDecision(key ports.ReviewKey, columns decisionColumns) (domain.StageReviewDecision, error) {
	stage, err := stageFrom(columns.stage)
	if err != nil {
		return domain.StageReviewDecision{}, err
	}
	scope, err := domain.NewScopeVersionReference(columns.scope)
	if err != nil {
		return domain.StageReviewDecision{}, err
	}
	verdict, err := verdictFrom(columns.verdict)
	if err != nil {
		return domain.StageReviewDecision{}, err
	}
	spec := domain.StageReviewDecisionSpec{
		Stage:        stage,
		Objective:    key.Objective,
		Scope:        scope,
		Candidates:   key.Candidates,
		EvidencePack: columns.evidencePack,
		Verdict:      verdict,
		DecidedBy:    columns.decidedBy,
		DecidedAt:    columns.decidedAt,
		EffectiveAt:  columns.effectiveAt,
	}
	if columns.disposition != nil {
		disposition, err := dispositionFrom(*columns.disposition)
		if err != nil {
			return domain.StageReviewDecision{}, err
		}
		spec.Disposition = disposition
	}
	var deviationRows []deviationRow
	if err := json.Unmarshal(columns.deviations, &deviationRows); err != nil {
		return domain.StageReviewDecision{}, err
	}
	for _, row := range deviationRows {
		spec.Deviations = append(spec.Deviations, domain.AcceptedDeviation{
			Scope:         row.Scope,
			Control:       row.Control,
			Owner:         row.Owner,
			CloseBy:       row.CloseBy,
			ResidualRisk:  row.ResidualRisk,
			AcceptanceRef: row.AcceptanceRef,
		})
	}
	// 读回的东西同样要过一遍不变量——Go/No-Go 的形状规则在重建处再验一次。
	return domain.RecordStageReview(spec)
}

// AuthorityIntervals 实现 ports.AuthorityIntervalStore。只追加：重叠预检在应用层
// （DetectAuthorityConflicts 先于任何落库），库不重判重叠；同维完全重复的行由唯一
// 约束拦下兼作幂等——重放补追加不长第二行。
type AuthorityIntervals struct {
	db *bentopg.DB
}

func NewAuthorityIntervals(db *bentopg.DB) (*AuthorityIntervals, error) {
	if db == nil {
		return nil, fmt.Errorf("pilot governance postgres: db is nil")
	}
	return &AuthorityIntervals{db: db}, nil
}

func (repository *AuthorityIntervals) ListCurrent(ctx context.Context) ([]domain.AuthorityInterval, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list authority intervals: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT object_scope, capability, fact_kind, authority, from_at, to_at
		   FROM pilot_governance.authority_interval
		  ORDER BY interval_id`)
	if err != nil {
		return nil, fmt.Errorf("list authority intervals: %w", err)
	}
	defer rows.Close()

	var intervals []domain.AuthorityInterval
	for rows.Next() {
		var interval domain.AuthorityInterval
		var toAt *time.Time
		if err := rows.Scan(&interval.ObjectScope, &interval.Capability,
			&interval.FactKind, &interval.Authority, &interval.From, &toAt); err != nil {
			return nil, fmt.Errorf("list authority intervals: %w", err)
		}
		interval.From = interval.From.UTC()
		if toAt != nil {
			interval.To = toAt.UTC()
		}
		intervals = append(intervals, interval)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list authority intervals: %w", err)
	}
	return intervals, nil
}

func (repository *AuthorityIntervals) Append(ctx context.Context, interval domain.AuthorityInterval) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("append authority interval: %w", err)
	}

	var toAt *time.Time
	if !interval.To.IsZero() {
		to := interval.To.UTC()
		toAt = &to
	}
	_, err = executor.Exec(ctx,
		`INSERT INTO pilot_governance.authority_interval
			(object_scope, capability, fact_kind, authority, from_at, to_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT DO NOTHING`,
		interval.ObjectScope,
		interval.Capability,
		interval.FactKind,
		interval.Authority,
		interval.From.UTC(),
		toAt,
	)
	if err != nil {
		return fmt.Errorf("append authority interval: %w", err)
	}
	return nil
}

func stageFrom(raw string) (domain.ExecutionStage, error) {
	for _, stage := range []domain.ExecutionStage{
		domain.NotYetInExecution,
		domain.HistoricalReplay,
		domain.ShadowRun,
		domain.LimitedProduction,
	} {
		if stage.String() == raw {
			return stage, nil
		}
	}
	return 0, fmt.Errorf("unknown execution stage %q", raw)
}

func verdictFrom(raw string) (domain.ReviewVerdict, error) {
	switch raw {
	case domain.StageGo.String():
		return domain.StageGo, nil
	case domain.StageNoGo.String():
		return domain.StageNoGo, nil
	default:
		return 0, fmt.Errorf("unknown review verdict %q", raw)
	}
}

func dispositionFrom(raw string) (domain.NoGoDisposition, error) {
	for _, disposition := range []domain.NoGoDisposition{
		domain.KeepCurrentScope,
		domain.SuspendNewAdmission,
		domain.FixAndReassess,
		domain.ObjectLevelTakeover,
	} {
		if disposition.String() == raw {
			return disposition, nil
		}
	}
	return 0, fmt.Errorf("unknown no-go disposition %q", raw)
}

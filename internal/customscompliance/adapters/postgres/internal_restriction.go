package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// InternalRestrictions 实现 ports.InternalRestrictionStore（ADR-0334，迁移 0028）。四张表：限制头、覆盖项、决定、决定条目；
// 读回把头与覆盖项交 EstablishInternalRestriction、决定按接受次序（sequence）逐条交 Decide，每一步都过领域构造重验——库里
// 没有任何限制级或覆盖项级的可覆盖状态列，适用性每次从覆盖项开始时的状态与决定重演。
type InternalRestrictions struct {
	db *bentopg.DB
}

func NewInternalRestrictions(db *bentopg.DB) (*InternalRestrictions, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &InternalRestrictions{db: db}, nil
}

// Save 写限制头与它的覆盖项。同标识已有记录时答`已有记录`（ADR-0031），覆盖项不再写。
func (repository *InternalRestrictions) Save(
	ctx context.Context,
	tenant domain.TenantID,
	restriction domain.InternalRestriction,
) (ports.InternalRestrictionSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.InternalRestrictionSaveOutcomeInvalid, fmt.Errorf("save internal restriction: %w", err)
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.internal_compliance_restriction
			(tenant_id, restriction_id, source_ref, basis_ref, decision_mode,
			 decided_by_ref, release_condition_ref, started_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), restriction.ID().String(), restriction.Source().String(), restriction.Basis().String(),
		restriction.Mode().String(), restriction.DecidedBy().String(), restriction.Condition().String(),
		restriction.StartedAt(),
	)
	if err != nil {
		return ports.InternalRestrictionSaveOutcomeInvalid, fmt.Errorf("save internal restriction: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.InternalRestrictionAlreadyRecorded, nil
	}
	for _, item := range restriction.Coverage() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO customs_compliance.internal_restriction_coverage
				(tenant_id, restriction_id, scope_ref, action, initial_standing)
			 VALUES ($1, $2, $3, $4, $5)`,
			tenant.String(), restriction.ID().String(), item.Scope.String(), item.Action.String(), item.Standing.String(),
		); err != nil {
			return ports.InternalRestrictionSaveOutcomeInvalid, fmt.Errorf("save internal restriction coverage: %w", err)
		}
	}
	return ports.InternalRestrictionSaved, nil
}

// AppendDecision 追加一条决定及其条目。决定标识重复、同一覆盖项第二次确认或解除由主键与部分唯一索引拒收，作为错误交回。
func (repository *InternalRestrictions) AppendDecision(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.InternalRestrictionID,
	decision domain.RestrictionDecision,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("append internal restriction decision: %w", err)
	}
	if _, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.internal_restriction_decision
			(tenant_id, restriction_id, decision_id, kind, source_ref, evidence_ref,
			 decided_by_ref, authority_ref, decided_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		tenant.String(), id.String(), decision.ID().String(), decision.Kind().String(), decision.Source().String(),
		decision.Evidence().String(), decision.DecidedBy().String(), decision.Authority().String(), decision.At(),
	); err != nil {
		return fmt.Errorf("append internal restriction decision: %w", err)
	}
	for _, key := range decision.Covers() {
		if _, err := executor.Exec(ctx,
			`INSERT INTO customs_compliance.internal_restriction_decision_item
				(tenant_id, restriction_id, decision_id, kind, scope_ref, action)
			 VALUES ($1, $2, $3, $4, $5, $6)`,
			tenant.String(), id.String(), decision.ID().String(), decision.Kind().String(),
			key.Scope.String(), key.Action.String(),
		); err != nil {
			return fmt.Errorf("append internal restriction decision item: %w", err)
		}
	}
	return nil
}

func (repository *InternalRestrictions) FindByID(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.InternalRestrictionID,
) (domain.InternalRestriction, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.InternalRestriction{}, false, fmt.Errorf("find internal restriction: %w", err)
	}
	return loadInternalRestriction(ctx, querier, tenant, id.String(), false)
}

// FindForDecision 先对限制头行取 FOR UPDATE 行锁再读回：锁与随后的追加同一事务，同一限制上的两个决定串行。
func (repository *InternalRestrictions) FindForDecision(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.InternalRestrictionID,
) (domain.InternalRestriction, bool, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return domain.InternalRestriction{}, false, fmt.Errorf("find internal restriction for decision: %w", err)
	}
	return loadInternalRestriction(ctx, executor, tenant, id.String(), true)
}

// ListInternalByScope 按（租户，范围）盘出覆盖该范围的全部限制，每条带它全部覆盖项与决定——某覆盖项已解除的限制照交，
// 动作时点上它是否仍约束由领域按时点判，库不预筛。次序按限制开始时间与标识稳定。
func (repository *InternalRestrictions) ListInternalByScope(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.DecisionScopeReference,
) ([]domain.InternalRestriction, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list internal restrictions: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT r.restriction_id
		   FROM customs_compliance.internal_compliance_restriction r
		  WHERE r.tenant_id = $1
		    AND EXISTS (SELECT 1 FROM customs_compliance.internal_restriction_coverage c
		                 WHERE c.tenant_id = r.tenant_id AND c.restriction_id = r.restriction_id
		                   AND c.scope_ref = $2)
		  ORDER BY r.started_at, r.restriction_id`,
		tenant.String(), scope.String())
	if err != nil {
		return nil, fmt.Errorf("list internal restrictions: %w", err)
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, fmt.Errorf("list internal restrictions: %w", err)
		}
		ids = append(ids, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list internal restrictions: %w", err)
	}
	restrictions := make([]domain.InternalRestriction, 0, len(ids))
	for _, id := range ids {
		restriction, found, err := loadInternalRestriction(ctx, querier, tenant, id, false)
		if err != nil {
			return nil, err
		}
		if found {
			restrictions = append(restrictions, restriction)
		}
	}
	return restrictions, nil
}

// loadInternalRestriction 读回一份限制：头与覆盖项交建立构造，决定按接受次序逐条交 Decide。读回过领域构造重验，库里的行
// 若与领域不变量对不上（迁移 CHECK 之外的坏行）作为错误交回，不悄悄修。lock 为真时对头行加 FOR UPDATE。
func loadInternalRestriction(
	ctx context.Context,
	querier bentopg.Querier,
	tenant domain.TenantID,
	restrictionID string,
	lock bool,
) (domain.InternalRestriction, bool, error) {
	headQuery := `SELECT source_ref, basis_ref, decision_mode, decided_by_ref, release_condition_ref, started_at
		   FROM customs_compliance.internal_compliance_restriction
		  WHERE tenant_id = $1 AND restriction_id = $2`
	if lock {
		headQuery += ` FOR UPDATE`
	}
	var (
		source, basis, mode, decidedBy, condition string
		startedAt                                 time.Time
	)
	if err := querier.QueryRow(ctx, headQuery, tenant.String(), restrictionID).
		Scan(&source, &basis, &mode, &decidedBy, &condition, &startedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return domain.InternalRestriction{}, false, nil
		}
		return domain.InternalRestriction{}, false, fmt.Errorf("load internal restriction: %w", err)
	}

	spec := domain.InternalRestrictionSpec{StartedAt: startedAt}
	var err error
	if spec.ID, err = domain.NewInternalRestrictionID(restrictionID); err != nil {
		return domain.InternalRestriction{}, false, fmt.Errorf("load internal restriction: %w", err)
	}
	if spec.Source, err = domain.NewRestrictionSource(source); err != nil {
		return domain.InternalRestriction{}, false, fmt.Errorf("load internal restriction: %w", err)
	}
	if spec.Basis, err = domain.NewRestrictionBasis(basis); err != nil {
		return domain.InternalRestriction{}, false, fmt.Errorf("load internal restriction: %w", err)
	}
	if spec.DecidedBy, err = domain.NewRestrictionDecider(decidedBy); err != nil {
		return domain.InternalRestriction{}, false, fmt.Errorf("load internal restriction: %w", err)
	}
	if spec.Condition, err = domain.NewRestrictionReleaseCondition(condition); err != nil {
		return domain.InternalRestriction{}, false, fmt.Errorf("load internal restriction: %w", err)
	}
	switch mode {
	case "RULE":
		spec.Mode = domain.RuleBasedDecision
	case "MANUAL":
		spec.Mode = domain.ManualDecision
	default:
		return domain.InternalRestriction{}, false, fmt.Errorf("load internal restriction: unknown decision mode %q", mode)
	}

	coverageRows, err := querier.Query(ctx,
		`SELECT scope_ref, action, initial_standing
		   FROM customs_compliance.internal_restriction_coverage
		  WHERE tenant_id = $1 AND restriction_id = $2
		  ORDER BY scope_ref, action`,
		tenant.String(), restrictionID)
	if err != nil {
		return domain.InternalRestriction{}, false, fmt.Errorf("load internal restriction coverage: %w", err)
	}
	for coverageRows.Next() {
		var scopeRef, action, standing string
		if err := coverageRows.Scan(&scopeRef, &action, &standing); err != nil {
			coverageRows.Close()
			return domain.InternalRestriction{}, false, fmt.Errorf("load internal restriction coverage: %w", err)
		}
		item, err := rebuildCoverage(scopeRef, action, standing)
		if err != nil {
			coverageRows.Close()
			return domain.InternalRestriction{}, false, err
		}
		spec.Coverage = append(spec.Coverage, item)
	}
	coverageRows.Close()
	if err := coverageRows.Err(); err != nil {
		return domain.InternalRestriction{}, false, fmt.Errorf("load internal restriction coverage: %w", err)
	}
	restriction, err := domain.EstablishInternalRestriction(spec)
	if err != nil {
		return domain.InternalRestriction{}, false, fmt.Errorf("rebuild internal restriction: %w", err)
	}

	decisionRows, err := querier.Query(ctx,
		`SELECT d.decision_id, d.kind, d.source_ref, d.evidence_ref, d.decided_by_ref, d.authority_ref, d.decided_at,
		        i.scope_ref, i.action
		   FROM customs_compliance.internal_restriction_decision d
		   JOIN customs_compliance.internal_restriction_decision_item i
		     ON i.tenant_id = d.tenant_id AND i.restriction_id = d.restriction_id AND i.decision_id = d.decision_id
		  WHERE d.tenant_id = $1 AND d.restriction_id = $2
		  ORDER BY d.sequence, i.scope_ref, i.action`,
		tenant.String(), restrictionID)
	if err != nil {
		return domain.InternalRestriction{}, false, fmt.Errorf("load internal restriction decisions: %w", err)
	}
	var (
		decisionSpecs []domain.RestrictionDecisionSpec
		lastID        string
	)
	for decisionRows.Next() {
		var (
			decisionID, kind, sourceRef, evidence, decider, authority, scopeRef, action string
			decidedAt                                                                   time.Time
		)
		if err := decisionRows.Scan(&decisionID, &kind, &sourceRef, &evidence, &decider, &authority, &decidedAt,
			&scopeRef, &action); err != nil {
			decisionRows.Close()
			return domain.InternalRestriction{}, false, fmt.Errorf("load internal restriction decisions: %w", err)
		}
		if decisionID != lastID {
			next, err := rebuildDecisionSpec(decisionID, kind, sourceRef, evidence, decider, authority, decidedAt)
			if err != nil {
				decisionRows.Close()
				return domain.InternalRestriction{}, false, err
			}
			decisionSpecs = append(decisionSpecs, next)
			lastID = decisionID
		}
		key, err := rebuildCoverageKey(scopeRef, action)
		if err != nil {
			decisionRows.Close()
			return domain.InternalRestriction{}, false, err
		}
		last := &decisionSpecs[len(decisionSpecs)-1]
		last.Covers = append(last.Covers, key)
	}
	decisionRows.Close()
	if err := decisionRows.Err(); err != nil {
		return domain.InternalRestriction{}, false, fmt.Errorf("load internal restriction decisions: %w", err)
	}
	for _, decisionSpec := range decisionSpecs {
		decision, err := domain.NewRestrictionDecision(decisionSpec)
		if err != nil {
			return domain.InternalRestriction{}, false, fmt.Errorf("rebuild internal restriction decision: %w", err)
		}
		if restriction, err = restriction.Decide(decision); err != nil {
			return domain.InternalRestriction{}, false, fmt.Errorf("replay internal restriction decision: %w", err)
		}
	}
	return restriction, true, nil
}

func rebuildCoverageKey(scopeRef, action string) (domain.CoverageKey, error) {
	scope, err := domain.NewDecisionScopeReference(scopeRef)
	if err != nil {
		return domain.CoverageKey{}, fmt.Errorf("rebuild internal restriction coverage: %w", err)
	}
	guarded, err := guardedActionFrom(action)
	if err != nil {
		return domain.CoverageKey{}, err
	}
	return domain.CoverageKey{Scope: scope, Action: guarded}, nil
}

func rebuildCoverage(scopeRef, action, standing string) (domain.CoverageSpec, error) {
	key, err := rebuildCoverageKey(scopeRef, action)
	if err != nil {
		return domain.CoverageSpec{}, err
	}
	item := domain.CoverageSpec{Scope: key.Scope, Action: key.Action}
	switch standing {
	case "IN_FORCE":
		item.Standing = domain.CoverageInForce
	case "UNCONFIRMED":
		item.Standing = domain.CoverageUnconfirmed
	case "CONFLICTING":
		item.Standing = domain.CoverageConflicting
	default:
		return domain.CoverageSpec{}, fmt.Errorf("customs compliance postgres: unknown coverage standing %q", standing)
	}
	return item, nil
}

func rebuildDecisionSpec(
	decisionID, kind, sourceRef, evidence, decider, authority string,
	decidedAt time.Time,
) (domain.RestrictionDecisionSpec, error) {
	spec := domain.RestrictionDecisionSpec{At: decidedAt}
	var err error
	if spec.ID, err = domain.NewRestrictionDecisionID(decisionID); err != nil {
		return spec, fmt.Errorf("rebuild internal restriction decision: %w", err)
	}
	if spec.Source, err = domain.NewRestrictionSource(sourceRef); err != nil {
		return spec, fmt.Errorf("rebuild internal restriction decision: %w", err)
	}
	if spec.Evidence, err = domain.NewRestrictionEvidence(evidence); err != nil {
		return spec, fmt.Errorf("rebuild internal restriction decision: %w", err)
	}
	if spec.DecidedBy, err = domain.NewRestrictionDecider(decider); err != nil {
		return spec, fmt.Errorf("rebuild internal restriction decision: %w", err)
	}
	if spec.Authority, err = domain.NewRestrictionAuthority(authority); err != nil {
		return spec, fmt.Errorf("rebuild internal restriction decision: %w", err)
	}
	switch kind {
	case "CONFIRM":
		spec.Kind = domain.ConfirmCoverage
	case "RELEASE":
		spec.Kind = domain.ReleaseCoverage
	default:
		return spec, fmt.Errorf("customs compliance postgres: unknown restriction decision kind %q", kind)
	}
	return spec, nil
}

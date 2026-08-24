package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// FollowUps 实现 ports.FollowUpStore。目标建立与替代关系建立走 ON CONFLICT 代数；
// 替代生效是同一关系的状态推进，UpdateRelation 只动生效三列。
type FollowUps struct {
	db *bentopg.DB
}

func NewFollowUps(db *bentopg.DB) (*FollowUps, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &FollowUps{db: db}, nil
}

func (repository *FollowUps) FindTarget(
	ctx context.Context,
	key ports.FollowUpTargetKey,
) (domain.FollowUpTarget, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.FollowUpTarget{}, false, fmt.Errorf("find follow-up target: %w", err)
	}

	var caseRef, unitID, scopeRef string
	var formedAt time.Time
	err = querier.QueryRow(ctx,
		`SELECT case_ref, unit_id, scope_ref, formed_at
		   FROM customs_compliance.follow_up_target
		  WHERE tenant_id = $1 AND trigger_ref = $2 AND version_id = $3 AND kind = $4`,
		key.TenantID.String(), key.Trigger.String(), key.Version.String(), key.Kind.String(),
	).Scan(&caseRef, &unitID, &scopeRef, &formedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.FollowUpTarget{}, false, nil
	}
	if err != nil {
		return domain.FollowUpTarget{}, false, fmt.Errorf("find follow-up target: %w", err)
	}

	target, err := rebuildFollowUpTarget(key, caseRef, unitID, scopeRef, formedAt)
	if err != nil {
		return domain.FollowUpTarget{}, false, fmt.Errorf("rebuild follow-up target: %w", err)
	}
	return target, true, nil
}

func (repository *FollowUps) SaveTarget(
	ctx context.Context,
	key ports.FollowUpTargetKey,
	target domain.FollowUpTarget,
) (ports.FollowUpSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.FollowUpSaveOutcomeInvalid, fmt.Errorf("save follow-up target: %w", err)
	}
	if key.Trigger != target.Trigger() ||
		key.Version != target.Version() ||
		key.Kind != target.Kind() {
		return ports.FollowUpSaveOutcomeInvalid,
			fmt.Errorf("save follow-up target: key disagrees with the target it claims to index")
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.follow_up_target
			(tenant_id, trigger_ref, version_id, kind, case_ref, unit_id, scope_ref, formed_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (tenant_id, trigger_ref, version_id, kind) DO NOTHING`,
		key.TenantID.String(),
		key.Trigger.String(),
		key.Version.String(),
		key.Kind.String(),
		target.CaseRef().String(),
		target.Unit().String(),
		target.Scope().String(),
		target.FormedAt(),
	)
	if err != nil {
		return ports.FollowUpSaveOutcomeInvalid, fmt.Errorf("save follow-up target: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.FollowUpAlreadyRecorded, nil
	}
	return ports.FollowUpSaved, nil
}

func (repository *FollowUps) FindRelation(
	ctx context.Context,
	key ports.FollowUpTargetKey,
) (domain.ReplacementRelation, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ReplacementRelation{}, false, fmt.Errorf("find replacement relation: %w", err)
	}

	var (
		caseRef, unitID, scopeRef, replacementUnit string
		formedAt                                   time.Time
		effective                                  bool
		externalResult                             *string
		effectiveAt                                *time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT t.case_ref, t.unit_id, t.scope_ref, t.formed_at,
		        r.replacement_unit, r.effective, r.external_result, r.effective_at
		   FROM customs_compliance.follow_up_replacement r
		   JOIN customs_compliance.follow_up_target t
		     ON t.tenant_id = r.tenant_id
		    AND t.trigger_ref = r.trigger_ref
		    AND t.version_id = r.version_id
		    AND t.kind = r.kind
		  WHERE r.tenant_id = $1 AND r.trigger_ref = $2
		    AND r.version_id = $3 AND r.kind = $4`,
		key.TenantID.String(), key.Trigger.String(), key.Version.String(), key.Kind.String(),
	).Scan(&caseRef, &unitID, &scopeRef, &formedAt,
		&replacementUnit, &effective, &externalResult, &effectiveAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ReplacementRelation{}, false, nil
	}
	if err != nil {
		return domain.ReplacementRelation{}, false, fmt.Errorf("find replacement relation: %w", err)
	}

	target, err := rebuildFollowUpTarget(key, caseRef, unitID, scopeRef, formedAt)
	if err != nil {
		return domain.ReplacementRelation{}, false, fmt.Errorf("rebuild replacement relation: %w", err)
	}
	unit, err := domain.NewDeclarationUnitID(replacementUnit)
	if err != nil {
		return domain.ReplacementRelation{}, false, fmt.Errorf("rebuild replacement relation: %w", err)
	}
	relation, err := domain.ProposeReplacement(target, unit)
	if err != nil {
		return domain.ReplacementRelation{}, false, fmt.Errorf("rebuild replacement relation: %w", err)
	}
	if effective {
		if externalResult == nil || effectiveAt == nil {
			return domain.ReplacementRelation{}, false,
				fmt.Errorf("rebuild replacement relation: effective row missing result or time")
		}
		relation, err = relation.TakeEffect(*externalResult, *effectiveAt)
		if err != nil {
			return domain.ReplacementRelation{}, false, fmt.Errorf("rebuild replacement relation: %w", err)
		}
	}
	return relation, true, nil
}

func (repository *FollowUps) SaveRelation(
	ctx context.Context,
	key ports.FollowUpTargetKey,
	relation domain.ReplacementRelation,
) (ports.FollowUpSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.FollowUpSaveOutcomeInvalid, fmt.Errorf("save replacement relation: %w", err)
	}
	target := relation.Target()
	if key.Trigger != target.Trigger() ||
		key.Version != target.Version() ||
		key.Kind != target.Kind() {
		return ports.FollowUpSaveOutcomeInvalid,
			fmt.Errorf("save replacement relation: key disagrees with the relation it claims to index")
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.follow_up_replacement
			(tenant_id, trigger_ref, version_id, kind, replacement_unit,
			 effective, external_result, effective_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (tenant_id, trigger_ref, version_id, kind) DO NOTHING`,
		key.TenantID.String(),
		key.Trigger.String(),
		key.Version.String(),
		key.Kind.String(),
		relation.ReplacementUnit().String(),
		relation.Effective(),
		effectiveResultColumn(relation),
		effectiveAtColumn(relation),
	)
	if err != nil {
		return ports.FollowUpSaveOutcomeInvalid, fmt.Errorf("save replacement relation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.FollowUpAlreadyRecorded, nil
	}
	return ports.FollowUpSaved, nil
}

func (repository *FollowUps) UpdateRelation(
	ctx context.Context,
	key ports.FollowUpTargetKey,
	relation domain.ReplacementRelation,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("update replacement relation: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`UPDATE customs_compliance.follow_up_replacement
		    SET effective = $5, external_result = $6, effective_at = $7
		  WHERE tenant_id = $1 AND trigger_ref = $2 AND version_id = $3 AND kind = $4`,
		key.TenantID.String(),
		key.Trigger.String(),
		key.Version.String(),
		key.Kind.String(),
		relation.Effective(),
		effectiveResultColumn(relation),
		effectiveAtColumn(relation),
	)
	if err != nil {
		return fmt.Errorf("update replacement relation: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update replacement relation: %s not found", key.Version)
	}
	return nil
}

func rebuildFollowUpTarget(
	key ports.FollowUpTargetKey,
	caseRef, unitID, scopeRef string,
	formedAt time.Time,
) (domain.FollowUpTarget, error) {
	caseID, err := domain.NewCustomsCaseID(caseRef)
	if err != nil {
		return domain.FollowUpTarget{}, err
	}
	unit, err := domain.NewDeclarationUnitID(unitID)
	if err != nil {
		return domain.FollowUpTarget{}, err
	}
	scope, err := domain.NewDecisionScopeReference(scopeRef)
	if err != nil {
		return domain.FollowUpTarget{}, err
	}
	return domain.FormFollowUpTarget(domain.FollowUpTargetSpec{
		Kind:     key.Kind,
		Trigger:  key.Trigger,
		CaseRef:  caseID,
		Unit:     unit,
		Version:  key.Version,
		Scope:    scope,
		FormedAt: formedAt,
	})
}

func effectiveResultColumn(relation domain.ReplacementRelation) *string {
	if result, ok := relation.ExternalResult(); ok {
		return &result
	}
	return nil
}

func effectiveAtColumn(relation domain.ReplacementRelation) *time.Time {
	if at, ok := relation.EffectiveAt(); ok {
		return &at
	}
	return nil
}

// Manifests 实现 ports.ManifestStore。Save 接受首版（同舱单已有引用答已有记录）；
// Update 推进来源版本——原引用与历史关联由领域留在新引用的 priorVersion 里。
type Manifests struct {
	db *bentopg.DB
}

func NewManifests(db *bentopg.DB) (*Manifests, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &Manifests{db: db}, nil
}

func (repository *Manifests) FindByManifest(
	ctx context.Context,
	tenant domain.TenantID,
	manifest domain.ExternalManifestID,
) (domain.ExternalManifestReference, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ExternalManifestReference{}, false, fmt.Errorf("find manifest reference: %w", err)
	}

	var (
		versionID, carrier, procedure, direction, scope, sourceFact string
		acceptedAt                                                  time.Time
		priorVersion, associationUnit                               *string
	)
	err = querier.QueryRow(ctx,
		`SELECT version_id, carrier_ref, procedure_ref, direction, scope_ref,
		        source_fact, accepted_at, prior_version, association_unit
		   FROM customs_compliance.external_manifest_reference
		  WHERE tenant_id = $1 AND manifest_id = $2`,
		tenant.String(), manifest.String(),
	).Scan(&versionID, &carrier, &procedure, &direction, &scope,
		&sourceFact, &acceptedAt, &priorVersion, &associationUnit)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ExternalManifestReference{}, false, nil
	}
	if err != nil {
		return domain.ExternalManifestReference{}, false, fmt.Errorf("find manifest reference: %w", err)
	}

	reference, err := rebuildManifest(manifest, versionID, carrier, procedure, direction,
		scope, sourceFact, acceptedAt, priorVersion, associationUnit)
	if err != nil {
		return domain.ExternalManifestReference{}, false, fmt.Errorf("rebuild manifest reference: %w", err)
	}
	return reference, true, nil
}

func (repository *Manifests) Save(
	ctx context.Context,
	tenant domain.TenantID,
	reference domain.ExternalManifestReference,
) (ports.ManifestSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ManifestSaveOutcomeInvalid, fmt.Errorf("save manifest reference: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.external_manifest_reference
			(tenant_id, manifest_id, version_id, carrier_ref, procedure_ref, direction,
			 scope_ref, source_fact, accepted_at, prior_version, association_unit)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT (tenant_id, manifest_id) DO NOTHING`,
		tenant.String(),
		reference.Manifest().String(),
		reference.Version().String(),
		reference.Carrier().String(),
		reference.Procedure().String(),
		reference.Direction().String(),
		reference.Scope().String(),
		reference.SourceFact(),
		reference.AcceptedAt(),
		manifestPriorColumn(reference),
		manifestAssociationColumn(reference),
	)
	if err != nil {
		return ports.ManifestSaveOutcomeInvalid, fmt.Errorf("save manifest reference: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.ManifestAlreadyRecorded, nil
	}
	return ports.ManifestSaved, nil
}

func (repository *Manifests) Update(
	ctx context.Context,
	tenant domain.TenantID,
	reference domain.ExternalManifestReference,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("update manifest reference: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`UPDATE customs_compliance.external_manifest_reference
		    SET version_id = $3, scope_ref = $4, source_fact = $5, accepted_at = $6,
		        prior_version = $7, association_unit = $8
		  WHERE tenant_id = $1 AND manifest_id = $2`,
		tenant.String(),
		reference.Manifest().String(),
		reference.Version().String(),
		reference.Scope().String(),
		reference.SourceFact(),
		reference.AcceptedAt(),
		manifestPriorColumn(reference),
		manifestAssociationColumn(reference),
	)
	if err != nil {
		return fmt.Errorf("update manifest reference: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update manifest reference: %s not found", reference.Manifest())
	}
	return nil
}

func rebuildManifest(
	manifest domain.ExternalManifestID,
	versionID, carrier, procedure, direction, scope, sourceFact string,
	acceptedAt time.Time,
	priorVersion, associationUnit *string,
) (domain.ExternalManifestReference, error) {
	spec := domain.ExternalManifestReferenceSpec{
		Manifest:   manifest,
		SourceFact: sourceFact,
		AcceptedAt: acceptedAt,
	}
	var err error
	if spec.Version, err = domain.NewManifestSourceVersion(versionID); err != nil {
		return domain.ExternalManifestReference{}, err
	}
	if spec.Carrier, err = domain.NewCarrierResponsibilityReference(carrier); err != nil {
		return domain.ExternalManifestReference{}, err
	}
	if spec.Procedure, err = domain.NewCustomsProcedureReference(procedure); err != nil {
		return domain.ExternalManifestReference{}, err
	}
	if spec.Direction, err = manifestDirectionFrom(direction); err != nil {
		return domain.ExternalManifestReference{}, err
	}
	if spec.Scope, err = domain.NewDecisionScopeReference(scope); err != nil {
		return domain.ExternalManifestReference{}, err
	}
	var prior domain.ManifestSourceVersion
	if priorVersion != nil {
		if prior, err = domain.NewManifestSourceVersion(*priorVersion); err != nil {
			return domain.ExternalManifestReference{}, err
		}
	}
	var association domain.DeclarationUnitID
	if associationUnit != nil {
		if association, err = domain.NewDeclarationUnitID(*associationUnit); err != nil {
			return domain.ExternalManifestReference{}, err
		}
	}
	return domain.RehydrateManifestReference(spec, prior, association)
}

func manifestDirectionFrom(value string) (domain.ManifestDirection, error) {
	switch value {
	case "IMPORT":
		return domain.ImportManifest, nil
	case "EXPORT":
		return domain.ExportManifest, nil
	default:
		return domain.ManifestDirectionInvalid,
			fmt.Errorf("customs compliance postgres: unknown manifest direction %q", value)
	}
}

func manifestPriorColumn(reference domain.ExternalManifestReference) *string {
	if version, ok := reference.PriorVersion(); ok {
		value := version.String()
		return &value
	}
	return nil
}

func manifestAssociationColumn(reference domain.ExternalManifestReference) *string {
	if unit, ok := reference.Association(); ok {
		value := unit.String()
		return &value
	}
	return nil
}

// CaseClosures 实现 ports.CaseClosureStore。一案至多一份关闭记录：首次关闭走
// ON CONFLICT 代数；重开追加只更新 reopenings 列，原关闭核对与决定时间不动。
type CaseClosures struct {
	db *bentopg.DB
}

func NewCaseClosures(db *bentopg.DB) (*CaseClosures, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &CaseClosures{db: db}, nil
}

type closureItemRow struct {
	Obligation string `json:"obligation"`
	Scope      string `json:"scope"`
	State      string `json:"state"`
	Basis      string `json:"basis"`
	HandedTo   string `json:"handedTo,omitempty"`
}

type reopeningRow struct {
	LateFact      string    `json:"lateFact"`
	AffectedItems []string  `json:"affectedItems"`
	Authority     string    `json:"authority"`
	ReopenedAt    time.Time `json:"reopenedAt"`
}

func (repository *CaseClosures) FindByCase(
	ctx context.Context,
	tenant domain.TenantID,
	caseRef domain.CustomsCaseID,
) (*domain.CustomsCaseClosure, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("find case closure: %w", err)
	}

	var (
		cutoffAt, verifiedAt, closedAt time.Time
		decidedBy                      string
		itemsRaw, reopeningsRaw        []byte
	)
	err = querier.QueryRow(ctx,
		`SELECT cutoff_at, verified_at, decided_by, closed_at, items, reopenings
		   FROM customs_compliance.case_closure
		  WHERE tenant_id = $1 AND case_ref = $2`,
		tenant.String(), caseRef.String(),
	).Scan(&cutoffAt, &verifiedAt, &decidedBy, &closedAt, &itemsRaw, &reopeningsRaw)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("find case closure: %w", err)
	}

	closure, err := rebuildCaseClosure(caseRef, cutoffAt, verifiedAt, decidedBy, closedAt, itemsRaw, reopeningsRaw)
	if err != nil {
		return nil, false, fmt.Errorf("rebuild case closure: %w", err)
	}
	return closure, true, nil
}

func (repository *CaseClosures) Save(
	ctx context.Context,
	tenant domain.TenantID,
	closure *domain.CustomsCaseClosure,
) (ports.CaseClosureSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CaseClosureSaveOutcomeInvalid, fmt.Errorf("save case closure: %w", err)
	}
	if closure == nil {
		return ports.CaseClosureSaveOutcomeInvalid, fmt.Errorf("save case closure: closure is nil")
	}

	itemsRaw, err := marshalClosureItems(closure.Verification().Items())
	if err != nil {
		return ports.CaseClosureSaveOutcomeInvalid, fmt.Errorf("save case closure: %w", err)
	}
	reopeningsRaw, err := marshalReopenings(closure.Reopenings())
	if err != nil {
		return ports.CaseClosureSaveOutcomeInvalid, fmt.Errorf("save case closure: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.case_closure
			(tenant_id, case_ref, cutoff_at, verified_at, decided_by, closed_at, items, reopenings)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT (tenant_id, case_ref) DO NOTHING`,
		tenant.String(),
		closure.CaseRef().String(),
		closure.Verification().CutoffAt(),
		closure.Verification().VerifiedAt(),
		closure.DecidedBy(),
		closure.ClosedAt(),
		itemsRaw,
		reopeningsRaw,
	)
	if err != nil {
		return ports.CaseClosureSaveOutcomeInvalid, fmt.Errorf("save case closure: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return ports.CaseClosureSaved, nil
	}

	// 同案已有关闭：重开追加只动 reopenings，原关闭列不动。没有新重开就是已有记录。
	if len(closure.Reopenings()) == 0 {
		return ports.CaseClosureAlreadyRecorded, nil
	}
	tag, err = executor.Exec(ctx,
		`UPDATE customs_compliance.case_closure
		    SET reopenings = $3
		  WHERE tenant_id = $1 AND case_ref = $2`,
		tenant.String(), closure.CaseRef().String(), reopeningsRaw,
	)
	if err != nil {
		return ports.CaseClosureSaveOutcomeInvalid, fmt.Errorf("save case closure reopenings: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CaseClosureSaveOutcomeInvalid, fmt.Errorf("save case closure reopenings: %s not found", closure.CaseRef())
	}
	return ports.CaseClosureSaved, nil
}

func marshalClosureItems(items []domain.ClosureObligationItem) ([]byte, error) {
	rows := make([]closureItemRow, 0, len(items))
	for _, item := range items {
		rows = append(rows, closureItemRow{
			Obligation: item.Obligation,
			Scope:      item.Scope,
			State:      item.State.String(),
			Basis:      item.Basis,
			HandedTo:   item.HandedTo,
		})
	}
	return json.Marshal(rows)
}

func marshalReopenings(reopenings []domain.ControlledReopening) ([]byte, error) {
	rows := make([]reopeningRow, 0, len(reopenings))
	for _, reopening := range reopenings {
		rows = append(rows, reopeningRow{
			LateFact:      reopening.LateFact,
			AffectedItems: append([]string(nil), reopening.AffectedItems...),
			Authority:     reopening.Authority,
			ReopenedAt:    reopening.ReopenedAt,
		})
	}
	return json.Marshal(rows)
}

func rebuildCaseClosure(
	caseRef domain.CustomsCaseID,
	cutoffAt, verifiedAt time.Time,
	decidedBy string,
	closedAt time.Time,
	itemsRaw, reopeningsRaw []byte,
) (*domain.CustomsCaseClosure, error) {
	var itemRows []closureItemRow
	if err := json.Unmarshal(itemsRaw, &itemRows); err != nil {
		return nil, fmt.Errorf("items: %w", err)
	}
	items := make([]domain.ClosureObligationItem, 0, len(itemRows))
	for _, row := range itemRows {
		state, err := obligationStateFrom(row.State)
		if err != nil {
			return nil, err
		}
		items = append(items, domain.ClosureObligationItem{
			Obligation: row.Obligation,
			Scope:      row.Scope,
			State:      state,
			Basis:      row.Basis,
			HandedTo:   row.HandedTo,
		})
	}
	verification, err := domain.VerifyClosure(caseRef, cutoffAt, items, verifiedAt)
	if err != nil {
		return nil, err
	}
	closure, err := domain.CloseCase(verification, decidedBy, closedAt)
	if err != nil {
		return nil, err
	}

	var reopeningRows []reopeningRow
	if err := json.Unmarshal(reopeningsRaw, &reopeningRows); err != nil {
		return nil, fmt.Errorf("reopenings: %w", err)
	}
	for _, row := range reopeningRows {
		if err := closure.Reopen(domain.ControlledReopening{
			LateFact:      row.LateFact,
			AffectedItems: append([]string(nil), row.AffectedItems...),
			Authority:     row.Authority,
			ReopenedAt:    row.ReopenedAt,
		}); err != nil {
			return nil, err
		}
	}
	return closure, nil
}

func obligationStateFrom(value string) (domain.ObligationItemState, error) {
	switch value {
	case "CONCLUDED":
		return domain.ObligationConcluded, nil
	case "HANDED_OVER":
		return domain.ObligationHandedOver, nil
	case "UNRESOLVED":
		return domain.ObligationUnresolved, nil
	default:
		return domain.ObligationItemStateInvalid,
			fmt.Errorf("customs compliance postgres: unknown obligation state %q", value)
	}
}

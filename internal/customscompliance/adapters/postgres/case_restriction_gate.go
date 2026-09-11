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

// 本文件是案件、限制与门禁三库的 PostgreSQL 适配器。三者都经领域构造门读回：案件
// 与门禁不可变（重建即重走构造全不变量），限制的解除是单事件精确重放（建立 + 一次
// ReleaseByRegulatoryOutcome——值都在手，不是按时序重演）。

// CustomsCases 实现 ports.CustomsCaseStore（写入代数同 ADR-0031）。案件是责任容器
// 不是状态机——适配器没有 UPDATE 语句。
type CustomsCases struct {
	db *bentopg.DB
}

func NewCustomsCases(db *bentopg.DB) (*CustomsCases, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &CustomsCases{db: db}, nil
}

// caseParcelRow 与 caseRoleRow 是 jsonb 里的行形状——只引用不复制源事实。
type caseParcelRow struct {
	Parcel    string `json:"parcel"`
	Customer  string `json:"customer"`
	SourceRef string `json:"sourceRef"`
}

type caseRoleRow struct {
	Role      string `json:"role"`
	Party     string `json:"party"`
	Authority string `json:"authority"`
}

// FindByKey 按（租户+四维监管范围）取回案件。读回经 EstablishCustomsCase 整门重验
// ——包裹关联非空不重、角色快照逐项完整都在那里把关。
func (repository *CustomsCases) FindByKey(
	ctx context.Context,
	key ports.CustomsCaseKey,
) (domain.CustomsCase, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.CustomsCase{}, false, fmt.Errorf("find customs case: %w", err)
	}

	var (
		caseID               string
		parcelsRaw, rolesRaw []byte
		establishedAt        time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT case_id, parcels, roles, established_at
		   FROM customs_compliance.customs_case
		  WHERE tenant_id = $1 AND jurisdiction_ref = $2 AND direction = $3
		    AND procedure_ref = $4 AND obligation_ref = $5`,
		key.TenantID.String(), key.Jurisdiction.String(), key.Direction.String(),
		key.Procedure.String(), key.Obligation.String(),
	).Scan(&caseID, &parcelsRaw, &rolesRaw, &establishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CustomsCase{}, false, nil
	}
	if err != nil {
		return domain.CustomsCase{}, false, fmt.Errorf("find customs case: %w", err)
	}

	id, err := domain.NewCustomsCaseID(caseID)
	if err != nil {
		return domain.CustomsCase{}, false, fmt.Errorf("rebuild customs case: %w", err)
	}
	customsCase, err := rebuildCustomsCase(
		id, key.Jurisdiction, key.Direction, key.Procedure, key.Obligation,
		parcelsRaw, rolesRaw, establishedAt)
	if err != nil {
		return domain.CustomsCase{}, false, err
	}
	return customsCase, true, nil
}

// FindByID 按铸造标识取回案件（ADR-0073 决定五的反查读口）：提交链写入前核案件存在
// 靠它。库侧 customs_case_id_unique 保同租户一标识至多一行。
func (repository *CustomsCases) FindByID(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.CustomsCaseID,
) (domain.CustomsCase, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.CustomsCase{}, false, fmt.Errorf("find customs case by id: %w", err)
	}

	var (
		jurisdiction, direction, procedure, obligation string
		parcelsRaw, rolesRaw                           []byte
		establishedAt                                  time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT jurisdiction_ref, direction, procedure_ref, obligation_ref,
		        parcels, roles, established_at
		   FROM customs_compliance.customs_case
		  WHERE tenant_id = $1 AND case_id = $2`,
		tenant.String(), id.String(),
	).Scan(&jurisdiction, &direction, &procedure, &obligation, &parcelsRaw, &rolesRaw, &establishedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.CustomsCase{}, false, nil
	}
	if err != nil {
		return domain.CustomsCase{}, false, fmt.Errorf("find customs case by id: %w", err)
	}

	jurisdictionRef, err := domain.NewRegulatoryJurisdictionReference(jurisdiction)
	if err != nil {
		return domain.CustomsCase{}, false, fmt.Errorf("rebuild customs case: %w", err)
	}
	directionValue, err := manifestDirectionFrom(direction)
	if err != nil {
		return domain.CustomsCase{}, false, fmt.Errorf("rebuild customs case: %w", err)
	}
	procedureRef, err := domain.NewCustomsProcedureReference(procedure)
	if err != nil {
		return domain.CustomsCase{}, false, fmt.Errorf("rebuild customs case: %w", err)
	}
	obligationRef, err := domain.NewObligationScopeReference(obligation)
	if err != nil {
		return domain.CustomsCase{}, false, fmt.Errorf("rebuild customs case: %w", err)
	}
	customsCase, err := rebuildCustomsCase(
		id, jurisdictionRef, directionValue, procedureRef, obligationRef,
		parcelsRaw, rolesRaw, establishedAt)
	if err != nil {
		return domain.CustomsCase{}, false, err
	}
	return customsCase, true, nil
}

// rebuildCustomsCase 把行状态折回案件聚合，两个读口共用：读回经 EstablishCustomsCase
// 整门重验，坏行在这里炸成错误不进编排。
func rebuildCustomsCase(
	id domain.CustomsCaseID,
	jurisdiction domain.RegulatoryJurisdictionReference,
	direction domain.ManifestDirection,
	procedure domain.CustomsProcedureReference,
	obligation domain.ObligationScopeReference,
	parcelsRaw, rolesRaw []byte,
	establishedAt time.Time,
) (domain.CustomsCase, error) {
	spec := domain.CustomsCaseSpec{
		ID:            id,
		Jurisdiction:  jurisdiction,
		Direction:     direction,
		Procedure:     procedure,
		Obligation:    obligation,
		EstablishedAt: establishedAt,
	}
	var parcels []caseParcelRow
	if err := json.Unmarshal(parcelsRaw, &parcels); err != nil {
		return domain.CustomsCase{}, fmt.Errorf("rebuild customs case: parcels: %w", err)
	}
	for _, row := range parcels {
		spec.Parcels = append(spec.Parcels, domain.CaseParcelAssociation{
			Parcel:    row.Parcel,
			Customer:  row.Customer,
			SourceRef: row.SourceRef,
		})
	}
	var roles []caseRoleRow
	if err := json.Unmarshal(rolesRaw, &roles); err != nil {
		return domain.CustomsCase{}, fmt.Errorf("rebuild customs case: roles: %w", err)
	}
	for _, row := range roles {
		spec.Roles = append(spec.Roles, domain.CaseRoleSnapshot{
			Role:      row.Role,
			Party:     row.Party,
			Authority: row.Authority,
		})
	}

	customsCase, err := domain.EstablishCustomsCase(spec)
	if err != nil {
		return domain.CustomsCase{}, fmt.Errorf("rebuild customs case: %w", err)
	}
	return customsCase, nil
}

// Save 写下一个案件。同一法律行为已有案件时答`已有记录`——业务答案不是错误
// （ADR-0031）；ON CONFLICT DO NOTHING 保事务可用，编排拿到它还要同事务读回赢家作答。
func (repository *CustomsCases) Save(
	ctx context.Context,
	key ports.CustomsCaseKey,
	customsCase domain.CustomsCase,
) (ports.CustomsCaseSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CustomsCaseSaveOutcomeInvalid, fmt.Errorf("save customs case: %w", err)
	}
	if key.Jurisdiction != customsCase.Jurisdiction() ||
		key.Direction != customsCase.Direction() ||
		key.Procedure != customsCase.Procedure() ||
		key.Obligation != customsCase.Obligation() {
		return ports.CustomsCaseSaveOutcomeInvalid,
			fmt.Errorf("save customs case: key disagrees with the case it claims to index")
	}

	parcels := make([]caseParcelRow, 0, len(customsCase.Parcels()))
	for _, association := range customsCase.Parcels() {
		parcels = append(parcels, caseParcelRow{
			Parcel:    association.Parcel,
			Customer:  association.Customer,
			SourceRef: association.SourceRef,
		})
	}
	roles := make([]caseRoleRow, 0, len(customsCase.Roles()))
	for _, snapshot := range customsCase.Roles() {
		roles = append(roles, caseRoleRow{
			Role:      snapshot.Role,
			Party:     snapshot.Party,
			Authority: snapshot.Authority,
		})
	}
	parcelsRaw, err := json.Marshal(parcels)
	if err != nil {
		return ports.CustomsCaseSaveOutcomeInvalid, fmt.Errorf("save customs case: %w", err)
	}
	rolesRaw, err := json.Marshal(roles)
	if err != nil {
		return ports.CustomsCaseSaveOutcomeInvalid, fmt.Errorf("save customs case: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.customs_case
			(tenant_id, jurisdiction_ref, direction, procedure_ref, obligation_ref,
			 case_id, parcels, roles, established_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (tenant_id, jurisdiction_ref, direction, procedure_ref, obligation_ref)
		 DO NOTHING`,
		key.TenantID.String(),
		key.Jurisdiction.String(),
		key.Direction.String(),
		key.Procedure.String(),
		key.Obligation.String(),
		customsCase.ID().String(),
		parcelsRaw,
		rolesRaw,
		customsCase.EstablishedAt(),
	)
	if err != nil {
		return ports.CustomsCaseSaveOutcomeInvalid, fmt.Errorf("save customs case: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CustomsCaseAlreadyRecorded, nil
	}
	return ports.CustomsCaseSaved, nil
}

// Restrictions 实现 ports.RestrictionStore。建立与解除分开：建立走 ON CONFLICT
// 代数，解除是同一限制的状态推进——Update 只动解除两列，约束集与范围在建立时固定。
type Restrictions struct {
	db *bentopg.DB
}

func NewRestrictions(db *bentopg.DB) (*Restrictions, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &Restrictions{db: db}, nil
}

func (repository *Restrictions) FindByID(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.RestrictionID,
) (domain.RegulatoryRestriction, bool, error) {
	restrictions, err := repository.list(ctx,
		`tenant_id = $1 AND restriction_id = $2`,
		tenant.String(), id.String())
	if err != nil {
		return domain.RegulatoryRestriction{}, false, err
	}
	if len(restrictions) == 0 {
		return domain.RegulatoryRestriction{}, false, nil
	}
	return restrictions[0], true, nil
}

// ListByScope 按（租户+范围）盘出全部限制——仍有效与否由领域判（准入判断自己跳过
// 已解除的），库不预筛；次序按生效时间与标识稳定。
func (repository *Restrictions) ListByScope(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.DecisionScopeReference,
) ([]domain.RegulatoryRestriction, error) {
	return repository.list(ctx,
		`tenant_id = $1 AND scope_ref = $2`,
		tenant.String(), scope.String())
}

func (repository *Restrictions) list(
	ctx context.Context,
	where string,
	args ...any,
) ([]domain.RegulatoryRestriction, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list restrictions: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT restriction_id, decision_id, scope_ref, constrains, effective_at,
		        released_by, released_at
		   FROM customs_compliance.regulatory_restriction
		  WHERE `+where+`
		  ORDER BY effective_at, restriction_id`,
		args...,
	)
	if err != nil {
		return nil, fmt.Errorf("list restrictions: %w", err)
	}
	defer rows.Close()

	var restrictions []domain.RegulatoryRestriction
	for rows.Next() {
		var (
			restrictionID, decisionID, scopeRef string
			constrainsRaw                       []byte
			effectiveAt                         time.Time
			releasedBy                          *string
			releasedAt                          *time.Time
		)
		if err := rows.Scan(&restrictionID, &decisionID, &scopeRef, &constrainsRaw,
			&effectiveAt, &releasedBy, &releasedAt); err != nil {
			return nil, fmt.Errorf("list restrictions: %w", err)
		}
		restriction, err := rebuildRestriction(
			restrictionID, decisionID, scopeRef, constrainsRaw, effectiveAt, releasedBy, releasedAt)
		if err != nil {
			return nil, err
		}
		restrictions = append(restrictions, restriction)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list restrictions: %w", err)
	}
	return restrictions, nil
}

// Save 建立一份限制。同标识已有记录时答`已有记录`（ADR-0031）。建立时解除列照对象
// 实况序列化——正常建立是 NULL，形状由迁移 CHECK 把关。
func (repository *Restrictions) Save(
	ctx context.Context,
	tenant domain.TenantID,
	restriction domain.RegulatoryRestriction,
) (ports.RestrictionSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.RestrictionSaveOutcomeInvalid, fmt.Errorf("save restriction: %w", err)
	}

	constrainsRaw, err := marshalGuardedActions(restriction.Constrains())
	if err != nil {
		return ports.RestrictionSaveOutcomeInvalid, fmt.Errorf("save restriction: %w", err)
	}
	releasedBy, releasedAt := releaseColumns(restriction)

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.regulatory_restriction
			(tenant_id, restriction_id, decision_id, scope_ref, constrains,
			 effective_at, released_by, released_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT DO NOTHING`,
		tenant.String(),
		restriction.ID().String(),
		restriction.Decision().String(),
		restriction.Scope().String(),
		constrainsRaw,
		restriction.EffectiveAt(),
		releasedBy,
		releasedAt,
	)
	if err != nil {
		return ports.RestrictionSaveOutcomeInvalid, fmt.Errorf("save restriction: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.RestrictionAlreadyRecorded, nil
	}
	return ports.RestrictionSaved, nil
}

// Update 落解除这一步状态推进：只动解除两列——限制身份、约束集与范围在建立时固定，
// 重写等值也不给入口。行不存在如实报错；租户条件进语句（ADR-0003）。
func (repository *Restrictions) Update(
	ctx context.Context,
	tenant domain.TenantID,
	restriction domain.RegulatoryRestriction,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("update restriction: %w", err)
	}
	releasedBy, releasedAt := releaseColumns(restriction)

	tag, err := executor.Exec(ctx,
		`UPDATE customs_compliance.regulatory_restriction
		    SET released_by = $3, released_at = $4
		  WHERE tenant_id = $1 AND restriction_id = $2`,
		tenant.String(),
		restriction.ID().String(),
		releasedBy,
		releasedAt,
	)
	if err != nil {
		return fmt.Errorf("update restriction: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("update restriction: %s not found", restriction.ID())
	}
	return nil
}

// GateVerifications 实现 ports.GateVerificationStore（写入代数同 ADR-0031）。条件
// 状态变化换指纹换版——同一状态重复核对不出第二版，由主键含指纹承担。
type GateVerifications struct {
	db *bentopg.DB
}

func NewGateVerifications(db *bentopg.DB) (*GateVerifications, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &GateVerifications{db: db}, nil
}

// FindByKey 按（租户+三维身份+指纹）取回门禁核对。读回经 VerifyReleaseGate 整门
// 重验——非`不适用`必带前置条件清单在那里把关。
func (repository *GateVerifications) FindByKey(
	ctx context.Context,
	key ports.GateVerificationKey,
) (domain.ReleaseGateVerification, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ReleaseGateVerification{}, false, fmt.Errorf("find gate verification: %w", err)
	}

	var (
		preconditionsRaw []byte
		conclusionRaw    string
		verifiedAt       time.Time
		reading          dutyReadingColumns
	)
	err = querier.QueryRow(ctx,
		`SELECT preconditions, conclusion, verified_at,
		        duty_state, duty_coverage, duty_delta, duty_validity, duty_ref, funds_ref, duty_version_digest
		   FROM customs_compliance.gate_verification
		  WHERE tenant_id = $1 AND scope_ref = $2 AND action = $3
		    AND boundary_ref = $4 AND findings_digest = $5`,
		key.TenantID.String(), key.Scope.String(), key.Action.String(),
		key.Boundary.String(), key.Digest,
	).Scan(&preconditionsRaw, &conclusionRaw, &verifiedAt,
		&reading.state, &reading.coverage, &reading.delta, &reading.validity,
		&reading.duty, &reading.funds, &reading.version)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ReleaseGateVerification{}, false, nil
	}
	if err != nil {
		return domain.ReleaseGateVerification{}, false, fmt.Errorf("find gate verification: %w", err)
	}

	var references []string
	if err := json.Unmarshal(preconditionsRaw, &references); err != nil {
		return domain.ReleaseGateVerification{}, false, fmt.Errorf("rebuild gate verification: %w", err)
	}
	preconditions := make([]domain.PreconditionReference, 0, len(references))
	for _, reference := range references {
		precondition, err := domain.NewPreconditionReference(reference)
		if err != nil {
			return domain.ReleaseGateVerification{}, false, fmt.Errorf("rebuild gate verification: %w", err)
		}
		preconditions = append(preconditions, precondition)
	}
	conclusion, err := gateConclusionFrom(conclusionRaw)
	if err != nil {
		return domain.ReleaseGateVerification{}, false, err
	}

	gate, err := domain.VerifyReleaseGate(
		key.Scope, key.Action, key.Boundary, preconditions, conclusion, verifiedAt)
	if err != nil {
		return domain.ReleaseGateVerification{}, false, fmt.Errorf("rebuild gate verification: %w", err)
	}
	if reading.present() {
		attached, err := reading.attachTo(gate)
		if err != nil {
			return domain.ReleaseGateVerification{}, false, fmt.Errorf("rebuild gate verification: %w", err)
		}
		gate = attached
	}
	return gate, true, nil
}

// dutyReadingColumns 是门禁记录上「税费付款」那一道读数的七列（0019 加列，票 sa-cc/06），同生同灭。
// 没挂读数的版本七列皆 NULL——不构成前置条件那一形，或加列之前的旧版。
type dutyReadingColumns struct {
	state, coverage, delta, validity *string
	duty, funds, version             *string
}

func (columns dutyReadingColumns) present() bool {
	return columns.state != nil
}

// attachTo 把七列折回读数挂到判断上；词形经封闭集译回，集外即库被旁路改过。
func (columns dutyReadingColumns) attachTo(gate domain.ReleaseGateVerification) (domain.ReleaseGateVerification, error) {
	if columns.coverage == nil || columns.delta == nil || columns.validity == nil ||
		columns.duty == nil || columns.funds == nil || columns.version == nil {
		return domain.ReleaseGateVerification{}, errors.New("the duty payment reading columns are not paired")
	}
	state, err := closedWord(*columns.state, domain.PreconditionMet, domain.PreconditionUnmet)
	if err != nil {
		return domain.ReleaseGateVerification{}, err
	}
	coverage, err := closedWord(*columns.coverage, domain.CoverageNone, domain.CoveragePartial, domain.CoverageFull)
	if err != nil {
		return domain.ReleaseGateVerification{}, err
	}
	delta, err := closedWord(*columns.delta, domain.DeltaNone, domain.DeltaShort, domain.DeltaExcess)
	if err != nil {
		return domain.ReleaseGateVerification{}, err
	}
	validity, err := closedWord(*columns.validity, domain.FundsFactValid, domain.FundsFactInvalidated)
	if err != nil {
		return domain.ReleaseGateVerification{}, err
	}
	duty, err := domain.NewAssessedDutyReference(*columns.duty)
	if err != nil {
		return domain.ReleaseGateVerification{}, err
	}
	funds, err := domain.NewExternalFundsFactReference(*columns.funds)
	if err != nil {
		return domain.ReleaseGateVerification{}, err
	}
	return gate.WithDutyPayment(domain.DutyPaymentGateReading{
		State:        state,
		Coverage:     coverage,
		Delta:        delta,
		Validity:     validity,
		Verification: domain.DutyVerificationReference{Duty: duty, Funds: funds, Version: *columns.version},
	})
}

// dutyReadingArgs 把判断上的读数展成七个 INSERT 参数；没挂读数即七个 NULL。
func dutyReadingArgs(gate domain.ReleaseGateVerification) []any {
	reading, has := gate.DutyPayment()
	if !has {
		return []any{nil, nil, nil, nil, nil, nil, nil}
	}
	return []any{
		reading.State.String(), reading.Coverage.String(), reading.Delta.String(), reading.Validity.String(),
		reading.Verification.Duty.String(), reading.Verification.Funds.String(), reading.Verification.Version,
	}
}

// Save 写下一次门禁核对。同键已有记录时答`已有记录`（ADR-0031）——同一条件状态
// 重复核对不出第二版；键与判断分岔的写入在门口拦（键不是标签，是判断身份）。
func (repository *GateVerifications) Save(
	ctx context.Context,
	key ports.GateVerificationKey,
	gate domain.ReleaseGateVerification,
) (ports.GateVerificationSaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.GateVerificationSaveOutcomeInvalid, fmt.Errorf("save gate verification: %w", err)
	}
	if key.Scope != gate.Scope() || !gate.AppliesTo(key.Action, key.Boundary) {
		return ports.GateVerificationSaveOutcomeInvalid,
			fmt.Errorf("save gate verification: key disagrees with the verification it claims to index")
	}

	references := make([]string, 0, len(gate.Preconditions()))
	for _, precondition := range gate.Preconditions() {
		references = append(references, precondition.String())
	}
	preconditionsRaw, err := json.Marshal(references)
	if err != nil {
		return ports.GateVerificationSaveOutcomeInvalid, fmt.Errorf("save gate verification: %w", err)
	}

	args := []any{
		key.TenantID.String(),
		key.Scope.String(),
		key.Action.String(),
		key.Boundary.String(),
		key.Digest,
		preconditionsRaw,
		gate.Conclusion().String(),
		gate.VerifiedAt(),
	}
	args = append(args, dutyReadingArgs(gate)...)
	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.gate_verification
			(tenant_id, scope_ref, action, boundary_ref, findings_digest,
			 preconditions, conclusion, verified_at,
			 duty_state, duty_coverage, duty_delta, duty_validity, duty_ref, funds_ref, duty_version_digest)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		 ON CONFLICT DO NOTHING`,
		args...,
	)
	if err != nil {
		return ports.GateVerificationSaveOutcomeInvalid, fmt.Errorf("save gate verification: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.GateVerificationAlreadyRecorded, nil
	}
	return ports.GateVerificationSaved, nil
}

// rebuildRestriction 把一行译回限制：建立经 EstablishRestriction 整门重验；已解除
// 的行再精确重放一次 ReleaseByRegulatoryOutcome——单事件、值都在手，不是按时序重演。
func rebuildRestriction(
	restrictionID, decisionID, scopeRef string,
	constrainsRaw []byte,
	effectiveAt time.Time,
	releasedBy *string,
	releasedAt *time.Time,
) (domain.RegulatoryRestriction, error) {
	spec := domain.RegulatoryRestrictionSpec{EffectiveAt: effectiveAt}
	var err error
	if spec.ID, err = domain.NewRestrictionID(restrictionID); err != nil {
		return domain.RegulatoryRestriction{}, fmt.Errorf("rebuild restriction: %w", err)
	}
	if spec.Decision, err = domain.NewRegulatoryDecisionID(decisionID); err != nil {
		return domain.RegulatoryRestriction{}, fmt.Errorf("rebuild restriction: %w", err)
	}
	if spec.Scope, err = domain.NewDecisionScopeReference(scopeRef); err != nil {
		return domain.RegulatoryRestriction{}, fmt.Errorf("rebuild restriction: %w", err)
	}
	var actions []string
	if err := json.Unmarshal(constrainsRaw, &actions); err != nil {
		return domain.RegulatoryRestriction{}, fmt.Errorf("rebuild restriction: constrains: %w", err)
	}
	for _, action := range actions {
		guarded, err := guardedActionFrom(action)
		if err != nil {
			return domain.RegulatoryRestriction{}, err
		}
		spec.Constrains = append(spec.Constrains, guarded)
	}

	restriction, err := domain.EstablishRestriction(spec)
	if err != nil {
		return domain.RegulatoryRestriction{}, fmt.Errorf("rebuild restriction: %w", err)
	}
	if releasedBy == nil {
		return restriction, nil
	}
	release, err := domain.NewRegulatoryReleaseReference(*releasedBy)
	if err != nil {
		return domain.RegulatoryRestriction{}, fmt.Errorf("rebuild restriction: %w", err)
	}
	if releasedAt == nil {
		// 迁移 CHECK 拦了单边解除；走到这里说明库被绕过迁移改写。
		return domain.RegulatoryRestriction{}, fmt.Errorf("rebuild restriction: release columns disagree")
	}
	released, err := restriction.ReleaseByRegulatoryOutcome(release, *releasedAt)
	if err != nil {
		return domain.RegulatoryRestriction{}, fmt.Errorf("rebuild restriction: %w", err)
	}
	return released, nil
}

func releaseColumns(restriction domain.RegulatoryRestriction) (*string, *time.Time) {
	release, at, released := restriction.Release()
	if !released {
		return nil, nil
	}
	reference := release.String()
	return &reference, &at
}

func marshalGuardedActions(actions []domain.GuardedAction) ([]byte, error) {
	values := make([]string, 0, len(actions))
	for _, action := range actions {
		values = append(values, action.String())
	}
	return json.Marshal(values)
}

// guardedActionFrom 把列值译回封闭四值。迁移 CHECK 已拦住集合外取值。
func guardedActionFrom(value string) (domain.GuardedAction, error) {
	switch value {
	case "OUTBOUND_RELEASE":
		return domain.OutboundRelease, nil
	case "LOADING_DEPARTURE":
		return domain.LoadingDeparture, nil
	case "CROSS_CUSTOMS_MOVEMENT":
		return domain.CrossCustomsMovement, nil
	case "FINAL_DELIVERY":
		return domain.FinalDelivery, nil
	default:
		return domain.GuardedActionInvalid,
			fmt.Errorf("customs compliance postgres: unknown guarded action %q", value)
	}
}

// gateConclusionFrom 把列值译回封闭五值。迁移 CHECK 已拦住集合外取值。
func gateConclusionFrom(value string) (domain.GateConclusion, error) {
	switch value {
	case "UNMET":
		return domain.GateUnmet, nil
	case "PARTIALLY_MET":
		return domain.GatePartiallyMet, nil
	case "MET":
		return domain.GateMet, nil
	case "CONFLICTING":
		return domain.GateConflicting, nil
	case "NOT_APPLICABLE":
		return domain.GateNotApplicable, nil
	default:
		return domain.GateConclusionInvalid,
			fmt.Errorf("customs compliance postgres: unknown gate conclusion %q", value)
	}
}

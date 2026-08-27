package postgres

import (
	"context"
	"fmt"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 案件配置面五本登记册的写口。0006/0007/0008 建的表与 `*_view.go` 的读口此前只有读
// 没有写：非测试代码里一行 INSERT 都没有，于是「未配置」这句话在生产上说了也没处配。
// 本文件补的是那半边门。
//
// 三条纪律贯穿全部五本册子：
//
//   - **一律不 UPSERT。** 每个登记方法都是 `INSERT ... ON CONFLICT DO NOTHING`，
//     同键已在册就交回`已登记`。库这一层不判「内容一不一样」——那是编排读回既有登记
//     自己比的事（同 SubmitDeclarationHandler 先 FindByKey 再比指纹）。这样「重放同
//     一份」与「换了内容」在用例结果上分得开，而库里任何一行都不会被顶替。
//   - **失效走状态推进，不走删除。** 就绪与授权的撤销是 UPDATE 那两列，且 WHERE 带
//     `revoked_at IS NULL`：原依据与形成时间原样留在行内，重复撤销撞不动已撤销的行。
//   - **目录与明细分开写。** 义务与门禁各有「目录在场」与「明细清单」两个独立信号，
//     写口跟着分成两个方法——合成一个就再也表达不出「已登记且本次空清单」。

// ReadinessRegistrations 实现 ports.ReadinessRegistry。
type ReadinessRegistrations struct {
	db *bentopg.DB
}

func NewReadinessRegistrations(db *bentopg.DB) (*ReadinessRegistrations, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &ReadinessRegistrations{db: db}, nil
}

var _ ports.ReadinessRegistry = (*ReadinessRegistrations)(nil)

// RegisterReadiness 登记一条就绪判断。只写原判断三件（单元、依据、形成时间）——已撤销
// 的判断不从这里进库：那是 RevokeReadiness 对已在册行的状态推进，从登记口整行写入会
// 把「先就绪后失效」压成「一进来就是失效的」，两者在审计上不是一回事。
func (registry *ReadinessRegistrations) RegisterReadiness(
	ctx context.Context,
	tenant domain.TenantID,
	judgment domain.ReadinessJudgment,
) (ports.CaseConfigurationSaveOutcome, error) {
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register readiness: %w", err)
	}
	if !judgment.Effective() {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register readiness: a revoked judgment cannot enter through the registration port")
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.readiness_judgment
			(tenant_id, unit_id, basis_ref, judged_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (tenant_id, unit_id) DO NOTHING`,
		tenant.String(),
		judgment.Unit().String(),
		judgment.Basis().String(),
		judgment.JudgedAt().UTC(),
	)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register readiness: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	return ports.CaseConfigurationRegistered, nil
}

// RevokeReadiness 把`不再就绪`写到已在册的那一行上。WHERE 带 `revoked_at IS NULL`：
// 重复撤销匹配不到行，于是撞不掉第一次撤销的原因与时间——谁先撤销成功谁算。
func (registry *ReadinessRegistrations) RevokeReadiness(
	ctx context.Context,
	tenant domain.TenantID,
	judgment domain.ReadinessJudgment,
) error {
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("revoke readiness: %w", err)
	}
	cause, at, revoked := judgment.Revocation()
	if !revoked {
		return fmt.Errorf("revoke readiness: the judgment carries no revocation")
	}

	tag, err := executor.Exec(ctx,
		`UPDATE customs_compliance.readiness_judgment
		    SET revoked_by = $3, revoked_at = $4
		  WHERE tenant_id = $1 AND unit_id = $2 AND revoked_at IS NULL`,
		tenant.String(), judgment.Unit().String(), cause, at.UTC(),
	)
	if err != nil {
		return fmt.Errorf("revoke readiness: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("revoke readiness: %s is not registered or already revoked", judgment.Unit())
	}
	return nil
}

// SubmissionAuthorityRegistrations 实现 ports.SubmissionAuthorityRegistry。与就绪
// 同形而分表分类型：两条轨分别形成和失效（CONTEXT 244），共用一个类型就迟早共用一次
// 写入。
type SubmissionAuthorityRegistrations struct {
	db *bentopg.DB
}

func NewSubmissionAuthorityRegistrations(db *bentopg.DB) (*SubmissionAuthorityRegistrations, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &SubmissionAuthorityRegistrations{db: db}, nil
}

var _ ports.SubmissionAuthorityRegistry = (*SubmissionAuthorityRegistrations)(nil)

func (registry *SubmissionAuthorityRegistrations) GrantSubmissionAuthority(
	ctx context.Context,
	tenant domain.TenantID,
	authorization domain.SubmissionAuthorization,
) (ports.CaseConfigurationSaveOutcome, error) {
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("grant submission authority: %w", err)
	}
	if !authorization.Effective() {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("grant submission authority: a revoked authorization cannot enter through the registration port")
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.submission_authority
			(tenant_id, unit_id, authority_ref, granted_at)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT (tenant_id, unit_id) DO NOTHING`,
		tenant.String(),
		authorization.Unit().String(),
		authorization.Authority().String(),
		authorization.GrantedAt().UTC(),
	)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("grant submission authority: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	return ports.CaseConfigurationRegistered, nil
}

func (registry *SubmissionAuthorityRegistrations) RevokeSubmissionAuthority(
	ctx context.Context,
	tenant domain.TenantID,
	authorization domain.SubmissionAuthorization,
) error {
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("revoke submission authority: %w", err)
	}
	cause, at, revoked := authorization.Revocation()
	if !revoked {
		return fmt.Errorf("revoke submission authority: the authorization carries no revocation")
	}

	tag, err := executor.Exec(ctx,
		`UPDATE customs_compliance.submission_authority
		    SET revoked_by = $3, revoked_at = $4
		  WHERE tenant_id = $1 AND unit_id = $2 AND revoked_at IS NULL`,
		tenant.String(), authorization.Unit().String(), cause, at.UTC(),
	)
	if err != nil {
		return fmt.Errorf("revoke submission authority: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("revoke submission authority: %s is not granted or already revoked", authorization.Unit())
	}
	return nil
}

// InterpretationRuleRegistrations 实现 ports.InterpretationRuleRegistry。登记面按
// （租户，结果层，适用辖区，法定生效区间起）立键，区间不重叠由迁移的排他约束守着。
//
// 唯一的 UPDATE 是换版给开放前版落终点——与就绪/授权撤销同款的状态推进：rule_ref 与
// applies_from 没有任何改写路径，终点只从 NULL 走到后继起点、只走一次。撞键或撞重叠
// 都折成`已登记`交回，内容是否同一份由编排读回自己比（同其余五本册子）。
type InterpretationRuleRegistrations struct {
	db *bentopg.DB
}

func NewInterpretationRuleRegistrations(db *bentopg.DB) (*InterpretationRuleRegistrations, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &InterpretationRuleRegistrations{db: db}, nil
}

var _ ports.InterpretationRuleRegistry = (*InterpretationRuleRegistrations)(nil)

func (registry *InterpretationRuleRegistrations) RegisterInterpretationRule(
	ctx context.Context,
	tenant domain.TenantID,
	layer domain.ResultLayer,
	jurisdiction domain.RegulatoryJurisdictionReference,
	rule domain.InterpretationRuleReference,
	appliesFrom time.Time,
) (ports.CaseConfigurationSaveOutcome, error) {
	// 封闭六层之外的取值登记不进去，也不该悄悄写成一行：那是调用方的编程错误，与
	// 「实例还没登记」是两回事（同读口那一句）。零起点同理——法定生效起点在键上，
	// timestamptz 装得下 0001 年，缺格会静默变成一个错的版本边界。
	layerText := layer.String()
	if layerText == "" {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register interpretation rule: unknown result layer %d", layer)
	}
	if appliesFrom.IsZero() {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register interpretation rule: the applicable interval has no start")
	}

	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register interpretation rule: %w", err)
	}

	// 换版：给同支（租户，层，辖区）下起点更早的开放版落终点。先跑它才插得进后继——
	// 开放区间与任何更晚起点的登记在排他约束上相斥。行锁顺带串行化同支并发换版：后到
	// 者在锁上等到先到者提交后重评 WHERE，落不了第二次终点，其 INSERT 再被排他约束
	// 折成`已登记`。若本次登记随后撞键落不进去，这条 UPDATE 必然没匹配过行（撞键处
	// 已有同起点行，等于同支已有覆盖该起点的区间，开放前版与之重叠、按不重叠不变量
	// 不可能在册），不会白改。
	if _, err := executor.Exec(ctx,
		`UPDATE customs_compliance.interpretation_rule
		    SET applies_until = $4
		  WHERE tenant_id = $1 AND result_layer = $2 AND jurisdiction_ref = $3
		    AND applies_until IS NULL AND applies_from < $4`,
		tenant.String(), layerText, jurisdiction.String(), appliesFrom.UTC(),
	); err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register interpretation rule: %w", err)
	}

	// 无 conflict target 的 DO NOTHING 同时吃主键撞键与排他约束撞重叠：两者都折成
	// `已登记`，是重放还是冲突由编排读回比对。
	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.interpretation_rule
			(tenant_id, result_layer, jurisdiction_ref, applies_from, rule_ref)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT DO NOTHING`,
		tenant.String(), layerText, jurisdiction.String(), appliesFrom.UTC(), rule.String(),
	)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register interpretation rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	return ports.CaseConfigurationRegistered, nil
}

// ObligationInventoryRegistrations 实现 ports.ObligationInventoryRegistry。
type ObligationInventoryRegistrations struct {
	db *bentopg.DB
}

func NewObligationInventoryRegistrations(db *bentopg.DB) (*ObligationInventoryRegistrations, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &ObligationInventoryRegistrations{db: db}, nil
}

var _ ports.ObligationInventoryRegistry = (*ObligationInventoryRegistrations)(nil)

// RegisterObligationCatalog 只登目录行。登了目录还没登任何明细，读口就答「已登记且
// 本截点空清单」——那是「此案在此截点无适用义务」的如实答案，不是未决。
func (registry *ObligationInventoryRegistrations) RegisterObligationCatalog(
	ctx context.Context,
	tenant domain.TenantID,
	caseRef domain.CustomsCaseID,
	registeredAt time.Time,
) (ports.CaseConfigurationSaveOutcome, error) {
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register obligation catalog: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.closure_obligation_catalog
			(tenant_id, case_ref, registered_at)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (tenant_id, case_ref) DO NOTHING`,
		tenant.String(), caseRef.String(), registeredAt.UTC(),
	)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register obligation catalog: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	return ports.CaseConfigurationRegistered, nil
}

// RegisterObligationItem 登记一项义务及其适用区间。目录不在时外键会挡下——先登目录
// 再登明细的顺序由库守着，不靠调用方记得。
func (registry *ObligationInventoryRegistrations) RegisterObligationItem(
	ctx context.Context,
	tenant domain.TenantID,
	caseRef domain.CustomsCaseID,
	registration ports.ObligationRegistration,
) (ports.CaseConfigurationSaveOutcome, error) {
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register obligation item: %w", err)
	}
	if registration.AppliesFrom.IsZero() {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register obligation item: the applicable interval has no start")
	}
	state := registration.Item.State.String()
	if state == "" {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register obligation item: unknown obligation item state %d", registration.Item.State)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.closure_obligation_item
			(tenant_id, case_ref, obligation, scope, item_state, basis, handed_to,
			 applies_from, applies_until)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT (tenant_id, case_ref, obligation) DO NOTHING`,
		tenant.String(),
		caseRef.String(),
		registration.Item.Obligation,
		registration.Item.Scope,
		state,
		registration.Item.Basis,
		handedToColumn(registration.Item),
		registration.AppliesFrom.UTC(),
		appliesUntilColumn(registration),
	)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register obligation item: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	return ports.CaseConfigurationRegistered, nil
}

// handedToColumn 把承接对象译成可空列。非承接项必须交回 NULL——库的 CHECK 要求
// 「承接项指名接收责任方、非承接项不带承接对象」双向成立，空串会被当成「带了一个
// 空名字」而撞上 not-blank。
func handedToColumn(item domain.ClosureObligationItem) *string {
	if item.State != domain.ObligationHandedOver {
		return nil
	}
	handedTo := item.HandedTo
	return &handedTo
}

// appliesUntilColumn 把零值终点译成 NULL：「尚无终点」与「某刻失效」在读口的半开
// 区间里是两个不同答案，零时刻会被读成 0001 年就已失效。
func appliesUntilColumn(registration ports.ObligationRegistration) *time.Time {
	if registration.AppliesUntil.IsZero() {
		return nil
	}
	until := registration.AppliesUntil.UTC()
	return &until
}

// GateConditionRegistrations 实现 ports.GateConditionRegistry。
type GateConditionRegistrations struct {
	db *bentopg.DB
}

func NewGateConditionRegistrations(db *bentopg.DB) (*GateConditionRegistrations, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &GateConditionRegistrations{db: db}, nil
}

var _ ports.GateConditionRegistry = (*GateConditionRegistrations)(nil)

// RegisterGateCatalog 只登目录行。登了目录不登任何前置条件，读口答「已登记且空清单」
// ——领域折成`不适用`，即「此动作在此边界本就不受门禁」。这一格必须能单独登出来，
// 否则「不受管」就只能靠「查不到」冒充，等于把门禁放开。
func (registry *GateConditionRegistrations) RegisterGateCatalog(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.DecisionScopeReference,
	action domain.GuardedAction,
	boundary domain.CustomsProcedureReference,
	registeredAt time.Time,
) (ports.CaseConfigurationSaveOutcome, error) {
	actionText := action.String()
	if actionText == "" {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register gate catalog: unknown guarded action %d", action)
	}

	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register gate catalog: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.gate_condition_catalog
			(tenant_id, scope_ref, action, boundary_ref, registered_at)
		 VALUES ($1, $2, $3, $4, $5)
		 ON CONFLICT (tenant_id, scope_ref, action, boundary_ref) DO NOTHING`,
		tenant.String(), scope.String(), actionText, boundary.String(), registeredAt.UTC(),
	)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register gate catalog: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	return ports.CaseConfigurationRegistered, nil
}

func (registry *GateConditionRegistrations) RegisterGateFinding(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.DecisionScopeReference,
	action domain.GuardedAction,
	boundary domain.CustomsProcedureReference,
	finding domain.PreconditionFinding,
) (ports.CaseConfigurationSaveOutcome, error) {
	actionText := action.String()
	if actionText == "" {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register gate finding: unknown guarded action %d", action)
	}
	stateText := finding.State.String()
	if stateText == "" {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register gate finding: unknown precondition state %d", finding.State)
	}

	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register gate finding: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.gate_condition_finding
			(tenant_id, scope_ref, action, boundary_ref, precondition_ref, finding_state)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (tenant_id, scope_ref, action, boundary_ref, precondition_ref) DO NOTHING`,
		tenant.String(), scope.String(), actionText, boundary.String(),
		finding.Precondition.String(), stateText,
	)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register gate finding: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	return ports.CaseConfigurationRegistered, nil
}

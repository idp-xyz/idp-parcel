package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 门禁目录里「税费付款」那一道的规则行（0019 建表，票 sa-cc/06，ADR-0137 决定三）与付款核对册的「当前版」
// 读口。规则行挂在门禁目录既有登记册上——写口落在 GateConditionRegistrations、读口落在 GateConditionView，
// 与目录 / 认定同一对适配器、同一把三维键；「当前版」读口落在 DutyPaymentReconciliation 上，与核对的
// 点读 / 写口同一张表。分开成文件只是为了不让 0008 那两张表的适配器再长；类型没有分开。

var (
	_ ports.DutyPaymentGateRuleRegistry = (*GateConditionRegistrations)(nil)
	_ ports.DutyPaymentGateRuleView     = (*GateConditionView)(nil)
	_ ports.CurrentDutyVerificationView = (*DutyPaymentReconciliation)(nil)
)

// RegisterDutyPaymentGateRule 登记「税费付款」那一道的规则。registered_at 取事务内的库时钟：它是登记这一
// 动作的时间，不是规则的任何业务时间。写入代数与其余登记册同——一律不 UPSERT，同键由 DO NOTHING 折成
// `已登记`交回；无 UPDATE 路径，改规则走复核另登。目录行不在场时外键拒，与认定行同判据。
func (registry *GateConditionRegistrations) RegisterDutyPaymentGateRule(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.DecisionScopeReference,
	action domain.GuardedAction,
	boundary domain.CustomsProcedureReference,
	rule domain.DutyPaymentGateRule,
) (ports.CaseConfigurationSaveOutcome, error) {
	actionText := action.String()
	if actionText == "" {
		return ports.CaseConfigurationSaveOutcomeInvalid,
			fmt.Errorf("register duty payment gate rule: unknown guarded action %d", action)
	}
	coverage, delta, validity, err := dutyRuleSetsJSON(rule)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register duty payment gate rule: %w", err)
	}
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register duty payment gate rule: %w", err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO customs_compliance.gate_condition_duty_payment_rule
			(tenant_id, scope_ref, action, boundary_ref,
			 not_a_precondition, accept_coverage, accept_delta, accept_validity, registered_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, now())
		 ON CONFLICT DO NOTHING`,
		tenant.String(), scope.String(), actionText, boundary.String(),
		rule.NotAPrecondition(), coverage, delta, validity,
	)
	if err != nil {
		return ports.CaseConfigurationSaveOutcomeInvalid, fmt.Errorf("register duty payment gate rule: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ports.CaseConfigurationAlreadyRegistered, nil
	}
	return ports.CaseConfigurationRegistered, nil
}

// dutyRuleSetsJSON 把三个接受集合编成 jsonb 数组的字节；「不构成前置条件」三个都是空数组。零值规则（两形
// 都不是）是调用方编程错误，响亮拒——落库会撞 CHECK 而报成「依赖故障」。
func dutyRuleSetsJSON(rule domain.DutyPaymentGateRule) ([]byte, []byte, []byte, error) {
	coverage, delta, validity := rule.Accepts()
	if !rule.NotAPrecondition() && (len(coverage) == 0 || len(delta) == 0 || len(validity) == 0) {
		return nil, nil, nil, errors.New("the rule is zero-valued")
	}
	coverageJSON, err := json.Marshal(closedWords(coverage))
	if err != nil {
		return nil, nil, nil, err
	}
	deltaJSON, err := json.Marshal(closedWords(delta))
	if err != nil {
		return nil, nil, nil, err
	}
	validityJSON, err := json.Marshal(closedWords(validity))
	if err != nil {
		return nil, nil, nil, err
	}
	return coverageJSON, deltaJSON, validityJSON, nil
}

// closedWords 把一个封闭集的取值写成库列的词形；空切片写成空数组而不是 null，CHECK 靠数组长度分两形。
func closedWords[T fmt.Stringer](members []T) []string {
	words := make([]string, 0, len(members))
	for _, member := range members {
		words = append(words, member.String())
	}
	return words
}

// closedSet 把库里一列 jsonb 数组译回封闭集的取值；集外词即库被旁路改过，作错误抛出。
func closedSet[T fmt.Stringer](raw []byte, members ...T) ([]T, error) {
	var words []string
	if err := json.Unmarshal(raw, &words); err != nil {
		return nil, err
	}
	set := make([]T, 0, len(words))
	for _, word := range words {
		member, err := closedWord(word, members...)
		if err != nil {
			return nil, err
		}
		set = append(set, member)
	}
	return set, nil
}

// LoadDutyPaymentGateRule 按门禁三维键取回规则行。found=false 即这一道尚未登规则——门禁编排答「规则未
// 配置」；读回经领域构造重建，库里一行若立不起 DutyPaymentGateRule，说明有人绕过写口改了它，作错误抛出。
func (view *GateConditionView) LoadDutyPaymentGateRule(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.DecisionScopeReference,
	action domain.GuardedAction,
	boundary domain.CustomsProcedureReference,
) (domain.DutyPaymentGateRule, bool, error) {
	none := domain.DutyPaymentGateRule{}
	actionText := action.String()
	if actionText == "" {
		return none, false, fmt.Errorf("load duty payment gate rule: unknown guarded action %d", action)
	}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load duty payment gate rule: %w", err)
	}

	var (
		notAPrecondition                      bool
		coverageJSON, deltaJSON, validityJSON []byte
	)
	err = querier.QueryRow(ctx,
		`SELECT not_a_precondition, accept_coverage, accept_delta, accept_validity
		   FROM customs_compliance.gate_condition_duty_payment_rule
		  WHERE tenant_id = $1 AND scope_ref = $2 AND action = $3 AND boundary_ref = $4`,
		tenant.String(), scope.String(), actionText, boundary.String(),
	).Scan(&notAPrecondition, &coverageJSON, &deltaJSON, &validityJSON)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load duty payment gate rule: %w", err)
	}

	rule, err := rebuildDutyPaymentGateRule(notAPrecondition, coverageJSON, deltaJSON, validityJSON)
	if err != nil {
		return none, false, fmt.Errorf("rebuild duty payment gate rule: %w", err)
	}
	return rule, true, nil
}

// rebuildDutyPaymentGateRule 把一行折回领域规则。词形经 closedWord 译回封闭集，集外即库被旁路改过。
func rebuildDutyPaymentGateRule(
	notAPrecondition bool,
	coverageJSON, deltaJSON, validityJSON []byte,
) (domain.DutyPaymentGateRule, error) {
	if notAPrecondition {
		return domain.DutyPaymentNotAPrecondition(), nil
	}
	coverage, err := closedSet(coverageJSON, domain.CoverageNone, domain.CoveragePartial, domain.CoverageFull)
	if err != nil {
		return domain.DutyPaymentGateRule{}, err
	}
	// 差额的 PENDING 与有效性的 CONFLICTING / PENDING 不在可接受的词里：库上 CHECK 已拦，这里的集合是第二道。
	delta, err := closedSet(deltaJSON, domain.DeltaNone, domain.DeltaShort, domain.DeltaExcess)
	if err != nil {
		return domain.DutyPaymentGateRule{}, err
	}
	validity, err := closedSet(validityJSON, domain.FundsFactValid, domain.FundsFactInvalidated)
	if err != nil {
		return domain.DutyPaymentGateRule{}, err
	}
	return domain.AcceptDutyPaymentWhen(coverage, delta, validity)
}

// LoadCurrentDutyVerification 按（租户、范围）取回核对时刻最新的那一版；同一时刻并存的两版按版本指纹字典序
// 取定，让「当前」在同一份数据上只有一个答案（读口头注的口径）。读回经 rebuildVerification 整门重验。
func (store *DutyPaymentReconciliation) LoadCurrentDutyVerification(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.DecisionScopeReference,
) (ports.DutyVerificationRecord, bool, error) {
	none := ports.DutyVerificationRecord{}
	if strings.TrimSpace(scope.String()) == "" {
		return none, false, fmt.Errorf("load current duty verification: the scope is blank")
	}
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load current duty verification: %w", err)
	}

	var (
		dutyRaw, fundsRaw, digest, procedure, coverage, delta, validity, basis string
		verifiedAt                                                             time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT duty_ref, funds_ref, version_digest, procedure_ref, coverage, delta, validity, basis, verified_at
		   FROM customs_compliance.duty_payment_verification
		  WHERE tenant_id = $1 AND scope_ref = $2
		  ORDER BY verified_at DESC, version_digest ASC
		  LIMIT 1`,
		tenant.String(), scope.String(),
	).Scan(&dutyRaw, &fundsRaw, &digest, &procedure, &coverage, &delta, &validity, &basis, &verifiedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load current duty verification: %w", err)
	}

	key := ports.DutyVerificationKey{TenantID: tenant, Scope: scope, Digest: digest}
	if key.Duty, err = domain.NewAssessedDutyReference(dutyRaw); err != nil {
		return none, false, fmt.Errorf("rebuild duty verification: %w", err)
	}
	if key.Funds, err = domain.NewExternalFundsFactReference(fundsRaw); err != nil {
		return none, false, fmt.Errorf("rebuild duty verification: %w", err)
	}
	verification, err := rebuildVerification(key, procedure, coverage, delta, validity, verifiedAt)
	if err != nil {
		return none, false, fmt.Errorf("rebuild duty verification: %w", err)
	}
	return ports.DutyVerificationRecord{Key: key, Verification: verification, Basis: basis}, true, nil
}

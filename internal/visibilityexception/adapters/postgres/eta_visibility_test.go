package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件对真实 PostgreSQL 16 证 ETA 客户可见性判断库（0031）与 ETA 对客规则册：版本行只增不改写、当前标记的乐观锁、
// 账户与租户隔离、结论与字段成对由迁移 CHECK 把关；规则按判断时点适用、缺失与冲突如实交回而不是错误。

var etaJudgedAt = time.Date(2026, 8, 13, 15, 0, 0, 0, time.UTC)

func newETAVisibility(t *testing.T) (*adapter.ETAVisibilityJudgements, *adapter.ETAVisibilityRules, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	judgements, err := adapter.NewETAVisibilityJudgements(db)
	if err != nil {
		t.Fatalf("构造判断库：%v", err)
	}
	rules, err := adapter.NewETAVisibilityRules(db)
	if err != nil {
		t.Fatalf("构造规则视图：%v", err)
	}
	return judgements, rules, db.Transactor(), pool
}

func etaFinding(t *testing.T, etaVersion string, basis domain.ETAVisibilityRuleBasis) domain.ETAVisibilityFinding {
	t.Helper()
	eta, err := domain.FormETAPrediction(domain.ETAPredictionSpec{
		Version:     build(t, domain.NewETAVersionID, etaVersion),
		Parcel:      build(t, domain.NewTrackedParcelReference, "parcel-1"),
		Milestone:   build(t, domain.NewMilestoneReference, "DELIVERED"),
		Source:      domain.OperatorDerivedETA,
		Inputs:      build(t, domain.NewPredictionInputsReference, "inputs/"+etaVersion),
		Model:       build(t, domain.NewPredictionModelReference, "eta-model/v4"),
		RangeFrom:   etaJudgedAt.Add(30 * time.Hour),
		RangeTo:     etaJudgedAt.Add(42 * time.Hour),
		Confidence:  build(t, domain.NewConfidenceReference, "MEDIUM/history-based"),
		PredictedAt: etaJudgedAt,
	})
	if err != nil {
		t.Fatalf("形成 ETA：%v", err)
	}
	finding, err := domain.AssessETAVisibility(eta, basis)
	if err != nil {
		t.Fatalf("作发现：%v", err)
	}
	return finding
}

func etaRuleBasis(t *testing.T, qualifying string) domain.ETAVisibilityRuleBasis {
	t.Helper()
	rule, err := domain.NewETAVisibilityRule(domain.ETAVisibilityRuleSpec{
		GateVersion:     build(t, domain.NewETAQualityGateVersion, "gate/v1"),
		ServiceRule:     build(t, domain.NewCustomerServiceRuleVersion, "contract-a/v3"),
		TargetMilestone: build(t, domain.NewMilestoneReference, "DELIVERED"),
		Qualifying:      []domain.ConfidenceReference{build(t, domain.NewConfidenceReference, qualifying)},
		Expression:      domain.ETAShownAsTimeRange,
		StopBoundary:    build(t, domain.NewETAStopShowingBoundary, "stop/after-delivered"),
	})
	if err != nil {
		t.Fatalf("规则：%v", err)
	}
	return domain.ConfiguredETARule(rule)
}

func firstJudgement(t *testing.T, version, etaVersion string, basis domain.ETAVisibilityRuleBasis) domain.ETAVisibilityJudgement {
	t.Helper()
	judgement, err := domain.FormETAVisibilityJudgement(
		build(t, domain.NewETAVisibilityJudgementVersionID, version),
		build(t, domain.NewCustomerAccountReference, "customer-a"),
		etaFinding(t, etaVersion, basis), etaJudgedAt)
	if err != nil {
		t.Fatalf("形成判断：%v", err)
	}
	return judgement
}

func countJudgementVersions(t *testing.T, pool *pgxpool.Pool) int {
	t.Helper()
	var n int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM visibility_exception.eta_visibility_judgement_version`).Scan(&n); err != nil {
		t.Fatalf("数判断版本行：%v", err)
	}
	return n
}

// TestACustomerVisibleJudgementIsReadBackWithItsBasis 证首版原样读回：结论、理由、候选 ETA 版本与来源、门槛版本、
// 规则版本、获准对客表达与停止展示边界、判断时点都在，无前版。
func TestACustomerVisibleJudgementIsReadBackWithItsBasis(t *testing.T) {
	judgements, _, transactor, _ := newETAVisibility(t)
	ctx := t.Context()
	tenant := tenantOf(t, "tenant-a")
	judgement := firstJudgement(t, "evj-1", "eta-1", etaRuleBasis(t, "MEDIUM/history-based"))

	within2(t, transactor, ctx, func(txCtx context.Context) error {
		return judgements.Save(txCtx, tenant, judgement)
	})

	found, exists, err := judgements.FindCurrent(ctx, tenant, judgement.Customer(), judgement.Parcel())
	if err != nil || !exists {
		t.Fatalf("FindCurrent exists = %v err = %v", exists, err)
	}
	if found.Finding() != judgement.Finding() || found.Version() != judgement.Version() ||
		!found.JudgedAt().Equal(etaJudgedAt) {
		t.Fatalf("读回 %#v，与写入 %#v 不同", found, judgement)
	}
	if expression, boundary, permitted := found.CustomerExpression(); !permitted ||
		expression != domain.ETAShownAsTimeRange || boundary.String() != "stop/after-delivered" {
		t.Fatalf("对客表达读回 %q %q %v", expression, boundary, permitted)
	}
	if _, superseding := found.PriorVersion(); superseding {
		t.Fatal("首版读回带了前版")
	}
}

// TestSupersessionAppendsAVersionAndKeepsTheOriginalReadable 证 UC-VE-003「原判断不覆盖」：替代追加版本行并移标记，
// 被替代的判断按版本仍读得回、一列不变；待判断和不可见两格的空列也原样读回。
func TestSupersessionAppendsAVersionAndKeepsTheOriginalReadable(t *testing.T) {
	judgements, _, transactor, pool := newETAVisibility(t)
	ctx := t.Context()
	tenant := tenantOf(t, "tenant-a")
	first := firstJudgement(t, "evj-1", "eta-1", etaRuleBasis(t, "MEDIUM/history-based"))
	within2(t, transactor, ctx, func(txCtx context.Context) error { return judgements.Save(txCtx, tenant, first) })

	notVisible, err := first.Supersede(build(t, domain.NewETAVisibilityJudgementVersionID, "evj-2"),
		etaFinding(t, "eta-2", etaRuleBasis(t, "HIGH/carrier-confirmed")), etaJudgedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("替代：%v", err)
	}
	within2(t, transactor, ctx, func(txCtx context.Context) error { return judgements.Save(txCtx, tenant, notVisible) })

	pending, err := notVisible.Supersede(build(t, domain.NewETAVisibilityJudgementVersionID, "evj-3"),
		etaFinding(t, "eta-3", domain.ConflictingETARules()), etaJudgedAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("再替代：%v", err)
	}
	within2(t, transactor, ctx, func(txCtx context.Context) error { return judgements.Save(txCtx, tenant, pending) })

	current, _, err := judgements.FindCurrent(ctx, tenant, first.Customer(), first.Parcel())
	if err != nil || current.Version() != pending.Version() || current.Reason() != domain.CustomerRuleInConflict {
		t.Fatalf("当前版 = %#v err = %v", current, err)
	}
	if prior, superseding := current.PriorVersion(); !superseding || prior != notVisible.Version() {
		t.Fatalf("prior = %q", prior)
	}
	for _, original := range []domain.ETAVisibilityJudgement{first, notVisible} {
		kept, exists, err := judgements.FindVersion(ctx, tenant, first.Customer(), first.Parcel(), original.Version())
		if err != nil || !exists || kept.Finding() != original.Finding() {
			t.Fatalf("按版本读回 %q：exists = %v err = %v kept = %#v", original.Version(), exists, err, kept)
		}
	}
	if n := countJudgementVersions(t, pool); n != 3 {
		t.Fatalf("判断版本行 = %d，want 3（替代追加、不改写）", n)
	}
}

// TestAStaleSupersessionAndADuplicateFirstAreRefusedAndRolledBack 证当前标记的乐观锁：首发时已有别人占住当前、替代时当前
// 版已不是所指前版，都交回 ErrCurrentJudgementChanged；输家已写下的版本行随事务回滚。
func TestAStaleSupersessionAndADuplicateFirstAreRefusedAndRolledBack(t *testing.T) {
	judgements, _, transactor, pool := newETAVisibility(t)
	ctx := t.Context()
	tenant := tenantOf(t, "tenant-a")
	first := firstJudgement(t, "evj-1", "eta-1", etaRuleBasis(t, "MEDIUM/history-based"))
	within2(t, transactor, ctx, func(txCtx context.Context) error { return judgements.Save(txCtx, tenant, first) })

	rival := firstJudgement(t, "evj-rival", "eta-1", etaRuleBasis(t, "MEDIUM/history-based"))
	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return judgements.Save(txCtx, tenant, rival)
	})
	if !errors.Is(err, adapter.ErrCurrentJudgementChanged) {
		t.Fatalf("重复首版 err = %v, want ErrCurrentJudgementChanged", err)
	}

	second, err := first.Supersede(build(t, domain.NewETAVisibilityJudgementVersionID, "evj-2"),
		etaFinding(t, "eta-2", domain.UnconfiguredETARule()), etaJudgedAt.Add(time.Hour))
	if err != nil {
		t.Fatalf("替代：%v", err)
	}
	within2(t, transactor, ctx, func(txCtx context.Context) error { return judgements.Save(txCtx, tenant, second) })

	// 另一路仍以 first 为前版再替代：当前已是 second。
	stale, err := first.Supersede(build(t, domain.NewETAVisibilityJudgementVersionID, "evj-stale"),
		etaFinding(t, "eta-3", domain.UnconfiguredETARule()), etaJudgedAt.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("陈旧替代：%v", err)
	}
	err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return judgements.Save(txCtx, tenant, stale)
	})
	if !errors.Is(err, adapter.ErrCurrentJudgementChanged) {
		t.Fatalf("陈旧替代 err = %v, want ErrCurrentJudgementChanged", err)
	}
	if n := countJudgementVersions(t, pool); n != 2 {
		t.Fatalf("判断版本行 = %d；输家的版本行没有随事务回滚", n)
	}
	current, _, _ := judgements.FindCurrent(ctx, tenant, first.Customer(), first.Parcel())
	if current.Version() != second.Version() {
		t.Fatalf("当前版 = %q，赢家被覆盖了", current.Version())
	}
}

// TestJudgementsAreIsolatedByTenantAndAccount 证双重隔离：别的租户、别的账户按同一包裹查不到，也读不回别人的版本；
// 否定结果不分成因。
func TestJudgementsAreIsolatedByTenantAndAccount(t *testing.T) {
	judgements, _, transactor, _ := newETAVisibility(t)
	ctx := t.Context()
	tenant := tenantOf(t, "tenant-a")
	judgement := firstJudgement(t, "evj-1", "eta-1", etaRuleBasis(t, "MEDIUM/history-based"))
	within2(t, transactor, ctx, func(txCtx context.Context) error { return judgements.Save(txCtx, tenant, judgement) })

	otherAccount := build(t, domain.NewCustomerAccountReference, "customer-b")
	for name, probe := range map[string]func() (bool, error){
		"别的租户": func() (bool, error) {
			_, found, err := judgements.FindCurrent(ctx, tenantOf(t, "tenant-b"), judgement.Customer(), judgement.Parcel())
			return found, err
		},
		"别的账户": func() (bool, error) {
			_, found, err := judgements.FindCurrent(ctx, tenant, otherAccount, judgement.Parcel())
			return found, err
		},
		"别的账户按版本": func() (bool, error) {
			_, found, err := judgements.FindVersion(ctx, tenant, otherAccount, judgement.Parcel(), judgement.Version())
			return found, err
		},
	} {
		if found, err := probe(); err != nil || found {
			t.Fatalf("%s：found = %v err = %v，want 查不到", name, found, err)
		}
	}
}

// TestTheMigrationRefusesAJudgementWhoseShapeTheConclusionForbids 证迁移 CHECK 先于领域重建口挡一道：可见缺表达、
// 不可见带表达、待判断带规则版本、理由与结论不配、封闭集合外的取值都进不了库。
func TestTheMigrationRefusesAJudgementWhoseShapeTheConclusionForbids(t *testing.T) {
	_, _, _, pool := newETAVisibility(t)
	const insert = `INSERT INTO visibility_exception.eta_visibility_judgement_version
		(tenant_id, customer_account_id, parcel_id, version, milestone_ref, candidate_eta_version, candidate_source,
		 conclusion, reason, gate_version, service_rule_version, expression, stop_boundary_ref, judged_at)
		VALUES ('t', 'c', 'p', $1, 'DELIVERED', 'eta-1', $2, $3, $4, $5, $6, $7, $8, now())`
	text := func(value string) *string { return &value }
	for name, args := range map[string][]any{
		"可见缺表达":   {"v1", "OPERATOR_DERIVED", "CUSTOMER_VISIBLE_ETA", "QUALITY_MET_GATE", text("g"), text("s"), nil, text("b")},
		"可见缺停止边界": {"v2", "OPERATOR_DERIVED", "CUSTOMER_VISIBLE_ETA", "QUALITY_MET_GATE", text("g"), text("s"), text("TIME_RANGE"), nil},
		"不可见带表达":  {"v3", "OPERATOR_DERIVED", "ETA_NOT_VISIBLE", "QUALITY_BELOW_GATE", text("g"), text("s"), text("TIME_RANGE"), nil},
		"待判断带门槛":  {"v4", "OPERATOR_DERIVED", "ETA_VISIBILITY_PENDING", "RULE_NOT_CONFIGURED", text("g"), nil, nil, nil},
		"理由与结论不配": {"v5", "OPERATOR_DERIVED", "ETA_VISIBILITY_PENDING", "QUALITY_MET_GATE", nil, nil, nil, nil},
		"结论越界":    {"v6", "OPERATOR_DERIVED", "VISIBLE", "QUALITY_MET_GATE", text("g"), text("s"), text("TIME_RANGE"), text("b")},
		"来源越界":    {"v7", "SOMEONE", "ETA_VISIBILITY_PENDING", "RULE_CONFLICT", nil, nil, nil, nil},
	} {
		_, err := pool.Exec(t.Context(), insert, args...)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
			t.Errorf("%s：err = %v，want check_violation 23514", name, err)
		}
	}
}

func insertRule(t *testing.T, pool *pgxpool.Pool, customer, serviceRule string, from time.Time, to *time.Time, qualifying ...string) error {
	t.Helper()
	_, err := pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.eta_visibility_rule
			(tenant_id, customer_account_ref, service_rule_version, gate_version, target_milestone_ref,
			 qualifying_confidences, expression, stop_boundary_ref, effective_from, effective_to, approved_by)
		 VALUES ('tenant-a', $1, $2, 'gate/v1', 'DELIVERED', $3, 'TIME_RANGE', 'stop/after-delivered', $4, $5, 'cs-lead')`,
		customer, serviceRule, qualifying, from, to)
	return err
}

// TestTheRuleViewAnswersMissingConflictingOrTheOneApplicableRule 证规则按判断时点适用：没登记答未配置，登记后三样（门槛版本、
// 规则版本、目标里程碑、达标集合、表达、停止边界）原样读回；生效区间含起不含止，换版相接处取新版；同一时点两行适用答
// 冲突——都是确定的业务答案，不是错误；别的租户与别的账户看不见。
func TestTheRuleViewAnswersMissingConflictingOrTheOneApplicableRule(t *testing.T) {
	_, rules, _, pool := newETAVisibility(t)
	ctx := t.Context()
	tenant := tenantOf(t, "tenant-a")
	customer := build(t, domain.NewCustomerAccountReference, "customer-a")
	t0 := etaJudgedAt
	assess := func(who domain.CustomerAccountReference, tenantID domain.TenantID, at time.Time) domain.ETAVisibilityRuleBasis {
		t.Helper()
		basis, err := rules.AssessETAVisibilityRule(ctx, tenantID, who, at)
		if err != nil {
			t.Fatalf("assess: %v", err)
		}
		return basis
	}

	if state := assess(customer, tenant, t0).State(); state != domain.ETARuleNotConfigured {
		t.Fatalf("空册 = %q, want NOT_CONFIGURED", state)
	}

	closeAt := t0.Add(48 * time.Hour)
	if err := insertRule(t, pool, "customer-a", "contract-a/v1", t0, &closeAt, "MEDIUM/history-based", "HIGH/carrier-confirmed"); err != nil {
		t.Fatalf("登记 v1：%v", err)
	}
	if err := insertRule(t, pool, "customer-a", "contract-a/v2", closeAt, nil, "HIGH/carrier-confirmed"); err != nil {
		t.Fatalf("登记 v2：%v", err)
	}

	basis := assess(customer, tenant, t0.Add(time.Hour))
	rule, configured := basis.Rule()
	if !configured || rule.ServiceRule().String() != "contract-a/v1" || rule.GateVersion().String() != "gate/v1" ||
		rule.TargetMilestone().String() != "DELIVERED" || rule.Expression() != domain.ETAShownAsTimeRange ||
		rule.StopBoundary().String() != "stop/after-delivered" || len(rule.Qualifying()) != 2 {
		t.Fatalf("v1 读回 %#v", rule)
	}
	if state := assess(customer, tenant, t0.Add(-time.Second)).State(); state != domain.ETARuleNotConfigured {
		t.Fatalf("生效前一刻 = %q, want NOT_CONFIGURED（含起）", state)
	}
	next, _ := assess(customer, tenant, closeAt).Rule()
	if next.ServiceRule().String() != "contract-a/v2" {
		t.Fatalf("换版相接处取 %q, want v2（含起不含止）", next.ServiceRule())
	}
	if state := assess(build(t, domain.NewCustomerAccountReference, "customer-b"), tenant, t0.Add(time.Hour)).State(); state != domain.ETARuleNotConfigured {
		t.Fatalf("别的账户 = %q, want NOT_CONFIGURED", state)
	}
	if state := assess(customer, tenantOf(t, "tenant-b"), t0.Add(time.Hour)).State(); state != domain.ETARuleNotConfigured {
		t.Fatalf("别的租户 = %q, want NOT_CONFIGURED", state)
	}

	// 闭区间重叠：库的唯一索引只拦未闭区间，读口兜这一格——同一时点两行适用是冲突，不任选其一。
	overlapFrom, overlapTo := t0.Add(24*time.Hour), t0.Add(40*time.Hour)
	if err := insertRule(t, pool, "customer-a", "contract-a/overlap", overlapFrom, &overlapTo, "LOW"); err != nil {
		t.Fatalf("登记重叠版：%v", err)
	}
	if state := assess(customer, tenant, t0.Add(30*time.Hour)).State(); state != domain.ETARulesConflicting {
		t.Fatalf("重叠时点 = %q, want CONFLICTING", state)
	}
}

// TestTheRuleTableRefusesWhatTheRuleCannotMean 证规则册的库内闸：达标集合为空、表达越界、区间倒置都进不了库；同一账户
// 至多一个未闭区间版本。
func TestTheRuleTableRefusesWhatTheRuleCannotMean(t *testing.T) {
	_, _, _, pool := newETAVisibility(t)
	t0 := etaJudgedAt
	early := t0.Add(-time.Hour)

	if err := insertRule(t, pool, "customer-a", "v-empty", t0, nil); err == nil {
		t.Error("达标集合为空的规则进了库")
	}
	if err := insertRule(t, pool, "customer-a", "v-inverted", t0, &early, "HIGH"); err == nil {
		t.Error("区间倒置的规则进了库")
	}
	if err := insertRule(t, pool, "customer-a", "v-open-1", t0, nil, "HIGH"); err != nil {
		t.Fatalf("登记首个未闭区间版本：%v", err)
	}
	if err := insertRule(t, pool, "customer-a", "v-open-2", t0.Add(time.Hour), nil, "HIGH"); err == nil {
		t.Error("同一账户并存两个未闭区间版本")
	}
	if err := insertRule(t, pool, "customer-b", "v-open-1", t0, nil, "HIGH"); err != nil {
		t.Errorf("别的账户的未闭区间版本被误拦：%v", err)
	}
}

// TestTheRuleTableRefusesABlankOrNullQualifyingConfidence 证达标集合的每个元素都要非空白：读口把行重建成领域规则时
// 空白依据会被领域层拒收，那会被译成规则读不动、判断落未决，消费者永远回滚重投；坏行只能挡在写入口。集合里有一个
// 空白或 NULL 元素就整行拒（check_violation），好元素不抵消坏元素。
func TestTheRuleTableRefusesABlankOrNullQualifyingConfidence(t *testing.T) {
	_, _, _, pool := newETAVisibility(t)
	t0 := etaJudgedAt

	for name, qualifying := range map[string][]string{
		"只有空格":     {"  "},
		"空串":       {""},
		"制表符":      {"\t"},
		"好元素带空白元素": {"HIGH/carrier-confirmed", "  "},
	} {
		var pgErr *pgconn.PgError
		// 每例用自己的账户：未闭区间唯一索引只拦同账户，不该盖过要看的那道 CHECK。
		err := insertRule(t, pool, "customer-"+name, "v-1", t0, nil, qualifying...)
		if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
			t.Errorf("%s：err = %v，want check_violation 23514", name, err)
		}
	}

	_, err := pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.eta_visibility_rule
			(tenant_id, customer_account_ref, service_rule_version, gate_version, target_milestone_ref,
			 qualifying_confidences, expression, stop_boundary_ref, effective_from, approved_by)
		 VALUES ('tenant-a', 'customer-null', 'v-1', 'gate/v1', 'DELIVERED',
			 ARRAY['HIGH/carrier-confirmed', NULL]::text[], 'TIME_RANGE', 'stop/after-delivered', $1, 'cs-lead')`, t0)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23514" {
		t.Errorf("含 NULL 元素：err = %v，want check_violation 23514", err)
	}

	if err := insertRule(t, pool, "customer-fine", "v-1", t0, nil, "HIGH/carrier-confirmed", "MEDIUM/history-based"); err != nil {
		t.Errorf("元素都非空白的集合被误拦：%v", err)
	}
}

// TestTheCurrentMarkerCanOnlyNameAStoredVersionOfItsOwnJudgementLine 证当前标记只能指同一条判断线上存下来的版本行：
// 标记改指不存在的版本、或指另一件包裹的版本，库都拒（foreign_key_violation）。没有这道外键，写错的标记会让
// FindCurrent 读不到版本行，判断线断在读口，原判断的前身也读不回。
func TestTheCurrentMarkerCanOnlyNameAStoredVersionOfItsOwnJudgementLine(t *testing.T) {
	judgements, _, transactor, pool := newETAVisibility(t)
	ctx := t.Context()
	tenant := tenantOf(t, "tenant-a")
	judgement := firstJudgement(t, "evj-1", "eta-1", etaRuleBasis(t, "MEDIUM/history-based"))
	within2(t, transactor, ctx, func(txCtx context.Context) error {
		return judgements.Save(txCtx, tenant, judgement)
	})

	for name, statement := range map[string]struct {
		sql  string
		args []any
	}{
		"改指不存在的版本": {
			`UPDATE visibility_exception.eta_visibility_judgement_current SET version = $1
			  WHERE tenant_id = 'tenant-a' AND customer_account_id = 'customer-a' AND parcel_id = 'parcel-1'`,
			[]any{"evj-ghost"},
		},
		"另一件包裹的标记指本件包裹的版本": {
			`INSERT INTO visibility_exception.eta_visibility_judgement_current
				(tenant_id, customer_account_id, parcel_id, version) VALUES ('tenant-a', 'customer-a', 'parcel-2', $1)`,
			[]any{"evj-1"},
		},
		"没有版本行的新标记": {
			`INSERT INTO visibility_exception.eta_visibility_judgement_current
				(tenant_id, customer_account_id, parcel_id, version) VALUES ('tenant-a', 'customer-a', 'parcel-3', $1)`,
			[]any{"evj-ghost"},
		},
	} {
		_, err := pool.Exec(ctx, statement.sql, statement.args...)
		var pgErr *pgconn.PgError
		if !errors.As(err, &pgErr) || pgErr.Code != "23503" {
			t.Errorf("%s：err = %v，want foreign_key_violation 23503", name, err)
		}
	}

	found, exists, err := judgements.FindCurrent(ctx, tenant, judgement.Customer(), judgement.Parcel())
	if err != nil || !exists || found.Version() != judgement.Version() {
		t.Fatalf("被拒的写入动了当前标记：exists = %v err = %v", exists, err)
	}
}

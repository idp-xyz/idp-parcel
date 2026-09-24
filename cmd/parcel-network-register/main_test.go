package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件证受控登记口的译装与退出码翻译：族路由、未知字段与未知族在入库前拒、受理门
// 拒绝译成退出码 1、未决译成退出码 3。替身立在 ports 写入口上，译装与受理门走的都是
// 真路径；写入口自身的版本纪律由适配器真库用例证。验证值一律 SYN- 合成（S 级只记 S）。
//
// 事务替身只透传闭包：事务纪律本身由适配器真库用例证（无事务登记被拒），登记口的
// 职责只是把用例包进一个事务。

type passthroughTransactor struct{}

func (passthroughTransactor) WithinTransaction(ctx context.Context, fn bentoapp.TxFunc) error {
	return fn(ctx)
}

// registryDouble 是 ports 写入口的替身，记谁被调了、带什么行到达。
type registryDouble struct {
	err error

	lastTenant domain.TenantID
	node       *ports.NodeDefinitionVersion
	connection *ports.ConnectionDefinitionVersion
	line       *ports.LineDefinitionVersion
	area       *ports.ServiceAreaDefinitionVersion
	calendar   *ports.ServiceCalendarDefinitionVersion
	strategy   *ports.RouteStrategyDefinitionVersion
	adjustment *ports.AvailabilityAdjustmentStatement
}

func (double *registryDouble) calls() int {
	count := 0
	for _, present := range []bool{
		double.node != nil, double.connection != nil, double.line != nil,
		double.area != nil, double.calendar != nil, double.strategy != nil,
		double.adjustment != nil,
	} {
		if present {
			count++
		}
	}
	return count
}

func (double *registryDouble) RegisterNodeVersion(
	_ context.Context, tenant domain.TenantID, row ports.NodeDefinitionVersion,
) error {
	double.lastTenant, double.node = tenant, &row
	return double.err
}

func (double *registryDouble) RegisterConnectionVersion(
	_ context.Context, tenant domain.TenantID, row ports.ConnectionDefinitionVersion,
) error {
	double.lastTenant, double.connection = tenant, &row
	return double.err
}

func (double *registryDouble) RegisterLineVersion(
	_ context.Context, tenant domain.TenantID, row ports.LineDefinitionVersion,
) error {
	double.lastTenant, double.line = tenant, &row
	return double.err
}

func (double *registryDouble) RegisterServiceAreaVersion(
	_ context.Context, tenant domain.TenantID, row ports.ServiceAreaDefinitionVersion,
) error {
	double.lastTenant, double.area = tenant, &row
	return double.err
}

func (double *registryDouble) RegisterServiceCalendarVersion(
	_ context.Context, tenant domain.TenantID, row ports.ServiceCalendarDefinitionVersion,
) error {
	double.lastTenant, double.calendar = tenant, &row
	return double.err
}

func (double *registryDouble) RegisterRouteStrategyVersion(
	_ context.Context, tenant domain.TenantID, row ports.RouteStrategyDefinitionVersion,
) error {
	double.lastTenant, double.strategy = tenant, &row
	return double.err
}

func (double *registryDouble) RegisterAvailabilityAdjustment(
	_ context.Context, tenant domain.TenantID, row ports.AvailabilityAdjustmentStatement,
) error {
	double.lastTenant, double.adjustment = tenant, &row
	return double.err
}

// registrarOver 交回只装了目录登记用例的那一集：七族测试用它，自动改路事实那一族的
// 位置留空——那一族走不到目录用例，装上反而看不出路由错位。
func registrarOver(t *testing.T, double *registryDouble) registrars {
	t.Helper()
	registrar, err := application.NewNetworkCatalogRegistration(double)
	if err != nil {
		t.Fatalf("构造登记用例：%v", err)
	}
	return registrars{catalog: registrar}
}

// factsRegistryDouble 是自动改路四条件事实写入口的替身。它比目录那个替身多一格：写口
// 的两格答案（已登记 / 已在册）由测试指定，`已在册`那一路编排还要读回既有版本比内容，
// 所以读回结果也得能摆布——幂等与冲突正是靠那次读回分开的。
type factsRegistryDouble struct {
	saveOutcome ports.AutoRerouteFactsSaveOutcome
	existing    ports.AutoRerouteFactsRecord
	found       bool
	err         error

	registered *ports.AutoRerouteFactsRecord
	lookups    int
}

func (double *factsRegistryDouble) RegisterAutoRerouteFacts(
	_ context.Context, record ports.AutoRerouteFactsRecord,
) (ports.AutoRerouteFactsSaveOutcome, error) {
	double.registered = &record
	if double.err != nil {
		return ports.AutoRerouteFactsSaveOutcomeInvalid, double.err
	}
	return double.saveOutcome, nil
}

func (double *factsRegistryDouble) FindAutoRerouteFacts(
	_ context.Context, _ domain.InitialRouteJudgmentKey, _ int,
) (ports.AutoRerouteFactsRecord, bool, error) {
	double.lookups++
	return double.existing, double.found, nil
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

// factsRegistrarsOver 交回只装了事实登记用例的那一集，时钟固定——登记时刻由用例取
// 时钟，不由登记行携带，钉一个固定值才验得出这条。
func factsRegistrarsOver(double *factsRegistryDouble, at time.Time) registrars {
	return registrars{
		autoRerouteFacts: application.NewRegisterAutoRerouteFactsHandler(
			application.RegisterAutoRerouteFactsDeps{Registry: double, Clock: fixedClock{at: at}},
		),
	}
}

// autoRerouteFactsRow 是一份齐全的四条件事实登记行，测试各例在它上面改一处。
const autoRerouteFactsRow = `{
	"tenant_id": "SYN-TENANT-1", "customer_account_id": "SYN-ACCOUNT-1",
	"shipment_request_id": "SYN-REQUEST-1", "acceptance_baseline": "SYN-BASELINE-1/v1",
	"declared_parcel_id": "SYN-PARCEL-1", "service_purpose": "NETWORK_SERVICE",
	"version": 1,
	"policy_allows_automatic": true, "at_controlled_node": true,
	"only_unexecuted_affected": false,
	"unresolved_restrictions": ["SYN-RESTRICTION-1"],
	"outstanding_responsibilities": ["SYN-RESPONSIBILITY-1"],
	"strategy_basis": "SYN-STRATEGY-1/v1"
}`

// Covers: 第八族的绿路径——判断键六维、三个折算结论、两份清单与折算依据逐件到达写
// 入口；登记时刻取时钟而不取登记行；`已登记`译成退出码 0。
func TestExecuteRegistersAutoRerouteFacts(t *testing.T) {
	minted := time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC)
	double := &factsRegistryDouble{saveOutcome: ports.AutoRerouteFactsRegistered}

	message, code := execute(t.Context(), kindAutoRerouteFacts, []byte(autoRerouteFactsRow),
		factsRegistrarsOver(double, minted), passthroughTransactor{})
	if code != exitRegistered || !strings.Contains(message, "REGISTERED") {
		t.Fatalf("message=%q code=%d, 想要 REGISTERED/0", message, code)
	}
	if double.registered == nil {
		t.Fatal("事实行没有到达写入口")
	}

	record := *double.registered
	if !record.Key.MinimumIdentityEstablished() {
		t.Fatalf("判断键六维没有齐备地译装：%+v", record.Key)
	}
	if record.Key.TenantID.String() != "SYN-TENANT-1" ||
		record.Key.DeclaredParcelID.String() != "SYN-PARCEL-1" {
		t.Fatalf("键没有原样译装：%+v", record.Key)
	}
	if record.Version != 1 || record.StrategyBasis != "SYN-STRATEGY-1/v1" {
		t.Fatalf("版本或折算依据没有原样译装：%+v", record)
	}
	if !record.Facts.PolicyAllowsAutomatic || !record.Facts.AtControlledNode ||
		record.Facts.OnlyUnexecutedAffected {
		t.Fatalf("三个折算结论没有逐格译装：%+v", record.Facts)
	}
	if len(record.Facts.UnresolvedRestrictions) != 1 ||
		len(record.Facts.OutstandingResponsibilities) != 1 {
		t.Fatalf("两份清单没有原样译装：%+v", record.Facts)
	}
	if !record.RegisteredAt.Equal(minted) {
		t.Fatalf("登记时刻 = %v；它该由时钟给，不由登记行带入", record.RegisteredAt)
	}
}

// factsOnRegister 造一版既有陈述，供`已在册`那一路读回比对。restriction 给不同值就得到
// 一版内容不同的陈述。
func factsOnRegister(t *testing.T, restriction, basis string) ports.AutoRerouteFactsRecord {
	t.Helper()
	restrictionRef, err := domain.NewRestrictionReference(restriction)
	if err != nil {
		t.Fatalf("构造限制引用：%v", err)
	}
	responsibilityRef, err := domain.NewResponsibilityReference("SYN-RESPONSIBILITY-1")
	if err != nil {
		t.Fatalf("构造责任引用：%v", err)
	}
	return ports.AutoRerouteFactsRecord{
		Version: 1,
		Facts: domain.AutoRerouteFacts{
			PolicyAllowsAutomatic:       true,
			AtControlledNode:            true,
			OnlyUnexecutedAffected:      false,
			UnresolvedRestrictions:      []domain.RestrictionReference{restrictionRef},
			OutstandingResponsibilities: []domain.ResponsibilityReference{responsibilityRef},
		},
		StrategyBasis: basis,
	}
}

// Covers: 同键同版本重登不覆盖，两格治理答案各自具名且都译成退出码 2——`已存在`是重放
// 同一份，`内容冲突`是换了内容。**两格都不是失败**：原行不被顶替，续办属治理裁决，折成
// 退出码 1 会让操作员以为改一改输入重试就成。
func TestExecuteTranslatesAutoRerouteFactsGovernanceAnswers(t *testing.T) {
	cases := map[string]struct {
		existing ports.AutoRerouteFactsRecord
		want     string
	}{
		"重放同一份内容":     {factsOnRegister(t, "SYN-RESTRICTION-1", "SYN-STRATEGY-1/v1"), "ALREADY_EXISTS"},
		"同键同版本换了内容":   {factsOnRegister(t, "SYN-RESTRICTION-2", "SYN-STRATEGY-1/v1"), "CONTENT_CONFLICT"},
		"同键同版本换了折算依据": {factsOnRegister(t, "SYN-RESTRICTION-1", "SYN-STRATEGY-2/v1"), "CONTENT_CONFLICT"},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			double := &factsRegistryDouble{
				saveOutcome: ports.AutoRerouteFactsAlreadyRegistered,
				existing:    test.existing,
				found:       true,
			}
			message, code := execute(t.Context(), kindAutoRerouteFacts, []byte(autoRerouteFactsRow),
				factsRegistrarsOver(double, time.Now()), passthroughTransactor{})
			if code != exitAttention {
				t.Fatalf("message=%q code=%d, 想要治理答案/2", message, code)
			}
			if !strings.Contains(message, test.want) {
				t.Fatalf("message=%q, 想要含 %s", message, test.want)
			}
			if double.lookups != 1 {
				t.Fatalf("读回次数 = %d；幂等与冲突的分界正是靠那一次读回", double.lookups)
			}
		})
	}
}

// Covers: 事实族的译装门与受理门都在入库前拒，译成退出码 1。译装门管「这一维写坏了」，
// 受理门管「这一格确实缺了」——两者都不得到达写入口，也都不得补默认值：补一个占位会把
// 「还没人陈述过」变成一份能进改路评估的事实。
func TestExecuteRefusesIncompleteAutoRerouteFacts(t *testing.T) {
	cases := map[string]struct {
		payload string
		want    string
	}{
		"未知字段": {strings.Replace(autoRerouteFactsRow, `"version": 1,`,
			`"version": 1, "threshold": 0.15,`, 1), ""},
		"空白租户": {strings.Replace(autoRerouteFactsRow, `"tenant_id": "SYN-TENANT-1"`,
			`"tenant_id": "  "`, 1), ""},
		"缺服务目的": {strings.Replace(autoRerouteFactsRow, `"service_purpose": "NETWORK_SERVICE"`,
			`"service_purpose": ""`, 1), ""},
		"版本缺": {strings.Replace(autoRerouteFactsRow, `"version": 1,`,
			`"version": 0,`, 1), "VERSION_MISSING"},
		"折算依据缺": {strings.Replace(autoRerouteFactsRow, `"strategy_basis": "SYN-STRATEGY-1/v1"`,
			`"strategy_basis": ""`, 1), "STRATEGY_BASIS_MISSING"},
		"限制清单有空白元素": {strings.Replace(autoRerouteFactsRow, `["SYN-RESTRICTION-1"]`,
			`["SYN-RESTRICTION-1", ""]`, 1), "RESTRICTION_BLANK"},
		"责任清单有空白元素": {strings.Replace(autoRerouteFactsRow, `["SYN-RESPONSIBILITY-1"]`,
			`[""]`, 1), "RESPONSIBILITY_BLANK"},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			double := &factsRegistryDouble{saveOutcome: ports.AutoRerouteFactsRegistered}
			message, code := execute(t.Context(), kindAutoRerouteFacts, []byte(test.payload),
				factsRegistrarsOver(double, time.Now()), passthroughTransactor{})
			if code != exitUsage {
				t.Fatalf("message=%q code=%d, 想要拒绝/1", message, code)
			}
			if test.want != "" && !strings.Contains(message, test.want) {
				t.Fatalf("message=%q, 想要指名 %s", message, test.want)
			}
			if double.registered != nil {
				t.Fatalf("被拒的登记到达了写入口：%+v", double.registered)
			}
		})
	}
}

// Covers: 事实族的依赖故障译成退出码 3——登记与否未知，原因在消息里。它与治理答案分开
// 是因为续办动作相反：未决可以原样重试，治理答案重试一万次都是同一格。
func TestExecuteSurfacesUndecidedAutoRerouteFacts(t *testing.T) {
	boom := errors.New("auto reroute facts store is down")
	double := &factsRegistryDouble{err: boom}
	message, code := execute(t.Context(), kindAutoRerouteFacts, []byte(autoRerouteFactsRow),
		factsRegistrarsOver(double, time.Now()), passthroughTransactor{})
	if code != exitUndecided || !strings.Contains(message, "auto reroute facts store is down") {
		t.Fatalf("message=%q code=%d, 想要未决/3 且带成因", message, code)
	}
}

// Covers: 绿路径与可选终点的译装——未闭区间不带 effective_to（Has 位为假），补历史
// 带 effective_to（Has 位为真且值原样）；行与租户逐字段到达写入口，`已登记`译成 0。
func TestExecuteRegistersANodeAndTranslatesTheOptionalEnd(t *testing.T) {
	open := []byte(`{
		"tenant_id": "SYN-TENANT-1", "code": "SYN-NODE-HUB", "version": 1,
		"business_timezone": "Asia/Shanghai", "effective_from": "2026-08-20T12:00:00Z"
	}`)
	double := &registryDouble{}
	message, code := execute(t.Context(), kindNode, open, registrarOver(t, double), passthroughTransactor{})
	if code != exitRegistered || !strings.Contains(message, "REGISTERED") {
		t.Fatalf("message=%q code=%d, 想要 REGISTERED/0", message, code)
	}
	if double.node == nil || double.calls() != 1 {
		t.Fatalf("节点行没有到达写入口：%+v", double)
	}
	if double.node.Code != "SYN-NODE-HUB" || double.node.Version != 1 ||
		double.node.BusinessTimezone != "Asia/Shanghai" ||
		!double.node.EffectiveFrom.Equal(time.Date(2026, 8, 20, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("行没有原样译装：%+v", double.node)
	}
	if double.node.HasEffectiveTo {
		t.Fatal("没给 effective_to 却译出了终点")
	}
	if double.lastTenant.String() != "SYN-TENANT-1" {
		t.Fatalf("租户没有原样到达：%q", double.lastTenant.String())
	}

	closed := []byte(`{
		"tenant_id": "SYN-TENANT-1", "code": "SYN-NODE-HUB", "version": 1,
		"business_timezone": "Asia/Shanghai",
		"effective_from": "2026-08-20T12:00:00Z", "effective_to": "2026-08-21T12:00:00Z"
	}`)
	double = &registryDouble{}
	if _, code := execute(t.Context(), kindNode, closed, registrarOver(t, double), passthroughTransactor{}); code != exitRegistered {
		t.Fatalf("补历史闭区间应过门，code = %d", code)
	}
	if double.node == nil || !double.node.HasEffectiveTo ||
		!double.node.EffectiveTo.Equal(time.Date(2026, 8, 21, 12, 0, 0, 0, time.UTC)) {
		t.Fatalf("闭区间终点没有原样译装：%+v", double.node)
	}
}

// Covers: 族路由封闭七格——每族落到自己的写方法，封闭枚举逐格译（调整族的
// target_kind 与 kind 都是词到格）。
func TestExecuteRoutesEachKindToItsRegisterMethod(t *testing.T) {
	payloads := map[string]string{
		kindNode: `{"tenant_id":"SYN-TENANT-1","code":"SYN-NODE-A","version":1,
			"business_timezone":"Asia/Shanghai","effective_from":"2026-08-20T12:00:00Z"}`,
		kindConnection: `{"tenant_id":"SYN-TENANT-1","code":"SYN-CONN-A-B","version":1,
			"from_node":"SYN-NODE-A","to_node":"SYN-NODE-B",
			"business_timezone":"Asia/Shanghai","effective_from":"2026-08-20T12:00:00Z"}`,
		kindLine: `{"tenant_id":"SYN-TENANT-1","code":"SYN-LINE-EU","version":1,
			"segments":["SYN-CONN-A-B"],"business_timezone":"Asia/Shanghai",
			"applicable_scope":"SYN-SCOPE-DECLARED","effective_from":"2026-08-20T12:00:00Z"}`,
		kindServiceArea: `{"tenant_id":"SYN-TENANT-1","code":"SYN-AREA-DE","version":1,
			"effective_from":"2026-08-20T12:00:00Z"}`,
		kindServiceCalendar: `{"tenant_id":"SYN-TENANT-1","target_kind":"NODE",
			"target_code":"SYN-NODE-A","version":1,"effective_from":"2026-08-20T12:00:00Z"}`,
		kindAvailabilityAdjustment: `{"tenant_id":"SYN-TENANT-1","code":"SYN-ADJ-1","version":1,
			"target_kind":"LINE","target_code":"SYN-LINE-EU","kind":"SUSPENSION",
			"source":"SYN-NET-OPS/EVT-1","effective_at":"2026-08-20T12:00:00Z"}`,
		kindRouteStrategy: `{"tenant_id":"SYN-TENANT-1","code":"SYN-STRATEGY-1","version":1,
			"applicable_scope":"SYN-SCOPE-DECLARED","effective_from":"2026-08-20T12:00:00Z"}`,
	}
	arrived := map[string]func(double *registryDouble) bool{
		kindNode:            func(d *registryDouble) bool { return d.node != nil },
		kindConnection:      func(d *registryDouble) bool { return d.connection != nil },
		kindLine:            func(d *registryDouble) bool { return d.line != nil },
		kindServiceArea:     func(d *registryDouble) bool { return d.area != nil },
		kindServiceCalendar: func(d *registryDouble) bool { return d.calendar != nil },
		kindAvailabilityAdjustment: func(d *registryDouble) bool {
			return d.adjustment != nil &&
				d.adjustment.TargetKind == ports.TargetLine &&
				d.adjustment.Kind == ports.AdjustmentSuspension
		},
		kindRouteStrategy: func(d *registryDouble) bool { return d.strategy != nil },
	}

	for kind, payload := range payloads {
		double := &registryDouble{}
		message, code := execute(t.Context(), kind, []byte(payload), registrarOver(t, double), passthroughTransactor{})
		if code != exitRegistered {
			t.Fatalf("%s: message=%q code=%d, 想要 0", kind, message, code)
		}
		if double.calls() != 1 || !arrived[kind](double) {
			t.Fatalf("%s: 路由错位：%+v", kind, double)
		}
	}
}

// Covers: ADR-0146 决定二的登记半边——路由策略版本的 ranking_form 经领域构造门译成族内形态到达
// 写入口；不带这一格即「未声明」，照旧登得进，不替租户选一种。
func TestExecuteTranslatesTheDeclaredRankingForm(t *testing.T) {
	declared := []byte(`{"tenant_id":"SYN-TENANT-1","code":"SYN-STRATEGY-1","version":1,
		"applicable_scope":"SYN-SCOPE-DECLARED","effective_from":"2026-08-20T12:00:00Z",
		"ranking_form":"COST_SINGLE_DIMENSION"}`)
	double := &registryDouble{}
	if message, code := execute(t.Context(), kindRouteStrategy, declared, registrarOver(t, double), passthroughTransactor{}); code != exitRegistered {
		t.Fatalf("message=%q code=%d, 想要 REGISTERED/0", message, code)
	}
	if double.strategy == nil || double.strategy.RankingForm != domain.CostSingleDimensionRanking {
		t.Fatalf("排序形态没有译成族内形态到达写入口：%+v", double.strategy)
	}

	undeclared := []byte(`{"tenant_id":"SYN-TENANT-1","code":"SYN-STRATEGY-1","version":1,
		"applicable_scope":"SYN-SCOPE-DECLARED","effective_from":"2026-08-20T12:00:00Z"}`)
	double = &registryDouble{}
	if message, code := execute(t.Context(), kindRouteStrategy, undeclared, registrarOver(t, double), passthroughTransactor{}); code != exitRegistered {
		t.Fatalf("message=%q code=%d; 没声明形态的版本应照旧登得进", message, code)
	}
	if double.strategy == nil || double.strategy.RankingForm != domain.RankingFormUndeclared {
		t.Fatalf("没给 ranking_form 却译出了形态：%+v", double.strategy)
	}
}

// Covers: 译装门在入库前拒——未知族、未知字段、封闭枚举外的词、空白租户都不到达
// 写入口，全部译成退出码 1。
func TestExecuteRefusesAtTheTranslationGate(t *testing.T) {
	cases := map[string]struct {
		kind    string
		payload string
	}{
		"未知族": {"moon-route", `{}`},
		"未知字段": {kindNode, `{"tenant_id":"SYN-TENANT-1","code":"SYN-NODE-A","version":1,
			"business_timezone":"Asia/Shanghai","effective_from":"2026-08-20T12:00:00Z",
			"timezone":"Asia/Shanghai"}`},
		"适用对象类别不在封闭三类": {kindServiceCalendar, `{"tenant_id":"SYN-TENANT-1",
			"target_kind":"WAREHOUSE","target_code":"SYN-NODE-A","version":1,
			"effective_from":"2026-08-20T12:00:00Z"}`},
		"调整种类不在封闭四格": {kindAvailabilityAdjustment, `{"tenant_id":"SYN-TENANT-1",
			"code":"SYN-ADJ-1","version":1,"target_kind":"LINE","target_code":"SYN-LINE-EU",
			"kind":"PAUSE","source":"SYN-NET-OPS/EVT-1","effective_at":"2026-08-20T12:00:00Z"}`,
		},
		"空白租户": {kindNode, `{"tenant_id":"  ","code":"SYN-NODE-A","version":1,
			"business_timezone":"Asia/Shanghai","effective_from":"2026-08-20T12:00:00Z"}`},
		"排序形态不在族内": {kindRouteStrategy, `{"tenant_id":"SYN-TENANT-1","code":"SYN-STRATEGY-1",
			"version":1,"applicable_scope":"SYN-SCOPE-DECLARED","effective_from":"2026-08-20T12:00:00Z",
			"ranking_form":"TIMELINESS_FIRST"}`},
		"排序形态给了空词": {kindRouteStrategy, `{"tenant_id":"SYN-TENANT-1","code":"SYN-STRATEGY-1",
			"version":1,"applicable_scope":"SYN-SCOPE-DECLARED","effective_from":"2026-08-20T12:00:00Z",
			"ranking_form":""}`},
	}

	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			double := &registryDouble{}
			message, code := execute(t.Context(), test.kind, []byte(test.payload), registrarOver(t, double), passthroughTransactor{})
			if code != exitUsage {
				t.Fatalf("message=%q code=%d, 想要译装拒绝/1", message, code)
			}
			if double.calls() != 0 {
				t.Fatal("译装不成的字节到达了写入口")
			}
		})
	}
}

// Covers: 受理门拒绝是登记方要改输入的答案——译成退出码 1，拒绝理由在消息里指名，
// 写入口零调用。
func TestExecuteTranslatesGateRefusals(t *testing.T) {
	missingTimezone := []byte(`{
		"tenant_id": "SYN-TENANT-1", "code": "SYN-NODE-HUB", "version": 1,
		"effective_from": "2026-08-20T12:00:00Z"
	}`)
	double := &registryDouble{}
	message, code := execute(t.Context(), kindNode, missingTimezone, registrarOver(t, double), passthroughTransactor{})
	if code != exitUsage || !strings.Contains(message, "TIMEZONE_MISSING") {
		t.Fatalf("message=%q code=%d, 想要 REFUSED(TIMEZONE_MISSING)/1", message, code)
	}
	if double.calls() != 0 {
		t.Fatal("被拒的登记到达了写入口")
	}
}

// Covers: 依赖故障译成退出码 3——登记与否未知，原因在消息里。
func TestExecuteSurfacesUndecided(t *testing.T) {
	boom := errors.New("catalog store is down")
	double := &registryDouble{err: boom}
	payload := []byte(`{
		"tenant_id": "SYN-TENANT-1", "code": "SYN-AREA-DE", "version": 1,
		"effective_from": "2026-08-20T12:00:00Z"
	}`)
	message, code := execute(t.Context(), kindServiceArea, payload, registrarOver(t, double), passthroughTransactor{})
	if code != exitUndecided || !strings.Contains(message, "catalog store is down") {
		t.Fatalf("message=%q code=%d, 想要未决/3 且带成因", message, code)
	}
}

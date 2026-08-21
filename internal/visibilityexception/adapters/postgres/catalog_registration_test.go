package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var registrarBaseAt = time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)

type registrarFixture struct {
	pool       *pgxpool.Pool
	db         *bentopg.DB
	registrar  *adapter.CatalogRegistrar
	transactor bentoapp.Transactor
}

func newRegistrarFixture(t *testing.T) *registrarFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registrar, err := adapter.NewCatalogRegistrar(db)
	if err != nil {
		t.Fatalf("构造目录写入方：%v", err)
	}
	return &registrarFixture{pool: pool, db: db, registrar: registrar, transactor: db.Transactor()}
}

func registrarTenant(t *testing.T) domain.TenantID {
	t.Helper()
	return build(t, domain.NewTenantID, "tenant-a")
}

// registerWithin 在一个事务里跑一次登记并交回结果。写入口按 RequireExecutor 语义要求
// 调用方的事务在场——抬头与整版条目必须同一提交。
func registerWithin(
	t *testing.T,
	fixture *registrarFixture,
	register func(context.Context) (ports.CatalogRegistrationOutcome, error),
) ports.CatalogRegistrationOutcome {
	t.Helper()
	var outcome ports.CatalogRegistrationOutcome
	if err := fixture.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var err error
		outcome, err = register(txCtx)
		return err
	}); err != nil {
		t.Fatalf("事务内登记失败：%v", err)
	}
	return outcome
}

func mappingHeader(version string, from time.Time) ports.CatalogVersionHeader {
	return ports.CatalogVersionHeader{
		Version:       version,
		ApprovedBy:    "tracking-ops",
		EffectiveFrom: from,
	}
}

func closedHeader(version string, from, to time.Time) ports.CatalogVersionHeader {
	header := mappingHeader(version, from)
	header.EffectiveTo = to
	header.HasEffectiveTo = true
	return header
}

func mappingEntry(t *testing.T, kind, milestone string) ports.MilestoneMappingEntry {
	t.Helper()
	return ports.MilestoneMappingEntry{
		Source:    domain.SourceNodeOperations,
		Kind:      build(t, domain.NewSourceFactKind, kind),
		Milestone: build(t, domain.NewMilestoneReference, milestone),
	}
}

func (fixture *registrarFixture) countVersions(t *testing.T, table string) int {
	t.Helper()
	var count int
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM visibility_exception.`+table).Scan(&count); err != nil {
		t.Fatalf("数 %s 行数：%v", table, err)
	}
	return count
}

// Covers: 票 09 的整条主张——五处哨兵同根堵在「有装载无写入」。这条钉住写入口一落地
// 哨兵就熄得掉：登记一版映射之后，只读装载口第一次能对一份事实答出归类，而在此之前
// 它除了 `MAPPING_NOT_CONFIGURED` 别无可答。
func TestRegisteredMappingBecomesReadableByTheLoadingPort(t *testing.T) {
	fixture := newRegistrarFixture(t)
	tenant := registrarTenant(t)
	view, err := adapter.NewMilestoneMappings(fixture.db, tenant)
	if err != nil {
		t.Fatalf("构造映射视图：%v", err)
	}
	fact := catalogFact(t, "scan/origin", "node-intake", registrarBaseAt.Add(time.Hour))

	if _, configured, err := view.ClassifyFact(t.Context(), fact); err != nil || configured {
		t.Fatalf("登记之前必须答未配置：err=%v configured=%v", err, configured)
	}

	outcome := registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterMilestoneMapping(txCtx, tenant, ports.MilestoneMappingRegistration{
			Header:  mappingHeader("map/v1", registrarBaseAt),
			Entries: []ports.MilestoneMappingEntry{mappingEntry(t, "node-intake", "PICKED_UP")},
		})
	})
	if outcome != ports.CatalogVersionRegistered {
		t.Fatalf("首登应成功，实得 %s", outcome)
	}

	answer, configured, err := view.ClassifyFact(t.Context(), fact)
	if err != nil || !configured {
		t.Fatalf("登记之后应答已配置：err=%v configured=%v", err, configured)
	}
	if !answer.Classified || answer.Milestone.String() != "PICKED_UP" ||
		answer.Mapping.String() != "map/v1" {
		t.Fatalf("归类答复 = classified=%v %s / %s",
			answer.Classified, answer.Milestone, answer.Mapping)
	}
}

// Covers: ADR-0068 的接续闭合——登记未闭新版时，前一个未闭版本按新版生效时间补上终点。
// 那不是改写历史：旧版在自己的区间内仍被选中（这里用生效前一秒的事实证），只是不再
// 参与新判断（「新版本自明确生效时间起参与新判断」）。
func TestRegisteringAnOpenVersionClosesThePredecessorAtItsStart(t *testing.T) {
	fixture := newRegistrarFixture(t)
	tenant := registrarTenant(t)
	view, err := adapter.NewMilestoneMappings(fixture.db, tenant)
	if err != nil {
		t.Fatalf("构造映射视图：%v", err)
	}
	switchAt := registrarBaseAt.Add(48 * time.Hour)

	registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterMilestoneMapping(txCtx, tenant, ports.MilestoneMappingRegistration{
			Header:  mappingHeader("map/v1", registrarBaseAt),
			Entries: []ports.MilestoneMappingEntry{mappingEntry(t, "node-intake", "PICKED_UP")},
		})
	})
	outcome := registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterMilestoneMapping(txCtx, tenant, ports.MilestoneMappingRegistration{
			Header:  mappingHeader("map/v2", switchAt),
			Entries: []ports.MilestoneMappingEntry{mappingEntry(t, "node-intake", "ARRIVED_AT_NODE")},
		})
	})
	if outcome != ports.CatalogVersionRegistered {
		t.Fatalf("登记第二版应成功（前版接续闭合），实得 %s", outcome)
	}

	before, _, err := view.ClassifyFact(t.Context(),
		catalogFact(t, "scan/a", "node-intake", switchAt.Add(-time.Second)))
	if err != nil {
		t.Fatalf("读生效前一秒：%v", err)
	}
	if before.Mapping.String() != "map/v1" || before.Milestone.String() != "PICKED_UP" {
		t.Fatalf("生效前一秒应按 v1 判，实得 %s / %s", before.Mapping, before.Milestone)
	}

	// 生效当刻属新版：区间是 [from, to)，边界归后者。
	at, _, err := view.ClassifyFact(t.Context(), catalogFact(t, "scan/b", "node-intake", switchAt))
	if err != nil {
		t.Fatalf("读生效当刻：%v", err)
	}
	if at.Mapping.String() != "map/v2" || at.Milestone.String() != "ARRIVED_AT_NODE" {
		t.Fatalf("生效当刻应按 v2 判，实得 %s / %s", at.Mapping, at.Milestone)
	}
}

// Covers: 票 09 红线「同一时点两个适用版本是错误，登记口须在写入侧防重叠，不靠读侧
// 兜」。补一段与既有已闭区间相交的历史必须被挡在库外——读口的 ErrAmbiguousCatalog
// 兜的是已经坏了的数据，而这一格是不让它坏。
func TestOverlappingHistoricalRangeIsRefusedAtTheWriteSide(t *testing.T) {
	fixture := newRegistrarFixture(t)
	tenant := registrarTenant(t)

	registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterMilestoneMapping(txCtx, tenant, ports.MilestoneMappingRegistration{
			Header: closedHeader("map/v1", registrarBaseAt, registrarBaseAt.Add(72*time.Hour)),
			Entries: []ports.MilestoneMappingEntry{
				mappingEntry(t, "node-intake", "PICKED_UP")},
		})
	})

	outcome := registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterMilestoneMapping(txCtx, tenant, ports.MilestoneMappingRegistration{
			Header: closedHeader("map/v2",
				registrarBaseAt.Add(24*time.Hour), registrarBaseAt.Add(96*time.Hour)),
			Entries: []ports.MilestoneMappingEntry{
				mappingEntry(t, "node-intake", "ARRIVED_AT_NODE")},
		})
	})
	if outcome != ports.CatalogVersionOverlapsExisting {
		t.Fatalf("重叠区间应被拒，实得 %s", outcome)
	}
	if count := fixture.countVersions(t, "milestone_mapping_version"); count != 1 {
		t.Fatalf("被拒的登记不该留下版本行，实得 %d 行", count)
	}
	if count := fixture.countVersions(t, "milestone_mapping_entry"); count != 1 {
		t.Fatalf("被拒的登记不该留下条目行，实得 %d 行", count)
	}

	// 首尾相接不算重叠：区间半开，前一版的终点正是后一版的起点。
	adjacent := registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterMilestoneMapping(txCtx, tenant, ports.MilestoneMappingRegistration{
			Header: mappingHeader("map/v3", registrarBaseAt.Add(72*time.Hour)),
			Entries: []ports.MilestoneMappingEntry{
				mappingEntry(t, "node-intake", "ARRIVED_AT_NODE")},
		})
	})
	if adjacent != ports.CatalogVersionRegistered {
		t.Fatalf("首尾相接的区间应登得进，实得 %s", adjacent)
	}
}

// Covers: 判定必须排在唯一那次 UPDATE 之前。撞版本号时若已经先接续闭合了前一版，
// 调用方一提交就只剩一个被停用的当前版本而没有接替者——目录静默变成「此刻无适用
// 版本」，读口据此作出的未归类看起来还是有依据的。这条钉住被拒时事务干净得像没来过。
func TestRefusedRegistrationLeavesThePredecessorOpen(t *testing.T) {
	fixture := newRegistrarFixture(t)
	tenant := registrarTenant(t)

	registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterMilestoneMapping(txCtx, tenant, ports.MilestoneMappingRegistration{
			Header:  mappingHeader("map/v1", registrarBaseAt),
			Entries: []ports.MilestoneMappingEntry{mappingEntry(t, "node-intake", "PICKED_UP")},
		})
	})

	outcome := registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterMilestoneMapping(txCtx, tenant, ports.MilestoneMappingRegistration{
			Header:  mappingHeader("map/v1", registrarBaseAt.Add(24*time.Hour)),
			Entries: []ports.MilestoneMappingEntry{mappingEntry(t, "node-intake", "DEPARTED_NODE")},
		})
	})
	if outcome != ports.CatalogVersionAlreadyRegistered {
		t.Fatalf("撞版本号应答已登记（不可覆盖），实得 %s", outcome)
	}

	var openVersions int
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM visibility_exception.milestone_mapping_version
		  WHERE tenant_id = $1 AND effective_to IS NULL`, tenant.String(),
	).Scan(&openVersions); err != nil {
		t.Fatalf("数未闭区间版本：%v", err)
	}
	if openVersions != 1 {
		t.Fatalf("被拒的登记不该顺手停用当前版本，未闭区间版本实得 %d 个", openVersions)
	}
	if count := fixture.countVersions(t, "milestone_mapping_entry"); count != 1 {
		t.Fatalf("被拒的登记不该写条目，实得 %d 行", count)
	}
}

// Covers: 租户是最高数据隔离边界（ADR-0003）。区间判定与接续闭合都按租户收窄——另一
// 租户的同名区间既不该挡住本租户的登记，也不该被本租户的登记闭合。
func TestRegistrationIsScopedToItsTenant(t *testing.T) {
	fixture := newRegistrarFixture(t)
	first := registrarTenant(t)
	second := build(t, domain.NewTenantID, "tenant-b")

	registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterMilestoneMapping(txCtx, first, ports.MilestoneMappingRegistration{
			Header:  mappingHeader("map/v1", registrarBaseAt),
			Entries: []ports.MilestoneMappingEntry{mappingEntry(t, "node-intake", "PICKED_UP")},
		})
	})
	outcome := registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterMilestoneMapping(txCtx, second, ports.MilestoneMappingRegistration{
			Header:  mappingHeader("map/v1", registrarBaseAt),
			Entries: []ports.MilestoneMappingEntry{mappingEntry(t, "node-intake", "ARRIVED_AT_NODE")},
		})
	})
	if outcome != ports.CatalogVersionRegistered {
		t.Fatalf("另一租户的同名同区间版本应登得进，实得 %s", outcome)
	}

	var stillOpen bool
	if err := fixture.pool.QueryRow(t.Context(),
		`SELECT effective_to IS NULL FROM visibility_exception.milestone_mapping_version
		  WHERE tenant_id = $1 AND mapping_version = 'map/v1'`, first.String(),
	).Scan(&stillOpen); err != nil {
		t.Fatalf("读第一个租户的版本：%v", err)
	}
	if !stillOpen {
		t.Fatal("另一租户的登记闭合了本租户的当前版本——租户维在写入侧漏了")
	}
}

// Covers: 分诊目录（`PAR-VIS-05`）的写入方与 `TRIAGE_RULES_NOT_CONFIGURED` 哨兵。条目
// 含可信度维：同一类型不同可信度可以走不同格，这正是四走向分界的所在。
func TestRegisteredTriageRulesBecomeReadable(t *testing.T) {
	fixture := newRegistrarFixture(t)
	tenant := registrarTenant(t)
	view, err := adapter.NewTriageRules(fixture.db, tenant)
	if err != nil {
		t.Fatalf("构造分诊视图：%v", err)
	}
	query := ports.TriageQuery{
		Kind:       build(t, domain.NewExceptionSignalKindReference, "CUSTOMS_HOLD"),
		Parcel:     build(t, domain.NewTrackedParcelReference, "parcel-1"),
		Rule:       build(t, domain.NewSignalRuleVersionReference, "signal-rule/v1"),
		Confidence: build(t, domain.NewConfidenceReference, "HIGH"),
	}

	if _, configured, err := view.TriageSignal(t.Context(), query); err != nil || configured {
		t.Fatalf("登记之前必须答未配置：err=%v configured=%v", err, configured)
	}

	outcome := registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterTriageRules(txCtx, tenant, ports.TriageRuleRegistration{
			// 生效时间取过去：分诊判的是手上这个活信号，装载口按 now() 选版。
			Header: mappingHeader("triage/v1", time.Now().UTC().Add(-time.Hour)),
			Entries: []ports.TriageRuleEntry{
				{Kind: query.Kind, Confidence: query.Confidence, Outcome: domain.AutoEstablishCase},
				{
					Kind:       query.Kind,
					Confidence: build(t, domain.NewConfidenceReference, "LOW"),
					Outcome:    domain.ManualReviewRequired,
				},
			},
		})
	})
	if outcome != ports.CatalogVersionRegistered {
		t.Fatalf("登记分诊规则应成功，实得 %s", outcome)
	}

	answer, configured, err := view.TriageSignal(t.Context(), query)
	if err != nil || !configured {
		t.Fatalf("登记之后应答已配置：err=%v configured=%v", err, configured)
	}
	if answer.Outcome != domain.AutoEstablishCase || answer.Rule.String() != "triage/v1" {
		t.Fatalf("高可信应命中自动建案，实得 %s / %s", answer.Outcome, answer.Rule)
	}

	query.Confidence = build(t, domain.NewConfidenceReference, "LOW")
	low, _, err := view.TriageSignal(t.Context(), query)
	if err != nil {
		t.Fatalf("读低可信：%v", err)
	}
	if low.Outcome != domain.ManualReviewRequired {
		t.Fatalf("低可信应命中人工复核，实得 %s", low.Outcome)
	}
}

// Covers: 披露策略（`PAR-VIS-09`）的写入方与「四维全部待确认」那处空册哨兵。四维各自
// 独立落状态与内容来处，展示带内容、待确认与不展示不带——由 domain.ViewDimension 在
// 构造期担保，库上四条 shape 约束是第二道网。
func TestRegisteredDisclosurePolicyBecomeReadable(t *testing.T) {
	fixture := newRegistrarFixture(t)
	tenant := registrarTenant(t)
	view, err := adapter.NewDisclosurePolicies(fixture.db, tenant)
	if err != nil {
		t.Fatalf("构造披露视图：%v", err)
	}
	customer := disclosureCustomer(t, "acct-1")
	projection := disclosureProjection(t, registrarBaseAt.Add(time.Hour))

	if _, configured, err := view.AssessDisclosure(t.Context(), customer, projection); err != nil || configured {
		t.Fatalf("登记之前必须答未配置：err=%v configured=%v", err, configured)
	}

	shown, err := domain.ShowDimension(build(t, domain.NewViewContentReference, "content/milestones"))
	if err != nil {
		t.Fatalf("构造展示维：%v", err)
	}
	outcome := registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterDisclosurePolicy(txCtx, tenant, ports.DisclosurePolicyRegistration{
			Header: mappingHeader("disclose/v1", registrarBaseAt),
			Entries: []ports.DisclosurePolicyEntry{{
				Customer:   customer,
				Milestones: shown,
				ETA:        domain.PendDimension(),
				Final:      domain.WithholdDimension(),
				Note:       domain.PendDimension(),
			}},
		})
	})
	if outcome != ports.CatalogVersionRegistered {
		t.Fatalf("登记披露策略应成功，实得 %s", outcome)
	}

	answer, configured, err := view.AssessDisclosure(t.Context(), customer, projection)
	if err != nil || !configured {
		t.Fatalf("登记之后应答已配置：err=%v configured=%v", err, configured)
	}
	assertDimension(t, "里程碑", answer.Milestones, shownDimension(t, "content/milestones"))
	assertDimension(t, "ETA", answer.ETA, pendingDimension())
	assertDimension(t, "终局", answer.Final, withheldDimension())
	assertDimension(t, "说明", answer.Note, pendingDimension())
}

// Covers: 通知策略（`PAR-VIS-07`）的写入方与 `NOTIFICATION_POLICY_NOT_CONFIGURED`
// 哨兵，外加 0019 补上的发布批准责任。时限是相对量：截止时间由库拿披露决定时间加出来，
// 所以这里证的是「登记 2 小时 → 读回决定时间 +2 小时」，不是一个钉死的绝对时刻。
func TestRegisteredNotificationPolicyBecomesReadable(t *testing.T) {
	fixture := newRegistrarFixture(t)
	tenant := registrarTenant(t)
	view, err := adapter.NewNotificationPolicies(fixture.db, tenant)
	if err != nil {
		t.Fatalf("构造通知策略视图：%v", err)
	}
	policy := build(t, domain.NewDisclosurePolicyReference, "disclose/v1")
	decidedAt := registrarBaseAt.Add(6 * time.Hour)
	disclosure, err := domain.DecideDisclosure(
		build(t, domain.NewEpisodeID, "episode-1"),
		build(t, domain.NewCustomerAccountReference, "acct-1"),
		policy,
		domain.DiscloseToCustomer,
		build(t, domain.NewDisclosureContentReference, "content/notice"),
		decidedAt,
	)
	if err != nil {
		t.Fatalf("构造披露决定：%v", err)
	}

	if _, configured, err := view.DirectNotification(t.Context(), disclosure); err != nil || configured {
		t.Fatalf("登记之前必须答未配置：err=%v configured=%v", err, configured)
	}

	registration := ports.NotificationPolicyRegistration{
		Policy:        policy,
		Channel:       build(t, domain.NewNotificationChannelReference, "SMS"),
		DeadlineAfter: 2 * time.Hour,
		Obligation:    build(t, domain.NewDisclosurePolicyReference, "DELIVERED"),
		ApprovedBy:    "customer-service",
	}
	outcome := registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterNotificationPolicy(txCtx, tenant, registration)
	})
	if outcome != ports.CatalogVersionRegistered {
		t.Fatalf("登记通知策略应成功，实得 %s", outcome)
	}

	directive, configured, err := view.DirectNotification(t.Context(), disclosure)
	if err != nil || !configured {
		t.Fatalf("登记之后应答已配置：err=%v configured=%v", err, configured)
	}
	if directive.Channel.String() != "SMS" || directive.Obligation.String() != "DELIVERED" {
		t.Fatalf("渠道与义务判据 = %s / %s", directive.Channel, directive.Obligation)
	}
	if !directive.Deadline.Equal(decidedAt.Add(2 * time.Hour)) {
		t.Fatalf("截止时间应是决定时间 +2h，实得 %s", directive.Deadline)
	}

	// 撞既有行落`已登记`而不是覆盖：换渠道要换披露策略引用（新旧两行并存），覆盖会
	// 把一条已批准的渠道悄悄换掉，而据它发出的历史通知仍然指着这一行。
	registration.Channel = build(t, domain.NewNotificationChannelReference, "EMAIL")
	again := registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterNotificationPolicy(txCtx, tenant, registration)
	})
	if again != ports.CatalogVersionAlreadyRegistered {
		t.Fatalf("重复登记应答已登记，实得 %s", again)
	}
	unchanged, _, err := view.DirectNotification(t.Context(), disclosure)
	if err != nil {
		t.Fatalf("重读通知指令：%v", err)
	}
	if unchanged.Channel.String() != "SMS" {
		t.Fatalf("被拒的重复登记改动了既有行，渠道实得 %s", unchanged.Channel)
	}
}

// Covers: 索赔资格声明与申请人授权目录（`PAR-VIS-08` 的两角，0011 与 0018）的写入方，
// 以及 `ELIGIBILITY_CATALOGUE_NOT_CONFIGURED` 哨兵。声明与覆盖类型同一提交——分两步会
// 出现一段「声明已在、覆盖类型还没写」的窗口，那期间任一索赔都会被判成 ADR-0051 的
// 永久`不予受理`。
func TestRegisteredClaimCataloguesBecomeReadable(t *testing.T) {
	fixture := newRegistrarFixture(t)
	tenant := registrarTenant(t)
	view, err := adapter.NewClaimEligibilityRules(fixture.db, tenant)
	if err != nil {
		t.Fatalf("构造资格视图：%v", err)
	}
	query := ports.EligibilityQuery{
		Batch:     build(t, domain.NewClaimBatchReference, "batch-1"),
		Item:      build(t, domain.NewClaimItemID, "item-1"),
		Customer:  build(t, domain.NewCustomerAccountReference, "acct-1"),
		Contract:  build(t, domain.NewContractScopeReference, "contract/v1"),
		Target:    build(t, domain.NewRequestScopeReference, "parcel-1"),
		Kind:      build(t, domain.NewClaimKindReference, "LOSS"),
		Applicant: build(t, domain.NewApplicantReference, "applicant-1"),
	}

	if _, present, err := view.RulesForClaim(t.Context(), query); err != nil || present {
		t.Fatalf("登记之前声明必须不在场：err=%v present=%v", err, present)
	}

	outcome := registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterClaimEligibility(txCtx, tenant, ports.ClaimEligibilityRegistration{
			Header:   ports.CatalogApprovalHeader{Version: "claim/v1", ApprovedBy: "customer-service"},
			Contract: query.Contract,
			CoveredKinds: []domain.ClaimKindReference{
				query.Kind,
				build(t, domain.NewClaimKindReference, "DAMAGE"),
			},
		})
	})
	if outcome != ports.CatalogVersionRegistered {
		t.Fatalf("登记索赔声明应成功，实得 %s", outcome)
	}

	rules, present, err := view.RulesForClaim(t.Context(), query)
	if err != nil || !present {
		t.Fatalf("登记之后声明应在场：err=%v present=%v", err, present)
	}
	if !rules.KindCovered || rules.RuleVersion != "claim/v1" {
		t.Fatalf("覆盖判定 = %v，版本 %q", rules.KindCovered, rules.RuleVersion)
	}
	// 授权目录还没登记：如实答未登记，不拿空名单冒充「无人获授权」。
	if rules.Authorization.Registered {
		t.Fatal("授权目录尚未登记却答了已登记")
	}

	authorized := registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterClaimAuthorization(txCtx, tenant, ports.ClaimAuthorizationRegistration{
			Header:     ports.CatalogApprovalHeader{Version: "authz/v1", ApprovedBy: "customer-service"},
			Customer:   query.Customer,
			Applicants: []domain.ApplicantReference{query.Applicant},
		})
	})
	if authorized != ports.CatalogVersionRegistered {
		t.Fatalf("登记授权目录应成功，实得 %s", authorized)
	}

	rules, _, err = view.RulesForClaim(t.Context(), query)
	if err != nil {
		t.Fatalf("重读资格规则：%v", err)
	}
	if !rules.Authorization.Registered || rules.Authorization.RuleVersion != "authz/v1" {
		t.Fatalf("授权目录 = %+v", rules.Authorization)
	}
	if len(rules.Authorization.AuthorizedApplicants) != 1 ||
		rules.Authorization.AuthorizedApplicants[0].String() != "applicant-1" {
		t.Fatalf("名单没有把查询申请人答成在列，实得 %+v", rules.Authorization.AuthorizedApplicants)
	}

	// 未登记类型仍答不覆盖：声明在场，「不在集合内」这时才说得通。
	query.Kind = build(t, domain.NewClaimKindReference, "DELAY")
	uncovered, present, err := view.RulesForClaim(t.Context(), query)
	if err != nil || !present {
		t.Fatalf("换类型重读：err=%v present=%v", err, present)
	}
	if uncovered.KindCovered {
		t.Fatal("没登记的索赔类型被答成了承担")
	}
}

// Covers: 目录在场而名单为空是「当前不授权任何人」，与「还没登记」不是一回事——后者
// 由目录行不在场表达（0018）。两者的恢复动作不同：一个改名单，一个先建目录。
func TestEmptyApplicantListRegistersAsAnEmptyRoster(t *testing.T) {
	fixture := newRegistrarFixture(t)
	tenant := registrarTenant(t)
	view, err := adapter.NewClaimEligibilityRules(fixture.db, tenant)
	if err != nil {
		t.Fatalf("构造资格视图：%v", err)
	}
	query := ports.EligibilityQuery{
		Customer:  build(t, domain.NewCustomerAccountReference, "acct-1"),
		Contract:  build(t, domain.NewContractScopeReference, "contract/v1"),
		Kind:      build(t, domain.NewClaimKindReference, "LOSS"),
		Applicant: build(t, domain.NewApplicantReference, "applicant-1"),
	}

	registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterClaimEligibility(txCtx, tenant, ports.ClaimEligibilityRegistration{
			Header:       ports.CatalogApprovalHeader{Version: "claim/v1", ApprovedBy: "customer-service"},
			Contract:     query.Contract,
			CoveredKinds: []domain.ClaimKindReference{query.Kind},
		})
	})
	registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterClaimAuthorization(txCtx, tenant, ports.ClaimAuthorizationRegistration{
			Header:   ports.CatalogApprovalHeader{Version: "authz/v1", ApprovedBy: "customer-service"},
			Customer: query.Customer,
		})
	})

	rules, _, err := view.RulesForClaim(t.Context(), query)
	if err != nil {
		t.Fatalf("读资格规则：%v", err)
	}
	if !rules.Authorization.Registered {
		t.Fatal("空名单的目录仍然是已登记——「还没登记」由目录行不在场表达")
	}
	if len(rules.Authorization.AuthorizedApplicants) != 0 {
		t.Fatalf("空名单不该长出成员，实得 %+v", rules.Authorization.AuthorizedApplicants)
	}
}

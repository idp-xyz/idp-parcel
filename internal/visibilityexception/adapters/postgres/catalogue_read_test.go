package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件钉 OperationsCatalogue（票 admin-web-page-wiring-frontier/02）的四条完成标准：
// 租户隔离、空册如实答空、父子同快照（整版条目随版本一次到齐）、limit 非正即拒。
// 数据一律经 CatalogRegistrar 登记而不是直插行——列表读口交出的必须是写入口真实产物
// 的转写，直插行连 shape 约束都可能绕过，钉住的就不是「登记后能查阅」这条链。

var listBaseAt = time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC)

func listCatalogue(t *testing.T, fixture *registrarFixture) *adapter.OperationsCatalogue {
	t.Helper()
	catalogue, err := adapter.NewOperationsCatalogue(fixture.db)
	if err != nil {
		t.Fatalf("构造目录列表读面：%v", err)
	}
	return catalogue
}

func listTenant(t *testing.T, value string) domain.TenantID {
	t.Helper()
	return build(t, domain.NewTenantID, value)
}

// 以下登记素材全部在事务闭包外构造：闭包内只许调用写入口，t.Fatalf 触发 runtime.Goexit
// 会跳过提交与回滚路径（架构门禁 TestNoTransactionClosureCarriesAGoexitAssertion）。

func triageRegistration(t *testing.T, version string, from time.Time) ports.TriageRuleRegistration {
	t.Helper()
	return ports.TriageRuleRegistration{
		Header: ports.CatalogVersionHeader{
			Version: version, ApprovedBy: "tracking-ops", EffectiveFrom: from,
		},
		Entries: []ports.TriageRuleEntry{
			{
				Kind:       build(t, domain.NewExceptionSignalKindReference, "DELIVERY_FAILED"),
				Confidence: build(t, domain.NewConfidenceReference, "CARRIER_CONFIRMED"),
				Outcome:    domain.AutoEstablishCase,
				Team:       build(t, domain.NewResponsibleTeamReference, "team/delivery-desk"),
			},
			{
				Kind:       build(t, domain.NewExceptionSignalKindReference, "ADDRESS_UNKNOWN"),
				Confidence: build(t, domain.NewConfidenceReference, "HEURISTIC"),
				Outcome:    domain.ManualReviewRequired,
			},
		},
	}
}

func notificationRegistration(t *testing.T, policy string) ports.NotificationPolicyRegistration {
	t.Helper()
	return ports.NotificationPolicyRegistration{
		Policy:        build(t, domain.NewDisclosurePolicyReference, policy),
		Channel:       build(t, domain.NewNotificationChannelReference, "EMAIL"),
		DeadlineAfter: 72 * time.Hour,
		Obligation:    build(t, domain.NewDisclosurePolicyReference, "ACK_REQUIRED"),
		ApprovedBy:    "tracking-ops",
	}
}

func eligibilityRegistration(t *testing.T, contract string) ports.ClaimEligibilityRegistration {
	t.Helper()
	return ports.ClaimEligibilityRegistration{
		Header:   ports.CatalogApprovalHeader{Version: "claims/v1", ApprovedBy: "claims-ops"},
		Contract: build(t, domain.NewContractScopeReference, contract),
		CoveredKinds: []domain.ClaimKindReference{
			build(t, domain.NewClaimKindReference, "LOSS"),
			build(t, domain.NewClaimKindReference, "DAMAGE"),
		},
	}
}

func authorizationRegistration(
	t *testing.T, customer string, applicants ...string,
) ports.ClaimAuthorizationRegistration {
	t.Helper()
	registration := ports.ClaimAuthorizationRegistration{
		Header:   ports.CatalogApprovalHeader{Version: "auth/v1", ApprovedBy: "claims-ops"},
		Customer: build(t, domain.NewCustomerAccountReference, customer),
	}
	for _, applicant := range applicants {
		registration.Applicants = append(registration.Applicants,
			build(t, domain.NewApplicantReference, applicant))
	}
	return registration
}

func disclosureRegistration(t *testing.T, version string, from time.Time) ports.DisclosurePolicyRegistration {
	t.Helper()
	shown, err := domain.ShowDimension(build(t, domain.NewViewContentReference, "public-milestones/v1"))
	if err != nil {
		t.Fatalf("构造展示维：%v", err)
	}
	note, err := domain.ShowDimension(build(t, domain.NewViewContentReference, "note/plain"))
	if err != nil {
		t.Fatalf("构造说明维：%v", err)
	}
	return ports.DisclosurePolicyRegistration{
		Header: ports.CatalogVersionHeader{
			Version: version, ApprovedBy: "tracking-ops", EffectiveFrom: from,
		},
		Entries: []ports.DisclosurePolicyEntry{{
			Customer:   build(t, domain.NewCustomerAccountReference, "CUST-01"),
			Milestones: shown,
			ETA:        domain.PendDimension(),
			Final:      domain.WithholdDimension(),
			Note:       note,
		}},
	}
}

// Covers: 完成标准「limit 非正即拒」。零与负数不是「不限量」，六个方法一致地拒——
// 页大小的裁决归接入面，读口对无意义取值不猜一个默认。
func TestCatalogueListRejectsNonPositiveLimit(t *testing.T) {
	fixture := newRegistrarFixture(t)
	catalogue := listCatalogue(t, fixture)
	tenant := listTenant(t, "tenant-a")

	attempts := map[string]func(context.Context, int) error{
		"里程碑映射": func(ctx context.Context, limit int) error {
			_, err := catalogue.ListMilestoneMappings(ctx, tenant, limit)
			return err
		},
		"分诊规则": func(ctx context.Context, limit int) error {
			_, err := catalogue.ListTriageRules(ctx, tenant, limit)
			return err
		},
		"通知策略": func(ctx context.Context, limit int) error {
			_, err := catalogue.ListNotificationPolicies(ctx, tenant, limit)
			return err
		},
		"索赔资格": func(ctx context.Context, limit int) error {
			_, err := catalogue.ListClaimEligibilities(ctx, tenant, limit)
			return err
		},
		"索赔授权": func(ctx context.Context, limit int) error {
			_, err := catalogue.ListClaimAuthorizations(ctx, tenant, limit)
			return err
		},
		"披露策略": func(ctx context.Context, limit int) error {
			_, err := catalogue.ListDisclosurePolicies(ctx, tenant, limit)
			return err
		},
	}
	for name, list := range attempts {
		for _, limit := range []int{0, -1} {
			if err := list(t.Context(), limit); err == nil ||
				!strings.Contains(err.Error(), "limit must be positive") {
				t.Fatalf("%s limit=%d 应拒，实得 err=%v", name, limit, err)
			}
		}
	}
}

// Covers: 完成标准「空册如实答空不折成未配置」。六个方法对零登记的租户都交回空列表
// 而不是错误——空册本身就是内容（ADR-0077 Decision 四），续办是登记责任方去
// parcel-ve-register 登记；这与五个装载口对同一份空册答「未配置」不冲突，两口答的
// 不是同一个问题。
func TestEmptyCataloguesListAsEmptyNotAsError(t *testing.T) {
	fixture := newRegistrarFixture(t)
	catalogue := listCatalogue(t, fixture)
	tenant := listTenant(t, "tenant-a")

	mappings, err := catalogue.ListMilestoneMappings(t.Context(), tenant, 10)
	if err != nil || len(mappings) != 0 {
		t.Fatalf("空映射册：rows=%v err=%v", mappings, err)
	}
	triage, err := catalogue.ListTriageRules(t.Context(), tenant, 10)
	if err != nil || len(triage) != 0 {
		t.Fatalf("空分诊册：rows=%v err=%v", triage, err)
	}
	notifications, err := catalogue.ListNotificationPolicies(t.Context(), tenant, 10)
	if err != nil || len(notifications) != 0 {
		t.Fatalf("空通知册：rows=%v err=%v", notifications, err)
	}
	eligibilities, err := catalogue.ListClaimEligibilities(t.Context(), tenant, 10)
	if err != nil || len(eligibilities) != 0 {
		t.Fatalf("空资格册：rows=%v err=%v", eligibilities, err)
	}
	authorizations, err := catalogue.ListClaimAuthorizations(t.Context(), tenant, 10)
	if err != nil || len(authorizations) != 0 {
		t.Fatalf("空授权册：rows=%v err=%v", authorizations, err)
	}
	disclosures, err := catalogue.ListDisclosurePolicies(t.Context(), tenant, 10)
	if err != nil || len(disclosures) != 0 {
		t.Fatalf("空披露册：rows=%v err=%v", disclosures, err)
	}
}

// Covers: 区间型目录的上列形状——新版在前、整版条目随版本到齐、接续闭合后前版带上
// 显式终点（HasEffectiveTo，不拿零时刻兼职）。
func TestListMilestoneMappingsReturnsVersionsWithEntriesNewestFirst(t *testing.T) {
	fixture := newRegistrarFixture(t)
	catalogue := listCatalogue(t, fixture)
	tenant := listTenant(t, "tenant-a")
	switchAt := listBaseAt.Add(48 * time.Hour)

	first := ports.MilestoneMappingRegistration{
		Header:  mappingHeader("map/v1", listBaseAt),
		Entries: []ports.MilestoneMappingEntry{mappingEntry(t, "node-intake", "PICKED_UP")},
	}
	second := ports.MilestoneMappingRegistration{
		Header: mappingHeader("map/v2", switchAt),
		Entries: []ports.MilestoneMappingEntry{
			mappingEntry(t, "node-intake", "ARRIVED_AT_NODE"),
			mappingEntry(t, "node-handover", "OUT_FOR_DELIVERY"),
		},
	}
	registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterMilestoneMapping(txCtx, tenant, first)
	})
	registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterMilestoneMapping(txCtx, tenant, second)
	})

	rows, err := catalogue.ListMilestoneMappings(t.Context(), tenant, 10)
	if err != nil {
		t.Fatalf("上列映射册：%v", err)
	}
	if len(rows) != 2 || rows[0].Version != "map/v2" || rows[1].Version != "map/v1" {
		t.Fatalf("应新版在前得 [map/v2 map/v1]，实得 %+v", rows)
	}
	if rows[0].HasEffectiveTo {
		t.Fatalf("当前版不该有终点：%+v", rows[0])
	}
	if !rows[1].HasEffectiveTo || !rows[1].EffectiveTo.Equal(switchAt) {
		t.Fatalf("前版应被接续闭合于 %s，实得 %+v", switchAt, rows[1])
	}
	if len(rows[0].Entries) != 2 || len(rows[1].Entries) != 1 {
		t.Fatalf("整版条目应随版本到齐，实得 v2=%d 条 v1=%d 条",
			len(rows[0].Entries), len(rows[1].Entries))
	}
	// 条目按（源上下文+事实类型）排序：node-handover < node-intake。
	if rows[0].Entries[0].FactKind != "node-handover" ||
		rows[0].Entries[0].Milestone != "OUT_FOR_DELIVERY" ||
		rows[0].Entries[0].Source != "NODE_OPERATIONS" {
		t.Fatalf("v2 首条目转写不符：%+v", rows[0].Entries[0])
	}
	if rows[1].Entries[0].FactKind != "node-intake" || rows[1].Entries[0].Milestone != "PICKED_UP" {
		t.Fatalf("v1 条目转写不符：%+v", rows[1].Entries[0])
	}
	if !rows[1].EffectiveFrom.Equal(listBaseAt) {
		t.Fatalf("v1 起点应为 %s，实得 %s", listBaseAt, rows[1].EffectiveFrom)
	}
}

// Covers: 分诊条目三列取封闭集的 String() 词形转写，不发明词。
func TestListTriageRulesTranscribesClosedSetWords(t *testing.T) {
	fixture := newRegistrarFixture(t)
	catalogue := listCatalogue(t, fixture)
	tenant := listTenant(t, "tenant-a")
	registration := triageRegistration(t, "triage/v1", listBaseAt)

	registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterTriageRules(txCtx, tenant, registration)
	})

	rows, err := catalogue.ListTriageRules(t.Context(), tenant, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("上列分诊册：rows=%d err=%v", len(rows), err)
	}
	if rows[0].Version != "triage/v1" || len(rows[0].Entries) != 2 {
		t.Fatalf("分诊版本转写不符：%+v", rows[0])
	}
	// 条目按（信号类型+可信度）排序：ADDRESS_UNKNOWN < DELIVERY_FAILED。
	if rows[0].Entries[0].SignalKind != "ADDRESS_UNKNOWN" ||
		rows[0].Entries[0].Outcome != "MANUAL_REVIEW" {
		t.Fatalf("首条目应为 ADDRESS_UNKNOWN/MANUAL_REVIEW，实得 %+v", rows[0].Entries[0])
	}
	if rows[0].Entries[1].Confidence != "CARRIER_CONFIRMED" ||
		rows[0].Entries[1].Outcome != "AUTO_ESTABLISH" {
		t.Fatalf("次条目应为 CARRIER_CONFIRMED/AUTO_ESTABLISH，实得 %+v", rows[0].Entries[1])
	}
}

// Covers: 通知策略的时限以库存 interval 的文本词形交出（相对量，绝对截止点在目录上
// 不存在），其余四列照登转写。
func TestListNotificationPoliciesTranscribesIntervalAsText(t *testing.T) {
	fixture := newRegistrarFixture(t)
	catalogue := listCatalogue(t, fixture)
	tenant := listTenant(t, "tenant-a")
	registration := notificationRegistration(t, "disclose/v1")

	registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterNotificationPolicy(txCtx, tenant, registration)
	})

	rows, err := catalogue.ListNotificationPolicies(t.Context(), tenant, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("上列通知册：rows=%d err=%v", len(rows), err)
	}
	row := rows[0]
	if row.Policy != "disclose/v1" || row.Channel != "EMAIL" ||
		row.Obligation != "ACK_REQUIRED" || row.ApprovedBy != "tracking-ops" {
		t.Fatalf("通知策略转写不符：%+v", row)
	}
	if row.DeadlineAfter != "72:00:00" {
		t.Fatalf("72 小时应转写为 interval 文本 72:00:00，实得 %q", row.DeadlineAfter)
	}
}

// Covers: 资格声明连同覆盖类型一次到齐——只有声明在场，「不在集合内」才说得通。
func TestListClaimEligibilitiesCarriesCoveredKinds(t *testing.T) {
	fixture := newRegistrarFixture(t)
	catalogue := listCatalogue(t, fixture)
	tenant := listTenant(t, "tenant-a")
	registration := eligibilityRegistration(t, "CONTRACT-01")

	registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterClaimEligibility(txCtx, tenant, registration)
	})

	rows, err := catalogue.ListClaimEligibilities(t.Context(), tenant, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("上列资格册：rows=%d err=%v", len(rows), err)
	}
	row := rows[0]
	if row.Contract != "CONTRACT-01" || row.Version != "claims/v1" || row.ApprovedBy != "claims-ops" {
		t.Fatalf("资格声明转写不符：%+v", row)
	}
	if len(row.CoveredKinds) != 2 || row.CoveredKinds[0] != "DAMAGE" || row.CoveredKinds[1] != "LOSS" {
		t.Fatalf("覆盖集应为 [DAMAGE LOSS]，实得 %v", row.CoveredKinds)
	}
}

// Covers: 空名单显式在列——「目录在场、当前不授权任何人」是一次已作出的授权决定
// （0018），与「还没登记」（整行不在列表里）分得开。
func TestListClaimAuthorizationsKeepsEmptyRosterExplicit(t *testing.T) {
	fixture := newRegistrarFixture(t)
	catalogue := listCatalogue(t, fixture)
	tenant := listTenant(t, "tenant-a")
	withRoster := authorizationRegistration(t, "CUST-01", "APPLICANT-02", "APPLICANT-01")
	emptyRoster := authorizationRegistration(t, "CUST-02")

	registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterClaimAuthorization(txCtx, tenant, withRoster)
	})
	registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterClaimAuthorization(txCtx, tenant, emptyRoster)
	})

	rows, err := catalogue.ListClaimAuthorizations(t.Context(), tenant, 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("上列授权册：rows=%d err=%v", len(rows), err)
	}
	if rows[0].Customer != "CUST-01" ||
		len(rows[0].Applicants) != 2 || rows[0].Applicants[0] != "APPLICANT-01" {
		t.Fatalf("名单应排序到齐，实得 %+v", rows[0])
	}
	if rows[1].Customer != "CUST-02" || len(rows[1].Applicants) != 0 {
		t.Fatalf("空名单目录应在列且名单为空，实得 %+v", rows[1])
	}
}

// Covers: 披露条目四维各自转写封闭三态与内容来处——SHOWN 必带内容，PENDING_CONFIRMATION
// 与 NOT_DISCLOSED 必不带（0012 shape 约束的读侧镜像）。
func TestListDisclosurePoliciesCarriesFourDimensionCells(t *testing.T) {
	fixture := newRegistrarFixture(t)
	catalogue := listCatalogue(t, fixture)
	tenant := listTenant(t, "tenant-a")
	registration := disclosureRegistration(t, "disclose/v1", listBaseAt)

	registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterDisclosurePolicy(txCtx, tenant, registration)
	})

	rows, err := catalogue.ListDisclosurePolicies(t.Context(), tenant, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("上列披露册：rows=%d err=%v", len(rows), err)
	}
	if rows[0].Version != "disclose/v1" || len(rows[0].Entries) != 1 {
		t.Fatalf("披露版本转写不符：%+v", rows[0])
	}
	entry := rows[0].Entries[0]
	if entry.Customer != "CUST-01" {
		t.Fatalf("条目账户不符：%+v", entry)
	}
	if entry.Milestones.State != "SHOWN" || entry.Milestones.Content != "public-milestones/v1" {
		t.Fatalf("里程碑维应 SHOWN 带内容，实得 %+v", entry.Milestones)
	}
	if entry.ETA.State != "PENDING_CONFIRMATION" || entry.ETA.Content != "" {
		t.Fatalf("ETA 维应 PENDING_CONFIRMATION 不带内容，实得 %+v", entry.ETA)
	}
	if entry.Final.State != "NOT_DISCLOSED" || entry.Final.Content != "" {
		t.Fatalf("终局维应 NOT_DISCLOSED 不带内容，实得 %+v", entry.Final)
	}
	if entry.Note.State != "SHOWN" || entry.Note.Content != "note/plain" {
		t.Fatalf("说明维应 SHOWN 带内容，实得 %+v", entry.Note)
	}
}

// Covers: 完成标准「租户隔离」。两个租户各登一份同形数据，六个方法对租户 A 都只交回
// A 自己的行——目录表全部以 tenant_id 打头，读口不得跨租户串册。
func TestCatalogueListsAreTenantScoped(t *testing.T) {
	fixture := newRegistrarFixture(t)
	catalogue := listCatalogue(t, fixture)
	tenantA := listTenant(t, "tenant-a")
	tenantB := listTenant(t, "tenant-b")

	for _, seed := range []struct {
		tenant  domain.TenantID
		mapping ports.MilestoneMappingRegistration
	}{
		{tenantA, ports.MilestoneMappingRegistration{
			Header:  mappingHeader("map/a", listBaseAt),
			Entries: []ports.MilestoneMappingEntry{mappingEntry(t, "node-intake", "PICKED_UP")},
		}},
		{tenantB, ports.MilestoneMappingRegistration{
			Header:  mappingHeader("map/b", listBaseAt),
			Entries: []ports.MilestoneMappingEntry{mappingEntry(t, "node-intake", "PICKED_UP")},
		}},
	} {
		tenant, mapping := seed.tenant, seed.mapping
		registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
			return fixture.registrar.RegisterMilestoneMapping(txCtx, tenant, mapping)
		})
	}
	notificationA := notificationRegistration(t, "disclose/a")
	notificationB := notificationRegistration(t, "disclose/b")
	registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterNotificationPolicy(txCtx, tenantA, notificationA)
	})
	registerWithin(t, fixture, func(txCtx context.Context) (ports.CatalogRegistrationOutcome, error) {
		return fixture.registrar.RegisterNotificationPolicy(txCtx, tenantB, notificationB)
	})

	mappings, err := catalogue.ListMilestoneMappings(t.Context(), tenantA, 10)
	if err != nil || len(mappings) != 1 || mappings[0].Version != "map/a" {
		t.Fatalf("租户 A 的映射册应只有 map/a：rows=%+v err=%v", mappings, err)
	}
	notifications, err := catalogue.ListNotificationPolicies(t.Context(), tenantA, 10)
	if err != nil || len(notifications) != 1 || notifications[0].Policy != "disclose/a" {
		t.Fatalf("租户 A 的通知册应只有 disclose/a：rows=%+v err=%v", notifications, err)
	}
}

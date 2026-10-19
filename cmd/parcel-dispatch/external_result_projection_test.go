package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	vecc "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/customscompliance"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	veregistration "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证 CC 外部结果已收 → VE 投影这一路走的是生产 wireDispatcher（ADR-0197）：信封
// 定稿 PUBLISHED，结果范围指名的申报单元每个成员各落一份本层事实。里程碑映射直接登
// scripts/demo-seeds 的演示映射文件，证的是种子里关务那一行真命中适配器产出的类型词，
// 而且只命中全部放行。CC 一侧照接收编排的提交形状落库：记录与交接意图同一笔事务。

const (
	deriveExternalResultConsumerName = "visibility-exception/derive-projection-from-external-result"
	externalResultCase               = "SYN-CASE-ER-01"
	externalResultUnit               = "SYN-DECL-UNIT-ER-01"
	externalResultVersion            = "SYN-DECL-VER-ER-01"
	externalResultMemberOne          = "SYN-PARCEL-ER-01"
	externalResultMemberTwo          = "SYN-PARCEL-ER-02"
	demoMappingVersion               = "SYN-VE-MAP-V1"
)

var demoMilestoneMappingSeed = filepath.Join("..", "..", "scripts", "demo-seeds", "data", "visibility", "01-milestone-mapping-v1.json")

// Covers: 放行层形成投影——全部放行一拍定稿，两个成员各得一条本层事实，按演示映射归入
// CUSTOMS_CLEARED；成员维与来源标识进引用、提交版本进版本维。
func TestAFullReleaseExternalResultClassifiesEveryMemberAsClearedUnderTheDemoMapping(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	registerDemoMilestoneMapping(t, fixture)
	saveExternalResultScope(t, fixture)
	eventID := recordReceivedExternalResult(t, fixture, "SYN-RESULT-RELEASE-01",
		ccdomain.ReleaseResultLayer, ccdomain.FullRelease, false)

	published, err := fixture.beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if published != 1 || outboxStatus(t, fixture.db, eventID) != "PUBLISHED" {
		t.Fatalf("外部结果没定稿：published = %d；失败码 = %q", published, recordedFailureCode(t, fixture.db, eventID))
	}
	for _, member := range []string{externalResultMemberOne, externalResultMemberTwo} {
		entry := onlyExternalResultEntry(t, fixture, member)
		if entry.Fact().Kind().String() != "external-result-release-full" {
			t.Fatalf("成员 %s kind = %q, want external-result-release-full", member, entry.Fact().Kind())
		}
		if entry.Fact().Fact().String() != "external-result/"+member+"/SYN-RESULT-RELEASE-01" {
			t.Fatalf("成员 %s fact = %q（成员维与来源标识进引用）", member, entry.Fact().Fact())
		}
		if entry.Fact().Version().String() != externalResultVersion {
			t.Fatalf("成员 %s version = %q, want %s", member, entry.Fact().Version(), externalResultVersion)
		}
		milestone, classified := entry.Milestone()
		if !classified || milestone.String() != "CUSTOMS_CLEARED" {
			t.Fatalf("成员 %s 归类 = %q（%v），want CUSTOMS_CLEARED——演示映射的关务那一行没命中全部放行",
				member, milestone, classified)
		}
		if entry.MappingVersion().String() != demoMappingVersion {
			t.Fatalf("成员 %s mapping = %q, want %s", member, entry.MappingVersion(), demoMappingVersion)
		}
	}
	if n := fixture.countInbox(t, deriveExternalResultConsumerName, eventID); n != 1 {
		t.Fatalf("VE inbox 行数 = %d, want 1", n)
	}
}

// Covers: 非放行层不形成清关——业务受理一拍定稿、形成本层未归类条目；部分放行同样只是
// 本层未归类。演示映射里没有任何一行能把它们映成 CUSTOMS_CLEARED。
func TestNonFullExternalResultsStayUnclassifiedUnderTheDemoMapping(t *testing.T) {
	for name, test := range map[string]struct {
		layer   ccdomain.ResultLayer
		release ccdomain.ReleaseKind
		kind    string
	}{
		"业务受理": {ccdomain.BusinessAcceptanceLayer, ccdomain.ReleaseKindInvalid, "external-result-business-acceptance"},
		"部分放行": {ccdomain.ReleaseResultLayer, ccdomain.PartialRelease, "external-result-release-partial"},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newSYNVerticalFixture(t)
			registerDemoMilestoneMapping(t, fixture)
			saveExternalResultScope(t, fixture)
			eventID := recordReceivedExternalResult(t, fixture, "SYN-RESULT-OTHER-01", test.layer, test.release, false)

			published, err := fixture.beat.DispatchOnce(t.Context())
			if err != nil {
				t.Fatalf("第一拍：%v", err)
			}
			if published != 1 || outboxStatus(t, fixture.db, eventID) != "PUBLISHED" {
				t.Fatalf("外部结果没定稿：published = %d；失败码 = %q", published, recordedFailureCode(t, fixture.db, eventID))
			}
			for _, member := range []string{externalResultMemberOne, externalResultMemberTwo} {
				entry := onlyExternalResultEntry(t, fixture, member)
				if entry.Fact().Kind().String() != test.kind {
					t.Fatalf("成员 %s kind = %q, want %s", member, entry.Fact().Kind(), test.kind)
				}
				if milestone, classified := entry.Milestone(); classified {
					t.Fatalf("成员 %s 归进了 %q——%s 不是全部放行，不得映成清关", member, milestone, name)
				}
				if entry.MappingVersion().String() != demoMappingVersion {
					t.Fatalf("成员 %s mapping = %q, want %s（同一映射版本下如实未归类）",
						member, entry.MappingVersion(), demoMappingVersion)
				}
			}
		})
	}
}

// Covers: 同层冲突的那份入账不派生——信封照样定稿，不留未决、不撞 no_subscriber，但成员
// 上不长出任何投影。
func TestALayerConflictExternalResultPublishesWithoutAProjection(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	registerDemoMilestoneMapping(t, fixture)
	saveExternalResultScope(t, fixture)
	eventID := recordReceivedExternalResult(t, fixture, "SYN-RESULT-CONFLICT-01",
		ccdomain.ReleaseResultLayer, ccdomain.FullRelease, true)

	published, err := fixture.beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if published != 1 || outboxStatus(t, fixture.db, eventID) != "PUBLISHED" {
		t.Fatalf("同层冲突的那份该入账定稿：published = %d；失败码 = %q",
			published, recordedFailureCode(t, fixture.db, eventID))
	}
	projections, err := vepostgres.NewProjections(fixture.db)
	if err != nil {
		t.Fatalf("构造投影读口：%v", err)
	}
	tenant := mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String())
	for _, member := range []string{externalResultMemberOne, externalResultMemberTwo} {
		if _, found, err := projections.FindCurrent(t.Context(), tenant,
			mustVE(t, vedomain.NewTrackedParcelReference, member)); err != nil || found {
			t.Fatalf("成员 %s 有投影（found=%v err=%v）——同层冲突的那份不该派生", member, found, err)
		}
	}
	if n := fixture.countOutboxOfType(t, trackingProjectionDerivedType); n != 0 {
		t.Fatalf("派生信封 = %d, want 0", n)
	}
}

// Covers: 外部结果这一路的失败分格——读不回接收记录、读不回范围单元与派生编排未决落
// dispatch.consumer_undecided，其余保持 dispatch.publish_failed。名单取
// veExternalResultUndecidedSentinels 本身：测试里重列一份会让漏登记的哨兵在测试里绿、
// 在生产里塌成 publish_failed。
func TestExternalResultFailuresLandInTheRightPartition(t *testing.T) {
	tests := []struct {
		name     string
		err      error
		wantCode string
	}{
		{"接收记录读不回", vecc.ErrExternalResultNotVisible, "dispatch.consumer_undecided"},
		{"范围单元读不回", vecc.ErrResultScopeUnitNotVisible, "dispatch.consumer_undecided"},
		{"派生编排未决", vecc.ErrProjectionUndecided, "dispatch.consumer_undecided"},
		{"记录不变量已破", vecc.ErrExternalResultRecordInconsistent, "dispatch.publish_failed"},
		{"引用译不出", vecc.ErrExternalResultUntranslatableAnswer, "dispatch.publish_failed"},
		{"交接待发", vecc.ErrProjectionHandoffPending, "dispatch.publish_failed"},
		{"封闭集合外", veconsume.ErrUnexpectedProjectionOutcome, "dispatch.publish_failed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			beat, db, store := beatWithExternalResultConsumer(t, &stallingConsumer{err: test.err})
			enqueueForBeat(t, db, store, "result-1", veinbox.ExternalResultReceivedEventType, `{}`)

			published, err := beat.DispatchOnce(t.Context())
			if err != nil {
				t.Fatalf("一拍：%v", err)
			}
			if published != 0 {
				t.Fatalf("失败的投递被定稿了 %d 条", published)
			}
			if got := recordedFailureCode(t, db, "result-1"); got != test.wantCode {
				t.Fatalf("failure_code = %q, want %q", got, test.wantCode)
			}
		})
	}
}

func beatWithExternalResultConsumer(t *testing.T, inner dispatch.Consumer) (Beat, *bentopg.DB, *outbox.Store) {
	t.Helper()

	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	routed, err := dispatch.WithUndecidedSentinels(inner, veExternalResultUndecidedSentinels...)
	if err != nil {
		t.Fatalf("包装未决哨兵：%v", err)
	}
	config := dispatch.Config{Limit: 10, LeaseFor: time.Minute, MaxAttempts: 5, RetryAfter: 30 * time.Second}
	publisher, err := dispatch.NewDirectPublisher(
		map[eventing.EventType]dispatch.Consumer{veinbox.ExternalResultReceivedEventType: routed},
		5*time.Second,
		config,
	)
	if err != nil {
		t.Fatalf("直投发布器：%v", err)
	}
	beat, err := dispatch.NewDispatcher(store, store, publisher, systemClock{}, config)
	if err != nil {
		t.Fatalf("派发器：%v", err)
	}
	return beat, db, store
}

// registerDemoMilestoneMapping 把演示种子的里程碑映射文件经生产翻译与登记用例登进库——
// 与 seed.sh 调 parcel-ve-register 走的是同一份翻译与同一个写入方。
func registerDemoMilestoneMapping(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()

	raw, err := os.ReadFile(demoMilestoneMappingSeed)
	if err != nil {
		t.Fatalf("读演示映射种子：%v", err)
	}
	command, err := veregistration.MilestoneMappingFromJSON(raw)
	if err != nil {
		t.Fatalf("翻译演示映射种子：%v", err)
	}
	if command.TenantID.String() != fixture.identity.TenantID().String() {
		t.Fatalf("种子租户 = %q，夹具租户 = %q", command.TenantID, fixture.identity.TenantID())
	}
	registrar, err := vepostgres.NewCatalogRegistrar(fixture.db)
	if err != nil {
		t.Fatalf("构造目录写入方：%v", err)
	}
	catalogs, err := veapplication.NewCatalogRegistration(registrar)
	if err != nil {
		t.Fatalf("构造登记用例：%v", err)
	}
	var result veapplication.RegisterCatalogResult
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var registerErr error
		result, registerErr = catalogs.RegisterMilestoneMapping(txCtx, command)
		return registerErr
	})
	if result.Outcome() != veapplication.CatalogRegistered {
		t.Fatalf("登记演示映射 outcome = %q，拒绝理由 = %q", result.Outcome(), result.RefusalReason())
	}
}

// saveExternalResultScope 站在 CC 侧落结果范围要指名的那个申报单元（及其案件）：双成员。
// 不交案件意图——本文件只看外部结果那一封信。
func saveExternalResultScope(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()

	cases, err := ccpostgres.NewCustomsCases(fixture.db)
	if err != nil {
		t.Fatalf("构造案件库：%v", err)
	}
	units, err := ccpostgres.NewDeclarationUnits(fixture.db)
	if err != nil {
		t.Fatalf("构造申报单元库：%v", err)
	}
	tenant := mustCC(t, ccdomain.NewTenantID, fixture.identity.TenantID().String())
	formedAt := time.Now().UTC().Add(-time.Hour)
	caseID := mustCC(t, ccdomain.NewCustomsCaseID, externalResultCase)
	customsCase, err := ccdomain.EstablishCustomsCase(ccdomain.CustomsCaseSpec{
		ID:           caseID,
		Jurisdiction: mustCC(t, ccdomain.NewRegulatoryJurisdictionReference, "SYN-JURISDICTION-ER"),
		Direction:    ccdomain.ImportManifest,
		Procedure:    mustCC(t, ccdomain.NewCustomsProcedureReference, "SYN-PROCEDURE-ER"),
		Obligation:   mustCC(t, ccdomain.NewObligationScopeReference, "SYN-OBLIGATION-ER"),
		Parcels: []ccdomain.CaseParcelAssociation{
			{Parcel: externalResultMemberOne, Customer: "SYN-CUSTOMER-01", SourceRef: "shipment-request/" + externalResultMemberOne},
			{Parcel: externalResultMemberTwo, Customer: "SYN-CUSTOMER-01", SourceRef: "shipment-request/" + externalResultMemberTwo},
		},
		EstablishedAt: formedAt,
	})
	if err != nil {
		t.Fatalf("建立关务案件：%v", err)
	}
	unit, err := ccdomain.FormDeclarationUnit(
		mustCC(t, ccdomain.NewDeclarationUnitID, externalResultUnit),
		caseID,
		customsCase.Procedure(),
		[]ccdomain.DeclaredParcelReference{
			mustCC(t, ccdomain.NewDeclaredParcelReference, externalResultMemberOne),
			mustCC(t, ccdomain.NewDeclaredParcelReference, externalResultMemberTwo),
		},
	)
	if err != nil {
		t.Fatalf("构造申报单元：%v", err)
	}
	key := ccports.CustomsCaseKey{
		TenantID:     tenant,
		Jurisdiction: customsCase.Jurisdiction(),
		Direction:    customsCase.Direction(),
		Procedure:    customsCase.Procedure(),
		Obligation:   customsCase.Obligation(),
	}
	var caseOutcome ccports.CustomsCaseSaveOutcome
	var unitOutcome ccports.DeclarationUnitSaveOutcome
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var saveErr error
		if caseOutcome, saveErr = cases.Save(txCtx, key, customsCase); saveErr != nil {
			return saveErr
		}
		unitOutcome, saveErr = units.Save(txCtx, tenant, unit, formedAt)
		return saveErr
	})
	if caseOutcome != ccports.CustomsCaseSaved || unitOutcome != ccports.DeclarationUnitSaved {
		t.Fatalf("save outcome = %v / %v, want 已写入", caseOutcome, unitOutcome)
	}
}

// recordReceivedExternalResult 照接收编排的提交形状落一条可归属的接收记录并交意图：领域
// 解释一层结果（放行层另拆放行三件），记录与信封同一笔事务。信封 ID 按生产同一公式重算。
func recordReceivedExternalResult(
	t *testing.T,
	fixture *synVerticalFixture,
	sourceID string,
	layer ccdomain.ResultLayer,
	release ccdomain.ReleaseKind,
	layerConflict bool,
) string {
	t.Helper()

	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	results, err := ccpostgres.NewExternalResults(fixture.db)
	if err != nil {
		t.Fatalf("构造外部结果库：%v", err)
	}
	handoff, err := ccpostgres.NewOutboxExternalResultHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造外部结果交接：%v", err)
	}

	tenant := mustCC(t, ccdomain.NewTenantID, fixture.identity.TenantID().String())
	occurredAt := time.Now().UTC().Add(-10 * time.Minute)
	scope := mustCC(t, ccdomain.NewDecisionScopeReference, externalResultUnit)
	result, err := ccdomain.InterpretExternalResult(ccdomain.ExternalResultSpec{
		Layer:        layer,
		SourceID:     sourceID,
		Role:         mustCC(t, ccdomain.NewSourceAuthorityRole, "SYN-REGULATOR"),
		RawSemantics: "SYN-RAW/" + layer.String(),
		Rule:         mustCC(t, ccdomain.NewInterpretationRuleReference, "SYN-RULE-ER-01"),
		Version:      mustCC(t, ccdomain.NewSubmissionVersionID, externalResultVersion),
		Attempt:      1,
		Scope:        scope,
		OccurredAt:   occurredAt,
		ReceivedAt:   occurredAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("解释外部结果：%v", err)
	}
	record := ccports.ExternalResultRecord{
		Key:           ccports.ExternalResultKey{TenantID: tenant, SourceID: sourceID},
		ContentDigest: "SYN-ER-DIGEST/" + sourceID,
		Result:        result,
		LayerConflict: layerConflict,
		RecordedAt:    occurredAt.Add(2 * time.Minute),
	}
	if layer == ccdomain.ReleaseResultLayer {
		outcome, err := ccdomain.ReceiveReleaseOutcome(release,
			mustCC(t, ccdomain.NewRegulatoryAuthorityReference, "SYN-AUTHORITY-ER"),
			scope, "", occurredAt.Add(time.Minute))
		if err != nil {
			t.Fatalf("构造放行事实：%v", err)
		}
		record.Release = &outcome
	}

	var saved ccports.ExternalResultSaveOutcome
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var saveErr error
		if saved, saveErr = results.Save(txCtx, record); saveErr != nil {
			return saveErr
		}
		return handoff.HandOffExternalResult(txCtx, ccports.ExternalResultHandoffIntent{Record: record})
	})
	if saved != ccports.ExternalResultSaved {
		t.Fatalf("save outcome = %v, want 已写入", saved)
	}
	return string(outboxintent.FingerprintEventID("external-result", tenant.String(), sourceID))
}

func onlyExternalResultEntry(t *testing.T, fixture *synVerticalFixture, member string) vedomain.MilestoneClassification {
	t.Helper()

	projections, err := vepostgres.NewProjections(fixture.db)
	if err != nil {
		t.Fatalf("构造投影读口：%v", err)
	}
	projection, found, err := projections.FindCurrent(t.Context(),
		mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String()),
		mustVE(t, vedomain.NewTrackedParcelReference, member))
	if err != nil {
		t.Fatalf("读当前投影：%v", err)
	}
	if !found {
		t.Fatalf("成员 %s 没有当前投影——逐成员展开没走到它", member)
	}
	entries := projection.Entries()
	if len(entries) != 1 {
		t.Fatalf("成员 %s entries = %d, want 1", member, len(entries))
	}
	return entries[0]
}

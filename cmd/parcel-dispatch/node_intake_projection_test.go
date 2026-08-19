package main

import (
	"testing"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件证 CONS-PROJ-B：节点收寄形成信封经 FanOut 先投 VE 投影、再投 PS 采用。
// 映射目录未配置必须未归类入账（MAPPING_NOT_CONFIGURED），不得折成视图不可用；
// PS 仍停在资格未证明。两本 inbox 分账。不种 PAR-VIS-01，不登记客户视图事件。

const (
	deriveProjectionConsumerName  = "visibility-exception/derive-projection-from-node-intake"
	trackingProjectionDerivedType = "visibility-exception.tracking-projection.derived"
	nodeIntakeFactVersion         = "SYN-INTAKE-V1"
)

// Covers: 生产 wireDispatcher 一拍——VE 投影未归类入账，PS 资格未证明让整封 Publish
// 失败。FanOut 先 VE 后 PS：投影不得堵在 PAR-COM-16 资格墙上。
func TestAFormedNodeIntakeDerivesAnUnclassifiedProjectionAndStopsAdoption(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()

	fixture.submit(t, ctx)
	fixture.recordPassingJudgments(t, ctx)
	if result := fixture.formDecision(t, ctx); result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED；pending = %q", result.State(), result.PendingReason())
	}

	seedSYNPCEligibility(t, fixture)
	eventID := recordFormedNodeIntake(t, fixture)
	assertAdoptionPreconditions(t, fixture)
	assertIntakeEligibilityUnproven(t, fixture, psdomain.NodeIntakeSource)

	published, err := fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("PS 未决却定稿了 %d 条；收寄信封失败码 = %q",
			published, recordedFailureCode(t, fixture.db, eventID))
	}
	if got := recordedFailureCode(t, fixture.db, eventID); got != "dispatch.consumer_undecided" {
		t.Fatalf("failure_code = %q, want dispatch.consumer_undecided（PS 资格未证明经 FanOut 让 Publish 失败；VE 成功不得把整封改成定稿）", got)
	}

	assertUnclassifiedNodeIntakeProjection(t, fixture)
	if n := fixture.countInbox(t, deriveProjectionConsumerName, eventID); n != 1 {
		t.Fatalf("VE inbox 行数 = %d, want 1", n)
	}
	assertNoAdoptionTrace(t, fixture, eventID)

	if n := fixture.countOutboxOfType(t, trackingProjectionDerivedType); n != 1 {
		t.Fatalf("投影交接信封 = %d, want 1——派生成功必须入队，且不得为测试去登记消费者", n)
	}

	versionAfterFirst := currentProjectionVersion(t, fixture)

	published, err = fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("重拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("重拍定稿了 %d 条", published)
	}
	if n := fixture.countInbox(t, deriveProjectionConsumerName, eventID); n != 1 {
		t.Fatalf("重拍后 VE inbox = %d, want 1——不得翻倍", n)
	}
	assertNoAdoptionTrace(t, fixture, eventID)
	if n := fixture.countOutboxOfType(t, nodeIntakeFormedType); n != 1 {
		t.Fatalf("收寄信封变成 %d 封——重投不得再入队一份", n)
	}
	if n := fixture.countOutboxOfType(t, trackingProjectionDerivedType); n != 1 {
		t.Fatalf("投影交接信封变成 %d 封", n)
	}
	if currentProjectionVersion(t, fixture) != versionAfterFirst {
		t.Fatal("重投又长了一版投影——已有结果路径必须复用当前版")
	}
	for _, id := range fixture.outboxIDsOfType(t, trackingProjectionDerivedType) {
		if got := recordedFailureCode(t, fixture.db, id); got != "" && got != "dispatch.no_subscriber" {
			t.Fatalf("投影交接 %s 失败码 = %q, want 空或 dispatch.no_subscriber", id, got)
		}
	}
}

func assertUnclassifiedNodeIntakeProjection(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()

	tenant := mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String())
	parcel := mustVE(t, vedomain.NewTrackedParcelReference, nodeIntakeAssociation)
	factRef := mustVE(t, vedomain.NewSourceFactReference, nodeIntakeSourceID)
	version := mustVE(t, vedomain.NewSourceFactVersion, nodeIntakeFactVersion)

	facts, err := vepostgres.NewAcceptedFacts(fixture.db)
	if err != nil {
		t.Fatalf("构造已接受事实读口：%v", err)
	}
	record, found, err := facts.FindByKey(t.Context(), veports.FactKey{
		Tenant:  tenant,
		Source:  vedomain.SourceNodeOperations,
		Fact:    factRef,
		Version: version,
	})
	if err != nil {
		t.Fatalf("读已接受事实：%v", err)
	}
	if !found {
		t.Fatal("映射未配置却没有事实行——未归类也必须入账")
	}
	if record.Fact.Source() != vedomain.SourceNodeOperations ||
		record.Fact.Parcel().String() != nodeIntakeAssociation ||
		record.Fact.Fact().String() != nodeIntakeSourceID ||
		record.Fact.Kind().String() != "node-intake" {
		t.Fatalf("事实维 source=%s parcel=%s fact=%s kind=%s",
			record.Fact.Source(), record.Fact.Parcel(), record.Fact.Fact(), record.Fact.Kind())
	}

	projections, err := vepostgres.NewProjections(fixture.db)
	if err != nil {
		t.Fatalf("构造投影读口：%v", err)
	}
	projection, found, err := projections.FindCurrent(t.Context(), tenant, parcel)
	if err != nil {
		t.Fatalf("读当前投影：%v", err)
	}
	if !found {
		t.Fatal("没有当前投影")
	}
	if len(projection.Entries()) != 1 {
		t.Fatalf("entries = %d, want 1", len(projection.Entries()))
	}
	entry := projection.Entries()[0]
	if _, classified := entry.Milestone(); classified {
		t.Fatal("映射未配置却归了类——禁止发明里程碑")
	}
	if entry.MappingVersion().String() != "MAPPING_NOT_CONFIGURED" {
		t.Fatalf("mapping = %q, want MAPPING_NOT_CONFIGURED", entry.MappingVersion())
	}
	if entry.Fact().Source() != vedomain.SourceNodeOperations ||
		entry.Fact().Parcel().String() != nodeIntakeAssociation ||
		entry.Fact().Fact().String() != nodeIntakeSourceID {
		t.Fatalf("条目事实维 source=%s parcel=%s fact=%s",
			entry.Fact().Source(), entry.Fact().Parcel(), entry.Fact().Fact())
	}
}

func currentProjectionVersion(t *testing.T, fixture *synVerticalFixture) string {
	t.Helper()
	tenant := mustVE(t, vedomain.NewTenantID, fixture.identity.TenantID().String())
	parcel := mustVE(t, vedomain.NewTrackedParcelReference, nodeIntakeAssociation)
	projections, err := vepostgres.NewProjections(fixture.db)
	if err != nil {
		t.Fatalf("构造投影读口：%v", err)
	}
	projection, found, err := projections.FindCurrent(t.Context(), tenant, parcel)
	if err != nil {
		t.Fatalf("读当前投影：%v", err)
	}
	if !found {
		t.Fatal("没有当前投影")
	}
	return projection.Version().String()
}

func mustVE[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件对真实 PostgreSQL 16 证采用那条链的诚实停点：真的已接受委托、真的节点收寄
// 记录、生产 wireDispatcher 一拍，停在`资格判断未决`（ELIGIBILITY_NOT_ESTABLISHED）。
//
// 已接受委托只能由 PS 应用编排形成（提交 → 判断齐 → 形成决定），因此夹具复用 SYN-V0
// 那一套：本包里没有第二条能造出真已接受行的路。手搓一份 ACCEPTED 快照塞库会绕开
// ADR-0061 的互证，测到的就不是重建门真的开着。

const (
	adoptNodeIntakeConsumerName = "parcel-shipment/adopt-node-intake"
	// 两个意图类型。消费方与提供方各写各的名字，测试跟着自己写一遍；psinbox 认的
	// 那个串与 NO 发出的这个串是否对得上，由路由表用例钉住，这里只用来数信封。
	nodeIntakeFormedType      = "node-operations.node-intake.formed"
	networkIntakeRecordedType = "parcel-shipment.network-intake.recorded"
	nodeIntakeSourceID        = "SYN-RECEPTION-01"
	// 收寄的版本化包裹关联必须落在已接受基线的成员集合里，否则采用会以
	// PARCEL_OUTSIDE_ACCEPTANCE_BASELINE 不采用——那是另一格，不是本用例要停的地方。
	nodeIntakeAssociation = "SYN-PARCEL-01"
)

// Covers: SYN-PC-SEED 的诚实停点——节点收寄形成信封经生产路由表投到 PS 采用消费者，
// 收寄读得回、目标委托反查得到、重建门开到已接受、闭包回指规则包、资格声明已配置，
// 整链一直走到硬资格未证明才停。
//
// 停点必须是 dispatch.consumer_undecided：声明列出硬资格，取证缝属实例半边，
// JudgeIntakeEligibility 答 NOT_ESTABLISHED（INTAKE_QUALIFICATION_UNPROVEN/第一项）。
// 空清单会直接 ESTABLISHED 并形成承诺；只靠失败码分不出 UNCONFIGURED 与本格，拍前用
// 真读口与生产资格适配器另证。同一拍会投接受信封：种子已嵌 NetworkServiceForm，
// 那封停在 ROUTE_EVIDENCE_NOT_CONFIGURED，不得形成路由计划；published==0 挡住误入账，
// assertNoAdoptionTrace 另数 initial_route 与 formed 信封。
//
// 未决不得留痕：inbox 无账、采用无行、下游意图不入队。重拍不得翻倍。
func TestAFormedNodeIntakeStopsAtUnprovenIntakeEligibility(t *testing.T) {
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
	assertSYNPCEligibilitySeeded(t, fixture)
	assertIntakeEligibilityUnproven(t, fixture, psdomain.NodeIntakeSource)

	published, err := fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("资格未成立却定稿了 %d 条；收寄信封失败码 = %q",
			published, recordedFailureCode(t, fixture.db, eventID))
	}
	if got := recordedFailureCode(t, fixture.db, eventID); got != "dispatch.consumer_undecided" {
		t.Fatalf("failure_code = %q, want dispatch.consumer_undecided（资格未证明，不是重建门也不是翻译）", got)
	}
	assertNoAdoptionTrace(t, fixture, eventID)

	// 同一份收寄信封再拍一次：inbox 无账，派发会再投；三样痕迹仍不得长出来，也不得翻倍。
	published, err = fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("重拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("重拍定稿了 %d 条", published)
	}
	if n := fixture.countOutboxOfType(t, nodeIntakeFormedType); n != 1 {
		t.Fatalf("收寄信封变成 %d 封——重投不得再入队一份", n)
	}
	assertNoAdoptionTrace(t, fixture, eventID)
}

// assertAdoptionPreconditions 把「停点不在前三步」钉住。
//
// 四个未决哨兵合用 dispatch.consumer_undecided 一个失败码，库里读不出是哪一个——单看
// 失败码，一份读不回来的收寄或一次落空的反查会与资格未成立长得一模一样。这里用真实
// 读口分别证掉收寄可见、关联已识别、反查恰命中这份委托；资格那一格由
// assertSYNPCEligibilitySeeded / assertIntakeEligibilityUnproven 另证。
func assertAdoptionPreconditions(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()

	receptions, err := nopostgres.NewReceptions(fixture.db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	tenant := mustNO(t, nodomain.NewTenantID, fixture.identity.TenantID().String())
	record, found, err := receptions.FindByKey(t.Context(),
		noports.ReceptionKey{TenantID: tenant, SourceID: nodeIntakeSourceID})
	if err != nil {
		t.Fatalf("读回收寄：%v", err)
	}
	if !found || record.Kind != noports.RecordIntakeFormed {
		t.Fatalf("收寄 found = %v kind = %v，本用例要停在资格而不是收寄可见性", found, record.Kind)
	}
	if _, identified := record.Intake.Association(); !identified {
		t.Fatal("收寄没有版本化包裹关联，会停在实物未识别而不是资格")
	}

	targets, err := pspostgres.NewShipmentRequests(fixture.db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	target, found, err := targets.FindCurrentAcceptedByParcel(t.Context(),
		fixture.identity.TenantID(), mustPS(t, psdomain.NewDeclaredParcelID, nodeIntakeAssociation))
	if err != nil {
		t.Fatalf("按包裹反查当前已接受委托：%v", err)
	}
	if !found || target.ShipmentRequestID() != fixture.requestID {
		t.Fatalf("反查 found = %v 委托 = %q，本用例要停在资格而不是目标缺席",
			found, target.ShipmentRequestID())
	}
}

func assertNoAdoptionTrace(t *testing.T, fixture *synVerticalFixture, eventID string) {
	t.Helper()

	if n := fixture.countInbox(t, adoptNodeIntakeConsumerName, eventID); n != 0 {
		t.Fatalf("inbox 行数 = %d, want 0——未决必须回滚，不能冒充已处理", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.intake_adoption`); n != 0 {
		t.Fatalf("intake_adoption 行数 = %d, want 0——资格未成立不得形成承诺或不采用", n)
	}
	if n := fixture.countOutboxOfType(t, networkIntakeRecordedType); n != 0 {
		t.Fatalf("发出了 %d 封 %s，会堵无订阅者分区", n, networkIntakeRecordedType)
	}
	fixture.assertNoInitialRoute(t)
}

// recordFormedNodeIntake 落一份形成格收寄判断并在同一事务交出发布意图，交回信封 ID。
//
// 落库与入队同生共死是 NO 侧的既有纪律（Receptions.Save 走 RequireExecutor）；分两个
// 事务写会造出「记录在、意图永久缺」的半截状态，而那正是消费方永远等不到的那一格。
func recordFormedNodeIntake(t *testing.T, fixture *synVerticalFixture) string {
	t.Helper()

	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	receptions, err := nopostgres.NewReceptions(fixture.db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	handoff, err := nopostgres.NewOutboxNodeIntakeHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造收寄交接：%v", err)
	}

	// 租户在两个上下文各有自己的类型，串在信封上传递；这里从 PS 身份取同一个串，
	// 免得夹具的租户与委托的租户各说各话。
	tenant := mustNO(t, nodomain.NewTenantID, fixture.identity.TenantID().String())
	receivedAt := time.Now().UTC().Add(-2 * time.Minute)
	intake, err := nodomain.FormNodeIntake(nodomain.NodeIntakeSpec{
		TenantID:    tenant,
		Unit:        mustNO(t, nodomain.NewHandlingUnitID, "SYN-UNIT-01"),
		Node:        mustNO(t, nodomain.NewNodeReference, "SYN-NODE-01"),
		DeliveredBy: mustNO(t, nodomain.NewDeliveringPartyReference, "SYN-COURIER-01"),
		Evidence:    mustNO(t, nodomain.NewReceptionEvidenceReference, "evidence/SYN-RECEIPT-01"),
		Version:     mustNO(t, nodomain.NewIntakeResultVersion, "SYN-INTAKE-V1"),
		Association: mustNO(t, nodomain.NewParcelAssociationReference, nodeIntakeAssociation),
		ReceivedAt:  receivedAt,
	})
	if err != nil {
		t.Fatalf("形成节点收寄：%v", err)
	}
	control, err := nodomain.EstablishPhysicalControl(nodomain.PhysicalControlSpec{
		TenantID:      tenant,
		Unit:          mustNO(t, nodomain.NewHandlingUnitID, "SYN-UNIT-01"),
		Node:          mustNO(t, nodomain.NewNodeReference, "SYN-NODE-01"),
		Kind:          nodomain.EstablishedByNodeIntake,
		Basis:         mustNO(t, nodomain.NewControlBasisReference, "SYN-INTAKE-V1"),
		EstablishedAt: receivedAt,
	})
	if err != nil {
		t.Fatalf("成立实物控制：%v", err)
	}

	record := noports.ReceptionRecord{
		Key:           noports.ReceptionKey{TenantID: tenant, SourceID: nodeIntakeSourceID},
		ContentDigest: "SYN-RECEPTION-DIGEST-01",
		Kind:          noports.RecordIntakeFormed,
		Intake:        intake,
		Control:       control,
		RecordedAt:    receivedAt.Add(time.Second),
	}
	var outcome noports.ReceptionSaveOutcome
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var saveErr error
		if outcome, saveErr = receptions.Save(txCtx, record); saveErr != nil {
			return saveErr
		}
		return handoff.HandOffNodeIntake(txCtx, noports.NodeIntakeHandoffIntent{Record: record})
	})
	if outcome != noports.ReceptionSaved {
		t.Fatalf("save outcome = %v, want 已写入", outcome)
	}
	return tenant.String() + "/" + nodeIntakeSourceID
}

// mustNO 是 mustPS 在 node-operations 值对象上的孪生。两份分开是为了让失败信息说清
// 立不起来的是哪一侧的值。
func mustNO[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

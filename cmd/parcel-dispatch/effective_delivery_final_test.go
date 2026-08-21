package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证终局那条链的诚实停点：真的已接受委托、真的有效交付
// 登记、生产 wireDispatcher 一拍，停在`终局规则未配置`（FINAL_RULE_UNCONFIGURED）。
//
// 已接受委托只能由 PS 应用编排形成，因此夹具复用 SYN-V0。手搓一份 ACCEPTED 快照塞库
// 会绕开 ADR-0061 的互证。交付登记走 TF 的 store + handoff，不走 HTTP、不走
// RegisterEffectiveDeliveryHandler。

const (
	formFinalFromDeliveryConsumerName = "parcel-shipment/form-final-from-delivery"
	effectiveDeliveryRegisteredType   = "transport-fulfillment.effective-delivery.registered"
	finalOutcomeFormedType            = "parcel-shipment.final-outcome.formed"
	effectiveDeliveryObject           = "SYN-PARCEL-01"
	effectiveDeliveryAttempt          = "SYN-DELIVERY-ATTEMPT-01"
	effectiveDeliveryVersion          = "SYN-DELIVERY-V1"
)

// Covers: SYN-PC-SEED 之后的诚实停点——有效交付登记信封经生产路由表投到 PS 终局
// 消费者，登记读得回、目标委托反查唯一命中、重建门开到已接受、闭包回指规则包，整链
// 一直走到终局声明未配置才停。
//
// 停点必须是 dispatch.consumer_undecided：SYN-PC-SEED 不种 PAR-COM-17 终局行，
// LoadFinalRule found=false，JudgeFinalOutcome 答未配置。种终局行会让本链越过本格，
// 那是实例半边，本用例禁止。同一拍会投接受信封：种子已嵌 NetworkServiceForm，那封
// 停在 ROUTE_EVIDENCE_NOT_CONFIGURED，不得形成路由计划；published==0 与
// assertNoInitialRoute 一起挡住。
//
// FanOut 先把同一封投给 VE：映射未配置时投影未归类入账，并入队
// tracking-projection.derived（由客户视图链接住，重拍时定稿）。本用例不停投影；PS 终局
// 未配置仍让整封 Publish 失败，第一拍 published 必须是 0。重拍得 1 而不是 0：交付信封
// 与派生信封自 ADR-0074 起不同分区，未决的交付不再把已派生的可见性堵在队头。
func TestARegisteredEffectiveDeliveryStopsAtUnconfiguredFinalRule(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()

	fixture.submit(t, ctx)
	fixture.recordPassingJudgments(t, ctx)
	if result := fixture.formDecision(t, ctx); result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED；pending = %q", result.State(), result.PendingReason())
	}

	seedSYNPCEligibility(t, fixture)
	eventID := recordRegisteredEffectiveDelivery(t, fixture)
	assertEffectiveDeliveryFinalPreconditions(t, fixture)
	assertSYNPCEligibilitySeeded(t, fixture)
	assertFinalRuleUnconfigured(t, fixture)

	published, err := fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("终局规则未配置却定稿了 %d 条；交付信封失败码 = %q",
			published, recordedFailureCode(t, fixture.db, eventID))
	}
	if got := recordedFailureCode(t, fixture.db, eventID); got != "dispatch.consumer_undecided" {
		t.Fatalf("failure_code = %q, want dispatch.consumer_undecided（规则未配置，不是可见性也不是目标缺席）", got)
	}
	assertNoFinalOutcomeTrace(t, fixture, eventID)

	// 重拍：交付信封仍未决，第一拍入队的派生信封被客户视图链接住并定稿（1）。
	published, err = fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("重拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("重拍定稿 %d 条, want 1（仅派生信封经视图链定稿）", published)
	}
	if n := fixture.countOutboxOfType(t, effectiveDeliveryRegisteredType); n != 1 {
		t.Fatalf("交付登记信封变成 %d 封——重投不得再入队一份", n)
	}
	assertNoFinalOutcomeTrace(t, fixture, eventID)
}

// assertEffectiveDeliveryFinalPreconditions 把「停点不在前两步」钉住。
//
// 三个未决哨兵合用 dispatch.consumer_undecided 一个失败码，库里读不出是哪一个——单看
// 失败码，一份读不回来的登记或一次落空的反查会与规则未配置长得一模一样。这里用真实
// 读口分别证掉登记可见、反查恰命中这份委托；规则那一格由 assertFinalRuleUnconfigured
// 另证。
func assertEffectiveDeliveryFinalPreconditions(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()

	deliveries, err := tfpostgres.NewEffectiveDeliveries(fixture.db)
	if err != nil {
		t.Fatalf("构造有效交付库：%v", err)
	}
	tenant := mustTF(t, tfdomain.NewTenantID, fixture.identity.TenantID().String())
	object := mustTF(t, tfdomain.NewCarriedObjectReference, effectiveDeliveryObject)
	attempt := mustTF(t, tfdomain.NewAttemptReference, effectiveDeliveryAttempt)
	record, found, err := deliveries.FindByKey(t.Context(), tfports.EffectiveDeliveryKey{
		TenantID: tenant,
		Object:   object,
		Attempt:  attempt,
	})
	if err != nil {
		t.Fatalf("读回有效交付：%v", err)
	}
	if !found {
		t.Fatal("有效交付读不回来，本用例要停在规则未配置而不是可见性")
	}
	if record.Delivery.Object() != object {
		t.Fatalf("登记对象 = %q, want %q", record.Delivery.Object(), object)
	}
	if record.Delivery.Version().String() != effectiveDeliveryVersion {
		t.Fatalf("当前版 = %q, want %s——不得按事件 ID 去读已翻旧的行",
			record.Delivery.Version(), effectiveDeliveryVersion)
	}

	targets, err := pspostgres.NewShipmentRequests(fixture.db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	target, found, err := targets.FindCurrentAcceptedByParcel(t.Context(),
		fixture.identity.TenantID(), mustPS(t, psdomain.NewDeclaredParcelID, effectiveDeliveryObject))
	if err != nil {
		t.Fatalf("按包裹反查当前已接受委托：%v", err)
	}
	if !found || target.ShipmentRequestID() != fixture.requestID {
		t.Fatalf("反查 found = %v 委托 = %q，本用例要停在规则未配置而不是目标缺席",
			found, target.ShipmentRequestID())
	}
}

// assertFinalRuleUnconfigured 用真读口钉死：闭包回指的规则包上没有终局声明父行。
func assertFinalRuleUnconfigured(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	tenant := mustPC(t, pcdomain.NewTenantID, fixture.identity.TenantID().String())

	resolutions, err := pcpostgres.NewCommercialResolutions(fixture.db)
	if err != nil {
		t.Fatalf("构造解析库：%v", err)
	}
	closure, found, err := resolutions.LoadResolution(t.Context(), tenant, mustPC(t, pcdomain.NewResolutionID, synPCResolutionID))
	if err != nil || !found {
		t.Fatalf("LoadResolution found = %v err = %v", found, err)
	}
	adopted, ok := closure.AdoptedFor(pcdomain.AcceptanceRulePackageObject)
	if !ok {
		t.Fatal("闭包没采用接单规则包——owner 找不到会把停点提前成规则包未固定")
	}

	declarations, err := pcpostgres.NewStageContentDeclarations(fixture.db)
	if err != nil {
		t.Fatalf("构造阶段内容读口：%v", err)
	}
	_, found, err = declarations.LoadFinalRule(t.Context(), tenant, adopted.Version())
	if err != nil {
		t.Fatalf("LoadFinalRule：%v", err)
	}
	if found {
		t.Fatal("终局声明已配置——种子不得 INSERT final_rule_*")
	}
}

func assertNoFinalOutcomeTrace(t *testing.T, fixture *synVerticalFixture, eventID string) {
	t.Helper()

	if n := fixture.countInbox(t, formFinalFromDeliveryConsumerName, eventID); n != 0 {
		t.Fatalf("inbox 行数 = %d, want 0——未决必须回滚，不能冒充已处理", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.final_outcome`); n != 0 {
		t.Fatalf("final_outcome 行数 = %d, want 0——规则未配置不得形成终局或不采用", n)
	}
	if n := fixture.countOutboxOfType(t, finalOutcomeFormedType); n != 0 {
		t.Fatalf("发出了 %d 封 %s，会堵无订阅者分区", n, finalOutcomeFormedType)
	}
	fixture.assertNoInitialRoute(t)
}

// recordRegisteredEffectiveDelivery 落一份有效交付并在同一事务交出发布意图，交回信封 ID。
//
// 落库与入队同生共死是 TF 侧的既有纪律。不走 RegisterEffectiveDeliveryHandler：那条路
// 要尝试视图与身份工厂，本用例要证的是消费侧停点，不是登记编排。对象必须是已接受成员
// SYN-PARCEL-01，否则停在目标缺席而不是规则未配置。
func recordRegisteredEffectiveDelivery(t *testing.T, fixture *synVerticalFixture) string {
	t.Helper()

	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	deliveries, err := tfpostgres.NewEffectiveDeliveries(fixture.db)
	if err != nil {
		t.Fatalf("构造有效交付库：%v", err)
	}
	handoff, err := tfpostgres.NewOutboxEffectiveDeliveryHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造有效交付交接：%v", err)
	}

	tenant := mustTF(t, tfdomain.NewTenantID, fixture.identity.TenantID().String())
	object := mustTF(t, tfdomain.NewCarriedObjectReference, effectiveDeliveryObject)
	attempt := mustTF(t, tfdomain.NewAttemptReference, effectiveDeliveryAttempt)
	occurredAt := time.Now().UTC().Add(-2 * time.Minute)
	delivery, err := tfdomain.RehydrateEffectiveDelivery(tfdomain.RehydrateEffectiveDeliverySpec{
		TenantID:   tenant,
		Object:     object,
		Attempt:    attempt,
		Place:      mustTF(t, tfdomain.NewAttemptPlaceReference, "SYN-DOOR-01"),
		Method:     mustTF(t, tfdomain.NewDeliveryMethodReference, "SYN-METHOD-01"),
		Recipient:  mustTF(t, tfdomain.NewReceivingPartyReference, "SYN-RECIPIENT-01"),
		Proof:      mustTF(t, tfdomain.NewDeliveryProofReference, "SYN-POD-01"),
		Version:    mustTF(t, tfdomain.NewDeliveryResultVersion, effectiveDeliveryVersion),
		OccurredAt: occurredAt,
	})
	if err != nil {
		t.Fatalf("重建有效交付：%v", err)
	}

	record := tfports.EffectiveDeliveryRecord{
		Key: tfports.EffectiveDeliveryKey{
			TenantID: tenant,
			Object:   object,
			Attempt:  attempt,
		},
		ContentDigest: "SYN-DELIVERY-DIGEST-01",
		Delivery:      delivery,
		RecordedAt:    occurredAt.Add(time.Second),
	}
	var outcome tfports.DeliverySaveOutcome
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var saveErr error
		if outcome, saveErr = deliveries.Save(txCtx, record); saveErr != nil {
			return saveErr
		}
		return handoff.HandOffEffectiveDelivery(txCtx, tfports.EffectiveDeliveryHandoffIntent{Record: record})
	})
	if outcome != tfports.DeliverySaved {
		t.Fatalf("save outcome = %v, want 已写入", outcome)
	}
	return tenant.String() + "/" + effectiveDeliveryObject + "/" + effectiveDeliveryAttempt +
		"/" + effectiveDeliveryVersion + "/effective-delivery"
}

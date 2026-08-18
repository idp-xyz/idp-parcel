package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证揽收采用那条链的诚实停点：真的已接受委托、真的对象级
// 揽收登记、生产 wireDispatcher 一拍，停在`资格判断未决`（ELIGIBILITY_UNCONFIGURED）。
//
// 已接受委托只能由 PS 应用编排形成，因此夹具复用 SYN-V0。手搓一份 ACCEPTED 快照塞库
// 会绕开 ADR-0061 的互证。对象级登记走 TF 的 registry + registration handoff，不走
// 尝试级 `offsite-pickup.formed`。

const (
	adoptOffsitePickupConsumerName = "parcel-shipment/adopt-offsite-pickup"
	offsitePickupRegisteredType    = "transport-fulfillment.offsite-pickup.registered"
	offsitePickupFormedType        = "transport-fulfillment.offsite-pickup.formed"
	offsitePickupObject            = "SYN-PARCEL-01"
	offsitePickupAttempt           = "SYN-ATTEMPT-01"
)

// Covers: CONS-PICKUP-B 的诚实停点——对象级揽收登记信封经生产路由表投到 PS 采用消费
// 者，登记读得回、目标委托反查唯一命中、重建门开到已接受，整链一直走到商业资格那一
// 步才停。
//
// 停点必须是 dispatch.consumer_undecided：装配从已接受快照回指闭包（ADR-0062），但夹具
// ResolutionID=`SYN-RES-01` 并未写入 PC 解析库，LoadResolution found=false，资格视图答
// 未配置，编排保持`资格判断未决`。不要为了让本用例变绿去种规则包。重建门与翻译失败都
// 不在哨兵名单里，会落 dispatch.publish_failed。
//
// 未决不得留痕：inbox 无账、采用无行、下游意图不入队。默认一份规则包就能让这三样都
// 长出来，而那等于替租户宣布这批收寄按哪套资格判断。重拍不得翻倍。
func TestARegisteredOffsitePickupStopsAtUnconfiguredIntakeEligibility(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()

	fixture.submit(t, ctx)
	fixture.recordPassingJudgments(t, ctx)
	if result := fixture.formDecision(t, ctx); result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED；pending = %q", result.State(), result.PendingReason())
	}

	eventID := recordRegisteredOffsitePickup(t, fixture)
	assertPickupAdoptionPreconditions(t, fixture)

	published, err := fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("资格未配置却定稿了 %d 条；揽收信封失败码 = %q",
			published, recordedFailureCode(t, fixture.db, eventID))
	}
	if got := recordedFailureCode(t, fixture.db, eventID); got != "dispatch.consumer_undecided" {
		t.Fatalf("failure_code = %q, want dispatch.consumer_undecided（资格未配置，不是重建门也不是翻译）", got)
	}
	assertNoPickupAdoptionTrace(t, fixture, eventID)

	published, err = fixture.beat.DispatchOnce(ctx)
	if err != nil {
		t.Fatalf("重拍：%v", err)
	}
	if published != 0 {
		t.Fatalf("重拍定稿了 %d 条", published)
	}
	if n := fixture.countOutboxOfType(t, offsitePickupRegisteredType); n != 1 {
		t.Fatalf("揽收登记信封变成 %d 封——重投不得再入队一份", n)
	}
	if n := fixture.countOutboxOfType(t, offsitePickupFormedType); n != 0 {
		t.Fatalf("发出了 %d 封尝试级 formed——本链不得走那条", n)
	}
	assertNoPickupAdoptionTrace(t, fixture, eventID)
}

// assertPickupAdoptionPreconditions 把「停点不在前两步」钉住。
//
// 三个未决哨兵合用 dispatch.consumer_undecided 一个失败码，库里读不出是哪一个——单看
// 失败码，一份读不回来的登记或一次落空的反查会与资格未配置长得一模一样。这里用真实
// 读口分别证掉登记可见、反查恰命中这份委托，剩下能到达的未决就只有资格那一格。
func assertPickupAdoptionPreconditions(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()

	registrations, err := tfpostgres.NewOffsitePickupRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("构造揽收登记库：%v", err)
	}
	tenant := mustTF(t, tfdomain.NewTenantID, fixture.identity.TenantID().String())
	object := mustTF(t, tfdomain.NewCarriedObjectReference, offsitePickupObject)
	attempt := mustTF(t, tfdomain.NewAttemptReference, offsitePickupAttempt)
	record, found, err := registrations.FindByKey(t.Context(), tfports.OffsitePickupKey{
		TenantID: tenant,
		Object:   object,
		Attempt:  attempt,
	})
	if err != nil {
		t.Fatalf("读回揽收登记：%v", err)
	}
	if !found {
		t.Fatal("揽收登记读不回来，本用例要停在资格而不是可见性")
	}
	if record.Pickup.Object() != object {
		t.Fatalf("登记对象 = %q, want %q", record.Pickup.Object(), object)
	}

	targets, err := pspostgres.NewShipmentRequests(fixture.db)
	if err != nil {
		t.Fatalf("构造委托仓储：%v", err)
	}
	target, found, err := targets.FindCurrentAcceptedByParcel(t.Context(),
		fixture.identity.TenantID(), mustPS(t, psdomain.NewDeclaredParcelID, offsitePickupObject))
	if err != nil {
		t.Fatalf("按包裹反查当前已接受委托：%v", err)
	}
	if !found || target.ShipmentRequestID() != fixture.requestID {
		t.Fatalf("反查 found = %v 委托 = %q，本用例要停在资格而不是目标缺席",
			found, target.ShipmentRequestID())
	}
}

func assertNoPickupAdoptionTrace(t *testing.T, fixture *synVerticalFixture, eventID string) {
	t.Helper()

	if n := fixture.countInbox(t, adoptOffsitePickupConsumerName, eventID); n != 0 {
		t.Fatalf("inbox 行数 = %d, want 0——未决必须回滚，不能冒充已处理", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.intake_adoption`); n != 0 {
		t.Fatalf("intake_adoption 行数 = %d, want 0——资格未配置不得形成承诺或不采用", n)
	}
	if n := fixture.countOutboxOfType(t, networkIntakeRecordedType); n != 0 {
		t.Fatalf("发出了 %d 封 %s，会堵无订阅者分区", n, networkIntakeRecordedType)
	}
}

// recordRegisteredOffsitePickup 落一份对象级揽收登记并在同一事务交出发布意图，交回信封 ID。
//
// 落库与入队同生共死是 TF 侧的既有纪律。分两个事务写会造出「记录在、意图永久缺」的半
// 截状态，而那正是消费方永远等不到的那一格。不走 formed：那一封信带一批对象。
func recordRegisteredOffsitePickup(t *testing.T, fixture *synVerticalFixture) string {
	t.Helper()

	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	registrations, err := tfpostgres.NewOffsitePickupRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("构造揽收登记库：%v", err)
	}
	handoff, err := tfpostgres.NewOutboxOffsitePickupRegistrationHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造揽收登记交接：%v", err)
	}

	tenant := mustTF(t, tfdomain.NewTenantID, fixture.identity.TenantID().String())
	occurredAt := time.Now().UTC().Add(-2 * time.Minute)
	pickup, err := tfdomain.FormOffsitePickup(tfdomain.OffsitePickupSpec{
		TenantID:   tenant,
		Object:     mustTF(t, tfdomain.NewCarriedObjectReference, offsitePickupObject),
		Task:       mustTF(t, tfdomain.NewPickupTaskReference, "SYN-PICKUP-TASK-01"),
		Attempt:    mustTF(t, tfdomain.NewAttemptReference, offsitePickupAttempt),
		Place:      mustTF(t, tfdomain.NewPickupPlaceReference, "SYN-PLACE-01"),
		Control:    mustTF(t, tfdomain.NewTransportControlReference, "SYN-CONTROL-01"),
		ExecutedBy: mustTF(t, tfdomain.NewExecutingPartyReference, "SYN-COURIER-01"),
		Version:    mustTF(t, tfdomain.NewPickupResultVersion, "SYN-PICKUP-V1"),
		OccurredAt: occurredAt,
	})
	if err != nil {
		t.Fatalf("形成对象级揽收：%v", err)
	}

	record := tfports.OffsitePickupRecord{
		Key: tfports.OffsitePickupKey{
			TenantID: pickup.TenantID(),
			Object:   pickup.Object(),
			Attempt:  pickup.Attempt(),
		},
		ContentDigest: "SYN-PICKUP-DIGEST-01",
		Pickup:        pickup,
		RecordedAt:    occurredAt.Add(time.Second),
	}
	var outcome tfports.OffsitePickupSaveOutcome
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var saveErr error
		if outcome, saveErr = registrations.Save(txCtx, record); saveErr != nil {
			return saveErr
		}
		return handoff.HandOffOffsitePickupRegistration(txCtx, tfports.OffsitePickupRegistrationIntent{Record: record})
	})
	if outcome != tfports.OffsitePickupSaved {
		t.Fatalf("save outcome = %v, want 已写入", outcome)
	}
	return tenant.String() + "/" + offsitePickupObject + "/" + offsitePickupAttempt + "/offsite-pickup-registration"
}

func mustTF[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

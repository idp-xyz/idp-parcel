package main

import (
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-bento-go/eventing"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/labelfinal"
	pstf "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/transportfulfillment"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
)

// 本文件对真实 PostgreSQL 证场外揽收采用、有效交付采用与面单终局判断（经首次有效收寄）三路经生产消费者构造认集运单元：载运对象是
// 节点作业册上的一袋，信封入账、不采用、不判终局、不落未决，也不留 PS 的采用或终局记录；对象串长得像袋号却不在册上，按包裹反查委托，
// 反查不中仍是各路的未决；单元册读不动落 dispatch.consumer_undecided，答复不一致落 dispatch.publish_failed。测试输入是隔离合成，
// 只记 `S`。

const (
	carriedBag                  = "SYN-BAG-CODE79"
	carriedBagLikeParcel        = "SYN-BAG-NOT-IN-BOOK-CODE79"
	judgeLabelOnCarrierInboxKey = "parcel-shipment/judge-label-final-from-carrier-first-effective-pickup"
)

// carriedRoute 是一路：build 是这一路的生产消费者构造，save 在运输履约的册上落一份对象为 object 的登记并交回那一封信的载荷。
type carriedRoute struct {
	name      string
	event     eventing.EventType
	inbox     string
	sentinels []error
	notFound  error
	build     func(*bentopg.DB, *outbox.Store, *inbox.Store, systemClock) (dispatch.Consumer, error)
	save      func(t *testing.T, fixture *synVerticalFixture, object string) string
}

func carriedRoutes() []carriedRoute {
	return []carriedRoute{
		{
			name: "场外揽收", event: psinbox.OffsitePickupRegisteredEventType, inbox: adoptOffsitePickupConsumerName,
			sentinels: offsitePickupUndecidedSentinels, notFound: pstf.ErrParcelTargetNotFound,
			build: adoptOffsitePickupConsumer,
			save: func(t *testing.T, fixture *synVerticalFixture, object string) string {
				saveControlFactPickup(t, fixture, object)
				return `{"tenantId":"` + fixture.identity.TenantID().String() + `","object":"` + object +
					`","attempt":"SYN-ATTEMPT-CODE75","pickupVersion":"SYN-PICKUP-CODE75-V1"}`
			},
		},
		{
			name: "有效交付", event: psinbox.EffectiveDeliveryRegisteredEventType, inbox: formFinalFromDeliveryConsumerName,
			sentinels: effectiveDeliveryUndecidedSentinels, notFound: pstf.ErrParcelTargetNotFound,
			build: adoptEffectiveDeliveryConsumer,
			save: func(t *testing.T, fixture *synVerticalFixture, object string) string {
				payload, _ := saveControlFactDelivery(t, fixture, object)
				return payload
			},
		},
		{
			name: "首次有效收寄", event: psinbox.CarrierFirstEffectivePickupRegisteredEventType, inbox: judgeLabelOnCarrierInboxKey,
			sentinels: carrierFirstEffectivePickupJudgmentUndecidedSentinels, notFound: labelfinal.ErrParcelTargetNotFound,
			build: judgeLabelFinalOnCarrierFirstEffectivePickupConsumer,
			save: func(t *testing.T, fixture *synVerticalFixture, object string) string {
				recordFormedCarrierPickup(t, fixture, object, controlFactOccurredAt)
				return `{"tenantId":"` + fixture.identity.TenantID().String() + `","fact":"SYN-CFEP-01","version":"SYN-CFEV-01","object":"` +
					object + `"}`
			},
		},
	}
}

// consumeCarried 落一份登记，再以 id 把那一封信交给这一路的生产消费者，交回消费结果。
func consumeCarried(t *testing.T, fixture *synVerticalFixture, route carriedRoute, object, id string) error {
	t.Helper()
	payload := route.save(t, fixture, object)
	outboxStore, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	inboxStore, err := inbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	consumer, err := route.build(fixture.db, outboxStore, inboxStore, systemClock{})
	if err != nil {
		t.Fatalf("装配%s那一路：%v", route.name, err)
	}
	return consumer.Consume(t.Context(), redeliveryEnvelope(fixture.identity.TenantID().String(), id, route.event, object, payload))
}

// assertNoPSTrace 钉住 PS 此刻的样子：没有采用记录、没有终局记录，也没有交下游的采用或终局意图。
func assertNoPSTrace(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.intake_adoption`); n != 0 {
		t.Fatalf("intake_adoption 行数 = %d, want 0——单元不是包裹，不采用", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.final_outcome`); n != 0 {
		t.Fatalf("final_outcome 行数 = %d, want 0——单元不形成终局", n)
	}
	for _, eventType := range []string{networkIntakeRecordedType, finalOutcomeFormedType} {
		if n := fixture.countOutboxOfType(t, eventType); n != 0 {
			t.Fatalf("发出了 %d 封 %s", n, eventType)
		}
	}
}

// undecidedBy 说 err 是不是落在这一路的未决名单里——dispatch.WithUndecidedSentinels 判 consumer_undecided 用的就是这个判据。
func undecidedBy(sentinels []error, err error) bool {
	for _, sentinel := range sentinels {
		if errors.Is(err, sentinel) {
			return true
		}
	}
	return false
}

// Covers: AT-PS-156、AT-PS-157、AT-PS-158 真库半边（证据 S）——三路各经生产消费者构造：对象是节点作业册上的一袋，信封入账（返回
// nil、inbox 一行），不采用、不判终局，不落未决，PS 没有采用或终局记录，也没有交下游的意图。袋没有委托，所以这一封不依赖任何
// 已接受委托；另有一格：一件已接受委托声明的包裹号与单元册上的一袋同串时按单元认（ADR-0214 决定二），即使它有已接受委托也不采用。
func TestAConsolidationUnitCarriedByTheSourceFactIsAccountedForWithoutAdoptionOrFinal(t *testing.T) {
	for _, route := range carriedRoutes() {
		t.Run(route.name+"/一袋", func(t *testing.T) {
			fixture := newSYNVerticalFixture(t)
			seedConsolidationUnit(t, fixture, carriedBag)

			if err := consumeCarried(t, fixture, route, carriedBag, "code79-bag"); err != nil {
				t.Fatalf("袋的信封没有入账：%v", err)
			}
			if n := fixture.countInbox(t, route.inbox, "code79-bag"); n != 1 {
				t.Fatalf("inbox 行数 = %d, want 1", n)
			}
			assertNoPSTrace(t, fixture)
		})

		t.Run(route.name+"/与已接受委托的包裹同串", func(t *testing.T) {
			fixture := acceptedSYNFixture(t)
			seedSYNPCEligibility(t, fixture)
			seedConsolidationUnit(t, fixture, "SYN-PARCEL-01")

			if err := consumeCarried(t, fixture, route, "SYN-PARCEL-01", "code79-same-string"); err != nil {
				t.Fatalf("同串按单元认，这一封应入账：%v", err)
			}
			if n := fixture.countInbox(t, route.inbox, "code79-same-string"); n != 1 {
				t.Fatalf("inbox 行数 = %d, want 1", n)
			}
			assertNoPSTrace(t, fixture)
		})
	}
}

// Covers: AT-PS-156、AT-PS-157、AT-PS-158 反面的真库半边（证据 S）——对象串长得像袋号却不在节点作业单元册上（册上另有一袋）：
// 按包裹反查委托，反查不中仍落各路的 ErrParcelTargetNotFound，落在这一路的未决名单里；信封不入账（回滚待重投）。
// 是不是单元只看单元册，不按串形或前缀猜。
func TestABagLikeObjectOutsideTheUnitBookIsStillLookedUpAndStaysUndecided(t *testing.T) {
	for _, route := range carriedRoutes() {
		t.Run(route.name, func(t *testing.T) {
			fixture := newSYNVerticalFixture(t)
			seedConsolidationUnit(t, fixture, carriedBag)

			err := consumeCarried(t, fixture, route, carriedBagLikeParcel, "code79-bag-like")
			if !errors.Is(err, route.notFound) {
				t.Fatalf("err = %v, want 反查不中的哨兵 %v", err, route.notFound)
			}
			if !undecidedBy(route.sentinels, err) {
				t.Fatalf("反查不中不在这一路的未决名单里：%v", err)
			}
			if n := fixture.countInbox(t, route.inbox, "code79-bag-like"); n != 0 {
				t.Fatalf("inbox 行数 = %d, want 0——未决必须回滚", n)
			}
			assertNoPSTrace(t, fixture)
		})
	}
}

// Covers: AT-PS-159 派发半边——三路问节点作业载运对象是不是单元：读不动落 dispatch.consumer_undecided，答复不一致保持
// dispatch.publish_failed。名单取各路的哨兵变量本身：测试里重列一份会让漏登记的哨兵在测试里绿、在生产里塌成 publish_failed。
func TestCarriedConsolidationUnitQuestionFailuresLandInTheRightPartition(t *testing.T) {
	for _, route := range carriedRoutes() {
		for _, test := range []struct {
			name     string
			err      error
			wantCode string
		}{
			{"单元册读不动", pstf.ErrConsolidationUnitNotVisible, "dispatch.consumer_undecided"},
			{"单元册答复不一致", pstf.ErrConsolidationUnitAnswerInconsistent, "dispatch.publish_failed"},
		} {
			t.Run(route.name+"/"+test.name, func(t *testing.T) {
				beat, db, store := beatWithSentinelledConsumer(t, &stallingConsumer{err: test.err}, route.event, route.sentinels)
				enqueueForBeat(t, db, store, "carried-1", route.event, `{}`)

				published, err := beat.DispatchOnce(t.Context())
				if err != nil {
					t.Fatalf("一拍：%v", err)
				}
				if published != 0 {
					t.Fatalf("失败的投递被定稿了 %d 条", published)
				}
				if got := recordedFailureCode(t, db, "carried-1"); got != test.wantCode {
					t.Fatalf("failure_code = %q, want %q", got, test.wantCode)
				}
			})
		}
	}
}

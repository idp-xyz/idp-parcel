package main

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-bento-go/eventing"

	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	vetf "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
)

// 本文件对真实 PostgreSQL 证交接、场外揽收、有效交付与外部承运轨迹各一路经生产装配的消费者认集运单元：对象是节点作业册上的
// 一袋，入账不派生，袋不立事实也不立投影；对象是包裹照旧派生。测试输入是隔离合成，只记 `S`。

const (
	controlFactBag                  = "SYN-BAG-CODE75"
	controlFactParcel               = "SYN-PARCEL-CODE75"
	deriveExternalTrackingInboxName = "visibility-exception/derive-projection-from-external-carrier-tracking"
)

var controlFactOccurredAt = time.Date(2026, 10, 21, 10, 0, 0, 0, time.UTC)

// seedConsolidationUnit 在节点作业的单元册上立一袋。这几路只问它是不是单元、不认成员，所以不播成员与收寄。
func seedConsolidationUnit(t *testing.T, fixture *synVerticalFixture, unit string) {
	t.Helper()
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO node_operations.consolidation_unit (tenant_id, unit_id, asset_ref, phase, members, snapshots, opened_source)
		 VALUES ($1, $2, 'SYN-ASSET-' || $2, 'OPEN', '[]'::jsonb, '[]'::jsonb,
		         jsonb_build_object('sourceId', 'SYN-OPEN-' || $2, 'performedBy', 'SYN-PACKER', 'evidence', 'SYN-WORK',
		                            'occurredAt', $3::timestamptz))`,
		fixture.identity.TenantID().String(), unit, controlFactOccurredAt.Add(-time.Hour)); err != nil {
		t.Fatalf("播种单元：%v", err)
	}
}

// controlFactRoute 是一路：save 在运输履约的册上落一份对象为 object 的登记，交回那一封信的载荷与这份事实在全程追踪的事实引用；
// build 是这一路的生产消费者构造。
type controlFactRoute struct {
	name  string
	event eventing.EventType
	inbox string
	build func(*bentopg.DB, *inbox.Store, *tenantBoundProjectionDerive) (dispatch.Consumer, error)
	save  func(t *testing.T, fixture *synVerticalFixture, object string) (payload, factRef string)
}

func controlFactRoutes() []controlFactRoute {
	return []controlFactRoute{
		{name: "交接", event: veinbox.TransportHandoverRegisteredEventType, inbox: deriveHandoverConsumerName,
			build: deriveHandoverConsumer, save: saveControlFactHandover},
		{name: "场外揽收", event: veinbox.OffsitePickupRegisteredEventType, inbox: derivePickupConsumerName,
			build: derivePickupConsumer, save: saveControlFactPickup},
		{name: "有效交付", event: veinbox.EffectiveDeliveryRegisteredEventType, inbox: deriveDeliveryConsumerName,
			build: deriveDeliveryConsumer, save: saveControlFactDelivery},
		{name: "外部承运轨迹", event: veinbox.ExternalCarrierTrackingJudgedEventType, inbox: deriveExternalTrackingInboxName,
			build: deriveExternalTrackingConsumer, save: saveControlFactExternalTracking},
	}
}

// consumeControlFact 落一份登记，再以 id 把那一封信交给这一路的生产消费者；交回这份事实在全程追踪的事实引用。
func consumeControlFact(t *testing.T, fixture *synVerticalFixture, route controlFactRoute, object, id string) string {
	t.Helper()
	payload, factRef := route.save(t, fixture, object)
	derive, inboxStore := redeliveryParts(t, fixture)
	consumer, err := route.build(fixture.db, inboxStore, derive)
	if err != nil {
		t.Fatalf("装配%s那一路：%v", route.name, err)
	}
	if err := consumer.Consume(t.Context(), redeliveryEnvelope(fixture.identity.TenantID().String(), id, route.event, object, payload)); err != nil {
		t.Fatalf("%s那一封：%v", route.name, err)
	}
	if n := fixture.countInbox(t, route.inbox, id); n != 1 {
		t.Fatalf("%s inbox 行数 = %d, want 1", route.name, n)
	}
	return factRef
}

// Covers: AT-VE-199 真库半边（证据 S）——各路经生产装配：对象是节点作业册上的一袋，那一封信入账（inbox 一行），袋不立已接受
// 事实也不立追踪投影；对象是不在单元册上的包裹，照旧派生一份事实、一个条目。
func TestAControlFactOnABagIsAccountedForWithoutATrackingProjection(t *testing.T) {
	for _, route := range controlFactRoutes() {
		t.Run(route.name, func(t *testing.T) {
			fixture := newSYNVerticalFixture(t)
			seedConsolidationUnit(t, fixture, controlFactBag)

			bagFact := consumeControlFact(t, fixture, route, controlFactBag, "code75-bag")
			if n := acceptedFactVersions(t, fixture, bagFact); n != 0 {
				t.Fatalf("袋被当成一件追踪包裹立了 %d 份事实", n)
			}
			if entries := projectionEntries(t, fixture, controlFactBag); len(entries) != 0 {
				t.Fatalf("袋立了追踪投影，条目 %d 个", len(entries))
			}

			parcelFact := consumeControlFact(t, fixture, route, controlFactParcel, "code75-parcel")
			if n := acceptedFactVersions(t, fixture, parcelFact); n != 1 {
				t.Fatalf("包裹上的事实 %d 份, want 1", n)
			}
			if entries := projectionEntries(t, fixture, controlFactParcel); len(entries) != 1 {
				t.Fatalf("包裹条目 %d 个, want 1", len(entries))
			}
		})
	}
}

// Covers: AT-VE-200 派发半边——各路问节点作业是不是单元读不动落 dispatch.consumer_undecided，答复不一致保持
// dispatch.publish_failed。名单取各路的哨兵变量本身，理由同移动事实那一份。
func TestControlFactUnitQuestionFailuresLandInTheRightPartition(t *testing.T) {
	lanes := []struct {
		name      string
		event     eventing.EventType
		sentinels []error
	}{
		{"交接", veinbox.TransportHandoverRegisteredEventType, veHandoverUndecidedSentinels},
		{"场外揽收", veinbox.OffsitePickupRegisteredEventType, vePickupUndecidedSentinels},
		{"有效交付", veinbox.EffectiveDeliveryRegisteredEventType, veDeliveryUndecidedSentinels},
		{"外部承运轨迹", veinbox.ExternalCarrierTrackingJudgedEventType, veExternalTrackingUndecidedSentinels},
	}
	for _, lane := range lanes {
		for _, test := range []struct {
			name     string
			err      error
			wantCode string
		}{
			{"集运单元读不动", vetf.ErrConsolidationMembershipNotVisible, "dispatch.consumer_undecided"},
			{"集运单元答复不一致", vetf.ErrConsolidationMembershipInconsistent, "dispatch.publish_failed"},
		} {
			t.Run(lane.name+"/"+test.name, func(t *testing.T) {
				beat, db, store := beatWithSentinelledConsumer(t, &stallingConsumer{err: test.err}, lane.event, lane.sentinels)
				enqueueForBeat(t, db, store, "control-fact-1", lane.event, `{}`)

				published, err := beat.DispatchOnce(t.Context())
				if err != nil {
					t.Fatalf("一拍：%v", err)
				}
				if published != 0 {
					t.Fatalf("失败的投递被定稿了 %d 条", published)
				}
				if got := recordedFailureCode(t, db, "control-fact-1"); got != test.wantCode {
					t.Fatalf("failure_code = %q, want %q", got, test.wantCode)
				}
			})
		}
	}
}

func saveControlFactHandover(t *testing.T, fixture *synVerticalFixture, object string) (string, string) {
	t.Helper()
	handovers, err := tfpostgres.NewTransportHandovers(fixture.db)
	if err != nil {
		t.Fatalf("构造交接登记库：%v", err)
	}
	tenant := mustTF(t, tfdomain.NewTenantID, fixture.identity.TenantID().String())
	handover, err := tfdomain.FormTransportHandover(tfdomain.TransportHandoverSpec{
		TenantID:          tenant,
		Object:            mustTF(t, tfdomain.NewCarriedObjectReference, object),
		Scope:             mustTF(t, tfdomain.NewHandoverScopeReference, "SYN-SCOPE-CODE75"),
		ReleasedBy:        mustTF(t, tfdomain.NewHandoverPartyReference, "SYN-NODE-01"),
		ReceivedBy:        mustTF(t, tfdomain.NewHandoverPartyReference, "SYN-CARRIER-01"),
		Verdict:           tfdomain.ObjectHandedOver,
		ReleasingEvidence: mustTF(t, tfdomain.NewHandoverEvidenceReference, "evidence/SYN-RELEASE-CODE75"),
		ReceivingEvidence: mustTF(t, tfdomain.NewHandoverEvidenceReference, "evidence/SYN-RECEIVE-CODE75"),
		Rule:              mustTF(t, tfdomain.NewHandoverRuleReference, "SYN-HANDOVER-RULE/v1"),
		Version:           mustTF(t, tfdomain.NewHandoverResultVersion, "SYN-HANDOVER-CODE75-V1"),
		JudgedAt:          controlFactOccurredAt,
	})
	if err != nil {
		t.Fatalf("形成交接：%v", err)
	}
	// 照登记编排的样子带上`已交接`必带的交出门禁读数（ADR-0213 决定四）：没声明交出节点，是不问关务就定得出的那一份。
	gateReading, _ := handover.UnaskedGateReading()
	if handover, err = handover.WithGateReading(gateReading); err != nil {
		t.Fatalf("挂交出门禁读数：%v", err)
	}
	record := tfports.TransportHandoverRecord{
		Key: tfports.TransportHandoverKey{
			TenantID: handover.TenantID(), Object: handover.Object(), Scope: handover.Scope(), Version: handover.Version(),
		},
		ContentDigest: "SYN-HANDOVER-DIGEST-CODE75-" + object,
		Handover:      handover,
		RecordedAt:    controlFactOccurredAt.Add(time.Second),
	}
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		outcome, err := handovers.Save(txCtx, record)
		if err == nil && outcome != tfports.HandoverSaved {
			t.Fatalf("交接 outcome = %v", outcome)
		}
		return err
	})
	return `{"tenantId":"` + tenant.String() + `","object":"` + object + `","scope":"SYN-SCOPE-CODE75","version":"SYN-HANDOVER-CODE75-V1"}`,
		"transport-handover/" + object + "/SYN-SCOPE-CODE75"
}

func saveControlFactPickup(t *testing.T, fixture *synVerticalFixture, object string) (string, string) {
	t.Helper()
	registrations, err := tfpostgres.NewOffsitePickupRegistrations(fixture.db)
	if err != nil {
		t.Fatalf("构造揽收登记库：%v", err)
	}
	tenant := mustTF(t, tfdomain.NewTenantID, fixture.identity.TenantID().String())
	pickup, err := tfdomain.FormOffsitePickup(tfdomain.OffsitePickupSpec{
		TenantID:   tenant,
		Object:     mustTF(t, tfdomain.NewCarriedObjectReference, object),
		Task:       mustTF(t, tfdomain.NewPickupTaskReference, "SYN-PICKUP-TASK-CODE75"),
		Attempt:    mustTF(t, tfdomain.NewAttemptReference, "SYN-ATTEMPT-CODE75"),
		Place:      mustTF(t, tfdomain.NewPickupPlaceReference, "SYN-PLACE-CODE75"),
		Control:    mustTF(t, tfdomain.NewTransportControlReference, "SYN-CONTROL-CODE75"),
		ExecutedBy: mustTF(t, tfdomain.NewExecutingPartyReference, "SYN-COURIER-CODE75"),
		Version:    mustTF(t, tfdomain.NewPickupResultVersion, "SYN-PICKUP-CODE75-V1"),
		OccurredAt: controlFactOccurredAt,
	})
	if err != nil {
		t.Fatalf("形成对象级揽收：%v", err)
	}
	record := tfports.OffsitePickupRecord{
		Key:           tfports.OffsitePickupKey{TenantID: pickup.TenantID(), Object: pickup.Object(), Attempt: pickup.Attempt()},
		ContentDigest: "SYN-PICKUP-DIGEST-CODE75-" + object,
		Pickup:        pickup,
		RecordedAt:    controlFactOccurredAt.Add(time.Second),
	}
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		outcome, err := registrations.Save(txCtx, record)
		if err == nil && outcome != tfports.OffsitePickupSaved {
			t.Fatalf("揽收 outcome = %v", outcome)
		}
		return err
	})
	return `{"tenantId":"` + tenant.String() + `","object":"` + object + `","attempt":"SYN-ATTEMPT-CODE75"}`,
		"offsite-pickup/" + object + "/SYN-ATTEMPT-CODE75"
}

func saveControlFactDelivery(t *testing.T, fixture *synVerticalFixture, object string) (string, string) {
	t.Helper()
	deliveries, err := tfpostgres.NewEffectiveDeliveries(fixture.db)
	if err != nil {
		t.Fatalf("构造有效交付库：%v", err)
	}
	tenant := mustTF(t, tfdomain.NewTenantID, fixture.identity.TenantID().String())
	carried := mustTF(t, tfdomain.NewCarriedObjectReference, object)
	attempt := mustTF(t, tfdomain.NewAttemptReference, "SYN-ATTEMPT-CODE75")
	gateReading, err := tfdomain.FormDeliveryGateReading(tfdomain.DeliveryGateReadingSpec{Stop: tfdomain.DeliveryNoActiveSegment})
	if err != nil {
		t.Fatalf("交付门禁读数：%v", err)
	}
	delivery, err := tfdomain.RehydrateEffectiveDelivery(tfdomain.RehydrateEffectiveDeliverySpec{
		TenantID:    tenant,
		Object:      carried,
		Attempt:     attempt,
		Place:       mustTF(t, tfdomain.NewAttemptPlaceReference, "SYN-DOOR-CODE75"),
		Method:      mustTF(t, tfdomain.NewDeliveryMethodReference, "SYN-METHOD-01"),
		Recipient:   mustTF(t, tfdomain.NewReceivingPartyReference, "SYN-RECIPIENT-01"),
		Proof:       mustTF(t, tfdomain.NewDeliveryProofReference, "SYN-POD-CODE75"),
		Version:     mustTF(t, tfdomain.NewDeliveryResultVersion, "SYN-DELIVERY-CODE75-V1"),
		OccurredAt:  controlFactOccurredAt,
		GateReading: gateReading,
	})
	if err != nil {
		t.Fatalf("重建有效交付：%v", err)
	}
	record := tfports.EffectiveDeliveryRecord{
		Key:           tfports.EffectiveDeliveryKey{TenantID: tenant, Object: carried, Attempt: attempt},
		ContentDigest: "SYN-DELIVERY-DIGEST-CODE75-" + object,
		Delivery:      delivery,
		RecordedAt:    controlFactOccurredAt.Add(time.Second),
	}
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		outcome, err := deliveries.Save(txCtx, record)
		if err == nil && outcome != tfports.DeliverySaved {
			t.Fatalf("交付 outcome = %v", outcome)
		}
		return err
	})
	return `{"tenantId":"` + tenant.String() + `","object":"` + object + `","attempt":"SYN-ATTEMPT-CODE75","version":"SYN-DELIVERY-CODE75-V1"}`,
		"effective-delivery/" + object + "/SYN-ATTEMPT-CODE75"
}

func saveControlFactExternalTracking(t *testing.T, fixture *synVerticalFixture, object string) (string, string) {
	t.Helper()
	facts, err := tfpostgres.NewExternalTrackingFacts(fixture.db)
	if err != nil {
		t.Fatalf("构造外部承运轨迹册：%v", err)
	}
	tenant := mustTF(t, tfdomain.NewTenantID, fixture.identity.TenantID().String())
	judgment, err := tfdomain.JudgeEffectiveTimeExplicitly(controlFactOccurredAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("有效时间判断：%v", err)
	}
	reference := "SYN-EXTF-CODE75-" + object
	spec := tfdomain.ExternalTrackingFactSpec{
		TenantID:    tenant,
		Fact:        mustTF(t, tfdomain.NewExternalTrackingFactReference, reference),
		Version:     mustTF(t, tfdomain.NewExternalTrackingFactVersion, "SYN-EXTV-1"),
		Source:      mustTF(t, tfdomain.NewTrackingSourceReference, "SYN-AGGREGATOR"),
		Credential:  mustTF(t, tfdomain.NewExternalCarrierCredentialReference, "SYN-CARRIER/"+object),
		Object:      mustTF(t, tfdomain.NewCarriedObjectReference, object),
		SourceEvent: mustTF(t, tfdomain.NewSourceEventReference, "SYN-EVT-"+object),
		Status:      mustTF(t, tfdomain.NewRawStatusReference, "IN_TRANSIT"),
		OccurredAt:  controlFactOccurredAt,
		ReceivedAt:  controlFactOccurredAt.Add(30 * time.Second),
		Effective:   judgment,
	}
	fact, err := tfdomain.AdoptExternalCarrierTracking(spec)
	if err != nil {
		t.Fatalf("外部承运轨迹事实：%v", err)
	}
	record := tfports.ExternalTrackingFactRecord{
		Key:        tfports.ExternalTrackingFactKey{TenantID: tenant, Fact: spec.Fact, Version: spec.Version},
		Fact:       fact,
		RecordedAt: controlFactOccurredAt.Add(time.Minute),
	}
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		outcome, err := facts.Save(txCtx, record)
		if err == nil && outcome != tfports.ExternalTrackingFactSaved {
			t.Fatalf("外部承运轨迹 outcome = %v", outcome)
		}
		return err
	})
	return `{"tenantId":"` + tenant.String() + `","fact":"` + reference + `","version":"SYN-EXTV-1"}`,
		"external-carrier-tracking/" + reference
}

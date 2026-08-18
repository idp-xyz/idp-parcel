package transportfulfillment_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/transportfulfillment"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
)

// 本文件证消费侧处理适配器：按信封的（租户+对象+尝试）重读 TF 有效交付，反查当前已
// 接受委托，再转交终局编排；以及可见性 / 不变量破坏 / 目标缺席 / 未决 / 未交出
// handoff 各自可识别。

type deliveryStoreDouble struct {
	record tfports.EffectiveDeliveryRecord
	found  bool
	err    error
	last   tfports.EffectiveDeliveryKey
}

func (double *deliveryStoreDouble) FindByKey(
	_ context.Context, key tfports.EffectiveDeliveryKey,
) (tfports.EffectiveDeliveryRecord, bool, error) {
	double.last = key
	if double.err != nil {
		return tfports.EffectiveDeliveryRecord{}, false, double.err
	}
	return double.record, double.found, nil
}

type deliveryTargetViewDouble struct {
	target psdomain.CurrentAcceptedParcelTarget
	found  bool
	err    error
	tenant string
	parcel string
}

func (double *deliveryTargetViewDouble) FindCurrentAcceptedByParcel(
	_ context.Context, tenant psdomain.TenantID, parcel psdomain.DeclaredParcelID,
) (psdomain.CurrentAcceptedParcelTarget, bool, error) {
	double.tenant = tenant.String()
	double.parcel = parcel.String()
	if double.err != nil {
		return psdomain.CurrentAcceptedParcelTarget{}, false, double.err
	}
	return double.target, double.found, nil
}

type deliveryAdopterDouble struct {
	calls  int
	target adapter.TargetShipment
}

func (double *deliveryAdopterDouble) AdoptFromEffectiveDelivery(
	_ context.Context, _ tfdomain.EffectiveDelivery, target adapter.TargetShipment,
) (psapplication.FormParcelFinalResult, error) {
	double.calls++
	double.target = target
	return psapplication.FormParcelFinalResult{}, errors.New("adopter should not be called")
}

type recordingFinalAdopter struct {
	inner  *adapter.DeliveryOutcomeAdapter
	target adapter.TargetShipment
}

func (recorder *recordingFinalAdopter) AdoptFromEffectiveDelivery(
	ctx context.Context, delivery tfdomain.EffectiveDelivery, target adapter.TargetShipment,
) (psapplication.FormParcelFinalResult, error) {
	recorder.target = target
	return recorder.inner.AdoptFromEffectiveDelivery(ctx, delivery, target)
}

type controllableFinalDownstream struct {
	err   error
	calls int
}

func (double *controllableFinalDownstream) HandOffFinalOutcome(
	context.Context, psports.FinalOutcomeHandoffIntent,
) error {
	double.calls++
	return double.err
}

type unconfiguredFinalRules struct{}

func (unconfiguredFinalRules) JudgeFinalOutcome(
	context.Context, psdomain.SourceIdentity, psdomain.ResponsibilityOutcome,
) (psports.FinalRuleJudgment, bool, error) {
	return psports.FinalRuleJudgment{}, false, nil
}

type finalHandlerConfig struct {
	rules      psports.FinalRuleView
	downstream psports.FinalOutcomeHandoff
	requests   psports.ShipmentRequestRepository
}

func deliveryFinalHandler(t *testing.T, config finalHandlerConfig) *psapplication.FormParcelFinalHandler {
	t.Helper()

	var rules psports.FinalRuleView = &finalRuleDouble{judgment: psports.FinalRuleJudgment{
		Satisfied:   true,
		Kind:        value(t, psdomain.NewFinalKindReference, "NETWORK_SERVICE_DELIVERED"),
		RuleVersion: value(t, psdomain.NewFinalRuleVersionReference, "final-rules/v1"),
	}}
	if config.rules != nil {
		rules = config.rules
	}
	var downstream psports.FinalOutcomeHandoff = finalDownstreamDouble{}
	if config.downstream != nil {
		downstream = config.downstream
	}
	requests := config.requests
	if requests == nil {
		requests = &requestStoreDouble{records: map[psdomain.SourceIdentity]psdomain.ShipmentRequest{
			identity(t): acceptedRequest(t),
		}}
	}
	return psapplication.NewFormParcelFinalHandler(psapplication.FormParcelFinalDeps{
		Requests:      requests,
		Rules:         rules,
		Finals:        &finalStoreDouble{byKey: map[psports.FinalAdoptionKey]psports.FinalOutcomeRecord{}},
		Cancellations: cancellationViewDouble{},
		Identities:    &finalIdentityDouble{},
		Downstream:    downstream,
		Clock:         fixedClock{at: deliveredAt.Add(time.Minute)},
	})
}

func registeredDelivery(t *testing.T, object string) tfports.EffectiveDeliveryRecord {
	t.Helper()

	attempt, err := tfdomain.FormFulfillmentAttempt(tfdomain.FulfillmentAttemptSpec{
		TenantID:    value(t, tfdomain.NewTenantID, "tenant-1"),
		Attempt:     value(t, tfdomain.NewAttemptReference, "attempt-1"),
		Task:        value(t, tfdomain.NewDispatchTaskReference, "delivery-task-1"),
		ExecutedBy:  value(t, tfdomain.NewExecutingPartyReference, "courier-1"),
		Place:       value(t, tfdomain.NewAttemptPlaceReference, "recipient-door"),
		PlannedFrom: deliveredAt.Add(-2 * time.Hour),
		PlannedTo:   deliveredAt.Add(2 * time.Hour),
		ArrivedAt:   deliveredAt.Add(-10 * time.Minute),
		Objects:     []tfdomain.CarriedObjectReference{value(t, tfdomain.NewCarriedObjectReference, object)},
		Evidence:    value(t, tfdomain.NewAttemptEvidenceReference, "GPS-TRACE/1"),
	})
	if err != nil {
		t.Fatalf("构造履约尝试：%v", err)
	}
	result, err := tfdomain.FormDeliveryAttemptResult(
		attempt,
		value(t, tfdomain.NewCarriedObjectReference, object),
		tfdomain.ObjectDelivered,
		tfdomain.AttemptResultBasisReference{},
		deliveredAt,
	)
	if err != nil {
		t.Fatalf("构造妥投结果：%v", err)
	}
	delivery, err := tfdomain.FormEffectiveDelivery(attempt, result, tfdomain.EffectiveDeliverySpec{
		Method:    value(t, tfdomain.NewDeliveryMethodReference, "HAND_TO_RECIPIENT"),
		Recipient: value(t, tfdomain.NewReceivingPartyReference, "recipient-1"),
		Proof:     value(t, tfdomain.NewDeliveryProofReference, "POD-3"),
		Version:   value(t, tfdomain.NewDeliveryResultVersion, "delivery-result/v1"),
	})
	if err != nil {
		t.Fatalf("构造有效交付：%v", err)
	}
	return tfports.EffectiveDeliveryRecord{
		Key: tfports.EffectiveDeliveryKey{
			TenantID: value(t, tfdomain.NewTenantID, "tenant-1"),
			Object:   value(t, tfdomain.NewCarriedObjectReference, object),
			Attempt:  value(t, tfdomain.NewAttemptReference, "attempt-1"),
		},
		ContentDigest: "digest-1",
		Delivery:      delivery,
		RecordedAt:    deliveredAt.Add(time.Second),
	}
}

func deliveryTarget(t *testing.T) psdomain.CurrentAcceptedParcelTarget {
	t.Helper()
	return pickupTarget(t)
}

func registeredDeliveryRef() psinbox.RegisteredEffectiveDelivery {
	return psinbox.RegisteredEffectiveDelivery{
		TenantID: "tenant-1",
		Object:   "parcel-1",
		Attempt:  "attempt-1",
	}
}

func TestARegisteredDeliveryLooksUpTheUniqueAcceptedTargetAndFormsFinal(t *testing.T) {
	store := &deliveryStoreDouble{record: registeredDelivery(t, "parcel-1"), found: true}
	targets := &deliveryTargetViewDouble{target: deliveryTarget(t), found: true}
	adopting := &recordingFinalAdopter{
		inner: adapter.NewDeliveryOutcomeAdapter(deliveryFinalHandler(t, finalHandlerConfig{})),
	}
	subject, err := adapter.NewAdoptOnEffectiveDeliveryAdapter(store, targets, adopting)
	if err != nil {
		t.Fatalf("构造处理适配器：%v", err)
	}

	if err := subject.HandleRegisteredEffectiveDelivery(t.Context(), registeredDeliveryRef()); err != nil {
		t.Fatalf("处理有效交付：%v", err)
	}

	if store.last.TenantID.String() != "tenant-1" ||
		store.last.Object.String() != "parcel-1" ||
		store.last.Attempt.String() != "attempt-1" {
		t.Fatalf("交付查询键 = %+v", store.last)
	}
	if targets.tenant != "tenant-1" || targets.parcel != "parcel-1" {
		t.Fatalf("反查租户/包裹 = %q / %q", targets.tenant, targets.parcel)
	}
	if adopting.target.Identity != identity(t) ||
		adopting.target.ShipmentRequestID.String() != "request-1" {
		t.Fatalf("目标指名 = %+v", adopting.target)
	}
	if adopting.target.SubmissionVersion.String() != "" {
		t.Fatalf("不得把投影上的提交版本编进终局命令：%q", adopting.target.SubmissionVersion)
	}
}

func TestAMissingEffectiveDeliveryIsContinuableUndecided(t *testing.T) {
	adopter := &deliveryAdopterDouble{}
	subject, err := adapter.NewAdoptOnEffectiveDeliveryAdapter(
		&deliveryStoreDouble{},
		&deliveryTargetViewDouble{target: deliveryTarget(t), found: true},
		adopter,
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredEffectiveDelivery(
		t.Context(), registeredDeliveryRef()); !errors.Is(err, adapter.ErrDeliveryNotVisible) {
		t.Fatalf("err = %v, want ErrDeliveryNotVisible", err)
	}
	if adopter.calls != 0 {
		t.Fatal("缺登记不该走到终局")
	}
}

func TestAnUnreadableDeliveryStoreIsContinuableUndecided(t *testing.T) {
	adopter := &deliveryAdopterDouble{}
	subject, err := adapter.NewAdoptOnEffectiveDeliveryAdapter(
		&deliveryStoreDouble{err: errors.New("store unavailable")},
		&deliveryTargetViewDouble{target: deliveryTarget(t), found: true},
		adopter,
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredEffectiveDelivery(
		t.Context(), registeredDeliveryRef()); !errors.Is(err, adapter.ErrDeliveryNotVisible) {
		t.Fatalf("err = %v, want ErrDeliveryNotVisible", err)
	}
	if adopter.calls != 0 {
		t.Fatal("读失败不该走到终局")
	}
}

// Covers: 登记本体与幂等键各说各话时不采认，且与「还看不见」分开报。按键去查却拿回
// 另一个键或另一个对象是仓储/数据不变量破坏（ADR-0029），重投同一内容不会自愈——混进
// 可续办那一格会让永久损坏被当成等依赖，一直重试到投递上限。B 票不得把它进
// WithUndecidedSentinels。
func TestADeliveryRecordThatDisagreesWithItsKeyIsInconsistentNotInvisible(t *testing.T) {
	for name, damage := range map[string]func(*testing.T, *tfports.EffectiveDeliveryRecord){
		"键的对象与信封不符": func(t *testing.T, record *tfports.EffectiveDeliveryRecord) {
			record.Key.Object = value(t, tfdomain.NewCarriedObjectReference, "parcel-other")
		},
		"本体与键不符": func(t *testing.T, record *tfports.EffectiveDeliveryRecord) {
			record.Delivery = registeredDelivery(t, "parcel-other").Delivery
		},
	} {
		t.Run(name, func(t *testing.T) {
			record := registeredDelivery(t, "parcel-1")
			damage(t, &record)
			adopter := &deliveryAdopterDouble{}
			subject, err := adapter.NewAdoptOnEffectiveDeliveryAdapter(
				&deliveryStoreDouble{record: record, found: true},
				&deliveryTargetViewDouble{target: deliveryTarget(t), found: true},
				adopter,
			)
			if err != nil {
				t.Fatalf("构造：%v", err)
			}

			first := subject.HandleRegisteredEffectiveDelivery(t.Context(), registeredDeliveryRef())
			if !errors.Is(first, adapter.ErrDeliveryRecordInconsistent) {
				t.Fatalf("err = %v, want ErrDeliveryRecordInconsistent", first)
			}
			if errors.Is(first, adapter.ErrDeliveryNotVisible) {
				t.Fatal("不变量破坏不得混进可续办的「还看不见」")
			}
			if second := subject.HandleRegisteredEffectiveDelivery(
				t.Context(), registeredDeliveryRef()); !errors.Is(second, adapter.ErrDeliveryRecordInconsistent) {
				t.Fatalf("重投 err = %v——同一内容重投不自愈", second)
			}
			if adopter.calls != 0 {
				t.Fatal("键与本体不符不该走到终局")
			}
		})
	}
}

func TestAMissingParcelTargetForADeliveryIsContinuableUndecided(t *testing.T) {
	adopter := &deliveryAdopterDouble{}
	subject, err := adapter.NewAdoptOnEffectiveDeliveryAdapter(
		&deliveryStoreDouble{record: registeredDelivery(t, "parcel-1"), found: true},
		&deliveryTargetViewDouble{},
		adopter,
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredEffectiveDelivery(
		t.Context(), registeredDeliveryRef()); !errors.Is(err, adapter.ErrParcelTargetNotFound) {
		t.Fatalf("err = %v, want ErrParcelTargetNotFound", err)
	}
	if adopter.calls != 0 {
		t.Fatal("没有可采认目标不该走到终局")
	}
}

// Covers: TF 的载运对象引用可能指集运单元而非包裹，而 TF 不带判别位。今天不猜成员
// 映射：集运单元号反查不到当前已接受委托，落与「委托还没到已接受」同一格。
func TestAConsolidationUnitLikeDeliveryObjectFallsIntoTargetNotFound(t *testing.T) {
	adopter := &deliveryAdopterDouble{}
	targets := &deliveryTargetViewDouble{}
	subject, err := adapter.NewAdoptOnEffectiveDeliveryAdapter(
		&deliveryStoreDouble{record: registeredDelivery(t, "bag-1"), found: true},
		targets,
		adopter,
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	reference := registeredDeliveryRef()
	reference.Object = "bag-1"
	if err := subject.HandleRegisteredEffectiveDelivery(
		t.Context(), reference); !errors.Is(err, adapter.ErrParcelTargetNotFound) {
		t.Fatalf("err = %v, want ErrParcelTargetNotFound", err)
	}
	if targets.parcel != "bag-1" {
		t.Fatalf("反查包裹 = %q——不得替 TF 猜集运单元的成员", targets.parcel)
	}
	if adopter.calls != 0 {
		t.Fatal("反查不中不该走到终局")
	}
}

func TestAnAmbiguousParcelTargetForADeliveryStaysIdentifiable(t *testing.T) {
	adopter := &deliveryAdopterDouble{}
	subject, err := adapter.NewAdoptOnEffectiveDeliveryAdapter(
		&deliveryStoreDouble{record: registeredDelivery(t, "parcel-1"), found: true},
		&deliveryTargetViewDouble{err: psdomain.ErrAmbiguousParcelTarget},
		adopter,
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	if err := subject.HandleRegisteredEffectiveDelivery(
		t.Context(), registeredDeliveryRef()); !errors.Is(err, psdomain.ErrAmbiguousParcelTarget) {
		t.Fatalf("err = %v, want ErrAmbiguousParcelTarget", err)
	}
	if adopter.calls != 0 {
		t.Fatal("歧义不得按 latest 采认")
	}
}

func TestAnUntranslatableDeliveryReferenceKeepsItsSentinel(t *testing.T) {
	adopter := &deliveryAdopterDouble{}
	subject, err := adapter.NewAdoptOnEffectiveDeliveryAdapter(
		&deliveryStoreDouble{record: registeredDelivery(t, "parcel-1"), found: true},
		&deliveryTargetViewDouble{target: deliveryTarget(t), found: true},
		adopter,
	)
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	for name, reference := range map[string]psinbox.RegisteredEffectiveDelivery{
		"空租户": {Object: "parcel-1", Attempt: "attempt-1"},
		"空对象": {TenantID: "tenant-1", Attempt: "attempt-1"},
		"空尝试": {TenantID: "tenant-1", Object: "parcel-1"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := subject.HandleRegisteredEffectiveDelivery(
				t.Context(), reference); !errors.Is(err, adapter.ErrUntranslatableAnswer) {
				t.Fatalf("err = %v, want ErrUntranslatableAnswer", err)
			}
		})
	}
	if adopter.calls != 0 {
		t.Fatal("引用译不出来不该走到终局")
	}
}

func TestFinalOutcomesMapToConsumptionSlots(t *testing.T) {
	deliveryThrough := func(t *testing.T, handler *psapplication.FormParcelFinalHandler) *adapter.AdoptOnEffectiveDeliveryAdapter {
		t.Helper()
		subject, err := adapter.NewAdoptOnEffectiveDeliveryAdapter(
			&deliveryStoreDouble{record: registeredDelivery(t, "parcel-1"), found: true},
			&deliveryTargetViewDouble{target: deliveryTarget(t), found: true},
			adapter.NewDeliveryOutcomeAdapter(handler),
		)
		if err != nil {
			t.Fatalf("构造处理适配器：%v", err)
		}
		return subject
	}

	t.Run("FINAL_FORMED 入账", func(t *testing.T) {
		subject := deliveryThrough(t, deliveryFinalHandler(t, finalHandlerConfig{}))
		if err := subject.HandleRegisteredEffectiveDelivery(t.Context(), registeredDeliveryRef()); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
	})

	t.Run("EXISTING_RESULT 入账", func(t *testing.T) {
		subject := deliveryThrough(t, deliveryFinalHandler(t, finalHandlerConfig{}))
		if err := subject.HandleRegisteredEffectiveDelivery(t.Context(), registeredDeliveryRef()); err != nil {
			t.Fatalf("首次：%v", err)
		}
		if err := subject.HandleRegisteredEffectiveDelivery(t.Context(), registeredDeliveryRef()); err != nil {
			t.Fatalf("重投 err = %v, want nil", err)
		}
	})

	t.Run("REQUEST_NOT_ACCEPTED 入账", func(t *testing.T) {
		subject := deliveryThrough(t, deliveryFinalHandler(t, finalHandlerConfig{
			requests: &requestStoreDouble{records: map[psdomain.SourceIdentity]psdomain.ShipmentRequest{}},
		}))
		if err := subject.HandleRegisteredEffectiveDelivery(t.Context(), registeredDeliveryRef()); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
	})

	t.Run("FINAL_RULE_UNCONFIGURED 回滚", func(t *testing.T) {
		subject := deliveryThrough(t, deliveryFinalHandler(t, finalHandlerConfig{
			rules: unconfiguredFinalRules{},
		}))
		if err := subject.HandleRegisteredEffectiveDelivery(
			t.Context(), registeredDeliveryRef()); !errors.Is(err, adapter.ErrFinalUndecided) {
			t.Fatalf("err = %v, want ErrFinalUndecided", err)
		}
	})

	t.Run("handoff 未交出即回滚", func(t *testing.T) {
		downstream := &controllableFinalDownstream{err: errors.New("outbox unavailable")}
		subject := deliveryThrough(t, deliveryFinalHandler(t, finalHandlerConfig{downstream: downstream}))
		if err := subject.HandleRegisteredEffectiveDelivery(
			t.Context(), registeredDeliveryRef()); !errors.Is(err, adapter.ErrFinalHandoffPending) {
			t.Fatalf("err = %v, want ErrFinalHandoffPending", err)
		}
		if downstream.calls != 1 {
			t.Fatalf("handoff 调用 = %d, want 1", downstream.calls)
		}

		downstream.err = nil
		if err := subject.HandleRegisteredEffectiveDelivery(t.Context(), registeredDeliveryRef()); err != nil {
			t.Fatalf("handoff 恢复后重投：%v", err)
		}
		if downstream.calls != 2 {
			t.Fatalf("已有结果路径应再交一次意图，调用 = %d", downstream.calls)
		}
	})
}

// Covers: 终局这条链的未交出 handoff 不得入账。inbox 若已 processed，恢复后不会再交意图。
func TestAPendingFinalHandoffDoesNotMarkTheInboxProcessed(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}

	downstream := &controllableFinalDownstream{err: errors.New("outbox unavailable")}
	processing, err := adapter.NewAdoptOnEffectiveDeliveryAdapter(
		&deliveryStoreDouble{record: registeredDelivery(t, "parcel-1"), found: true},
		&deliveryTargetViewDouble{target: deliveryTarget(t), found: true},
		adapter.NewDeliveryOutcomeAdapter(deliveryFinalHandler(t, finalHandlerConfig{downstream: downstream})),
	)
	if err != nil {
		t.Fatalf("构造处理适配器：%v", err)
	}
	consumer, err := psinbox.NewEffectiveDeliveryConsumer(db.Transactor(), store, processing)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}

	payload, err := json.Marshal(map[string]string{
		"tenantId": "tenant-1",
		"object":   "parcel-1",
		"attempt":  "attempt-1",
	})
	if err != nil {
		t.Fatalf("载荷：%v", err)
	}
	now := deliveredAt.Add(time.Minute)
	envelope := eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID("tenant-1/parcel-1/attempt-1/delivery-result/v1/effective-delivery"),
		Source:       "idp-parcel/transport-fulfillment",
		Type:         psinbox.EffectiveDeliveryRegisteredEventType,
		Version:      1,
		Scope:        "tenant-1",
		Subject:      "parcel-1/attempt-1",
		PartitionKey: "tenant-1/parcel-1",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}

	if err := consumer.Consume(t.Context(), envelope); !errors.Is(err, adapter.ErrFinalHandoffPending) {
		t.Fatalf("err = %v, want ErrFinalHandoffPending", err)
	}
	if err := consumer.Consume(t.Context(), envelope); !errors.Is(err, adapter.ErrFinalHandoffPending) {
		t.Fatalf("未入账的重投应再处理：%v", err)
	}
	if downstream.calls != 2 {
		t.Fatalf("inbox 若已 processed，第二次不会再交意图；调用 = %d", downstream.calls)
	}

	downstream.err = nil
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("handoff 恢复后重投：%v", err)
	}
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("已处理后的重复投递：%v", err)
	}
	if downstream.calls != 3 {
		t.Fatalf("成功入账后重复投递不应再交意图；调用 = %d", downstream.calls)
	}
}

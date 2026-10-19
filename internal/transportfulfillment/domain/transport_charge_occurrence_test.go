package domain_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

var occurredAt = time.Date(2026, 8, 10, 9, 0, 0, 0, time.UTC)

func occurrenceSpec(t *testing.T, occurrence, journey string) domain.TransportChargeOccurrenceSpec {
	t.Helper()
	return domain.TransportChargeOccurrenceSpec{
		TenantID:    mustValue(t, domain.NewTenantID, "tenant-1"),
		Occurrence:  mustValue(t, domain.NewChargeOccurrenceReference, occurrence),
		Journey:     mustValue(t, domain.NewJourneyReference, journey),
		LegalEntity: mustValue(t, domain.NewProcurementLegalEntityReference, "legal-1"),
		Provider:    mustValue(t, domain.NewServiceProviderReference, "partner-1"),
		Agreement:   mustValue(t, domain.NewAgreementSnapshotReference, "agreement-snapshot-1"),
		Reason:      domain.BookingOccurrence,
		FactBasis:   mustValue(t, domain.NewOccurrenceBasisReference, "BOOKING/booking-1"),
		Scope:       mustValue(t, domain.NewOccurrenceScopeReference, "scope-linehaul-1"),
		Members: []domain.CarriedObjectReference{
			mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
			mustValue(t, domain.NewCarriedObjectReference, "parcel-2"),
		},
		Quantity:   2,
		Unit:       mustValue(t, domain.NewQuantityUnitReference, "piece"),
		OccurredAt: occurredAt,
		Validity:   mustValue(t, domain.NewOccurrenceValidityVersion, "occurrence/v1"),
		Condition:  assertedCondition(t, "agreement-snapshot-1 booking clause met"),
	}
}

func assertedCondition(t *testing.T, assertion string) domain.OccurrenceCondition {
	t.Helper()
	condition, err := domain.NewAssertedOccurrenceCondition(assertion)
	if err != nil {
		t.Fatalf("断言夹具：%v", err)
	}
	return condition
}

// Covers: CONTEXT「每个运输收费发生项必须固定采购责任法人、服务提供方、采用的供应商
// 协议与履约条件、发生原因、唯一主要业务范围、对象成员、数量与单位、业务时间、有效性
// 和来源事实」与「它不是价格、供应商预期成本、供应商账单主张、审核应付或真实付款」——
// 类型上没有任何金额或币种字段可以承载金额结论。
func TestAChargeOccurrenceFixesItsSourcesWithoutAnyAmount(t *testing.T) {
	occurrenceType := reflect.TypeOf(domain.TransportChargeOccurrence{})
	for index := 0; index < occurrenceType.NumField(); index++ {
		field := occurrenceType.Field(index)
		name := strings.ToLower(field.Name)
		for _, forbidden := range []string{"amount", "money", "price", "cost", "fee", "currency"} {
			if strings.Contains(name, forbidden) {
				t.Fatalf("TransportChargeOccurrence 携带 %q——发生项就有了直接变成金额的地方", field.Name)
			}
		}
	}

	occurrence, err := domain.FormTransportChargeOccurrence(occurrenceSpec(t, "occurrence-1", "journey-1"))
	if err != nil {
		t.Fatalf("form occurrence: %v", err)
	}
	if occurrence.Reason() != domain.BookingOccurrence {
		t.Fatalf("reason = %q, want BOOKING", occurrence.Reason())
	}
	quantity, unit := occurrence.Quantity()
	if quantity != 2 || unit.String() != "piece" {
		t.Fatalf("quantity = %d %q（数量与单位成对固定）", quantity, unit)
	}
	if len(occurrence.Members()) != 2 {
		t.Fatalf("members = %d, want 2", len(occurrence.Members()))
	}
	if !occurrence.OccurredAt().Equal(occurredAt) {
		t.Fatalf("occurred at = %s", occurrence.OccurredAt())
	}
	if _, _, _, revised := occurrence.Revision(); revised {
		t.Fatal("首个版本凭空带上了修订标记")
	}

	broken := map[string]func(*domain.TransportChargeOccurrenceSpec){
		"no agreement":  func(spec *domain.TransportChargeOccurrenceSpec) { spec.Agreement = domain.AgreementSnapshotReference{} },
		"no fact basis": func(spec *domain.TransportChargeOccurrenceSpec) { spec.FactBasis = domain.OccurrenceBasisReference{} },
		"no journey":    func(spec *domain.TransportChargeOccurrenceSpec) { spec.Journey = domain.JourneyReference{} },
		"no legal entity": func(spec *domain.TransportChargeOccurrenceSpec) {
			spec.LegalEntity = domain.ProcurementLegalEntityReference{}
		},
		"no members":        func(spec *domain.TransportChargeOccurrenceSpec) { spec.Members = nil },
		"duplicate member":  func(spec *domain.TransportChargeOccurrenceSpec) { spec.Members = append(spec.Members, spec.Members[0]) },
		"zero quantity":     func(spec *domain.TransportChargeOccurrenceSpec) { spec.Quantity = 0 },
		"negative quantity": func(spec *domain.TransportChargeOccurrenceSpec) { spec.Quantity = -1 },
		"no unit":           func(spec *domain.TransportChargeOccurrenceSpec) { spec.Unit = domain.QuantityUnitReference{} },
		"no validity":       func(spec *domain.TransportChargeOccurrenceSpec) { spec.Validity = domain.OccurrenceValidityVersion{} },
		"invalid reason":    func(spec *domain.TransportChargeOccurrenceSpec) { spec.Reason = domain.ChargeOccurrenceReasonInvalid },
	}
	for name, breakSpec := range broken {
		t.Run(name, func(t *testing.T) {
			spec := occurrenceSpec(t, "occurrence-x", "journey-1")
			breakSpec(&spec)
			if _, err := domain.FormTransportChargeOccurrence(spec); !errors.Is(err, domain.ErrInvalidChargeOccurrence) {
				t.Fatalf("error = %v, want ErrInvalidChargeOccurrence", err)
			}
		})
	}

	t.Run("the reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []domain.ChargeOccurrenceReason{
			domain.BookingOccurrence, domain.CancellationOccurrence,
			domain.FailedAttemptOccurrence, domain.ActualFulfillmentOccurrence,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 4 {
			t.Fatalf("reason labels collapsed into %d", len(labels))
		}
		if domain.ChargeOccurrenceReason(len(labels)+1).String() != "" {
			t.Fatal("第五个原因取值带了标签——封闭集合被悄悄放开")
		}
	})
}

// Covers: `AT-TF-094`「第一次到场因客户未备货失败……第一次形成运输收费发生项」——失败
// 尝试费的事实依据与业务时间取自失败的对象结果；揽收到手的结果冒充失败尝试被拒。
func TestFailedAttemptOccurrenceTakesItsFactFromTheResult(t *testing.T) {
	attempt := formedAttempt(t, "attempt-1")
	failed, err := domain.FormAttemptObjectResult(
		attempt,
		mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		domain.GoodsNotReady,
		mustValue(t, domain.NewAttemptResultBasisReference, "reason-not-ready-1"),
		attemptArrivedAt.Add(5*time.Minute),
	)
	if err != nil {
		t.Fatalf("form failed result: %v", err)
	}

	spec := occurrenceSpec(t, "occurrence-1", "journey-1")
	spec.Members = []domain.CarriedObjectReference{failed.Object()}
	spec.Quantity = 1
	occurrence, err := domain.ChargeOccurrenceForFailedAttempt(spec, failed)
	if err != nil {
		t.Fatalf("form failed-attempt occurrence: %v", err)
	}
	if occurrence.Reason() != domain.FailedAttemptOccurrence {
		t.Fatalf("reason = %q, want FAILED_ATTEMPT", occurrence.Reason())
	}
	if occurrence.FactBasis().String() != "ATTEMPT-RESULT/attempt-1/parcel-1" {
		t.Fatalf("fact basis = %q（事实依据须取自结果）", occurrence.FactBasis())
	}
	if !occurrence.OccurredAt().Equal(failed.OccurredAt()) {
		t.Fatal("业务时间没有取自失败结果")
	}

	t.Run("a picked-up result cannot fund a failed-attempt charge", func(t *testing.T) {
		delivered, err := domain.FormAttemptObjectResult(
			attempt,
			mustValue(t, domain.NewCarriedObjectReference, "parcel-2"),
			domain.ObjectPickedUp,
			domain.AttemptResultBasisReference{},
			attemptArrivedAt.Add(6*time.Minute),
		)
		if err != nil {
			t.Fatalf("form picked-up result: %v", err)
		}
		if _, err := domain.ChargeOccurrenceForFailedAttempt(spec, delivered); !errors.Is(err, domain.ErrNotAFailedAttempt) {
			t.Fatalf("error = %v, want ErrNotAFailedAttempt", err)
		}
	})
}

// Covers: CONTEXT「原旅程中已经发生的……与替代、改送或退运旅程中新发生的范围必须分别
// 关联各自旅程……新旅程不得覆盖、搬移或自动净额抵销原旅程发生项」——两旅程各自独立
// 发生项；修订不换旅程（没有搬移入口）。
func TestJourneysKeepTheirOwnOccurrences(t *testing.T) {
	original, err := domain.FormTransportChargeOccurrence(occurrenceSpec(t, "occurrence-1", "journey-original"))
	if err != nil {
		t.Fatalf("form original journey occurrence: %v", err)
	}
	alternate, err := domain.FormTransportChargeOccurrence(occurrenceSpec(t, "occurrence-2", "journey-alternate"))
	if err != nil {
		t.Fatalf("form alternate journey occurrence: %v", err)
	}
	if original.Occurrence() == alternate.Occurrence() {
		t.Fatal("两旅程共用了一个发生项身份")
	}
	if original.Journey() == alternate.Journey() {
		t.Fatal("夹具两旅程塌成同一个")
	}

	revised, err := original.ReviseValidity(
		domain.OccurrenceSuperseded,
		mustValue(t, domain.NewOccurrenceValidityVersion, "occurrence/v2"),
		mustValue(t, domain.NewOccurrenceBasisReference, "CORRECTION/source-1"),
		occurredAt.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	if revised.Journey() != original.Journey() {
		t.Fatal("修订把发生项搬去了另一个旅程")
	}
}

// Covers: CONTEXT「来源更正……保留原发生项并形成失效、替代……关系；结算依据新的有效性
// 追加调整，不删除原成本」——修订回指前身、原值不动；沿用原版本号就是覆盖；修订走向
// 封闭二值（范围更正另票）。
func TestValidityRevisionFormsANewVersionWithoutDeletingTheOriginal(t *testing.T) {
	original, err := domain.FormTransportChargeOccurrence(occurrenceSpec(t, "occurrence-1", "journey-1"))
	if err != nil {
		t.Fatalf("form occurrence: %v", err)
	}
	revisedAt := occurredAt.Add(24 * time.Hour)

	invalidated, err := original.ReviseValidity(
		domain.OccurrenceInvalidated,
		mustValue(t, domain.NewOccurrenceValidityVersion, "occurrence/v2"),
		mustValue(t, domain.NewOccurrenceBasisReference, "CORRECTION/source-1"),
		revisedAt,
	)
	if err != nil {
		t.Fatalf("revise: %v", err)
	}
	predecessor, present := invalidated.Corrects()
	if !present || predecessor.String() != "occurrence/v1" {
		t.Fatalf("corrects = %q present=%v, want v1", predecessor, present)
	}
	kind, basis, at, revised := invalidated.Revision()
	if !revised || kind != domain.OccurrenceInvalidated {
		t.Fatalf("revision kind = %q revised=%v, want INVALIDATED", kind, revised)
	}
	if basis.String() != "CORRECTION/source-1" || !at.Equal(revisedAt) {
		t.Fatalf("revision basis = %q at = %s", basis, at)
	}
	if original.Validity().String() != "occurrence/v1" {
		t.Fatal("修订改写了原有效性版本")
	}
	if _, present := original.Corrects(); present {
		t.Fatal("原版本被修订动作反向打上了修订标记")
	}

	t.Run("reusing the original version is an overwrite and is refused", func(t *testing.T) {
		if _, err := original.ReviseValidity(
			domain.OccurrenceSuperseded,
			original.Validity(),
			mustValue(t, domain.NewOccurrenceBasisReference, "CORRECTION/source-2"),
			revisedAt,
		); !errors.Is(err, domain.ErrInvalidChargeOccurrence) {
			t.Fatalf("error = %v; 沿用原版本号就是覆盖", err)
		}
	})

	t.Run("a revision without a basis is refused", func(t *testing.T) {
		if _, err := original.ReviseValidity(
			domain.OccurrenceInvalidated,
			mustValue(t, domain.NewOccurrenceValidityVersion, "occurrence/v3"),
			domain.OccurrenceBasisReference{},
			revisedAt,
		); !errors.Is(err, domain.ErrInvalidChargeOccurrence) {
			t.Fatalf("error = %v; 没有更正来源的失效与数据丢失无从分辨", err)
		}
	})

	t.Run("a revision before the occurrence time is refused", func(t *testing.T) {
		if _, err := original.ReviseValidity(
			domain.OccurrenceInvalidated,
			mustValue(t, domain.NewOccurrenceValidityVersion, "occurrence/v4"),
			mustValue(t, domain.NewOccurrenceBasisReference, "CORRECTION/source-3"),
			occurredAt.Add(-time.Hour),
		); !errors.Is(err, domain.ErrInvalidChargeOccurrence) {
			t.Fatalf("error = %v; 修订不可能发生在发生之前", err)
		}
	})

	t.Run("the revision kind set is closed", func(t *testing.T) {
		if domain.OccurrenceRevisionKind(3).String() != "" {
			t.Fatal("第三个修订走向带了标签——封闭集合被悄悄放开")
		}
	})
}

func endedTrunkSegment(t *testing.T, objects ...string) domain.ActualFulfillmentSegment {
	t.Helper()
	rows := make([]domain.RehydrateParticipationSpec, 0, len(objects))
	for index, object := range objects {
		row := domain.RehydrateParticipationSpec{
			Object:     mustValue(t, domain.NewCarriedObjectReference, object),
			EntryKind:  domain.EnteredByTransportHandover,
			EntryBasis: mustValue(t, domain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/in-"+object),
			EnteredAt:  occurredAt,
		}
		// 成员按序依次晚一小时离场，终点各不相同才看得出业务时间取的是最晚那一刻；active- 起头的成员仍在场。
		if !strings.HasPrefix(object, "active-") {
			row.EndKind = domain.EndedByNextHandover
			row.EndBasis = mustValue(t, domain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/out-"+object)
			row.EndedAt = occurredAt.Add(time.Duration(index+1) * time.Hour)
		}
		rows = append(rows, row)
	}
	segment, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID:       mustValue(t, domain.NewTenantID, "tenant-1"),
		Segment:        mustValue(t, domain.NewFulfillmentSegmentReference, "segment-linehaul-1"),
		Participations: rows,
	})
	if err != nil {
		t.Fatalf("段夹具：%v", err)
	}
	return segment
}

// Covers: ADR-0207 决定一——实际履约发生项的依据与范围取段，业务时间取成员参与终点里最晚的一刻；成员在场或不在段
// 里都不形成，各给自己的错误。
func TestActualFulfillmentOccurrenceTakesItsFactFromTheEndedParticipations(t *testing.T) {
	segment := endedTrunkSegment(t, "parcel-1", "parcel-2", "active-parcel-3")
	spec := occurrenceSpec(t, "occurrence-1", "journey-1")
	spec.Members = []domain.CarriedObjectReference{
		mustValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		mustValue(t, domain.NewCarriedObjectReference, "parcel-2"),
	}
	occurrence, err := domain.ChargeOccurrenceForActualFulfillment(spec, segment)
	if err != nil {
		t.Fatalf("形成：%v", err)
	}
	if occurrence.Reason() != domain.ActualFulfillmentOccurrence ||
		occurrence.FactBasis().String() != "FULFILLMENT-SEGMENT/segment-linehaul-1" ||
		occurrence.Scope().String() != "segment-linehaul-1" {
		t.Fatalf("原因 / 依据 / 范围 = %q / %q / %q", occurrence.Reason(), occurrence.FactBasis(), occurrence.Scope())
	}
	if want := occurredAt.Add(2 * time.Hour); !occurrence.OccurredAt().Equal(want) {
		t.Fatalf("业务时间 = %v, want 最晚终点 %v", occurrence.OccurredAt(), want)
	}

	active := spec
	active.Members = []domain.CarriedObjectReference{mustValue(t, domain.NewCarriedObjectReference, "active-parcel-3")}
	if _, err := domain.ChargeOccurrenceForActualFulfillment(active, segment); !errors.Is(err, domain.ErrParticipationNotEnded) {
		t.Fatalf("在场成员 err = %v, want ErrParticipationNotEnded", err)
	}
	stranger := spec
	stranger.Members = []domain.CarriedObjectReference{mustValue(t, domain.NewCarriedObjectReference, "parcel-9")}
	if _, err := domain.ChargeOccurrenceForActualFulfillment(stranger, segment); !errors.Is(err, domain.ErrNotAParticipant) {
		t.Fatalf("段外对象 err = %v, want ErrNotAParticipant", err)
	}
}

// Covers: AT-TF-095——失败派送结果形成失败尝试费，依据前缀与揽收那一侧分开，范围取那一次尝试；妥投被拒。
func TestDeliveryAttemptOccurrenceTakesItsFactFromTheResult(t *testing.T) {
	attempt := formedAttempt(t, "attempt-7")
	object := attempt.Objects()[0]
	failed, err := domain.FormDeliveryAttemptResult(attempt, object, domain.NoOneToReceive,
		mustValue(t, domain.NewAttemptResultBasisReference, "reason-no-one-home"), attemptArrivedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("失败结果：%v", err)
	}
	spec := occurrenceSpec(t, "occurrence-1", "journey-1")
	spec.Members = []domain.CarriedObjectReference{object}
	occurrence, err := domain.ChargeOccurrenceForDeliveryAttempt(spec, failed)
	if err != nil {
		t.Fatalf("形成：%v", err)
	}
	if occurrence.FactBasis().String() != "DELIVERY-ATTEMPT-RESULT/attempt-7/"+object.String() ||
		occurrence.Scope().String() != "attempt-7" || !occurrence.OccurredAt().Equal(failed.OccurredAt()) {
		t.Fatalf("依据 / 范围 / 时点 = %q / %q / %v", occurrence.FactBasis(), occurrence.Scope(), occurrence.OccurredAt())
	}
	delivered, err := domain.FormDeliveryAttemptResult(attempt, object, domain.ObjectDelivered,
		domain.AttemptResultBasisReference{}, attemptArrivedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("妥投结果：%v", err)
	}
	if _, err := domain.ChargeOccurrenceForDeliveryAttempt(spec, delivered); !errors.Is(err, domain.ErrNotAFailedAttempt) {
		t.Fatalf("妥投 err = %v, want ErrNotAFailedAttempt", err)
	}
}

// Covers: ADR-0207 决定四——身份由（租户、原因、事实依据、成员集合）派生：成员换序同一身份，任一维变了换身份，
// 长度不随引用多长而涨；首版有效性是 V1。
func TestTheDerivedOccurrenceReferenceIsDeterministicAndBounded(t *testing.T) {
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")
	basis := mustValue(t, domain.NewOccurrenceBasisReference, "FULFILLMENT-SEGMENT/segment-1")
	one := mustValue(t, domain.NewCarriedObjectReference, "parcel-1")
	two := mustValue(t, domain.NewCarriedObjectReference, "parcel-2")
	derive := func(reason domain.ChargeOccurrenceReason, basis domain.OccurrenceBasisReference, members ...domain.CarriedObjectReference) string {
		t.Helper()
		reference, err := domain.DeriveChargeOccurrenceReference(tenant, reason, basis, members)
		if err != nil {
			t.Fatalf("派生：%v", err)
		}
		return reference.String()
	}
	forward := derive(domain.ActualFulfillmentOccurrence, basis, one, two)
	if forward != derive(domain.ActualFulfillmentOccurrence, basis, two, one) {
		t.Fatal("成员换序换了身份")
	}
	if forward == derive(domain.ActualFulfillmentOccurrence, basis, one) ||
		forward == derive(domain.FailedAttemptOccurrence, basis, one, two) ||
		forward == derive(domain.ActualFulfillmentOccurrence, mustValue(t, domain.NewOccurrenceBasisReference, "FULFILLMENT-SEGMENT/segment-2"), one, two) {
		t.Fatal("某一维变了身份却没变")
	}
	long := mustValue(t, domain.NewOccurrenceBasisReference, "FULFILLMENT-SEGMENT/"+strings.Repeat("x", 500))
	if len(derive(domain.ActualFulfillmentOccurrence, long, one)) != len(forward) || !strings.HasPrefix(forward, "TCO-") {
		t.Fatalf("身份长度随依据涨或类别段不对：%q", forward)
	}
	if domain.FirstOccurrenceValidity().String() != "V1" {
		t.Fatalf("首版 = %q", domain.FirstOccurrenceValidity())
	}
	if _, err := domain.DeriveChargeOccurrenceReference(tenant, domain.ActualFulfillmentOccurrence, basis, nil); err == nil {
		t.Fatal("没有成员也派生出了身份")
	}
}

// Covers: ADR-0207 决定三——发生条件必须有人作答：断言缺席的规格立不起发生项；断言原话原样留在发生项上。
func TestAnOccurrenceCarriesTheAssertedCondition(t *testing.T) {
	if _, err := domain.NewAssertedOccurrenceCondition("  "); err == nil {
		t.Fatal("空断言立起来了")
	}
	spec := occurrenceSpec(t, "occurrence-1", "journey-1")
	spec.Condition = domain.OccurrenceCondition{}
	if _, err := domain.FormTransportChargeOccurrence(spec); !errors.Is(err, domain.ErrInvalidChargeOccurrence) {
		t.Fatalf("无断言 err = %v", err)
	}
	occurrence, err := domain.FormTransportChargeOccurrence(occurrenceSpec(t, "occurrence-1", "journey-1"))
	if err != nil {
		t.Fatal(err)
	}
	condition := occurrence.Condition()
	if assertion, asserted := condition.Assertion(); condition.Basis() != domain.ConditionAssertedByRegistrant || !asserted ||
		assertion != "agreement-snapshot-1 booking clause met" {
		t.Fatalf("条件 = %q %q %v", condition.Basis(), assertion, asserted)
	}
}

// Covers: ADR-0207 决定四的「异内容答冲突」——内容逐件比，成员按集合比；读回换了成员顺序不是另一件事。
func TestSameContentComparesEveryFieldAndMembersAsASet(t *testing.T) {
	original, err := domain.FormTransportChargeOccurrence(occurrenceSpec(t, "occurrence-1", "journey-1"))
	if err != nil {
		t.Fatal(err)
	}
	reordered := occurrenceSpec(t, "occurrence-1", "journey-1")
	reordered.Members = []domain.CarriedObjectReference{reordered.Members[1], reordered.Members[0]}
	same, err := domain.FormTransportChargeOccurrence(reordered)
	if err != nil {
		t.Fatal(err)
	}
	if !original.SameContentAs(same) {
		t.Fatal("成员换序被判成异内容")
	}
	changed := occurrenceSpec(t, "occurrence-1", "journey-1")
	changed.Quantity = 3
	other, err := domain.FormTransportChargeOccurrence(changed)
	if err != nil {
		t.Fatal(err)
	}
	if original.SameContentAs(other) {
		t.Fatal("数量不同被判成同内容")
	}
}

// Covers: 原因字面量是登记册、信封消费方与结算触发册共用的四个词，认回封闭集合，集外的词拒。
func TestChargeOccurrenceReasonsParseFromTheirLiterals(t *testing.T) {
	for _, reason := range []domain.ChargeOccurrenceReason{
		domain.BookingOccurrence, domain.CancellationOccurrence, domain.FailedAttemptOccurrence, domain.ActualFulfillmentOccurrence,
	} {
		parsed, err := domain.ParseChargeOccurrenceReason(reason.String())
		if err != nil || parsed != reason {
			t.Fatalf("%q 认回 %v %v", reason, parsed, err)
		}
	}
	if _, err := domain.ParseChargeOccurrenceReason("actual_fulfillment"); err == nil {
		t.Fatal("大小写被折叠了")
	}
}

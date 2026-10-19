package domain_test

import (
	"errors"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件的节点与对象都是合成串，证据只记 `S`。

func deliveryGateFinding(t *testing.T, object string, answer domain.GateAnswer) domain.GateFinding {
	t.Helper()
	finding, err := domain.NewGateFinding(mustValue(t, domain.NewCarriedObjectReference, object), answer, "AADJ-"+object,
		[]domain.GateBasis{{Action: "FINAL_DELIVERY", Cause: "OUTSIDE_CUSTOMS_BOOKS"}})
	if err != nil {
		t.Fatalf("那一格：%v", err)
	}
	return finding
}

func askedDeliveryReading(t *testing.T, object, node string, answer domain.GateAnswer) domain.DeliveryGateReading {
	t.Helper()
	reading, err := domain.FormDeliveryGateReading(domain.DeliveryGateReadingSpec{
		Node:    mustValue(t, domain.NewNetworkNodeReference, node),
		Finding: deliveryGateFinding(t, object, answer),
	})
	if err != nil {
		t.Fatalf("问到关务的读数：%v", err)
	}
	return reading
}

func stoppedDeliveryReading(t *testing.T, stop domain.DeliveryGateStop) domain.DeliveryGateReading {
	t.Helper()
	reading, err := domain.FormDeliveryGateReading(domain.DeliveryGateReadingSpec{Stop: stop})
	if err != nil {
		t.Fatalf("停点读数：%v", err)
	}
	return reading
}

// Covers: `AT-TF-256`、`AT-TF-260`–`AT-TF-262`、`AT-TF-265` 领域半边——问了关务的读数：整格就是那件对象的那一格，at 必在。停点与答复
// 恰有其一；停在这一侧的读数不声称问过哪个节点；「交付节点不可得」的几种停点都落未决，读不动的几口都落读不动；存量回填
// 那一种停点只由重建门收。
func TestADeliveryGateReadingIsEitherAStopOrOneAnswer(t *testing.T) {
	reading := askedDeliveryReading(t, "parcel-1", "SYN-NODE-SIN-HUB", domain.GateBlocked)
	if reading.Answer() != domain.GateBlocked {
		t.Fatalf("读数 = %s", reading.Answer())
	}
	if node, asked := reading.Node(); !asked || node.String() != "SYN-NODE-SIN-HUB" {
		t.Fatalf("at = %q (%v)", node, asked)
	}
	if _, stopped := reading.Stop(); stopped {
		t.Fatal("问了关务的读数带了停点")
	}

	node := mustValue(t, domain.NewNetworkNodeReference, "SYN-NODE-SIN-HUB")
	finding := deliveryGateFinding(t, "parcel-1", domain.GateNotGuarded)
	for name, spec := range map[string]domain.DeliveryGateReadingSpec{
		"neither":                {},
		"both":                   {Node: node, Stop: domain.DeliveryEnteredByPickup, Finding: finding},
		"asked without a node":   {Finding: finding},
		"stop claiming an ask":   {Node: node, Stop: domain.DeliveryEnteredByPickup},
		"backfill at formation":  {Stop: domain.DeliveryRecordedBeforeGateReading},
		"stop out of the closed": {Stop: domain.DeliveryGateStop(99)},
	} {
		if _, err := domain.FormDeliveryGateReading(spec); !errors.Is(err, domain.ErrInvalidDeliveryGateReading) {
			t.Errorf("%s：err = %v, want ErrInvalidDeliveryGateReading", name, err)
		}
	}

	for stop, want := range map[domain.DeliveryGateStop]domain.GateAnswer{
		domain.DeliveryNoActiveSegment:             domain.GateUndecided,
		domain.DeliveryEnteredByPickup:             domain.GateUndecided,
		domain.DeliveryReleasingNodeNotDeclared:    domain.GateUndecided,
		domain.DeliveryEntryHandoverUnresolved:     domain.GateUndecided,
		domain.DeliveryRecordedBeforeGateReading:   domain.GateUndecided,
		domain.DeliveryAttemptViewUnavailable:      domain.GateUnreadable,
		domain.DeliverySegmentRegistryUnavailable:  domain.GateUnreadable,
		domain.DeliveryHandoverRegistryUnavailable: domain.GateUnreadable,
	} {
		if stop.Answer() != want {
			t.Errorf("%s 属 %s，want %s", stop, stop.Answer(), want)
		}
		if stop == domain.DeliveryRecordedBeforeGateReading {
			continue
		}
		if reading := stoppedDeliveryReading(t, stop); reading.Answer() != want {
			t.Errorf("%s 的读数 = %s", stop, reading.Answer())
		}
	}

	rehydrated, err := domain.RehydrateDeliveryGateReading(
		domain.DeliveryGateReadingSpec{Stop: domain.DeliveryRecordedBeforeGateReading}, domain.GateUndecided)
	if err != nil || rehydrated.Answer() != domain.GateUndecided {
		t.Fatalf("存量回填重建：%v %s", err, rehydrated.Answer())
	}
	if _, err := domain.RehydrateDeliveryGateReading(
		domain.DeliveryGateReadingSpec{Stop: domain.DeliveryEnteredByPickup}, domain.GateBlocked); !errors.Is(err, domain.ErrInvalidDeliveryGateReading) {
		t.Fatalf("整格与停点对不上却装回了：%v", err)
	}
}

// Covers: `AT-TF-263`、`AT-TF-264` 领域半边——读数随交付走：带上后答得出；POD 更正照前版承继同一份（更正改的是证据，不是
// 在哪、何时、交了哪件）。读数属不属于这一份由归属门守：问的是另一件对象、没有读数、新交付声称存量回填，都是
// ErrDeliveryGateReadingMisplaced；重建门同一道，只多收存量回填。
func TestADeliveryCarriesItsGateReadingThroughAPODCorrection(t *testing.T) {
	delivery := formedDelivery(t, "parcel-1")
	if _, read := delivery.GateReading(); read {
		t.Fatal("刚形成的交付已经带了读数")
	}
	reading := askedDeliveryReading(t, "parcel-1", "SYN-NODE-SIN-HUB", domain.GateNotGuarded)
	gated, err := delivery.WithGateReading(reading)
	if err != nil {
		t.Fatalf("带读数：%v", err)
	}
	corrected, err := gated.CorrectProof(
		mustValue(t, domain.NewDeliveryProofReference, "pod-2"),
		mustValue(t, domain.NewDeliveryResultVersion, "delivery-result/parcel-1/v2"),
		gated.OccurredAt().Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("更正：%v", err)
	}
	if inherited, read := corrected.GateReading(); !read || !reflect.DeepEqual(inherited, reading) {
		t.Fatalf("更正版的读数 = %+v (%v)，want 承继前版那一份", inherited, read)
	}

	for name, misplaced := range map[string]domain.DeliveryGateReading{
		"no reading":            {},
		"another object":        askedDeliveryReading(t, "parcel-2", "SYN-NODE-SIN-HUB", domain.GateNotGuarded),
		"backfill on a new one": backfillDeliveryReading(t),
	} {
		if _, err := delivery.WithGateReading(misplaced); !errors.Is(err, domain.ErrDeliveryGateReadingMisplaced) {
			t.Errorf("%s：err = %v, want ErrDeliveryGateReadingMisplaced", name, err)
		}
	}

	spec := rehydrateDeliverySpecOf(t, gated)
	spec.GateReading = backfillDeliveryReading(t)
	if _, err := domain.RehydrateEffectiveDelivery(spec); err != nil {
		t.Fatalf("存量回填那一版装不回：%v", err)
	}
	spec.GateReading = domain.DeliveryGateReading{}
	if _, err := domain.RehydrateEffectiveDelivery(spec); !errors.Is(err, domain.ErrInvalidRehydratedDelivery) {
		t.Fatalf("没有读数的那一行装回了：%v", err)
	}
	spec.GateReading = askedDeliveryReading(t, "parcel-2", "SYN-NODE-SIN-HUB", domain.GateNotGuarded)
	if _, err := domain.RehydrateEffectiveDelivery(spec); !errors.Is(err, domain.ErrInvalidRehydratedDelivery) {
		t.Fatalf("读数问的是另一件对象的那一行装回了：%v", err)
	}
}

func backfillDeliveryReading(t *testing.T) domain.DeliveryGateReading {
	t.Helper()
	reading, err := domain.RehydrateDeliveryGateReading(
		domain.DeliveryGateReadingSpec{Stop: domain.DeliveryRecordedBeforeGateReading}, domain.GateUndecided)
	if err != nil {
		t.Fatalf("存量回填读数：%v", err)
	}
	return reading
}

func rehydrateDeliverySpecOf(t *testing.T, delivery domain.EffectiveDelivery) domain.RehydrateEffectiveDeliverySpec {
	t.Helper()
	reading, _ := delivery.GateReading()
	return domain.RehydrateEffectiveDeliverySpec{
		TenantID:    delivery.TenantID(),
		Object:      delivery.Object(),
		Attempt:     delivery.Attempt(),
		Place:       delivery.Place(),
		Method:      delivery.Method(),
		Recipient:   delivery.Recipient(),
		Proof:       delivery.Proof(),
		Version:     delivery.Version(),
		OccurredAt:  delivery.OccurredAt(),
		GateReading: reading,
	}
}

// Covers: `AT-TF-256`、`AT-TF-262` 领域半边——at 取对象进段那次交接：凭`已交接`进段的参与说得出那一版的版本，并认得出那一版（同对象、
// 入场依据就是它的转出依据、起点就是它的业务时间）；凭收寄进的参与没有交接版本。入场依据只带版本，同对象在别的交接范围
// 用了同一版本号、业务时间不同的那一版不认。
func TestAParticipationNamesTheHandoverItEnteredBy(t *testing.T) {
	reference := mustValue(t, domain.NewFulfillmentSegmentReference, "segment-1")
	object := mustValue(t, domain.NewCarriedObjectReference, "parcel-1")
	handover := formedHandover(t, "parcel-1", domain.ObjectHandedOver)
	segment, err := domain.EstablishSegmentWithHandover(reference, handover, domain.PlannedSegmentReference{})
	if err != nil {
		t.Fatalf("establish: %v", err)
	}
	participation, _ := segment.ParticipationFor(object)
	version, named := participation.EntryHandoverVersion()
	if !named || version != handover.Version() {
		t.Fatalf("入场交接版本 = %q (%v)，want %q", version, named, handover.Version())
	}
	if !participation.EnteredByHandover(handover) {
		t.Fatal("认不出进段的那一版交接")
	}

	elsewhere := handoverSpec(t, "parcel-1", domain.ObjectHandedOver)
	elsewhere.Scope = mustValue(t, domain.NewHandoverScopeReference, "handover-scope-2")
	elsewhere.JudgedAt = handoverJudgedAt.Add(time.Hour)
	other, err := domain.FormTransportHandover(elsewhere)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if participation.EnteredByHandover(other) {
		t.Fatal("别的交接范围里同版本号、不同业务时间的那一版被认成了进段那一次")
	}
	if participation.EnteredByHandover(formedHandover(t, "parcel-1", domain.HandoverRefused)) {
		t.Fatal("拒收被认成了进段那一次")
	}

	picked, err := domain.EstablishSegmentWithPickup(reference, formedPickup(t, "parcel-1", "attempt-1"), domain.PlannedSegmentReference{})
	if err != nil {
		t.Fatalf("establish by pickup: %v", err)
	}
	byPickup, _ := picked.ParticipationFor(object)
	if _, named := byPickup.EntryHandoverVersion(); named {
		t.Fatal("凭收寄进段的参与说出了一个交接版本")
	}
}

package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件的节点与对象都是合成串，证据只记 `S`。

func handoverGateFinding(t *testing.T, object string, answer domain.GateAnswer) domain.GateFinding {
	t.Helper()
	finding, err := domain.NewGateFinding(mustValue(t, domain.NewCarriedObjectReference, object), answer, "AADJ-"+object,
		[]domain.GateBasis{{Action: "OUTBOUND_RELEASE", Cause: "OUTSIDE_CUSTOMS_BOOKS"}})
	if err != nil {
		t.Fatalf("那一格：%v", err)
	}
	return finding
}

func releasedHandover(t *testing.T, object, releasing, onward string) domain.TransportHandover {
	t.Helper()
	spec := handoverSpec(t, object, domain.ObjectHandedOver)
	spec.Nodes = handoverNodes(t, releasing, "", onward)
	handover, err := domain.FormTransportHandover(spec)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	return handover
}

func askedHandoverReading(t *testing.T, object, releasing, onward string, answer domain.GateAnswer) domain.HandoverGateReading {
	t.Helper()
	spec := domain.HandoverGateReadingSpec{
		Node:    mustValue(t, domain.NewNetworkNodeReference, releasing),
		Finding: handoverGateFinding(t, object, answer),
	}
	if onward != "" {
		spec.Onward = mustValue(t, domain.NewNetworkNodeReference, onward)
	}
	reading, err := domain.FormHandoverGateReading(spec)
	if err != nil {
		t.Fatalf("问到关务的读数：%v", err)
	}
	return reading
}

// Covers: AT-TF-236、AT-TF-238 领域半边——问了关务的读数：整格就是那件对象的那一格；交出节点必在，去向可缺，缺了就写明
// 去向未声明。停点与答复恰有其一；停在这一侧的读数不声称问过哪一份意图；存量回填那一种停点只由重建门收。
func TestAHandoverGateReadingIsEitherAStopOrOneAnswer(t *testing.T) {
	reading := askedHandoverReading(t, "parcel-1", "SYN-NODE-SZX-GATE", "", domain.GateBlocked)
	if reading.Answer() != domain.GateBlocked || !reading.OnwardNotDeclared() {
		t.Fatalf("读数 = %s onwardNotDeclared=%v", reading.Answer(), reading.OnwardNotDeclared())
	}
	if declared := askedHandoverReading(t, "parcel-1", "SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB", domain.GateUndecided); declared.OnwardNotDeclared() {
		t.Fatalf("声明了去向却写成未声明")
	}

	node := mustValue(t, domain.NewNetworkNodeReference, "SYN-NODE-SZX-GATE")
	finding := handoverGateFinding(t, "parcel-1", domain.GateNotGuarded)
	for name, spec := range map[string]domain.HandoverGateReadingSpec{
		"neither":                {},
		"both":                   {Node: node, Stop: domain.HandoverReleasingNodeNotDeclared, Finding: finding},
		"asked without a node":   {Finding: finding},
		"stop claiming an ask":   {Node: node, Stop: domain.HandoverReleasingNodeNotDeclared},
		"backfill at formation":  {Stop: domain.HandoverRecordedBeforeGateReading},
		"stop out of the closed": {Stop: domain.HandoverGateStop(99)},
	} {
		if _, err := domain.FormHandoverGateReading(spec); !errors.Is(err, domain.ErrInvalidHandoverGateReading) {
			t.Errorf("%s：err = %v, want ErrInvalidHandoverGateReading", name, err)
		}
	}

	for stop, want := range map[domain.HandoverGateStop]domain.GateAnswer{
		domain.HandoverReleasingNodeNotDeclared:  domain.GateUndecided,
		domain.HandoverRecordedBeforeGateReading: domain.GateUndecided,
		domain.HandoverGateRegistryUnavailable:   domain.GateUnreadable,
	} {
		if stop.Answer() != want {
			t.Errorf("%s 属 %s，want %s", stop, stop.Answer(), want)
		}
	}
}

// Covers: 重建门核存下的整格与停点、答复推出的那一格一致，收存量回填那一种停点。
func TestTheHandoverGateRehydrationDoorChecksTheRecordedAnswer(t *testing.T) {
	backfill := domain.HandoverGateReadingSpec{Stop: domain.HandoverRecordedBeforeGateReading}
	if reading, err := domain.RehydrateHandoverGateReading(backfill, domain.GateUndecided); err != nil || reading.Answer() != domain.GateUndecided {
		t.Fatalf("存量回填：%v", err)
	}
	if _, err := domain.RehydrateHandoverGateReading(backfill, domain.GateNotGuarded); !errors.Is(err, domain.ErrInvalidHandoverGateReading) {
		t.Fatalf("整格与停点对不上却装回了：%v", err)
	}
	asked := domain.HandoverGateReadingSpec{
		Node:    mustValue(t, domain.NewNetworkNodeReference, "SYN-NODE-SZX-GATE"),
		Finding: handoverGateFinding(t, "parcel-1", domain.GateAdmitted),
	}
	if _, err := domain.RehydrateHandoverGateReading(asked, domain.GateBlocked); !errors.Is(err, domain.ErrInvalidHandoverGateReading) {
		t.Fatalf("整格与那一格对不上却装回了：%v", err)
	}
}

// Covers: AT-TF-237、AT-TF-239、AT-TF-244 领域半边——「没声明交出节点就不问」只在 UnaskedGateReading 定义一处；读数只挂在
// `已交接`上，问的对象与意图必须是这一版的；「交接登记册读不动」只属更正版；更正版不承继前版的读数。
func TestAHandoverGateReadingBelongsToItsHandedOverVersion(t *testing.T) {
	unaskedHandover := releasedHandover(t, "parcel-1", "", "SYN-NODE-SIN-HUB")
	unasked, determined := unaskedHandover.UnaskedGateReading()
	if stop, _ := unasked.Stop(); !determined || stop != domain.HandoverReleasingNodeNotDeclared || unasked.Answer() != domain.GateUndecided {
		t.Fatalf("没声明交出节点时定不出读数：%+v", unasked)
	}
	if _, determined := releasedHandover(t, "parcel-1", "SYN-NODE-SZX-GATE", "").UnaskedGateReading(); determined {
		t.Fatalf("声明了交出节点却不必问关务")
	}
	refusedSpec := handoverSpec(t, "parcel-1", domain.HandoverRefused)
	refused, err := domain.FormTransportHandover(refusedSpec)
	if err != nil {
		t.Fatalf("form: %v", err)
	}
	if _, determined := refused.UnaskedGateReading(); determined {
		t.Fatalf("拒收也有读数")
	}

	gate := releasedHandover(t, "parcel-1", "SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB")
	placed, err := gate.WithGateReading(askedHandoverReading(t, "parcel-1", "SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB", domain.GateUndecided))
	if err != nil {
		t.Fatalf("属于这一版的读数挂不上：%v", err)
	}
	if reading, read := placed.GateReading(); !read || reading.Answer() != domain.GateUndecided {
		t.Fatalf("挂上的读数 = %+v", reading)
	}
	if _, read := gate.GateReading(); read {
		t.Fatalf("WithGateReading 改了接收者")
	}

	registryStop, err := domain.FormHandoverGateReading(domain.HandoverGateReadingSpec{Stop: domain.HandoverGateRegistryUnavailable})
	if err != nil {
		t.Fatalf("读不动停点：%v", err)
	}
	for name, try := range map[string]func() (domain.TransportHandover, error){
		"refusal": func() (domain.TransportHandover, error) {
			return refused.WithGateReading(unasked)
		},
		"zero reading": func() (domain.TransportHandover, error) {
			return gate.WithGateReading(domain.HandoverGateReading{})
		},
		"another object": func() (domain.TransportHandover, error) {
			return gate.WithGateReading(askedHandoverReading(t, "parcel-2", "SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB", domain.GateNotGuarded))
		},
		"another releasing node": func() (domain.TransportHandover, error) {
			return gate.WithGateReading(askedHandoverReading(t, "parcel-1", "SYN-NODE-SHA-HUB", "SYN-NODE-SIN-HUB", domain.GateNotGuarded))
		},
		"onward dropped": func() (domain.TransportHandover, error) {
			return gate.WithGateReading(askedHandoverReading(t, "parcel-1", "SYN-NODE-SZX-GATE", "", domain.GateNotGuarded))
		},
		"unasked on a declared releasing node": func() (domain.TransportHandover, error) {
			return gate.WithGateReading(unasked)
		},
		"registry stop on a first registration": func() (domain.TransportHandover, error) {
			return gate.WithGateReading(registryStop)
		},
	} {
		if _, err := try(); !errors.Is(err, domain.ErrHandoverGateReadingMisplaced) {
			t.Errorf("%s：err = %v, want ErrHandoverGateReadingMisplaced", name, err)
		}
	}

	corrected, err := placed.Correct(domain.HandoverCorrection{
		Verdict:           domain.ObjectHandedOver,
		ReleasingEvidence: mustValue(t, domain.NewHandoverEvidenceReference, "release-2"),
		ReceivingEvidence: mustValue(t, domain.NewHandoverEvidenceReference, "receive-2"),
		Rule:              mustValue(t, domain.NewHandoverRuleReference, "rule-v1"),
		Version:           mustValue(t, domain.NewHandoverResultVersion, "v2"),
		CorrectedAt:       handoverJudgedAt.Add(1),
	})
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	if _, read := corrected.GateReading(); read {
		t.Fatalf("更正版承继了前版的读数")
	}
	if _, err := corrected.WithGateReading(registryStop); err != nil {
		t.Fatalf("更正版挂不上「交接登记册读不动」：%v", err)
	}
}

// Covers: 交接的重建门——`已交接`必带读数且读数属于这一版（存量回填那一种不按节点核），拒收与待确认不带。
func TestTheHandoverRehydrationDoorRequiresAGateReadingExactlyOnHandedOver(t *testing.T) {
	gate := releasedHandover(t, "parcel-1", "SYN-NODE-SZX-GATE", "")
	spec := func(verdict domain.HandoverVerdict, reading domain.HandoverGateReading) domain.RehydrateTransportHandoverSpec {
		rehydrate := domain.RehydrateTransportHandoverSpec{
			TenantID:    gate.TenantID(),
			Object:      gate.Object(),
			Scope:       gate.Scope(),
			ReleasedBy:  gate.ReleasedBy(),
			ReceivedBy:  gate.ReceivedBy(),
			Verdict:     verdict,
			Version:     gate.Version(),
			JudgedAt:    gate.JudgedAt(),
			Nodes:       gate.Nodes(),
			GateReading: reading,
		}
		if verdict == domain.ObjectHandedOver {
			rehydrate.ReleasingEvidence, _ = gate.ReleasingEvidence()
			rehydrate.ReceivingEvidence, _ = gate.ReceivingEvidence()
			rehydrate.Rule, _ = gate.Rule()
		} else {
			rehydrate.Basis = mustValue(t, domain.NewHandoverBasisReference, "basis-1")
		}
		return rehydrate
	}
	backfill, err := domain.RehydrateHandoverGateReading(
		domain.HandoverGateReadingSpec{Stop: domain.HandoverRecordedBeforeGateReading}, domain.GateUndecided)
	if err != nil {
		t.Fatalf("回填：%v", err)
	}
	asked := askedHandoverReading(t, "parcel-1", "SYN-NODE-SZX-GATE", "", domain.GateNotGuarded)

	for name, ok := range map[string]domain.RehydrateTransportHandoverSpec{
		"asked":    spec(domain.ObjectHandedOver, asked),
		"backfill": spec(domain.ObjectHandedOver, backfill),
		"refused":  spec(domain.HandoverRefused, domain.HandoverGateReading{}),
	} {
		if _, err := domain.RehydrateTransportHandover(ok); err != nil {
			t.Errorf("%s：%v", name, err)
		}
	}
	for name, bad := range map[string]domain.RehydrateTransportHandoverSpec{
		"handed over without a reading": spec(domain.ObjectHandedOver, domain.HandoverGateReading{}),
		"refused with a reading":        spec(domain.HandoverRefused, backfill),
		"reading for another node":      spec(domain.ObjectHandedOver, askedHandoverReading(t, "parcel-1", "SYN-NODE-SHA-HUB", "", domain.GateNotGuarded)),
	} {
		if _, err := domain.RehydrateTransportHandover(bad); !errors.Is(err, domain.ErrInvalidRehydratedHandover) {
			t.Errorf("%s：err = %v, want ErrInvalidRehydratedHandover", name, err)
		}
	}
}

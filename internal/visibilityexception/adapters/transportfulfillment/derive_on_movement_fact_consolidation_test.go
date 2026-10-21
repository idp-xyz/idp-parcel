package transportfulfillment_test

import (
	"testing"

	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// bagDepartureRecord 是 move-1 / v1 的另一种读数：段里在控的 parcel-1 其实是一袋，袋里装着被扣的 PCL-HELD 与干净的 PCL-CLEAN；
// 同段的 parcel-2 不受门禁。
func bagDepartureRecord(t *testing.T) tfports.MovementFactRecord {
	t.Helper()
	record := movementRecord(t, tfdomain.DepartureFact)
	asked := func(object string, answer tfdomain.GateAnswer) tfdomain.GateFinding {
		built, err := tfdomain.NewGateFinding(movementValue(t, tfdomain.NewCarriedObjectReference, object), answer,
			"AADJ-"+object, []tfdomain.GateBasis{{Action: "LOADING_DEPARTURE", Cause: "SOME_CAUSE"}})
		if err != nil {
			t.Fatalf("逐对象那一格：%v", err)
		}
		return built
	}
	held, clean := asked("PCL-HELD", tfdomain.GateBlocked), asked("PCL-CLEAN", tfdomain.GateNotGuarded)
	bag, err := tfdomain.FormConsolidationUnitGateFinding(movementValue(t, tfdomain.NewCarriedObjectReference, "parcel-1"),
		[]tfdomain.GateFinding{held, clean}, nil)
	if err != nil {
		t.Fatalf("袋那一格：%v", err)
	}
	reading, err := tfdomain.FormDepartureGateReading(tfdomain.DepartureGateReadingSpec{
		From:     movementValue(t, tfdomain.NewNetworkNodeReference, "NODE-SHA"),
		To:       movementValue(t, tfdomain.NewNetworkNodeReference, "NODE-SZX"),
		Findings: []tfdomain.GateFinding{bag, held, clean, asked("parcel-2", tfdomain.GateNotGuarded)},
	})
	if err != nil {
		t.Fatalf("读数：%v", err)
	}
	spec := tfdomain.MovementFactSpec{
		TenantID: record.Fact.TenantID(), Fact: record.Fact.Fact(), Schedule: record.Fact.Schedule(), Kind: tfdomain.DepartureFact,
		Location: record.Fact.Location(), Source: record.Fact.Source(), Version: record.Fact.Version(),
		OccurredAt: record.Fact.OccurredAt(), GateReading: reading,
	}
	spec.Segment, _ = record.Fact.Segment()
	fact, err := tfdomain.RecordMovementFact(spec)
	if err != nil {
		t.Fatalf("事实：%v", err)
	}
	record.Fact = fact
	return record
}

// Covers: AT-VE-183（ADR-0225 决定七）——出发读数里袋那一格阻断时，信号落在袋里被关务判为阻断的那一件上，袋那一格不另形成信号：
// 它是由成员推出来的，再立一次就是同一次违规在分诊里记两回。
func TestABlockedBagSignalsTheBlockedMemberNotTheBag(t *testing.T) {
	fixture := newMovementFixture(t, bagDepartureRecord(t))
	if err := fixture.subject.HandleRecordedMovementFact(t.Context(), recordedMovementRef()); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if len(fixture.signals.parcels) != 1 || fixture.signals.parcels[0] != "PCL-HELD" {
		t.Fatalf("交进信号编排的 = %v，要只有袋里被扣的 PCL-HELD", fixture.signals.parcels)
	}
}

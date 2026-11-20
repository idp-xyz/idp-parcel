package main

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
)

// 本文件对真库证生产装配里的实测变化重判：称重不带来新位置，在已经判过的控制触发上再判一次；
// 先到的实测变化等位置，收寄复核落成之后补判。夹具为隔离合成，只记 S。

func measurementChangeLane(t *testing.T, syn *synVerticalFixture) dispatch.Consumer {
	t.Helper()
	store, err := inbox.NewStore(syn.db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	purpose := mustNR(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE")
	consumer, err := reassessOnMeasurementChangeConsumer(syn.db, store, purpose, systemClock{})
	if err != nil {
		t.Fatalf("装配实测变化复核：%v", err)
	}
	return consumer
}

func consumeMeasurementChange(
	t *testing.T, syn *synVerticalFixture, lane dispatch.Consumer, unit, sourceID string, at time.Time,
) {
	t.Helper()
	payload := []byte(`{"tenantId":"` + demoTenant + `","sourceId":"` + sourceID +
		`","objectKind":"HANDLING_UNIT","objectId":"` + unit + `"}`)
	if err := lane.Consume(t.Context(), eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID("measurement-change/" + sourceID),
		Source:       "idp-parcel/node-operations",
		Type:         nrinbox.ActualMeasurementChangedEventType,
		Version:      1,
		Scope:        demoTenant,
		Subject:      unit,
		PartitionKey: demoTenant + "/" + unit,
		OccurredAt:   at,
		RecordedAt:   at,
		ContentType:  eventing.JSONContentType,
		Payload:      payload,
	}); err != nil {
		t.Fatalf("消费实测变化 %s：%v；未判 = %+v", sourceID, err, notJudgedRows(t, syn))
	}
}

func measurementCorrelation(t *testing.T, sourceID string) string {
	t.Helper()
	correlation, err := nrdomain.NewMeasurementChangeCorrelation(
		mustNR(t, nrdomain.NewTenantID, demoTenant),
		mustNR(t, nrdomain.NewDeclaredParcelID, reassessHandoverParcel),
		sourceID,
	)
	if err != nil {
		t.Fatal(err)
	}
	return correlation.String()
}

func saveJudgedAnchor(t *testing.T, syn *synVerticalFixture, correlation, location string, at time.Time) {
	t.Helper()
	record := nrports.ReassessmentRecord{
		Correlation:       mustNR(t, nrdomain.NewRequestCorrelationID, correlation),
		Key:               reassessHandoverKey(t),
		Conclusion:        nrports.ReassessmentStillApplicable,
		ReviewedPlan:      mustNR(t, nrdomain.NewRoutePlanVersionID, "SYN-RPV-63"),
		TriggerControl:    nrdomain.NodeIntakeControl,
		TriggerLocation:   mustNR(t, nrdomain.NewActualLocationReference, location),
		TriggerOccurredAt: at,
		ReassessedAt:      at,
	}
	store, err := nrpostgres.NewRouteReassessments(syn.db)
	if err != nil {
		t.Fatal(err)
	}
	mustWithinTX(t, syn.transactor, t.Context(), func(txCtx context.Context) error {
		saved, err := store.Save(txCtx, record.Correlation, record)
		if err != nil {
			return err
		}
		if saved != nrports.ReassessmentSaved {
			t.Errorf("落下锚点 = %v, want SAVED", saved)
		}
		return nil
	})
}

// Covers: 收寄已判的包裹，登记一条实测并让生产装配里的这只消费者消费那封信封：复核册多一条重判，
// 控制依据、位置、业务时间等于锚点；锚点不在计划上，失效线向取数侧要成本，候选事实出处是 PHYSICAL。
func TestAMeasurementChangeRejudgesTheJudgedIntakeThroughTheProductionConsumer(t *testing.T) {
	syn := newSYNVerticalFixture(t)
	acceptDemoParcelWithGeoAndWeight(t, syn)
	registerDemoNetworkSeeds(t, syn.db)
	registerCostCard(t, syn.db, demoShapedCostCard(t, "CNY", ppdomain.PricingPlanStructures{}))
	seedWiringCustomsCatalog(t, syn.db, demoTenant, true)
	savePlan(t, syn, "SYN-LINE-CN-SG-01@1",
		"SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE", reassessHandoverReceiving, "SYN-NODE-SIN-LM")

	anchorAt := reassessHandoverAt.Add(-time.Hour)
	anchorLocation := "SYN-NODE-KUL-HUB"
	saveJudgedAnchor(t, syn, "intake-already-judged", anchorLocation, anchorAt)

	unit := "SYN-HU-RMR-01"
	sourceID := "SYN-RMR-MEASURE-01"
	associateDemoParcel(t, syn, unit, "SYN-RMR-RECEPTION-01", "v1")
	recordDemoMeasurement(t, syn, unit, sourceID, "7", time.Now().UTC().Add(-2*time.Hour).Truncate(time.Second))

	consumeMeasurementChange(t, syn, measurementChangeLane(t, syn), unit, sourceID, time.Now().UTC().Truncate(time.Second))

	got := reassessmentOf(t, syn, measurementCorrelation(t, sourceID))
	if got.TriggerControl != nrdomain.NodeIntakeControl || got.TriggerLocation.String() != anchorLocation ||
		!got.TriggerOccurredAt.Equal(anchorAt) {
		t.Fatalf("重判锚点 = %s %s %s，want 节点收寄 %s %s",
			got.TriggerControl, got.TriggerLocation, got.TriggerOccurredAt, anchorLocation, anchorAt)
	}
	if got.CandidateFactsBasis != nrdomain.ParcelFactsPhysical {
		t.Fatalf("候选事实出处 = %s，want PHYSICAL（结论 %s）", got.CandidateFactsBasis, got.Conclusion)
	}
}

// Covers: 先消费实测变化，当时还没有控制位置，入账未判；再消费收寄，收寄那条与补判那条都在册，
// 补判抄的是这次收寄的控制依据、位置与业务时间。
func TestAMeasurementChangeWaitingForAPositionIsCaughtUpWhenIntakeIsJudged(t *testing.T) {
	fixture := newReassessBothLanesFixture(t)
	unit := "SYN-HU-RMR-02"
	sourceID := "SYN-RMR-MEASURE-02"
	associateDemoParcel(t, fixture.syn, unit, "SYN-RMR-RECEPTION-02", "v1")

	changedAt := reassessHandoverAt.Add(-time.Hour)
	consumeMeasurementChange(t, fixture.syn, measurementChangeLane(t, fixture.syn), unit, sourceID, changedAt)
	if rows := reassessmentRows(t, fixture.syn); len(rows) != 0 {
		t.Fatalf("还没有控制位置却落下了复核：%+v", rows)
	}
	waiting := notJudgedRows(t, fixture.syn)
	correlation := measurementCorrelation(t, sourceID)
	if len(waiting) != 1 || waiting[0].correlation != correlation || waiting[0].reason != "CONTROL_POSITION_UNAVAILABLE" {
		t.Fatalf("未判 = %+v，want 这一条等位置", waiting)
	}

	intakeAt := reassessHandoverAt.Add(-48 * time.Hour)
	intake := fixture.intakeAt(t, psdomain.NodeIntakeSource, "SYN-INTAKE/132/v1", "SYN-NODE-SHA-HUB", intakeAt)
	deliver(t, fixture.syn, fixture.intake, intake, intake)

	intakeCorrelation := "intake/SYN-TENANT-01/SYN-PARCEL-01/NODE_INTAKE/SYN-INTAKE/132/v1"
	conclusions := conclusionsByCorrelation(t, fixture.syn)
	if conclusions[intakeCorrelation] == "" || conclusions[correlation] == "" {
		t.Fatalf("复核 = %v，want 收寄与补判各一条", conclusions)
	}
	caught := reassessmentOf(t, fixture.syn, correlation)
	if caught.TriggerControl != nrdomain.NodeIntakeControl || caught.TriggerLocation.String() != "SYN-NODE-SHA-HUB" ||
		!caught.TriggerOccurredAt.Equal(intakeAt) {
		t.Fatalf("补判锚点 = %s %s %s，want 这次收寄 SYN-NODE-SHA-HUB %s",
			caught.TriggerControl, caught.TriggerLocation, caught.TriggerOccurredAt, intakeAt)
	}
}

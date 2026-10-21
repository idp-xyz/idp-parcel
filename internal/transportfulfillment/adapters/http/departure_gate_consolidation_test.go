package tfhttp_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// bagMemberships 让段里在控的 parcel-1 是一袋，袋里装着 PCL-A 与 PCL-B。
func bagMemberships(t *testing.T) movementMembershipsDouble {
	t.Helper()
	return movementMembershipsDouble{units: map[string][]ports.ConsolidationMember{
		"parcel-1": {
			{Kind: ports.ConsolidationMemberParcel, Handling: "hu-a", Object: httpValue(t, domain.NewCarriedObjectReference, "PCL-A")},
			{Kind: ports.ConsolidationMemberParcel, Handling: "hu-b", Object: httpValue(t, domain.NewCarriedObjectReference, "PCL-B")},
		},
	}}
}

// Covers: AT-TF-276 线格式半边（ADR-0225 决定六）——袋那一格在逐对象那几格里带 `members`；成员那几格照旧只有对象与答复。袋里一件
// 阻断、一件不受门禁：袋那一格与被挡那一件在列，`members` 只指名被挡那一件，不受门禁那一件哪儿都不出现（AT-TF-318 线格式半边，
// ADR-0237）。出发与问询是同一个线格式，问询里的那一段逐字节相同（AT-TF-280 线格式半边）。
func TestTheBagCellListsItsMembersOnTheWire(t *testing.T) {
	customs := &movementCustomsDouble{answer: domain.GateBlocked, answers: map[string]domain.GateAnswer{"PCL-B": domain.GateNotGuarded}}
	handler := application.NewRecordMovementFactHandler(application.RecordMovementFactDeps{
		Facts:   newMovementFactRegistry(),
		Handoff: movementHandoffDouble{},
		Clock:   tfClock{at: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)},
	})
	record := tfhttp.NewRecordMovementFactEndpoint(
		&movementFactIntakeDouble{tenant: httpValue(t, domain.NewTenantID, "tenant-1")},
		readThenRecord{gate: movementGateReaderOver(t, customs, bagMemberships(t)), inner: handler})
	departure := defaultMovementFactBody()
	departure.Kind, departure.Fact, departure.Segment = "DEPARTURE", "movement-fact-bag-1", "segment-1"

	departed := postTo(t, record, "/transport-fulfillment/movement-facts", departure)
	if departed.Code != http.StatusCreated {
		t.Fatalf("status = %d; body = %s", departed.Code, departed.Body.String())
	}
	reading := decodeMovementFact(t, departed).GateReading
	if reading == nil || reading.Answer != "BLOCKED" || len(reading.Objects) != 2 {
		t.Fatalf("gateReading = %s", departed.Body.String())
	}
	cells := map[string]map[string]any{}
	for _, object := range reading.Objects {
		cells[object["object"].(string)] = object
	}
	members, _ := json.Marshal(cells["parcel-1"]["members"])
	if string(members) != `["PCL-A"]` || cells["parcel-1"]["answer"] != "BLOCKED" {
		t.Fatalf("袋那一格 = %+v", cells["parcel-1"])
	}
	if len(cells["PCL-A"]) != 2 || cells["PCL-A"]["answer"] != "BLOCKED" {
		t.Fatalf("成员那一格只该有对象与答复：%+v", cells["PCL-A"])
	}
	if strings.Contains(departed.Body.String(), "PCL-B") {
		t.Fatalf("不受门禁的成员出现在答复里：%s", departed.Body.String())
	}

	preview, err := application.NewPreviewDepartureGateHandler(movementGateReaderOver(t, customs, bagMemberships(t)))
	if err != nil {
		t.Fatalf("装问询编排：%v", err)
	}
	previewed := servePreview(tfhttp.NewPreviewDepartureGateEndpoint(workFactIntakeForTest(t), preview),
		commandRequest(`{"schedule":"schedule-1","segment":"segment-1","departsAt":"2026-09-05T10:00:00Z"}`))
	if previewed.Code != http.StatusOK || rawGateReading(t, previewed) != rawGateReading(t, departed) {
		t.Fatalf("问询 %d：%s，出发：%s", previewed.Code, rawGateReading(t, previewed), rawGateReading(t, departed))
	}
}

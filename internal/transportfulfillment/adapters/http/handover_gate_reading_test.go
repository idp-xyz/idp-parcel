package tfhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证交出门禁读数（ADR-0213 决定四）在线格式上的样子：`已交接`的答复带 gateReading，与出发读数同形；拒收不带；
// 阻断照样 201；重放回显首次读数。关务一侧是替身，证据只记 `S`。

// customsGateStub 照答一格；记下问过几次。
type customsGateStub struct {
	answer domain.GateAnswer
	asked  int
}

func (stub *customsGateStub) ReadHandoverGate(_ context.Context, query ports.HandoverGateQuery) (domain.GateFinding, error) {
	stub.asked++
	basis := domain.GateBasis{Action: "CROSS_CUSTOMS_MOVEMENT", Cause: "CUSTOMS_SCOPE_UNRESOLVED"}
	if stub.answer == domain.GateNotGuarded {
		basis = domain.GateBasis{Action: "OUTBOUND_RELEASE", Cause: "OUTSIDE_CUSTOMS_BOOKS"}
	}
	return domain.NewGateFinding(query.Object, stub.answer, "AADJ-STUB", []domain.GateBasis{basis})
}

// gatedHandover 照组合根（cmd/parcel-api 的 transactionalHandover）的次序：写事务之前认重放或问读数，再带着它登。本包不碰
// 事务，只留次序。
type gatedHandover struct {
	inner *application.RegisterTransportHandoverHandler
	gate  *application.HandoverGateReader
}

func (handler gatedHandover) Register(
	ctx context.Context,
	command application.RegisterTransportHandoverCommand,
) (application.RegisterTransportHandoverResult, error) {
	reading, replayed := handler.inner.RecordedHandoverGateReading(ctx, command)
	if !replayed {
		var err error
		if reading, err = handler.gate.ReadRegistration(ctx, command); err != nil {
			return application.RegisterTransportHandoverResult{}, err
		}
	}
	return handler.inner.RegisterWithGateReading(ctx, command, reading)
}

func (handler gatedHandover) Correct(
	ctx context.Context,
	command application.CorrectTransportHandoverCommand,
) (application.RegisterTransportHandoverResult, error) {
	reading, replayed := handler.inner.RecordedCorrectionGateReading(ctx, command)
	if !replayed {
		var err error
		if reading, err = handler.gate.ReadCorrection(ctx, command); err != nil {
			return application.RegisterTransportHandoverResult{}, err
		}
	}
	return handler.inner.CorrectWithGateReading(ctx, command, reading)
}

func gated(t *testing.T, registry *handoverRegistryDouble, inner *application.RegisterTransportHandoverHandler, customs ports.HandoverGateView) tfhttp.HandoverHandler {
	t.Helper()
	gate, err := application.NewHandoverGateReader(application.HandoverGateReadDeps{Handovers: registry, Customs: customs})
	if err != nil {
		t.Fatalf("交出门禁读数：%v", err)
	}
	return gatedHandover{inner: inner, gate: gate}
}

type gateReadingAnswer struct {
	Answer            string   `json:"answer"`
	At                string   `json:"at"`
	Onward            string   `json:"onward"`
	OnwardNotDeclared bool     `json:"onwardNotDeclared"`
	Stops             []string `json:"stops"`
	Objects           []struct {
		Object string `json:"object"`
		Answer string `json:"answer"`
	} `json:"objects"`
}

func gateReadingOf(t *testing.T, fields map[string]json.RawMessage) (gateReadingAnswer, bool) {
	t.Helper()
	raw, present := fields["gateReading"]
	if !present {
		return gateReadingAnswer{}, false
	}
	var reading gateReadingAnswer
	if err := json.Unmarshal(raw, &reading); err != nil {
		t.Fatalf("gateReading 解不开：%s", raw)
	}
	return reading, true
}

func gateBody(t *testing.T, object string) string {
	t.Helper()
	body := bodyWith(t, handoverRegistrationBody, "segment", nil)
	body = bodyWith(t, body, "plannedSegment", nil)
	body = bodyWith(t, body, "segmentServiceAction", nil)
	body = bodyWith(t, body, "basis", nil)
	return bodyWith(t, body, "object", object)
}

// Covers: AT-TF-245——`已交接`的答复带交出门禁读数：整格、问关务时的 at / onward 与那件对象的那一格，不带关务判断标识与
// 依据原文；阻断照样 201；没声明去向写明 onwardNotDeclared；没声明交出节点是停点「交出节点未声明」；拒收不带读数；原样
// 重发回显首次读数、不重问关务。
func TestTheHandoverEndpointRepliesWithTheGateReading(t *testing.T) {
	customs := &customsGateStub{answer: domain.GateBlocked}
	endpoint := nodeHandoverEndpointAsking(t, customs)

	toTrunk := bodyWith(t, bodyWith(t, gateBody(t, "SYN-PARCEL-65-01"), "releasingNode", "SYN-NODE-SZX-GATE"), "onwardNode", "SYN-NODE-SIN-HUB")
	status, fields := handoverAnswer(t, endpoint, toTrunk)
	reading, present := gateReadingOf(t, fields)
	if status != http.StatusCreated || !present || reading.Answer != "BLOCKED" || reading.At != "SYN-NODE-SZX-GATE" ||
		reading.Onward != "SYN-NODE-SIN-HUB" || reading.OnwardNotDeclared || len(reading.Stops) != 0 ||
		len(reading.Objects) != 1 || reading.Objects[0].Object != "SYN-PARCEL-65-01" || reading.Objects[0].Answer != "BLOCKED" {
		t.Fatalf("阻断的交出 status = %d gateReading = %s", status, fields["gateReading"])
	}
	for _, leaked := range []string{"AADJ-STUB", "CROSS_CUSTOMS_MOVEMENT"} {
		if strings.Contains(string(fields["gateReading"]), leaked) {
			t.Fatalf("答复带出了关务依据原文 %q：%s", leaked, fields["gateReading"])
		}
	}

	customs.answer = domain.GateNotGuarded
	status, fields = handoverAnswer(t, endpoint, toTrunk)
	if replayed, _ := gateReadingOf(t, fields); status != http.StatusOK || replayed.Answer != "BLOCKED" || customs.asked != 1 {
		t.Fatalf("原样重发 status = %d gateReading = %s，关务问了 %d 次", status, fields["gateReading"], customs.asked)
	}

	status, fields = handoverAnswer(t, endpoint, bodyWith(t, gateBody(t, "SYN-PARCEL-65-02"), "releasingNode", "SYN-NODE-SZX-GATE"))
	if reading, _ := gateReadingOf(t, fields); status != http.StatusCreated || reading.Answer != "NOT_GUARDED" || !reading.OnwardNotDeclared || reading.Onward != "" {
		t.Fatalf("没声明去向 status = %d gateReading = %s", status, fields["gateReading"])
	}

	status, fields = handoverAnswer(t, endpoint, gateBody(t, "SYN-PARCEL-65-03"))
	if reading, _ := gateReadingOf(t, fields); status != http.StatusCreated || reading.Answer != "UNDECIDED" ||
		len(reading.Stops) != 1 || reading.Stops[0] != "RELEASING_NODE_NOT_DECLARED" || reading.At != "" || len(reading.Objects) != 0 {
		t.Fatalf("没声明交出节点 status = %d gateReading = %s", status, fields["gateReading"])
	}

	refused := bodyWith(t, bodyWith(t, gateBody(t, "SYN-PARCEL-65-04"), "verdict", "REFUSED"), "basis", "SYN-BASIS/seal-broken")
	refused = bodyWith(t, bodyWith(t, refused, "rule", nil), "releasingNode", "SYN-NODE-SZX-GATE")
	status, fields = handoverAnswer(t, endpoint, refused)
	if _, present := gateReadingOf(t, fields); status != http.StatusCreated || present {
		t.Fatalf("拒收 status = %d body = %v", status, fields)
	}
}

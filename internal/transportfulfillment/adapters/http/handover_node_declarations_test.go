package tfhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证交接三格节点声明（ADR-0213）在线格式上的样子：首登载荷收、更正载荷不收、答复回显在册那一版、查阅册逐格
// 透出。节点代码都是合成串，证据只记 `S`。

// Covers: AT-TF-217——作业事实 Intake 把三个节点键逐字交进首登命令；不带这三键的载荷与它们进文法之前同形，三格空着。
func TestWorkFactCommandIntakeTranslatesHandoverNodeDeclarations(t *testing.T) {
	intake := workFactIntakeForTest(t)
	body := bodyWith(t, handoverRegistrationBody, "releasingNode", "SYN-NODE-SHA-HUB")
	body = bodyWith(t, body, "receivingNode", "SYN-NODE-SZX-GW")
	body = bodyWith(t, body, "onwardNode", "SYN-NODE-SIN-HUB")
	command, err := intake.IntakeHandoverRegistration(context.Background(), commandRequest(body))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if command.ReleasingNode != "SYN-NODE-SHA-HUB" || command.ReceivingNode != "SYN-NODE-SZX-GW" || command.OnwardNode != "SYN-NODE-SIN-HUB" {
		t.Fatalf("节点 = %q / %q / %q", command.ReleasingNode, command.ReceivingNode, command.OnwardNode)
	}

	plain, err := intake.IntakeHandoverRegistration(context.Background(), commandRequest(handoverRegistrationBody))
	if err != nil {
		t.Fatalf("不带节点键的载荷被拒：%v", err)
	}
	if plain.ReleasingNode != "" || plain.ReceivingNode != "" || plain.OnwardNode != "" {
		t.Fatalf("不带节点键却交出了节点：%+v", plain)
	}
}

// Covers: AT-TF-220 的线格式——更正照前版承继节点声明，更正载荷没有节点键；带了就是坏报文（400），不静默丢掉。
func TestAHandoverCorrectionPayloadCarryingANodeIsMalformed(t *testing.T) {
	intake := workFactIntakeForTest(t)
	for _, key := range []string{"releasingNode", "receivingNode", "onwardNode"} {
		_, err := intake.IntakeHandoverCorrection(context.Background(),
			commandRequest(bodyWith(t, workFactHandoverCorrectionBody, key, "SYN-NODE-SHA-HUB")))
		if !errors.Is(err, tfhttp.ErrMalformedRequest) {
			t.Fatalf("更正载荷带 %s：err = %v，want ErrMalformedRequest", key, err)
		}
	}
}

// nodeHandoverEndpoint 是首登口接上生产 Intake 与真编排之后的样子：载荷经 HandoverRegistrationPayload 进来，只有
// 登记册与段册是替身。
func nodeHandoverEndpoint(t *testing.T) http.Handler {
	t.Helper()
	at := time.Date(2026, 9, 24, 6, 0, 0, 0, time.UTC)
	registry, segments := newHandoverRegistry(), newSegmentRegistry()
	handler := application.NewRegisterTransportHandoverHandler(application.RegisterTransportHandoverDeps{
		Handovers:  registry,
		Segments:   segments,
		Downstream: handoverHandoffDouble{},
		Clock:      tfClock{at: at},
		ParticipationEnds: application.NewEndFulfillmentParticipationHandler(application.EndFulfillmentParticipationDeps{
			Segments:   segments,
			Handovers:  registry,
			Deliveries: newDeliveryStore(),
			Clock:      tfClock{at: at},
		}),
	})
	return tfhttp.NewRegisterTransportHandoverEndpoint(workFactIntakeForTest(t), handler)
}

func handoverAnswer(t *testing.T, endpoint http.Handler, body string) (int, map[string]json.RawMessage) {
	t.Helper()
	response := serveWorkFact(endpoint, body)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	return response.Code, fields
}

// Covers: AT-TF-217、AT-TF-218、AT-TF-219 的端点面——带交出节点首登答 201 并回显；原样重发答 200 `EXISTING_VERSION`、
// 回显在册那一版；改接收节点重发答 200 `SOURCE_CONFLICT`。AT-TF-216：不带节点的首登答复里没有节点键。
func TestTheHandoverEndpointEchoesTheDeclaredNodes(t *testing.T) {
	endpoint := nodeHandoverEndpoint(t)
	body := bodyWith(t, handoverRegistrationBody, "segment", nil)
	body = bodyWith(t, body, "plannedSegment", nil)
	body = bodyWith(t, body, "segmentServiceAction", nil)
	body = bodyWith(t, body, "basis", nil)
	declared := bodyWith(t, body, "releasingNode", "SYN-NODE-SHA-HUB")

	status, fields := handoverAnswer(t, endpoint, declared)
	if status != http.StatusCreated || string(fields["outcome"]) != `"HANDOVER_REGISTERED"` {
		t.Fatalf("首登 status = %d body = %v", status, fields)
	}
	if string(fields["releasingNode"]) != `"SYN-NODE-SHA-HUB"` {
		t.Fatalf("答复没回显交出节点：%v", fields)
	}
	for _, key := range []string{"receivingNode", "onwardNode"} {
		if _, present := fields[key]; present {
			t.Fatalf("没声明的 %s 出现在答复里：%v", key, fields)
		}
	}

	status, fields = handoverAnswer(t, endpoint, declared)
	if status != http.StatusOK || string(fields["outcome"]) != `"EXISTING_VERSION"` || string(fields["releasingNode"]) != `"SYN-NODE-SHA-HUB"` {
		t.Fatalf("原样重发 status = %d body = %v", status, fields)
	}

	status, fields = handoverAnswer(t, endpoint, bodyWith(t, declared, "receivingNode", "SYN-NODE-SIN-HUB"))
	if status != http.StatusOK || string(fields["outcome"]) != `"SOURCE_CONFLICT"` {
		t.Fatalf("改接收节点重发 status = %d body = %v", status, fields)
	}

	plain := bodyWith(t, body, "object", "SYN-PARCEL-08-06")
	status, fields = handoverAnswer(t, endpoint, plain)
	if status != http.StatusCreated {
		t.Fatalf("不带节点的首登 status = %d body = %v", status, fields)
	}
	for _, key := range []string{"releasingNode", "receivingNode", "onwardNode"} {
		if _, present := fields[key]; present {
			t.Fatalf("不带节点的首登答复里出现 %s：%v", key, fields)
		}
	}
}

// Covers: AT-TF-221——交接册 `registry=transport-handover` 逐格透出三个节点，没声明的那格不出现。
func TestTransportHandoverRegistryTranscribesNodeDeclarations(t *testing.T) {
	register := &stubReviewCatalogue{
		handovers: []ports.TransportHandoverCatalogueRow{
			{
				Object: "parcel-1", Scope: "scope-1", Version: "handover-v1",
				ReleasedBy: "node-1", ReceivedBy: "carrier-1", Verdict: "HANDED_OVER",
				ReleasingNode: "SYN-NODE-SHA-HUB", ReceivingNode: "SYN-NODE-SZX-GW", OnwardNode: "SYN-NODE-SIN-HUB",
				JudgedAt: catalogueBaseAt, RecordedAt: catalogueBaseAt,
			},
			{
				Object: "parcel-2", Scope: "scope-1", Version: "handover-v1",
				ReleasedBy: "node-1", ReceivedBy: "carrier-1", Verdict: "HANDED_OVER",
				ReleasingNode: "SYN-NODE-SHA-HUB",
				JudgedAt:      catalogueBaseAt, RecordedAt: catalogueBaseAt,
			},
		},
	}
	response := serveGet(recordsEndpoint(register), "/transport-fulfillment-records?registry=transport-handover")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Handovers []map[string]json.RawMessage `json:"handovers"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Handovers) != 2 {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	full := body.Handovers[0]
	if string(full["releasingNode"]) != `"SYN-NODE-SHA-HUB"` ||
		string(full["receivingNode"]) != `"SYN-NODE-SZX-GW"` ||
		string(full["onwardNode"]) != `"SYN-NODE-SIN-HUB"` {
		t.Fatalf("三格节点转写走样：%s", response.Body.String())
	}
	partial := body.Handovers[1]
	if string(partial["releasingNode"]) != `"SYN-NODE-SHA-HUB"` {
		t.Fatalf("交出节点转写走样：%s", response.Body.String())
	}
	for _, key := range []string{"receivingNode", "onwardNode"} {
		if _, present := partial[key]; present {
			t.Fatalf("没声明的 %s 出现在册面上：%s", key, response.Body.String())
		}
	}
}

package tfhttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证派送出发前交付门禁问询口（ADR-0238）的线格式：答复一律 200，`gateReading` 与有效交付口同一个线格式，挂作业事实那一面。
// 读数各口是替身：parcel-1 凭新加坡枢纽交给尾程的那次`已交接`进 segment-1。证据只记 `S`。

type previewDeliverySegments struct {
	record ports.FulfillmentSegmentRecord
}

func (double previewDeliverySegments) FindActiveSegments(
	context.Context, domain.TenantID, domain.CarriedObjectReference,
) ([]ports.FulfillmentSegmentKey, error) {
	return []ports.FulfillmentSegmentKey{double.record.Key}, nil
}

func (double previewDeliverySegments) FindByKey(context.Context, ports.FulfillmentSegmentKey) (ports.FulfillmentSegmentRecord, bool, error) {
	return double.record, true, nil
}

type previewDeliveryHandovers struct{ handovers []domain.TransportHandover }

func (double previewDeliveryHandovers) FindHandedOverByVersion(
	context.Context, domain.TenantID, domain.CarriedObjectReference, domain.HandoverResultVersion,
) ([]domain.TransportHandover, error) {
	return double.handovers, nil
}

type previewDeliveryCustoms struct{ answer domain.GateAnswer }

func (double previewDeliveryCustoms) ReadDeliveryGate(_ context.Context, query ports.DeliveryGateQuery) (domain.GateFinding, error) {
	return domain.NewGateFinding(query.Object, double.answer, "AADJ-STUB",
		[]domain.GateBasis{{Action: "FINAL_DELIVERY", Cause: "SOME_CAUSE", Sources: []string{"SRC-1"}}})
}

// unreadAttempts 是问询不该碰的派送尝试读面：派送出发之前还没有尝试。
type unreadAttempts struct{ t *testing.T }

func (double unreadAttempts) LoadDeliveryResult(
	context.Context, domain.TenantID, domain.AttemptReference, domain.CarriedObjectReference,
) (domain.FulfillmentAttempt, domain.DeliveryAttemptResult, bool, error) {
	double.t.Fatal("交付前问询读了派送尝试")
	return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false, nil
}

// previewDeliveryHandler 装一只真问询编排：parcel-1 凭新加坡枢纽交给尾程的那次`已交接`进 segment-1，关务按 answer 答。
func previewDeliveryHandler(t *testing.T, answer domain.GateAnswer) *application.PreviewDeliveryGateHandler {
	t.Helper()
	spec := domain.TransportHandoverSpec{
		TenantID:          httpValue(t, domain.NewTenantID, "tenant-1"),
		Object:            httpValue(t, domain.NewCarriedObjectReference, "parcel-1"),
		Scope:             httpValue(t, domain.NewHandoverScopeReference, "handover-scope-1"),
		ReleasedBy:        httpValue(t, domain.NewHandoverPartyReference, "SYN-NODE-SIN-HUB"),
		ReceivedBy:        httpValue(t, domain.NewHandoverPartyReference, "SYN-COURIER-SG-LM"),
		Verdict:           domain.ObjectHandedOver,
		ReleasingEvidence: httpValue(t, domain.NewHandoverEvidenceReference, "evidence-release-1"),
		ReceivingEvidence: httpValue(t, domain.NewHandoverEvidenceReference, "evidence-receive-1"),
		Rule:              httpValue(t, domain.NewHandoverRuleReference, "handover-rule/v1"),
		Version:           httpValue(t, domain.NewHandoverResultVersion, "handover-result/parcel-1/v1"),
		JudgedAt:          time.Date(2026, 9, 5, 8, 0, 0, 0, time.UTC),
	}
	spec.Nodes.Releasing = httpValue(t, domain.NewNetworkNodeReference, "SYN-NODE-SIN-HUB")
	handover, err := domain.FormTransportHandover(spec)
	if err != nil {
		t.Fatalf("交接：%v", err)
	}
	segmentRef := httpValue(t, domain.NewFulfillmentSegmentReference, "segment-1")
	segment, err := domain.EstablishSegmentWithHandover(segmentRef, handover, domain.PlannedSegmentReference{})
	if err != nil {
		t.Fatalf("立段：%v", err)
	}
	reader, err := application.NewDeliveryGateReader(application.DeliveryGateReadDeps{
		Attempts: unreadAttempts{t: t},
		Segments: previewDeliverySegments{record: ports.FulfillmentSegmentRecord{
			Key:     ports.FulfillmentSegmentKey{TenantID: spec.TenantID, Segment: segmentRef},
			Segment: segment,
		}},
		Handovers: previewDeliveryHandovers{handovers: []domain.TransportHandover{handover}},
		Customs:   previewDeliveryCustoms{answer: answer},
	})
	if err != nil {
		t.Fatalf("交付门禁读数：%v", err)
	}
	handler, err := application.NewPreviewDeliveryGateHandler(reader)
	if err != nil {
		t.Fatalf("问询编排：%v", err)
	}
	return handler
}

const deliveryPreviewBody = `{"object":"parcel-1","deliversAt":"2026-09-05T12:00:00Z"}`

// Covers: AT-TF-322 线格式半边——问询答 200 `DELIVERY_GATE_PREVIEWED`，读数阻断也是 200（派不派这一件归设备与现场）：整格、at 与那件
// 被挡的对象，不带关务判断标识与依据来源；那件对象得出放行时逐对象那一格不列它（ADR-0237）。AT-TF-323 线格式半边：`gateReading`
// 与把同一份读数登成交付时交付答复里的那一段逐字节相同——两口是同一个线格式。
func TestADeliveryPreviewAnswersOKWithTheDeliveryGateReading(t *testing.T) {
	blocked := previewDeliveryHandler(t, domain.GateBlocked)
	response := servePreview(tfhttp.NewPreviewDeliveryGateEndpoint(workFactIntakeForTest(t), blocked), commandRequest(deliveryPreviewBody))
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"outcome":"DELIVERY_GATE_PREVIEWED"`) {
		t.Fatalf("status = %d, want 200（读数阻断也只是答复）; body = %s", response.Code, response.Body.String())
	}
	reading, present := decodeDeliveryGateReading(t, response.Body.Bytes())
	if !present || reading.Answer != "BLOCKED" || reading.At != "SYN-NODE-SIN-HUB" || len(reading.Stops) != 0 ||
		len(reading.Objects) != 1 || reading.Objects[0].Object != "parcel-1" || reading.Objects[0].Answer != "BLOCKED" {
		t.Fatalf("答复 = %s", response.Body.String())
	}
	if strings.Contains(response.Body.String(), "AADJ-STUB") || strings.Contains(response.Body.String(), "SRC-1") {
		t.Fatalf("答复带出了关务判断标识或依据来源：%s", response.Body.String())
	}

	previewed, err := blocked.Preview(context.Background(), application.PreviewDeliveryGateCommand{
		TenantID: httpValue(t, domain.NewTenantID, "tenant-1"), Object: "parcel-1", DeliversAt: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("问询：%v", err)
	}
	asRead, _ := previewed.Reading()
	delivered := postJSON(t, newTFFixtureReading(t, asRead).register, defaultRegisterBody())
	if delivered.Code != http.StatusCreated || rawGateReading(t, delivered) != rawGateReading(t, response) {
		t.Fatalf("同一份读数在两口的线格式不同：\n问询 %s\n交付 %s", rawGateReading(t, response), rawGateReading(t, delivered))
	}

	clean := servePreview(tfhttp.NewPreviewDeliveryGateEndpoint(workFactIntakeForTest(t), previewDeliveryHandler(t, domain.GateNotGuarded)),
		commandRequest(deliveryPreviewBody))
	if reading, _ := decodeDeliveryGateReading(t, clean.Body.Bytes()); clean.Code != http.StatusOK || reading.Answer != "NOT_GUARDED" ||
		reading.At != "SYN-NODE-SIN-HUB" || strings.Contains(clean.Body.String(), `"objects"`) {
		t.Fatalf("得出放行的问询 = %d %s，want 200、整格照实、没有逐对象那一格", clean.Code, clean.Body.String())
	}
}

// Covers: AT-TF-325 线格式半边——缺拟交付时点或对象答 200 `INPUT_NOT_ACCEPTED`、不带读数；时点给了却解不出、载荷带两格以外的键
// （例如照抄交付载荷带上 `attempt`）是坏报文 400；方法不对 405。
func TestADeliveryPreviewRefusesWhatItCannotAccept(t *testing.T) {
	endpoint := tfhttp.NewPreviewDeliveryGateEndpoint(workFactIntakeForTest(t), previewDeliveryHandler(t, domain.GateNotGuarded))
	for name, body := range map[string]string{
		"缺时点": `{"object":"parcel-1"}`,
		"缺对象": `{"deliversAt":"2026-09-05T12:00:00Z"}`,
	} {
		response := servePreview(endpoint, commandRequest(body))
		if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"outcome":"INPUT_NOT_ACCEPTED"`) ||
			strings.Contains(response.Body.String(), "gateReading") {
			t.Fatalf("%s：%d %s, want 200 INPUT_NOT_ACCEPTED 不带读数", name, response.Code, response.Body.String())
		}
	}
	for name, body := range map[string]string{
		"时点解不出":  `{"object":"parcel-1","deliversAt":"tomorrow"}`,
		"带派送尝试键": `{"attempt":"attempt-1","object":"parcel-1","deliversAt":"2026-09-05T12:00:00Z"}`,
	} {
		response := servePreview(endpoint, commandRequest(body))
		if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
			t.Fatalf("%s：%d %s, want 400 MALFORMED_REQUEST", name, response.Code, response.Body.String())
		}
	}
	get := commandRequest("")
	get.Method = http.MethodGet
	if response := servePreview(endpoint, get); response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET：%d, want 405", response.Code)
	}
}

// Covers: AT-TF-325 线格式半边——没有作业事实登记授予答 403 `OPERATOR_NOT_GRANTED`，设备不在用答 403 `DEVICE_NOT_IN_USE`：与有效交付口
// 同一道门。渠道未配置答 403 且不读体。编排交回 error 是没形成答案，500 且不带 outcome（ADR-0022）。
func TestADeliveryPreviewIsRefusedWithoutTheDeliveryGrant(t *testing.T) {
	for code, refusal := range map[string]error{
		"OPERATOR_NOT_GRANTED": tfhttp.ErrOperatorNotGranted,
		"DEVICE_NOT_IN_USE":    tfhttp.ErrDeviceNotInUse,
	} {
		endpoint := tfhttp.NewPreviewDeliveryGateEndpoint(workFactIntakeWith(t, workFactAuthenticatorFake{err: refusal}),
			unreachableDeliveryPreviewer{t: t})
		response := servePreview(endpoint, commandRequest(deliveryPreviewBody))
		if response.Code != http.StatusForbidden || problemCode(t, response) != code {
			t.Fatalf("%s：%d %s", code, response.Code, response.Body.String())
		}
		assertNoOutcome(t, response)
	}

	probe := &readProbe{}
	response := servePreview(tfhttp.NewPreviewDeliveryGateEndpoint(tfhttp.UnconfiguredIntake{}, unreachableDeliveryPreviewer{t: t}),
		httptest.NewRequest(http.MethodPost, "/transport-fulfillment/delivery-gate-previews", probe))
	if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("未配置：%d %s", response.Code, response.Body.String())
	}
	if probe.read {
		t.Fatal("未配置的 Intake 读了业务内容")
	}

	failed := servePreview(tfhttp.NewPreviewDeliveryGateEndpoint(workFactIntakeForTest(t), failingDeliveryPreviewer{}),
		commandRequest(deliveryPreviewBody))
	if failed.Code != http.StatusInternalServerError || problemCode(t, failed) != "NO_ANSWER_FORMED" {
		t.Fatalf("%d %s", failed.Code, failed.Body.String())
	}
	assertNoOutcome(t, failed)
}

type failingDeliveryPreviewer struct{}

func (failingDeliveryPreviewer) Preview(context.Context, application.PreviewDeliveryGateCommand) (application.PreviewDeliveryGateResult, error) {
	return application.PreviewDeliveryGateResult{}, errors.New("reading could not be formed")
}

type unreachableDeliveryPreviewer struct{ t *testing.T }

func (previewer unreachableDeliveryPreviewer) Preview(context.Context, application.PreviewDeliveryGateCommand) (application.PreviewDeliveryGateResult, error) {
	previewer.t.Fatal("a refused request reached the delivery gate preview")
	return application.PreviewDeliveryGateResult{}, nil
}

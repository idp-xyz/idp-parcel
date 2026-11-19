package nodeopshttp_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
)

// 本文件证作业端的载荷是封闭的：节点来自认证结果、原件号只属于手工录入，作业端的载荷里出现这两格就是自报，整份拒成
// 载荷不成形，值与认证结果相同也拒。手工录入两格照收。夹具全为合成（S 级）。

const (
	deviceMeasurementRest = `"sourceId":"SYN-WEIGH-1","objectKind":"HANDLING_UNIT","objectId":"SYN-HU-1",` +
		`"location":"SYN-BENCH-01","basis":"SYN-PARCEL-1","performedBy":"SYN-SCALE-01","evidenceRef":"WEIGH/1",` +
		`"occurredAt":"2026-11-17T10:00:00+08:00","weightValue":"2.10","weightUnit":"KG",` +
		`"length":"30","width":"25","height":"20","dimensionUnit":"CM"`
	deviceInvalidationRest = `"sourceId":"SYN-VOID-1","targetSourceId":"SYN-WEIGH-1","reason":"称错件","performedBy":"SYN-SCALE-01",` +
		`"evidenceRef":"WEIGH/2","occurredAt":"2026-11-17T11:00:00+08:00"`
)

func devicePayload(rest string, extra string) string {
	if extra == "" {
		return "{" + rest + "}"
	}
	return "{" + rest + "," + extra + "}"
}

// Covers: 作业端的登记载荷带 node 或 originalDocument 即拒（MALFORMED_REQUEST），译不出命令、到不了编排；两格都不带照常译出，
// 节点取认证结果、原件号为空。
func TestADeviceMeasurementPayloadRefusesANodeOrAnOriginalDocument(t *testing.T) {
	handler := &recordingMeasurements{}
	endpoint := nodeopshttp.NewRecordMeasurementEndpoint(receptionIntakeForTest(t), handler)
	for name, extra := range map[string]string{
		"自报别的节点":  `"node":"SYN-NODE-SZX-01"`,
		"自报同一个节点": `"node":"` + workFactNode + `"`,
		"带原件号":    `"originalDocument":"SYN-DOC-1"`,
		"两格都带":    `"node":"` + workFactNode + `","originalDocument":"SYN-DOC-1"`,
	} {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, receptionRequest(devicePayload(deviceMeasurementRest, extra)))
		if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
			t.Fatalf("%s：答 %d %s，want 400 MALFORMED_REQUEST", name, response.Code, response.Body.String())
		}
		if len(handler.recorded) != 0 {
			t.Fatalf("%s：被拒的载荷到了编排", name)
		}
		if _, err := receptionIntakeForTest(t).IntakeMeasurement(context.Background(), receptionRequest(devicePayload(deviceMeasurementRest, extra))); !errors.Is(err, nodeopshttp.ErrMalformedRequest) {
			t.Fatalf("%s：intake err = %v，want ErrMalformedRequest", name, err)
		}
	}

	command, err := receptionIntakeForTest(t).IntakeMeasurement(context.Background(), receptionRequest(devicePayload(deviceMeasurementRest, "")))
	if err != nil {
		t.Fatalf("两格都不带：%v", err)
	}
	if command.Node != workFactNode || command.OriginalDocument != "" || command.EnteredBy != "" {
		t.Fatalf("命令 = %+v，want 节点取认证结果、没有原件号与录入人", command)
	}
}

// Covers: 作业端的作废载荷带 node 或 originalDocument 即拒（MALFORMED_REQUEST），到不了编排；两格都不带照常译出。
func TestADeviceInvalidationPayloadRefusesANodeOrAnOriginalDocument(t *testing.T) {
	handler := &recordingMeasurements{}
	endpoint := nodeopshttp.NewInvalidateMeasurementEndpoint(receptionIntakeForTest(t), handler)
	for name, extra := range map[string]string{
		"自报节点": `"node":"` + workFactNode + `"`,
		"带原件号": `"originalDocument":"SYN-DOC-1"`,
	} {
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, receptionRequest(devicePayload(deviceInvalidationRest, extra)))
		if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
			t.Fatalf("%s：答 %d %s，want 400 MALFORMED_REQUEST", name, response.Code, response.Body.String())
		}
		if _, err := receptionIntakeForTest(t).IntakeMeasurementInvalidation(context.Background(), receptionRequest(devicePayload(deviceInvalidationRest, extra))); !errors.Is(err, nodeopshttp.ErrMalformedRequest) {
			t.Fatalf("%s：intake err = %v，want ErrMalformedRequest", name, err)
		}
	}

	command, err := receptionIntakeForTest(t).IntakeMeasurementInvalidation(context.Background(), receptionRequest(devicePayload(deviceInvalidationRest, "")))
	if err != nil {
		t.Fatalf("两格都不带：%v", err)
	}
	if command.OriginalDocument != "" || command.EnteredBy != "" {
		t.Fatalf("命令 = %+v，want 没有原件号与录入人", command)
	}
}

// Covers: 手工录入的两格不受影响：登记必带 node 与 originalDocument，作废必带 originalDocument；缺了仍拒。
func TestTheManualPayloadsKeepTheirNodeAndOriginalDocument(t *testing.T) {
	intake, err := nodeopshttp.NewOperatorDecisionIntake(measurementDecisionAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	manualMeasurement := devicePayload(deviceMeasurementRest, `"node":"SYN-NODE-SHA-HUB","originalDocument":"SYN-DOC-1"`)
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(manualMeasurement))
	registered, err := intake.IntakeManualMeasurement(request.Context(), request)
	if err != nil || registered.Node != "SYN-NODE-SHA-HUB" || registered.OriginalDocument != "SYN-DOC-1" {
		t.Fatalf("手工登记 = %+v，err = %v，want 节点与原件号取载荷", registered, err)
	}
	request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(devicePayload(deviceMeasurementRest, `"node":"SYN-NODE-SHA-HUB"`)))
	if _, err := intake.IntakeManualMeasurement(request.Context(), request); !errors.Is(err, nodeopshttp.ErrMalformedRequest) {
		t.Fatalf("手工登记缺原件号：err = %v，want ErrMalformedRequest", err)
	}
	request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(devicePayload(deviceMeasurementRest, `"originalDocument":"SYN-DOC-1"`)))
	if _, err := intake.IntakeManualMeasurement(request.Context(), request); !errors.Is(err, nodeopshttp.ErrMalformedRequest) {
		t.Fatalf("手工登记缺节点：err = %v，want ErrMalformedRequest", err)
	}

	request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(devicePayload(deviceInvalidationRest, `"originalDocument":"SYN-DOC-1"`)))
	voided, err := intake.IntakeManualMeasurementInvalidation(request.Context(), request)
	if err != nil || voided.OriginalDocument != "SYN-DOC-1" {
		t.Fatalf("手工作废 = %+v，err = %v，want 原件号取载荷", voided, err)
	}
	request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(devicePayload(deviceInvalidationRest, "")))
	if _, err := intake.IntakeManualMeasurementInvalidation(request.Context(), request); !errors.Is(err, nodeopshttp.ErrMalformedRequest) {
		t.Fatalf("手工作废缺原件号：err = %v，want ErrMalformedRequest", err)
	}
}

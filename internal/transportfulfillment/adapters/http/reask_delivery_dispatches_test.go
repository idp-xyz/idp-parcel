package tfhttp_test

import (
	"context"
	"encoding/json"
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

// 本文件证按册重问那一口的线格式（ADR-0230 决定四、五）。逐件答复的透出由 cmd/parcel-api 的真库用例证；这里证整口的格子与状态码。

type emptyPendingQueue struct{}

func (emptyPendingQueue) ListPending(context.Context, domain.TenantID, time.Time, int) ([]ports.DeliveryDispatchPendingRecord, error) {
	return nil, nil
}

type noCadence struct{}

func (noCadence) CurrentCadence(context.Context, domain.TenantID) (ports.DeliveryDispatchReaskCadenceRecord, bool, error) {
	return ports.DeliveryDispatchReaskCadenceRecord{}, false, nil
}

type unreachableTrigger struct{ t *testing.T }

func (trigger unreachableTrigger) Trigger(context.Context, application.TriggerDeliveryDispatchCommand) (application.TriggerDeliveryDispatchResult, error) {
	trigger.t.Fatal("册上没有待形成的，执行器不该被问")
	return application.TriggerDeliveryDispatchResult{}, nil
}

func reaskEndpoint(t *testing.T, intake tfhttp.DeliveryDispatchReaskIntake) http.Handler {
	t.Helper()
	return tfhttp.NewReaskDeliveryDispatchesEndpoint(intake, application.NewReaskPendingDeliveryDispatchesHandler(application.ReaskPendingDeliveryDispatchesDeps{
		Pending: emptyPendingQueue{}, Cadences: noCadence{}, Trigger: unreachableTrigger{t: t},
		Clock: tfClock{at: time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)},
	}))
}

func postReask(t *testing.T, endpoint http.Handler, body string) (int, map[string]json.RawMessage) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment-delivery-dispatch-reasks", strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer t")
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, request)
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
		t.Fatalf("答复不是 JSON：%d %s", recorder.Code, recorder.Body.String())
	}
	return recorder.Code, decoded
}

// Covers: AT-TF-286 的线格式那一半（ADR-0230 决定五）——到期重问而间隔没登记：200、outcome REASK_CADENCE_NOT_CONFIGURED、items 是空数组
// 不是 null、不带到期时刻；全部重问而册上空：200 PENDING_DELIVERY_DISPATCHES_REASKED、items []；词不在集合内答 INPUT_NOT_ACCEPTED；
// 渠道没配答 403 ACCESS_CHANNEL_NOT_CONFIGURED。
func TestTheReaskFaceAnswersItsWholeFaceGrades(t *testing.T) {
	endpoint := reaskEndpoint(t, operatorIntake(t, authenticatorFake{}))
	for body, want := range map[string]string{
		`{"scope":"DUE","limit":10}`: "REASK_CADENCE_NOT_CONFIGURED",
		`{"scope":"ALL","limit":10}`: "PENDING_DELIVERY_DISPATCHES_REASKED",
		`{"scope":"ANY","limit":10}`: "INPUT_NOT_ACCEPTED",
		`{"scope":"ALL","limit":0}`:  "INPUT_NOT_ACCEPTED",
	} {
		status, answer := postReask(t, endpoint, body)
		var outcome string
		_ = json.Unmarshal(answer["outcome"], &outcome)
		if status != http.StatusOK || outcome != want || string(answer["items"]) != "[]" {
			t.Fatalf("%s 答 %d %v, want 200 %s、items []", body, status, answer, want)
		}
		if _, has := answer["dueBy"]; has {
			t.Fatalf("%s 不该带到期时刻：%v", body, answer)
		}
	}
	status, answer := postReask(t, reaskEndpoint(t, tfhttp.UnconfiguredIntake{}), `{"scope":"ALL","limit":10}`)
	if status != http.StatusForbidden || !strings.Contains(string(answer["error"]), `"ACCESS_CHANNEL_NOT_CONFIGURED"`) {
		t.Fatalf("渠道没配答 %d %v", status, answer)
	}
}

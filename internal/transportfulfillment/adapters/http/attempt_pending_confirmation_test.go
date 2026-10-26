package tfhttp_test

import (
	"context"
	"encoding/json"
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

// 本文件证逐对象结果「待确认」与揽收「执行方拒收」两格的传输面（票 docs-consistency/code/01）：词取领域常量自己的
// String()、来源逐对象从载荷收、答复逐对象透出来源；两族词各归各的封闭集。夹具全是合成串，证据只记 S。

const pendingPickupAttemptBody = `{"sourceId":"SYN-DEVICE-01/attempt-18","task":"SYN-TASK-18","attempt":"SYN-ATTEMPT-18",` +
	`"executedBy":"SYN-COURIER-18","place":"SYN-PLACE/shipper-dock-18","plannedFrom":"2026-09-24T08:00:00+08:00",` +
	`"plannedTo":"2026-09-24T10:00:00+08:00","arrivedAt":"2026-09-24T08:40:00+08:00","evidence":"SYN-EVIDENCE/visit-18",` +
	`"objects":[{"object":"SYN-PARCEL-18-01","outcome":"PENDING_CONFIRMATION","source":"SYN-SOURCE/vehicle-arrival+driver-scan",` +
	`"basis":"SYN-GAP/no-receipt","occurredAt":"2026-09-24T08:45:00+08:00"},` +
	`{"object":"SYN-PARCEL-18-02","outcome":"EXECUTOR_REFUSED","basis":"SYN-REASON/packaging-unacceptable+photo-1",` +
	`"occurredAt":"2026-09-24T08:46:00+08:00"}]}`

const pendingDeliveryAttemptBody = `{"task":"SYN-DISPATCH-70","attempt":"SYN-DELIVERY-ATTEMPT-70","executedBy":"SYN-COURIER-70",` +
	`"place":"SYN-PLACE/consignee-70","plannedFrom":"2026-09-25T09:00:00+08:00","plannedTo":"2026-09-25T12:00:00+08:00",` +
	`"arrivedAt":"2026-09-25T10:00:00+08:00","evidence":"SYN-EVIDENCE/arrival-70",` +
	`"objects":[{"object":"SYN-PARCEL-70-01","outcome":"PENDING_CONFIRMATION","source":"SYN-SOURCE/signature-scan+address-geofence",` +
	`"basis":"SYN-CONFLICT/signature-vs-address","occurredAt":"2026-09-25T10:05:00+08:00"}]}`

// Covers: 两族词表各加的那几格——待确认与执行方拒收的词逐字进命令、来源逐对象收；词归各自的封闭集：派送口不认揽收的
// 拒收词，含糊的词是坏报文（400），不留给编排去答。
func TestWorkFactIntakeTranslatesThePendingAndRefusalWords(t *testing.T) {
	pickup, err := workFactIntakeForTest(t).IntakePickupAttempt(context.Background(), commandRequest(pendingPickupAttemptBody))
	if err != nil {
		t.Fatalf("揽收 intake：%v", err)
	}
	if len(pickup.Objects) != 2 {
		t.Fatalf("objects = %d", len(pickup.Objects))
	}
	pending, refused := pickup.Objects[0], pickup.Objects[1]
	if pending.Outcome != domain.PickupPendingConfirmation || pending.Source.String() != "SYN-SOURCE/vehicle-arrival+driver-scan" ||
		pending.Basis.String() != "SYN-GAP/no-receipt" {
		t.Fatalf("待确认 = %+v，与载荷不符", pending)
	}
	if refused.Outcome != domain.ExecutorRefused || refused.Basis.String() != "SYN-REASON/packaging-unacceptable+photo-1" ||
		refused.Source.String() != "" {
		t.Fatalf("拒收 = %+v，与载荷不符——没给的来源不该被补上", refused)
	}

	delivery, err := workFactIntakeForTest(t).IntakeDeliveryAttempt(context.Background(), commandRequest(pendingDeliveryAttemptBody))
	if err != nil {
		t.Fatalf("派送 intake：%v", err)
	}
	if len(delivery.Objects) != 1 || delivery.Objects[0].Outcome != domain.DeliveryPendingConfirmation ||
		delivery.Objects[0].Source.String() != "SYN-SOURCE/signature-scan+address-geofence" {
		t.Fatalf("派送待确认 = %+v，与载荷不符", delivery.Objects)
	}

	for name, body := range map[string]string{
		"派送口收到揽收的拒收词": strings.Replace(pendingDeliveryAttemptBody, `"PENDING_CONFIRMATION"`, `"EXECUTOR_REFUSED"`, 1),
		"派送口收到含糊的词":   strings.Replace(pendingDeliveryAttemptBody, `"PENDING_CONFIRMATION"`, `"PENDING"`, 1),
	} {
		if _, err := workFactIntakeForTest(t).IntakeDeliveryAttempt(context.Background(), commandRequest(body)); !errors.Is(err, tfhttp.ErrMalformedRequest) {
			t.Fatalf("%s：err = %v, want ErrMalformedRequest", name, err)
		}
	}
	if _, err := workFactIntakeForTest(t).IntakePickupAttempt(context.Background(), commandRequest(
		strings.Replace(pendingPickupAttemptBody, `"PENDING_CONFIRMATION"`, `"UNCONFIRMED"`, 1))); !errors.Is(err, tfhttp.ErrMalformedRequest) {
		t.Fatalf("揽收口收到含糊的词：err = %v, want ErrMalformedRequest", err)
	}
}

type pendingObjectView struct {
	Object        string `json:"object"`
	Outcome       string `json:"outcome"`
	Basis         string `json:"basis"`
	Source        string `json:"source"`
	PickupVersion string `json:"pickupVersion"`
}

type pendingAttemptView struct {
	Outcome string              `json:"outcome"`
	Objects []pendingObjectView `json:"objects"`
}

func decodePendingAttempt(t *testing.T, recorder *httptest.ResponseRecorder) map[string]pendingObjectView {
	t.Helper()
	var view pendingAttemptView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v; body=%s", err, recorder.Body.String())
	}
	if view.Outcome != "ATTEMPT_RECORDED" {
		t.Fatalf("outcome = %q; body=%s", view.Outcome, recorder.Body.String())
	}
	objects := map[string]pendingObjectView{}
	for _, object := range view.Objects {
		objects[object.Object] = object
	}
	return objects
}

// Covers: `AT-TF-018`、`AT-TF-017`、`AT-TF-070` 的传输面——两口接真 intake 与真编排走一遍：首登 201，答复逐对象透出
// 待确认的来源与缺口、拒收的依据；待确认与拒收都没有揽收版本（没有形成揽收到手）。
func TestPendingAndRefusedObjectsPostedToTheEndpointsAreAnsweredPerObject(t *testing.T) {
	pickupEndpoint := tfhttp.NewPerformOffsitePickupEndpoint(workFactIntakeForTest(t),
		application.NewPerformOffsitePickupHandler(application.PerformOffsitePickupDeps{
			Attempts:   newPickupAttemptStore(),
			Versions:   &pickupVersionFactory{},
			Downstream: pickupHandoffDouble{},
			Clock:      tfClock{at: time.Date(2026, 9, 24, 1, 0, 0, 0, time.UTC)},
		}))
	recorder := httptest.NewRecorder()
	pickupEndpoint.ServeHTTP(recorder, commandRequest(pendingPickupAttemptBody))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("揽收 status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	pickupObjects := decodePendingAttempt(t, recorder)
	if got := pickupObjects["SYN-PARCEL-18-01"]; got.Outcome != "PENDING_CONFIRMATION" ||
		got.Source != "SYN-SOURCE/vehicle-arrival+driver-scan" || got.Basis != "SYN-GAP/no-receipt" || got.PickupVersion != "" {
		t.Fatalf("待确认答复 = %+v", got)
	}
	if got := pickupObjects["SYN-PARCEL-18-02"]; got.Outcome != "EXECUTOR_REFUSED" ||
		got.Basis != "SYN-REASON/packaging-unacceptable+photo-1" || got.Source != "" || got.PickupVersion != "" {
		t.Fatalf("拒收答复 = %+v", got)
	}

	deliveryEndpoint := tfhttp.NewRecordDeliveryAttemptEndpoint(workFactIntakeForTest(t),
		application.NewRecordDeliveryAttemptHandler(application.RecordDeliveryAttemptDeps{
			Attempts: &httpDeliveryAttemptStore{records: map[string]ports.DeliveryAttemptRecord{}},
			Tasks:    openDeliveryTaskFor(t, "SYN-DISPATCH-70", "SYN-PARCEL-70-01"),
			Clock:    httpFixedClock{at: time.Date(2026, 9, 25, 2, 30, 0, 0, time.UTC)},
		}))
	recorder = httptest.NewRecorder()
	deliveryEndpoint.ServeHTTP(recorder, commandRequest(pendingDeliveryAttemptBody))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("派送 status = %d; body=%s", recorder.Code, recorder.Body.String())
	}
	if got := decodePendingAttempt(t, recorder)["SYN-PARCEL-70-01"]; got.Outcome != "PENDING_CONFIRMATION" ||
		got.Source != "SYN-SOURCE/signature-scan+address-geofence" || got.Basis != "SYN-CONFLICT/signature-vs-address" {
		t.Fatalf("派送待确认答复 = %+v", got)
	}
}

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

// 本文件证派送尝试登记口的传输面（票 product-strategy-boundary/19）：线格式逐格译进命令、租户只来自注入，
// 以及端点把编排的答案按对象透出——201 只给新落的一次尝试，重放是形成了的答案（200）。

const isolatedDeliveryAttemptBody = `{"task":"SYN-DISPATCH-08-07","attempt":"SYN-DELIVERY-ATTEMPT-08-07",` +
	`"executedBy":"SYN-COURIER/07","place":"SYN-PLACE/consignee-07",` +
	`"plannedFrom":"2026-09-25T09:00:00+08:00","plannedTo":"2026-09-25T12:00:00+08:00",` +
	`"arrivedAt":"2026-09-25T10:00:00+08:00","evidence":"SYN-EVIDENCE/arrival-07",` +
	`"objects":[{"object":"SYN-PARCEL-08-07","outcome":"DELIVERED","occurredAt":"2026-09-25T10:05:00+08:00"},` +
	`{"object":"SYN-PARCEL-08-08","outcome":"NO_ONE_TO_RECEIVE","basis":"SYN-BASIS/no-one-home","occurredAt":"2026-09-25T10:06:00+08:00"}]}`

// Covers: 事实内容与业务时间照 ADR-0023 逐字从载荷收，租户来自注入；成败词不在封闭集里是坏报文。
func TestIsolatedCommandIntakeTranslatesDeliveryAttemptWithInjectedTenant(t *testing.T) {
	command, err := isolatedCommandIntakeForTest(t).IntakeDeliveryAttempt(context.Background(), commandRequest(isolatedDeliveryAttemptBody))
	if err != nil {
		t.Fatalf("intake：%v", err)
	}
	if command.TenantID.String() != isolatedCommandTenant || command.Task != "SYN-DISPATCH-08-07" ||
		command.Attempt != "SYN-DELIVERY-ATTEMPT-08-07" || command.ExecutedBy != "SYN-COURIER/07" ||
		command.Place != "SYN-PLACE/consignee-07" || command.Evidence != "SYN-EVIDENCE/arrival-07" {
		t.Fatalf("command = %+v，与注入与载荷不符", command)
	}
	if want := time.Date(2026, 9, 25, 2, 0, 0, 0, time.UTC); !command.ArrivedAt.Equal(want) {
		t.Fatalf("ArrivedAt = %s, want %s", command.ArrivedAt, want)
	}
	if len(command.Objects) != 2 ||
		command.Objects[0].Object.String() != "SYN-PARCEL-08-07" || command.Objects[0].Outcome != domain.ObjectDelivered ||
		command.Objects[1].Outcome != domain.NoOneToReceive || command.Objects[1].Basis.String() != "SYN-BASIS/no-one-home" {
		t.Fatalf("objects = %+v，与载荷不符", command.Objects)
	}

	_, err = isolatedCommandIntakeForTest(t).IntakeDeliveryAttempt(context.Background(),
		commandRequest(strings.Replace(isolatedDeliveryAttemptBody, `"DELIVERED"`, `"PICKED_UP"`, 1)))
	if !errors.Is(err, tfhttp.ErrMalformedRequest) {
		t.Fatalf("揽收侧的成败词：err = %v, want ErrMalformedRequest", err)
	}
}

// Covers: 端点接真编排走一遍——首登 201 且逐对象透出成败与依据，同一份再投 200 EXISTING_RESULT。
func TestADeliveryAttemptPostedToTheEndpointIsRecordedAndAnsweredPerObject(t *testing.T) {
	endpoint := tfhttp.NewRecordDeliveryAttemptEndpoint(isolatedCommandIntakeForTest(t),
		application.NewRecordDeliveryAttemptHandler(application.RecordDeliveryAttemptDeps{
			Attempts: &httpDeliveryAttemptStore{records: map[string]ports.DeliveryAttemptRecord{}},
			Tasks:    openDeliveryTaskFor(t, "SYN-DISPATCH-08-07", "SYN-PARCEL-08-07", "SYN-PARCEL-08-08"),
			Clock:    httpFixedClock{at: time.Date(2026, 9, 25, 2, 30, 0, 0, time.UTC)},
		}))

	first := postDeliveryAttempt(endpoint, isolatedDeliveryAttemptBody)
	if first.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body=%s", first.Code, first.Body.String())
	}
	answer := decodeDeliveryAttemptAnswer(t, first)
	if answer.Outcome != "ATTEMPT_RECORDED" || answer.Attempt != "SYN-DELIVERY-ATTEMPT-08-07" || answer.Task != "SYN-DISPATCH-08-07" {
		t.Fatalf("answer = %+v", answer)
	}
	perObject := map[string]string{}
	for _, object := range answer.Objects {
		perObject[object.Object] = object.Outcome + "|" + object.Basis
	}
	if perObject["SYN-PARCEL-08-07"] != "DELIVERED|" || perObject["SYN-PARCEL-08-08"] != "NO_ONE_TO_RECEIVE|SYN-BASIS/no-one-home" {
		t.Fatalf("objects = %v, want each object's outcome and basis", perObject)
	}

	replay := postDeliveryAttempt(endpoint, isolatedDeliveryAttemptBody)
	if replay.Code != http.StatusOK {
		t.Fatalf("replay status = %d, want 200", replay.Code)
	}
	if got := decodeDeliveryAttemptAnswer(t, replay).Outcome; got != "EXISTING_RESULT" {
		t.Fatalf("replay outcome = %q, want EXISTING_RESULT", got)
	}
}

type deliveryAttemptAnswer struct {
	Outcome               string `json:"outcome"`
	UndecidedReason       string `json:"undecidedReason"`
	Attempt               string `json:"attempt"`
	Task                  string `json:"task"`
	ContinuationReference string `json:"continuationReference"`
	Objects               []struct {
		Object  string `json:"object"`
		Outcome string `json:"outcome"`
		Basis   string `json:"basis"`
	} `json:"objects"`
}

func postDeliveryAttempt(endpoint http.Handler, body string) *httptest.ResponseRecorder {
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/transport-fulfillment/delivery-attempts", strings.NewReader(body)))
	return response
}

func decodeDeliveryAttemptAnswer(t *testing.T, response *httptest.ResponseRecorder) deliveryAttemptAnswer {
	t.Helper()
	var answer deliveryAttemptAnswer
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("decode answer: %v; body=%s", err, response.Body.String())
	}
	return answer
}

type httpFixedClock struct{ at time.Time }

func (clock httpFixedClock) Now() time.Time { return clock.at }

type httpDeliveryAttemptStore struct {
	records map[string]ports.DeliveryAttemptRecord
}

func (store *httpDeliveryAttemptStore) FindByKey(
	_ context.Context,
	key ports.DeliveryAttemptKey,
) (ports.DeliveryAttemptRecord, bool, error) {
	record, found := store.records[key.Attempt.String()]
	return record, found, nil
}

func (store *httpDeliveryAttemptStore) Save(
	_ context.Context,
	record ports.DeliveryAttemptRecord,
) (ports.DeliveryAttemptSaveOutcome, error) {
	if _, exists := store.records[record.Key.Attempt.String()]; exists {
		return ports.DeliveryAttemptAlreadyRecorded, nil
	}
	store.records[record.Key.Attempt.String()] = record
	return ports.DeliveryAttemptSaved, nil
}

type httpDeliveryTaskReader struct {
	record ports.DispatchTaskRecord
}

func (reader httpDeliveryTaskReader) FindByKey(
	_ context.Context,
	key ports.DispatchTaskKey,
) (ports.DispatchTaskRecord, bool, error) {
	if key.Task != reader.record.Key.Task {
		return ports.DispatchTaskRecord{}, false, nil
	}
	return reader.record, true, nil
}

func openDeliveryTaskFor(t *testing.T, task string, objects ...string) httpDeliveryTaskReader {
	t.Helper()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("build dispatch task: %v", err)
		}
	}
	tenant, err := domain.NewTenantID(isolatedCommandTenant)
	must(err)
	taskRef, err := domain.NewDispatchTaskReference(task)
	must(err)
	place, err := domain.NewAttemptPlaceReference("SYN-PLACE/consignee-07")
	must(err)
	conditions, err := domain.NewServiceConditionReference("SYN-CONDITION/signature-required")
	must(err)
	spec := domain.DispatchTaskSpec{
		TenantID:   tenant,
		Task:       taskRef,
		Kind:       domain.DeliveryDispatch,
		Place:      place,
		WindowFrom: time.Date(2026, 9, 25, 1, 0, 0, 0, time.UTC),
		WindowTo:   time.Date(2026, 9, 25, 4, 0, 0, 0, time.UTC),
		Conditions: conditions,
		OpenedAt:   time.Date(2026, 9, 24, 12, 0, 0, 0, time.UTC),
	}
	for _, raw := range objects {
		object, err := domain.NewCarriedObjectReference(raw)
		must(err)
		spec.Objects = append(spec.Objects, object)
	}
	opened, err := domain.OpenDispatchTask(spec)
	must(err)
	return httpDeliveryTaskReader{record: ports.DispatchTaskRecord{
		Key:  ports.DispatchTaskKey{TenantID: tenant, Task: taskRef},
		Task: opened,
	}}
}

type unreachableDeliveryAttemptHandler struct{ t *testing.T }

func (handler unreachableDeliveryAttemptHandler) Handle(
	context.Context,
	application.RecordDeliveryAttemptCommand,
) (application.RecordDeliveryAttemptResult, error) {
	handler.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.RecordDeliveryAttemptResult{}, nil
}

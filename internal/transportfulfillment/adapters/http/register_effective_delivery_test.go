package tfhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
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

func httpValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

// ---- 应用编排的端口替身（真实 handler，非结果构造）----

type deliveryViewDouble struct {
	outcome domain.DeliveryObjectOutcome
	err     error
}

func (double *deliveryViewDouble) LoadDeliveryResult(
	_ context.Context,
	_ domain.TenantID,
	attempt domain.AttemptReference,
	object domain.CarriedObjectReference,
) (domain.FulfillmentAttempt, domain.DeliveryAttemptResult, bool, error) {
	if double.err != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false, double.err
	}
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false, err
	}
	arrived := time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC)
	spec := domain.FulfillmentAttemptSpec{
		TenantID:    tenant,
		Attempt:     attempt,
		PlannedFrom: arrived.Add(-time.Hour),
		PlannedTo:   arrived.Add(3 * time.Hour),
		ArrivedAt:   arrived,
		Objects:     []domain.CarriedObjectReference{object},
	}
	var buildErr error
	if spec.Task, buildErr = domain.NewDispatchTaskReference("delivery-task-1"); buildErr != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false, buildErr
	}
	if spec.ExecutedBy, buildErr = domain.NewExecutingPartyReference("courier-1"); buildErr != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false, buildErr
	}
	if spec.Place, buildErr = domain.NewAttemptPlaceReference("door-1"); buildErr != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false, buildErr
	}
	if spec.Evidence, buildErr = domain.NewAttemptEvidenceReference("attempt-evidence-1"); buildErr != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false, buildErr
	}
	attemptValue, err := domain.FormFulfillmentAttempt(spec)
	if err != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false, err
	}
	basis := domain.AttemptResultBasisReference{}
	if double.outcome != domain.ObjectDelivered {
		if basis, err = domain.NewAttemptResultBasisReference("reason-1"); err != nil {
			return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false, err
		}
	}
	result, err := domain.FormDeliveryAttemptResult(attemptValue, object, double.outcome, basis, arrived.Add(10*time.Minute))
	if err != nil {
		return domain.FulfillmentAttempt{}, domain.DeliveryAttemptResult{}, false, err
	}
	return attemptValue, result, true, nil
}

type deliveryStoreDouble struct {
	records map[string]ports.EffectiveDeliveryRecord
}

func newDeliveryStore() *deliveryStoreDouble {
	return &deliveryStoreDouble{records: map[string]ports.EffectiveDeliveryRecord{}}
}

func storeKey(key ports.EffectiveDeliveryKey) string {
	return key.TenantID.String() + "|" + key.Object.String() + "|" + key.Attempt.String()
}

func (double *deliveryStoreDouble) FindByKey(
	_ context.Context,
	key ports.EffectiveDeliveryKey,
) (ports.EffectiveDeliveryRecord, bool, error) {
	record, found := double.records[storeKey(key)]
	return record, found, nil
}

func (double *deliveryStoreDouble) Save(
	_ context.Context,
	record ports.EffectiveDeliveryRecord,
) (ports.DeliverySaveOutcome, error) {
	if _, exists := double.records[storeKey(record.Key)]; exists {
		return ports.DeliveryAlreadyRegistered, nil
	}
	double.records[storeKey(record.Key)] = record
	return ports.DeliverySaved, nil
}

func (double *deliveryStoreDouble) Supersede(
	_ context.Context,
	record ports.EffectiveDeliveryRecord,
) (bool, error) {
	if _, exists := double.records[storeKey(record.Key)]; !exists {
		return false, nil
	}
	double.records[storeKey(record.Key)] = record
	return true, nil
}

type deliveryVersionFactory struct{ minted int }

func (double *deliveryVersionFactory) NextDeliveryResultVersion(context.Context) (domain.DeliveryResultVersion, error) {
	double.minted++
	return domain.NewDeliveryResultVersion(fmt.Sprintf("delivery-result/v%d", double.minted))
}

type deliveryHandoffDouble struct{}

func (deliveryHandoffDouble) HandOffEffectiveDelivery(context.Context, ports.EffectiveDeliveryHandoffIntent) error {
	return nil
}

type tfClock struct{ at time.Time }

func (clock tfClock) Now() time.Time { return clock.at }

// ---- 请求体与 intake 替身（信封固定、事实从体收）----

type registerBody struct {
	Attempt   string `json:"attempt"`
	Object    string `json:"object"`
	Method    string `json:"method"`
	Recipient string `json:"recipient"`
	Proof     string `json:"proof"`
}

type correctBody struct {
	Attempt     string `json:"attempt"`
	Object      string `json:"object"`
	NewProof    string `json:"newProof"`
	CorrectedAt string `json:"correctedAt"`
}

type bodyIntake struct {
	tenant       domain.TenantID
	err          error
	lastRegister *application.RegisterEffectiveDeliveryCommand
}

func (intake *bodyIntake) IntakeRegistration(
	_ context.Context,
	request *http.Request,
) (application.RegisterEffectiveDeliveryCommand, error) {
	if intake.err != nil {
		return application.RegisterEffectiveDeliveryCommand{}, intake.err
	}
	var body registerBody
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		return application.RegisterEffectiveDeliveryCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	command := application.RegisterEffectiveDeliveryCommand{
		TenantID:  intake.tenant,
		Attempt:   body.Attempt,
		Object:    body.Object,
		Method:    body.Method,
		Recipient: body.Recipient,
		Proof:     body.Proof,
	}
	intake.lastRegister = &command
	return command, nil
}

func (intake *bodyIntake) IntakeCorrection(
	_ context.Context,
	request *http.Request,
) (application.CorrectDeliveryProofCommand, error) {
	if intake.err != nil {
		return application.CorrectDeliveryProofCommand{}, intake.err
	}
	var body correctBody
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		return application.CorrectDeliveryProofCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	correctedAt, err := time.Parse(time.RFC3339, body.CorrectedAt)
	if err != nil {
		return application.CorrectDeliveryProofCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	return application.CorrectDeliveryProofCommand{
		TenantID:    intake.tenant,
		Attempt:     body.Attempt,
		Object:      body.Object,
		NewProof:    body.NewProof,
		CorrectedAt: correctedAt,
	}, nil
}

type tfFixture struct {
	intake   *bodyIntake
	view     *deliveryViewDouble
	register http.Handler
	correct  http.Handler
}

func newTFFixture(t *testing.T) *tfFixture {
	t.Helper()
	fixture := &tfFixture{
		intake: &bodyIntake{tenant: httpValue(t, domain.NewTenantID, "tenant-1")},
		view:   &deliveryViewDouble{outcome: domain.ObjectDelivered},
	}
	handler := application.NewRegisterEffectiveDeliveryHandler(application.RegisterEffectiveDeliveryDeps{
		Attempts:   fixture.view,
		Deliveries: newDeliveryStore(),
		Versions:   &deliveryVersionFactory{},
		Downstream: deliveryHandoffDouble{},
		Clock:      tfClock{at: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)},
	})
	fixture.register = tfhttp.NewRegisterEffectiveDeliveryEndpoint(fixture.intake, handler)
	fixture.correct = tfhttp.NewCorrectDeliveryProofEndpoint(fixture.intake, handler)
	return fixture
}

func defaultRegisterBody() registerBody {
	return registerBody{
		Attempt:   "attempt-1",
		Object:    "parcel-1",
		Method:    "signature",
		Recipient: "recipient-1",
		Proof:     "pod-photo-1",
	}
}

func postJSON(t *testing.T, endpoint http.Handler, value any) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment/deliveries", strings.NewReader(string(payload)))
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, request)
	return recorder
}

type deliveryView struct {
	Outcome               string `json:"outcome"`
	DeliveryVersion       string `json:"deliveryVersion"`
	Proof                 string `json:"proof"`
	Corrects              string `json:"corrects"`
	ContinuationReference string `json:"continuationReference"`
}

func decodeDelivery(t *testing.T, recorder *httptest.ResponseRecorder) deliveryView {
	t.Helper()
	var view deliveryView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return view
}

// Covers: ADR-0022「2xx 内部只区分有没有新落一版」——首登成立 201 带版本与 POD；POD
// 证据引用从设备体收（ADR-0023：适配器无时钟依赖，命令值一字不差来自请求体）。
func TestRegisterReportsCreatedWithTheDevicePOD(t *testing.T) {
	fixture := newTFFixture(t)
	body := defaultRegisterBody()
	body.Proof = "device-pod-42"

	response := postJSON(t, fixture.register, body)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201", response.Code)
	}
	view := decodeDelivery(t, response)
	if view.Outcome != "DELIVERY_REGISTERED" || view.DeliveryVersion == "" {
		t.Fatalf("view = %+v", view)
	}
	if view.Proof != "device-pod-42" {
		t.Fatalf("proof = %q; POD 没有原样来自设备体", view.Proof)
	}
	if fixture.intake.lastRegister.Proof != "device-pod-42" {
		t.Fatal("命令里的 POD 被代铸了")
	}
}

// Covers: ADR-0022「业务判别一律进响应体」——未生效（失败结果登不出交付）、冲突、重放、
// 未决都是形成了的答案，都是 200；`outcome` 区分。
func TestRegisterReportsOKForEveryAnswerThatRegisteredNothing(t *testing.T) {
	cases := map[string]struct {
		arrange func(*tfFixture) registerBody
		posts   int
		want    string
	}{
		"a failed result is NOT_EFFECTIVE": {
			arrange: func(fixture *tfFixture) registerBody {
				fixture.view.outcome = domain.NoOneToReceive
				return defaultRegisterBody()
			},
			posts: 1,
			want:  "NOT_EFFECTIVE",
		},
		"replay returns the existing version": {
			arrange: func(fixture *tfFixture) registerBody { return defaultRegisterBody() },
			posts:   2,
			want:    "EXISTING_VERSION",
		},
		"a different POD under the same key is a conflict": {
			arrange: func(fixture *tfFixture) registerBody { return defaultRegisterBody() },
			posts:   1,
			want:    "SOURCE_CONFLICT",
		},
		"a dependency failure is undecided": {
			arrange: func(fixture *tfFixture) registerBody {
				fixture.view.err = errors.New("view down")
				return defaultRegisterBody()
			},
			posts: 1,
			want:  "DELIVERY_UNDECIDED",
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newTFFixture(t)
			body := testCase.arrange(fixture)
			if testCase.want == "SOURCE_CONFLICT" {
				if first := postJSON(t, fixture.register, body); first.Code != http.StatusCreated {
					t.Fatalf("seed status = %d", first.Code)
				}
				body.Proof = "pod-photo-2"
			}
			var response *httptest.ResponseRecorder
			for post := 0; post < testCase.posts; post++ {
				response = postJSON(t, fixture.register, body)
			}
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200（未生效/未决也是答案）", response.Code)
			}
			if view := decodeDelivery(t, response); view.Outcome != testCase.want {
				t.Fatalf("outcome = %q, want %q", view.Outcome, testCase.want)
			}
		})
	}
}

// Covers: `AT-TF-072` 的 HTTP 面——更正走版本链：201 新落一版、`corrects` 透出前版
// 引用；更正无中生有（没有可更正的登记）是 200 未受理答案。
func TestCorrectExposesThePredecessorVersion(t *testing.T) {
	fixture := newTFFixture(t)
	if response := postJSON(t, fixture.register, defaultRegisterBody()); response.Code != http.StatusCreated {
		t.Fatalf("seed status = %d", response.Code)
	}

	response := postJSON(t, fixture.correct, correctBody{
		Attempt:     "attempt-1",
		Object:      "parcel-1",
		NewProof:    "pod-corrected-2",
		CorrectedAt: "2026-09-06T08:00:00Z",
	})

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201（更正也新落了一版）", response.Code)
	}
	view := decodeDelivery(t, response)
	if view.Outcome != "DELIVERY_CORRECTED" {
		t.Fatalf("outcome = %q", view.Outcome)
	}
	if view.Corrects != "delivery-result/v1" {
		t.Fatalf("corrects = %q, want delivery-result/v1（版本链透出前版）", view.Corrects)
	}
	if view.Proof != "pod-corrected-2" {
		t.Fatalf("proof = %q", view.Proof)
	}

	t.Run("correcting an absent registration is an answer, not a transport error", func(t *testing.T) {
		absent := postJSON(t, fixture.correct, correctBody{
			Attempt:     "attempt-9",
			Object:      "parcel-9",
			NewProof:    "pod-x",
			CorrectedAt: "2026-09-06T08:00:00Z",
		})
		if absent.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", absent.Code)
		}
		if view := decodeDelivery(t, absent); view.Outcome != "SOURCE_NOT_ACCEPTED" {
			t.Fatalf("outcome = %q", view.Outcome)
		}
	})
}

// 传输层分法：405 带 Allow；构造不出命令是 4xx；接入层其他失败与编排错误是 5xx；错误体
// 只有稳定 code 不回显请求（ADR-0029 不泄露存在性）；无名 outcome 是 500。
func TestTransportProblemsCarryOnlyStableCodes(t *testing.T) {
	t.Run("method not allowed", func(t *testing.T) {
		fixture := newTFFixture(t)
		request := httptest.NewRequest(http.MethodGet, "/transport-fulfillment/deliveries", nil)
		recorder := httptest.NewRecorder()
		fixture.register.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodPost {
			t.Fatalf("status = %d allow = %q", recorder.Code, recorder.Header().Get("Allow"))
		}
	})

	t.Run("a malformed request is 400 and does not echo the body", func(t *testing.T) {
		fixture := newTFFixture(t)
		request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment/deliveries", strings.NewReader("secret-not-json"))
		recorder := httptest.NewRecorder()
		fixture.register.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "MALFORMED_REQUEST") {
			t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
		}
		if strings.Contains(recorder.Body.String(), "secret-not-json") {
			t.Fatal("错误体回显了请求内容")
		}
	})

	t.Run("an intake infrastructure failure is 500", func(t *testing.T) {
		fixture := newTFFixture(t)
		fixture.intake.err = errors.New("session store down")
		response := postJSON(t, fixture.register, defaultRegisterBody())
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "INTAKE_FAILED") {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
	})

	t.Run("an orchestration error is 500 NO_ANSWER_FORMED", func(t *testing.T) {
		fixture := newTFFixture(t)
		endpoint := tfhttp.NewRegisterEffectiveDeliveryEndpoint(fixture.intake, failingDeliveryHandler{})
		response := func() *httptest.ResponseRecorder {
			payload, _ := json.Marshal(defaultRegisterBody())
			request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment/deliveries", strings.NewReader(string(payload)))
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, request)
			return recorder
		}()
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "NO_ANSWER_FORMED") {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
	})

	t.Run("an unnamed outcome is a 500, not a new business answer", func(t *testing.T) {
		fixture := newTFFixture(t)
		endpoint := tfhttp.NewCorrectDeliveryProofEndpoint(fixture.intake, unnamedDeliveryHandler{})
		payload, _ := json.Marshal(correctBody{Attempt: "a", Object: "o", NewProof: "p", CorrectedAt: "2026-09-06T08:00:00Z"})
		request := httptest.NewRequest(http.MethodPost, "/transport-fulfillment/deliveries", strings.NewReader(string(payload)))
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "UNNAMED_OUTCOME") {
			t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
		}
	})
}

type failingDeliveryHandler struct{}

func (failingDeliveryHandler) Register(
	context.Context,
	application.RegisterEffectiveDeliveryCommand,
) (application.RegisterEffectiveDeliveryResult, error) {
	return application.RegisterEffectiveDeliveryResult{}, errors.New("orchestration exploded")
}

func (failingDeliveryHandler) Correct(
	context.Context,
	application.CorrectDeliveryProofCommand,
) (application.RegisterEffectiveDeliveryResult, error) {
	return application.RegisterEffectiveDeliveryResult{}, errors.New("orchestration exploded")
}

type unnamedDeliveryHandler struct{}

func (unnamedDeliveryHandler) Register(
	context.Context,
	application.RegisterEffectiveDeliveryCommand,
) (application.RegisterEffectiveDeliveryResult, error) {
	return application.RegisterEffectiveDeliveryResult{}, nil
}

func (unnamedDeliveryHandler) Correct(
	context.Context,
	application.CorrectDeliveryProofCommand,
) (application.RegisterEffectiveDeliveryResult, error) {
	return application.RegisterEffectiveDeliveryResult{}, nil
}

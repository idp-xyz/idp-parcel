package nodeopshttp_test

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

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

func adapterValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

type identityViewDouble struct {
	candidates []domain.ParcelAssociationReference
	err        error
}

func (double *identityViewDouble) ResolveParcelIdentity(
	_ context.Context,
	_ domain.TenantID,
	_ ports.ExternalMarkObservation,
) ([]domain.ParcelAssociationReference, error) {
	return double.candidates, double.err
}

type receptionStoreDouble struct {
	byKey map[ports.ReceptionKey]ports.ReceptionRecord
	saved int
}

func newReceptionStore() *receptionStoreDouble {
	return &receptionStoreDouble{byKey: map[ports.ReceptionKey]ports.ReceptionRecord{}}
}

func (double *receptionStoreDouble) FindByKey(
	_ context.Context,
	key ports.ReceptionKey,
) (ports.ReceptionRecord, bool, error) {
	record, found := double.byKey[key]
	return record, found, nil
}

func (double *receptionStoreDouble) Save(
	_ context.Context,
	record ports.ReceptionRecord,
) (ports.ReceptionSaveOutcome, error) {
	if _, exists := double.byKey[record.Key]; exists {
		return ports.ReceptionAlreadyRecorded, nil
	}
	double.byKey[record.Key] = record
	double.saved++
	return ports.ReceptionSaved, nil
}

type versionFactoryDouble struct{ next int }

func (double *versionFactoryDouble) NextIntakeResultVersion(context.Context) (domain.IntakeResultVersion, error) {
	double.next++
	return domain.NewIntakeResultVersion(fmt.Sprintf("intake-result/v%d", double.next))
}

type downstreamDouble struct {
	intents []ports.NodeIntakeHandoffIntent
}

func (double *downstreamDouble) HandOffNodeIntake(
	_ context.Context,
	intent ports.NodeIntakeHandoffIntent,
) error {
	double.intents = append(double.intents, intent)
	return nil
}

type adapterClock struct{ at time.Time }

func (clock adapterClock) Now() time.Time { return clock.at }

// deviceBody 是测试里设备提交的请求体形状。事实身份与发生时间在体内——信封（租户、
// 节点、交付方）由「认证」给出，这正是 ADR-0023 的两半。
type deviceBody struct {
	SourceID   string `json:"sourceId"`
	Unit       string `json:"unit"`
	Mark       string `json:"mark"`
	Claim      string `json:"claim"`
	Evidence   string `json:"evidence"`
	Refusal    string `json:"refusal"`
	OccurredAt string `json:"occurredAt"`
}

// bodyIntake 是测试用的 ReceptionIntake：信封字段固定（模拟认证结果），事实字段一律
// 从请求体读出——它证明适配器把设备签发的身份与时间原样送进命令，服务器不代铸。
type bodyIntake struct {
	tenant      domain.TenantID
	node        domain.NodeReference
	deliveredBy domain.DeliveringPartyReference
	err         error
	lastCommand *application.ReceiveDeliveredUnitCommand
}

func (intake *bodyIntake) IntakeReception(
	_ context.Context,
	request *http.Request,
) (application.ReceiveDeliveredUnitCommand, error) {
	if intake.err != nil {
		return application.ReceiveDeliveredUnitCommand{}, intake.err
	}
	var body deviceBody
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		return application.ReceiveDeliveredUnitCommand{}, fmt.Errorf("%w: %v", nodeopshttp.ErrMalformedRequest, err)
	}
	occurredAt, err := time.Parse(time.RFC3339, body.OccurredAt)
	if err != nil {
		return application.ReceiveDeliveredUnitCommand{}, fmt.Errorf("%w: %v", nodeopshttp.ErrMalformedRequest, err)
	}
	unit, err := domain.NewHandlingUnitID(body.Unit)
	if err != nil {
		return application.ReceiveDeliveredUnitCommand{}, fmt.Errorf("%w: %v", nodeopshttp.ErrMalformedRequest, err)
	}
	command := application.ReceiveDeliveredUnitCommand{
		TenantID:    intake.tenant,
		SourceID:    body.SourceID,
		Node:        intake.node,
		DeliveredBy: intake.deliveredBy,
		Unit:        unit,
		Mark:        ports.ExternalMarkObservation{Mark: body.Mark},
		Claim:       application.ExplicitReception,
		Refusal:     body.Refusal,
		OccurredAt:  occurredAt,
	}
	if body.Claim == "REFUSAL" {
		command.Claim = application.ExplicitRefusal
	}
	if body.Evidence != "" {
		evidence, err := domain.NewReceptionEvidenceReference(body.Evidence)
		if err != nil {
			return application.ReceiveDeliveredUnitCommand{}, fmt.Errorf("%w: %v", nodeopshttp.ErrMalformedRequest, err)
		}
		command.Evidence = evidence
	}
	intake.lastCommand = &command
	return command, nil
}

type adapterFixture struct {
	intake   *bodyIntake
	identity *identityViewDouble
	store    *receptionStoreDouble
	handoff  *downstreamDouble
	endpoint http.Handler
}

func newAdapterFixture(t *testing.T) *adapterFixture {
	t.Helper()
	fixture := &adapterFixture{
		intake: &bodyIntake{
			tenant:      adapterValue(t, domain.NewTenantID, "tenant-1"),
			node:        adapterValue(t, domain.NewNodeReference, "node-origin"),
			deliveredBy: adapterValue(t, domain.NewDeliveringPartyReference, "customer-1"),
		},
		identity: &identityViewDouble{candidates: []domain.ParcelAssociationReference{
			adapterValue(t, domain.NewParcelAssociationReference, "parcel-1"),
		}},
		store:   newReceptionStore(),
		handoff: &downstreamDouble{},
	}
	handler := application.NewReceiveDeliveredUnitHandler(application.ReceiveDeliveredUnitDeps{
		Identity:   fixture.identity,
		Receptions: fixture.store,
		Versions:   &versionFactoryDouble{},
		Downstream: fixture.handoff,
		Clock:      adapterClock{at: time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC)},
	})
	fixture.endpoint = nodeopshttp.NewReceiveDeliveredUnitEndpoint(fixture.intake, handler)
	return fixture
}

func defaultBody() deviceBody {
	return deviceBody{
		SourceID:   "device-scan-7",
		Unit:       "unit-1",
		Mark:       "EXT-MARK-1",
		Claim:      "RECEPTION",
		Evidence:   "photo-1",
		OccurredAt: "2026-09-05T08:30:00Z",
	}
}

func (fixture *adapterFixture) post(t *testing.T, body deviceBody) *httptest.ResponseRecorder {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, "/node-operations/receptions", strings.NewReader(string(payload)))
	recorder := httptest.NewRecorder()
	fixture.endpoint.ServeHTTP(recorder, request)
	return recorder
}

type receptionView struct {
	Outcome               string   `json:"outcome"`
	IntakeVersion         string   `json:"intakeVersion"`
	Unit                  string   `json:"unit"`
	Association           string   `json:"association"`
	ControlBasis          string   `json:"controlBasis"`
	Candidates            []string `json:"candidates"`
	RefusalReason         string   `json:"refusalReason"`
	ContinuationReference string   `json:"continuationReference"`
}

func decodeReception(t *testing.T, recorder *httptest.ResponseRecorder) receptionView {
	t.Helper()
	var view receptionView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	return view
}

// Covers: ADR-0022「2xx 内部只区分有没有新建对象」——收寄成立用 201，设备不必解析响应
// 体就能确认那一条可以出队；成立答案带收寄版本、关联与控制依据。
func TestReceiveReportsCreatedWhenAnIntakeWasFormed(t *testing.T) {
	fixture := newAdapterFixture(t)

	response := fixture.post(t, defaultBody())

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusCreated)
	}
	view := decodeReception(t, response)
	if view.Outcome != "INTAKE_FORMED" {
		t.Fatalf("outcome = %q", view.Outcome)
	}
	if view.IntakeVersion == "" || view.Association != "parcel-1" || view.ControlBasis == "" {
		t.Fatalf("view = %+v; 成立答案少了版本/关联/控制依据", view)
	}
}

// Covers: ADR-0022「业务判别一律进响应体」——待识别、未形成、未决、已有结果与来源冲突
// 都是形成了的答案，都是 200；状态码不区分它们，`outcome` 区分。
func TestReceiveReportsOKForEveryAnswerThatFormedNoIntake(t *testing.T) {
	cases := map[string]struct {
		arrange func(*testing.T, *adapterFixture) deviceBody
		posts   int
		want    string
		inspect func(*testing.T, receptionView)
	}{
		"pending identification": {
			arrange: func(t *testing.T, fixture *adapterFixture) deviceBody {
				fixture.identity.candidates = nil
				return defaultBody()
			},
			posts: 1,
			want:  "PENDING_IDENTIFICATION",
		},
		"identity conflict pends with candidates": {
			arrange: func(t *testing.T, fixture *adapterFixture) deviceBody {
				fixture.identity.candidates = []domain.ParcelAssociationReference{
					adapterValue(t, domain.NewParcelAssociationReference, "parcel-1"),
					adapterValue(t, domain.NewParcelAssociationReference, "parcel-2"),
				}
				return defaultBody()
			},
			posts: 1,
			want:  "PENDING_IDENTIFICATION",
			inspect: func(t *testing.T, view receptionView) {
				if len(view.Candidates) != 2 {
					t.Fatalf("candidates = %v, want 2（身份冲突把候选交回）", view.Candidates)
				}
			},
		},
		"refusal forms no intake": {
			arrange: func(t *testing.T, fixture *adapterFixture) deviceBody {
				body := defaultBody()
				body.Claim = "REFUSAL"
				body.Refusal = "PACKAGING_UNSAFE"
				body.Evidence = ""
				return body
			},
			posts: 1,
			want:  "INTAKE_NOT_FORMED",
			inspect: func(t *testing.T, view receptionView) {
				if view.RefusalReason != "PACKAGING_UNSAFE" {
					t.Fatalf("refusalReason = %q", view.RefusalReason)
				}
			},
		},
		"dependency failure is undecided": {
			arrange: func(t *testing.T, fixture *adapterFixture) deviceBody {
				fixture.identity.err = errors.New("identity view down")
				return defaultBody()
			},
			posts: 1,
			want:  "RECEPTION_UNDECIDED",
			inspect: func(t *testing.T, view receptionView) {
				if view.ContinuationReference == "" {
					t.Fatal("未决没有带续办引用")
				}
			},
		},
		"replay": {
			arrange: func(t *testing.T, fixture *adapterFixture) deviceBody { return defaultBody() },
			posts:   2,
			want:    "EXISTING_RESULT",
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newAdapterFixture(t)
			body := testCase.arrange(t, fixture)
			var response *httptest.ResponseRecorder
			for post := 0; post < testCase.posts; post++ {
				response = fixture.post(t, body)
			}
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d（未决/业务负向也是答案）", response.Code, http.StatusOK)
			}
			view := decodeReception(t, response)
			if view.Outcome != testCase.want {
				t.Fatalf("outcome = %q, want %q", view.Outcome, testCase.want)
			}
			if testCase.inspect != nil {
				testCase.inspect(t, view)
			}
		})
	}
}

// Covers: UC-NO-002 幂等语义的 HTTP 面——离线设备重发同一条时同答：重放返回原收寄
// 版本、库里只落一次、意图重发同一份。
func TestReceiveReplayReportsTheSameAnswer(t *testing.T) {
	fixture := newAdapterFixture(t)

	first := decodeReception(t, fixture.post(t, defaultBody()))
	replay := decodeReception(t, fixture.post(t, defaultBody()))

	if replay.IntakeVersion != first.IntakeVersion {
		t.Fatalf("replay named %q, want the original %q", replay.IntakeVersion, first.IntakeVersion)
	}
	if fixture.store.saved != 1 {
		t.Fatalf("saved = %d, want 1", fixture.store.saved)
	}
	if len(fixture.handoff.intents) != 2 {
		t.Fatalf("intents = %d, want 2（重放重发同一份）", len(fixture.handoff.intents))
	}
}

// Covers: ADR-0023「设备签发的事实身份与发生时间从请求体收，服务器不代铸」——命令里的
// SourceID 与 OccurredAt 必须一字不差来自请求体；适配器没有时钟依赖，代铸无从发生。
func TestFactsComeFromTheDeviceBodyNotTheServer(t *testing.T) {
	fixture := newAdapterFixture(t)
	body := defaultBody()
	body.SourceID = "device-issued-42"
	body.OccurredAt = "2026-09-04T23:45:00Z"

	if response := fixture.post(t, body); response.Code != http.StatusCreated {
		t.Fatalf("status = %d", response.Code)
	}

	command := fixture.intake.lastCommand
	if command.SourceID != "device-issued-42" {
		t.Fatalf("sourceID = %q; 事实身份被代铸了", command.SourceID)
	}
	wantAt, _ := time.Parse(time.RFC3339, "2026-09-04T23:45:00Z")
	if !command.OccurredAt.Equal(wantAt) {
		t.Fatalf("occurredAt = %s; 发生时间被代铸了", command.OccurredAt)
	}
	// 落库的键与业务时间同样锚在设备值上——服务端时钟（2026-09-05T10:00Z）只进 RecordedAt。
	record, found, err := fixture.store.FindByKey(context.Background(), ports.ReceptionKey{
		TenantID: fixture.intake.tenant,
		SourceID: "device-issued-42",
	})
	if err != nil || !found {
		t.Fatalf("record not found by device-issued source: %v", err)
	}
	if !record.Intake.ReceivedAt().Equal(wantAt) {
		t.Fatalf("receivedAt = %s, want %s", record.Intake.ReceivedAt(), wantAt)
	}
}

// 传输层分法：405 带 Allow；构造不出命令是 4xx（重发不会变好）；接入层其他失败与编排
// 错误是 5xx（留在队里重发）；错误体只有稳定 code，无自由文本（ADR-0029 不泄露存在性）。
func TestTransportProblemsCarryOnlyStableCodes(t *testing.T) {
	t.Run("method not allowed", func(t *testing.T) {
		fixture := newAdapterFixture(t)
		request := httptest.NewRequest(http.MethodGet, "/node-operations/receptions", nil)
		recorder := httptest.NewRecorder()
		fixture.endpoint.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodPost {
			t.Fatalf("status = %d allow = %q", recorder.Code, recorder.Header().Get("Allow"))
		}
	})

	t.Run("a malformed request is 400", func(t *testing.T) {
		fixture := newAdapterFixture(t)
		request := httptest.NewRequest(http.MethodPost, "/node-operations/receptions", strings.NewReader("not-json"))
		recorder := httptest.NewRecorder()
		fixture.endpoint.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("status = %d, want 400", recorder.Code)
		}
		if !strings.Contains(recorder.Body.String(), "MALFORMED_REQUEST") {
			t.Fatalf("body = %s", recorder.Body.String())
		}
		if strings.Contains(recorder.Body.String(), "not-json") {
			t.Fatal("错误体回显了请求内容——自由文本会捎带存在性")
		}
	})

	t.Run("an intake infrastructure failure is 500", func(t *testing.T) {
		fixture := newAdapterFixture(t)
		fixture.intake.err = errors.New("session store down")
		response := fixture.post(t, defaultBody())
		if response.Code != http.StatusInternalServerError {
			t.Fatalf("status = %d, want 500（留在队里重发）", response.Code)
		}
		if !strings.Contains(response.Body.String(), "INTAKE_FAILED") {
			t.Fatalf("body = %s", response.Body.String())
		}
	})

	t.Run("an orchestration error is 500 NO_ANSWER_FORMED", func(t *testing.T) {
		fixture := newAdapterFixture(t)
		endpoint := nodeopshttp.NewReceiveDeliveredUnitEndpoint(fixture.intake, failingHandler{})
		payload, _ := json.Marshal(defaultBody())
		request := httptest.NewRequest(http.MethodPost, "/node-operations/receptions", strings.NewReader(string(payload)))
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "NO_ANSWER_FORMED") {
			t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
		}
	})

	t.Run("an unnamed outcome is a 500, not a new business answer", func(t *testing.T) {
		fixture := newAdapterFixture(t)
		endpoint := nodeopshttp.NewReceiveDeliveredUnitEndpoint(fixture.intake, unnamedOutcomeHandler{})
		payload, _ := json.Marshal(defaultBody())
		request := httptest.NewRequest(http.MethodPost, "/node-operations/receptions", strings.NewReader(string(payload)))
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "UNNAMED_OUTCOME") {
			t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
		}
	})
}

type failingHandler struct{}

func (failingHandler) Handle(
	context.Context,
	application.ReceiveDeliveredUnitCommand,
) (application.ReceiveDeliveredUnitResult, error) {
	return application.ReceiveDeliveredUnitResult{}, errors.New("orchestration exploded")
}

type unnamedOutcomeHandler struct{}

func (unnamedOutcomeHandler) Handle(
	context.Context,
	application.ReceiveDeliveredUnitCommand,
) (application.ReceiveDeliveredUnitResult, error) {
	return application.ReceiveDeliveredUnitResult{}, nil
}

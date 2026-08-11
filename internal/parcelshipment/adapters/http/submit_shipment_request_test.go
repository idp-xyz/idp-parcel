package shipmenthttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// Covers: ADR-0022 「2xx 内部只区分有没有新建对象」 — 建单成立用 201，这样离线客户端不必
// 解析响应体就能确认那一条可以出队。
func TestSubmitReportsCreatedWhenAShipmentRequestWasBuilt(t *testing.T) {
	fixture := newFixture(t)

	response := fixture.post(t)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusCreated)
	}
	body := decodeBody(t, response)
	if body.Outcome != "SUBMITTED" {
		t.Fatalf("outcome = %q, want SUBMITTED", body.Outcome)
	}
	if body.ShipmentRequestID == "" {
		t.Fatal("a built request was reported without naming the shipment request")
	}
}

// Covers: ADR-0022 「业务判别一律进响应体」 — 六种没有建单的答案都是答案，因此都是 200；
// 状态码不区分它们，`outcome` 区分。
func TestSubmitReportsOKForEveryAnswerThatBuiltNoRequest(t *testing.T) {
	cases := map[string]struct {
		arrange func(*fixture)
		want    string
	}{
		"replay": {
			arrange: func(value *fixture) { value.intake.commands = append(value.intake.commands, value.intake.commands[0]) },
			want:    "EXISTING_RESULT",
		},
		"ingress conflict": {
			arrange: func(value *fixture) { value.intake.commands = append(value.intake.commands, value.changedPayload(t)) },
			want:    "INGRESS_CONFLICT",
		},
		"input not accepted": {
			arrange: func(value *fixture) { value.intake.commands[0].DeclaredParcelIDs = nil },
			want:    "INPUT_NOT_ACCEPTED",
		},
		"other production authority": {
			arrange: func(value *fixture) { value.ownership.authority = domain.ProductionAuthorityOther },
			want:    "OTHER_PRODUCTION_AUTHORITY",
		},
		"ownership unresolved": {
			arrange: func(value *fixture) { value.ownership.authority = domain.ProductionAuthorityUnresolved },
			want:    "OWNERSHIP_UNRESOLVED",
		},
		"admission paused": {
			arrange: func(value *fixture) { value.ownership.control = domain.AdmissionControlPaused },
			want:    "ADMISSION_PAUSED",
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newFixture(t)
			testCase.arrange(fixture)

			// 重放与冲突要先有一次原始提交才谈得上，因此这两格发两次；其余一次即可。
			response := fixture.post(t)
			if len(fixture.intake.commands) > 1 {
				response = fixture.post(t)
			}

			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusOK)
			}
			if got := decodeBody(t, response).Outcome; got != testCase.want {
				t.Fatalf("outcome = %q, want %q", got, testCase.want)
			}
		})
	}
}

// Covers: UC-PS-001 一致性幂等与并发「接受结果响应丢失时，调用方必须能够通过稳定请求关联
// 查询原结果，不能通过再次建单恢复」 — 离线队列重发同一条时的核心路径。
func TestSubmitReplayReportsTheOriginalShipmentRequestWithoutBuildingASecond(t *testing.T) {
	fixture := newFixture(t)
	fixture.intake.commands = append(fixture.intake.commands, fixture.intake.commands[0])

	first := decodeBody(t, fixture.post(t))
	replay := decodeBody(t, fixture.post(t))

	if replay.ShipmentRequestID != first.ShipmentRequestID {
		t.Fatalf("replay named %q, want the original %q", replay.ShipmentRequestID, first.ShipmentRequestID)
	}
	if fixture.requests.insertCount != 1 {
		t.Fatalf("insert count = %d, want 1", fixture.requests.insertCount)
	}
}

// Covers: UC-PS-001 步骤 3A 与 `AT-PS-012` — 立不起最小委托身份是应用层形成的业务答案，
// 不是传输层的 400。适配器抢着报 400 会让这个结果在 HTTP 面上永远不可达。
func TestSubmitLetsTheApplicationFormInputNotAcceptedRatherThanRejectingItAtTheEdge(t *testing.T) {
	fixture := newFixture(t)
	fixture.intake.commands[0].DeclaredParcelIDs = nil

	response := fixture.post(t)

	if response.Code == http.StatusBadRequest {
		t.Fatal("the edge rejected input whose usability only the application can judge")
	}
	if got := decodeBody(t, response).Outcome; got != "INPUT_NOT_ACCEPTED" {
		t.Fatalf("outcome = %q, want INPUT_NOT_ACCEPTED", got)
	}
	if _, found := fixture.sources.stored(t, fixture.identity(t)); !found {
		t.Fatal("unusable input discarded the preserved source")
	}
}

// Covers: ADR-0022 结果语义 — 拒绝要带上它背后的门禁理由，否则操作员只知道被挡了，不知道
// 该找治理还是找客户。
func TestSubmitCarriesTheGateBlockReasonsBehindARefusal(t *testing.T) {
	fixture := newFixture(t)
	fixture.ownership.control = domain.AdmissionControlPaused

	body := decodeBody(t, fixture.post(t))

	if len(body.GateBlockReasons) == 0 {
		t.Fatal("a refusal reported no gate block reason")
	}
	if body.GateBlockReasons[0] != "ADMISSION_PAUSED" {
		t.Fatalf("block reason = %q, want ADMISSION_PAUSED", body.GateBlockReasons[0])
	}
	if body.ShipmentRequestID != "" {
		t.Fatal("a refusal named a shipment request")
	}
}

// Covers: ADR-0022 「4xx 与 5xx 响应不得携带 outcome 字段」 — 该字段的存在本身表示应用层
// 答过了，让它出现在没有答案的响应里，客户端就会把一次失败记成一个业务结果。
func TestSubmitReportsBadRequestWithoutAnOutcome(t *testing.T) {
	fixture := newFixture(t)
	fixture.intake.err = fmt.Errorf("no request key: %w", shipmenthttp.ErrMalformedRequest)

	response := fixture.post(t)

	if response.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
	}
	assertNoOutcomeField(t, response)
	if fixture.sources.preserveCount != 0 {
		t.Fatal("a request that formed no command still preserved a source")
	}
}

// Covers: ADR-0022 三类分法 — 依赖答不出是「没形成答案」，必须落在 5xx，离线客户端据此把
// 那一条留在队里。判成 4xx 会让它把一条本该重试的请求丢掉。
func TestSubmitReportsServerErrorWhenNoAnswerWasFormed(t *testing.T) {
	fixture := newFixture(t)
	fixture.sources.preserveErr = errors.New("preservation unavailable")

	response := fixture.post(t)

	if response.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusInternalServerError)
	}
	assertNoOutcomeField(t, response)
}

// Covers: ADR-0022 — 依赖失败的细节不出上下文边界。它可能带着租户或客户账户的存在性。
func TestSubmitDoesNotLeakTheUnderlyingFailure(t *testing.T) {
	fixture := newFixture(t)
	fixture.sources.preserveErr = errors.New("tenant-1 shard unreachable at 10.0.0.7")

	response := fixture.post(t)

	if strings.Contains(response.Body.String(), "tenant-1") || strings.Contains(response.Body.String(), "10.0.0.7") {
		t.Fatalf("response leaked the underlying failure: %s", response.Body.String())
	}
}

func TestSubmitRejectsOtherMethods(t *testing.T) {
	fixture := newFixture(t)
	request := httptest.NewRequest(http.MethodGet, "/shipment-requests", nil)
	response := httptest.NewRecorder()

	fixture.handler.ServeHTTP(response, request)

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
}

type responseBody struct {
	Outcome           string   `json:"outcome"`
	ShipmentRequestID string   `json:"shipmentRequestId"`
	GateBlockReasons  []string `json:"gateBlockReasons"`
}

func decodeBody(t *testing.T, response *httptest.ResponseRecorder) responseBody {
	t.Helper()
	if got := response.Header().Get("Content-Type"); got != "application/json" {
		t.Fatalf("Content-Type = %q, want application/json", got)
	}
	var body responseBody
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response %s: %v", response.Body.Bytes(), err)
	}
	return body
}

func assertNoOutcomeField(t *testing.T, response *httptest.ResponseRecorder) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode response %s: %v", response.Body.Bytes(), err)
	}
	if _, present := fields["outcome"]; present {
		t.Fatalf("a response that formed no answer carried an outcome: %s", response.Body.Bytes())
	}
}

type fixture struct {
	handler   http.Handler
	intake    *intakeDouble
	sources   *sourceRepositoryDouble
	requests  *shipmentRequestRepositoryDouble
	ownership *ownershipAuthorityDouble
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	value := &fixture{
		sources:   &sourceRepositoryDouble{records: map[domain.SourceIdentity]domain.SourceSubmissionFingerprint{}},
		requests:  &shipmentRequestRepositoryDouble{records: map[domain.SourceIdentity]domain.ShipmentRequest{}},
		ownership: &ownershipAuthorityDouble{t: t, authority: domain.ProductionAuthorityIDPParcel, control: domain.AdmissionControlOpen},
	}
	value.intake = &intakeDouble{commands: []application.SubmitShipmentRequestCommand{value.command(t)}}
	value.handler = shipmenthttp.NewSubmitShipmentRequestEndpoint(
		value.intake,
		application.NewSubmitShipmentRequestHandler(
			value.sources,
			value.requests,
			value.ownership,
			&identityFactoryDouble{},
			fixedClock{at: time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC)},
		),
	)
	return value
}

func (value *fixture) post(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, "/shipment-requests", strings.NewReader("{}"))
	response := httptest.NewRecorder()
	value.handler.ServeHTTP(response, request)
	return response
}

func (value *fixture) identity(t *testing.T) domain.SourceIdentity {
	t.Helper()
	identity, err := domain.NewSourceIdentity(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewCustomerAccountID, "customer-1"),
		mustValue(t, domain.NewSource, "source-a"),
		mustValue(t, domain.NewSourceRequestKey, "key-1"),
	)
	if err != nil {
		t.Fatalf("new source identity: %v", err)
	}
	return identity
}

func (value *fixture) command(t *testing.T) application.SubmitShipmentRequestCommand {
	t.Helper()
	scope, err := domain.NewAdmissionScope(
		mustValue(t, domain.NewAdmissionScopeReference, "scope-ref-1"),
		mustValue(t, domain.NewAdmissionScopeDigest, "scope-1"),
	)
	if err != nil {
		t.Fatalf("new admission scope: %v", err)
	}
	return application.SubmitShipmentRequestCommand{
		Identity:          value.identity(t),
		PayloadDigest:     mustValue(t, domain.NewPayloadDigest, "digest-1"),
		OccurredAt:        time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
		ReceivedAt:        time.Date(2026, 8, 7, 10, 0, 1, 0, time.UTC),
		BatchID:           mustValue(t, domain.NewSubmissionBatchID, "batch-1"),
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
		DeclaredParcelIDs: []domain.DeclaredParcelID{mustValue(t, domain.NewDeclaredParcelID, "parcel-1")},
		AdmissionScope:    scope,
		ExpectedRevision:  mustValue(t, domain.NewProductionOwnershipRevision, "rev-1"),
	}
}

func (value *fixture) changedPayload(t *testing.T) application.SubmitShipmentRequestCommand {
	t.Helper()
	changed := value.command(t)
	changed.PayloadDigest = mustValue(t, domain.NewPayloadDigest, "digest-2")
	return changed
}

type intakeDouble struct {
	commands []application.SubmitShipmentRequestCommand
	calls    int
	err      error
}

func (double *intakeDouble) IntakeSubmission(
	_ context.Context,
	_ *http.Request,
) (application.SubmitShipmentRequestCommand, error) {
	if double.err != nil {
		return application.SubmitShipmentRequestCommand{}, double.err
	}
	command := double.commands[min(double.calls, len(double.commands)-1)]
	double.calls++
	return command, nil
}

type sourceRepositoryDouble struct {
	records       map[domain.SourceIdentity]domain.SourceSubmissionFingerprint
	preserveErr   error
	preserveCount int
}

func (double *sourceRepositoryDouble) FindPreserved(
	_ context.Context,
	identity domain.SourceIdentity,
) (domain.SourceSubmissionFingerprint, bool, error) {
	existing, found := double.records[identity]
	return existing, found, nil
}

func (double *sourceRepositoryDouble) Preserve(
	_ context.Context,
	submission domain.SourceSubmissionFingerprint,
) error {
	if double.preserveErr != nil {
		return double.preserveErr
	}
	double.preserveCount++
	double.records[submission.Identity()] = submission
	return nil
}

func (double *sourceRepositoryDouble) AppendObservation(
	_ context.Context,
	_ domain.SourceSubmissionFingerprint,
) error {
	return nil
}

func (double *sourceRepositoryDouble) stored(
	t *testing.T,
	identity domain.SourceIdentity,
) (domain.SourceSubmissionFingerprint, bool) {
	t.Helper()
	existing, found := double.records[identity]
	return existing, found
}

type shipmentRequestRepositoryDouble struct {
	records     map[domain.SourceIdentity]domain.ShipmentRequest
	insertCount int
}

func (double *shipmentRequestRepositoryDouble) FindBySourceIdentity(
	_ context.Context,
	identity domain.SourceIdentity,
) (domain.ShipmentRequest, bool, error) {
	existing, found := double.records[identity]
	return existing, found, nil
}

func (double *shipmentRequestRepositoryDouble) Insert(
	_ context.Context,
	identity domain.SourceIdentity,
	request domain.ShipmentRequest,
) error {
	double.insertCount++
	double.records[identity] = request
	return nil
}

func (double *shipmentRequestRepositoryDouble) Save(
	_ context.Context,
	identity domain.SourceIdentity,
	request domain.ShipmentRequest,
) (ports.ShipmentRequestSaveOutcome, error) {
	double.records[identity] = request
	return ports.ShipmentRequestSaved, nil
}

type ownershipAuthorityDouble struct {
	t         *testing.T
	authority domain.ProductionAuthorityKind
	control   domain.AdmissionControl
}

func (double *ownershipAuthorityDouble) DecideProductionOwnership(
	_ context.Context,
	scope domain.AdmissionScope,
) (domain.ProductionOwnershipDecision, error) {
	double.t.Helper()

	spec := domain.ProductionOwnershipDecisionSpec{
		DecisionID:       mustValue(double.t, domain.NewProductionOwnershipDecisionID, "decision-1"),
		Scope:            scope,
		Authority:        double.authority,
		AdmissionControl: double.control,
		RuleVersion:      mustValue(double.t, domain.NewProductionOwnershipRuleVersion, "rule-1"),
		AsOf:             time.Date(2026, 8, 7, 11, 0, 0, 0, time.UTC),
		Validity:         validity(double.t),
		Revision:         mustValue(double.t, domain.NewProductionOwnershipRevision, "rev-1"),
		DecisionAt:       time.Date(2026, 8, 7, 11, 0, 0, 0, time.UTC),
	}
	switch double.authority {
	case domain.ProductionAuthorityOther:
		spec.OtherAuthorityRef = mustValue(double.t, domain.NewProductionAuthorityReference, "other-authority-1")
		spec.HandoffRef = mustValue(double.t, domain.NewHandoffConfirmationReference, "handoff-1")
	case domain.ProductionAuthorityUnresolved:
		spec.UnresolvedReason = domain.OwnershipUnresolvedAuthorityNotUnique
		spec.ContinuationRef = mustValue(double.t, domain.NewOwnershipContinuationReference, "continue-1")
	}
	if double.control == domain.AdmissionControlPaused {
		spec.SuspensionRef = mustValue(double.t, domain.NewOwnershipSuspensionReference, "suspend-1")
	}

	decision, err := domain.NewProductionOwnershipDecision(spec)
	if err != nil {
		double.t.Fatalf("new ownership decision: %v", err)
	}
	return decision, nil
}

type identityFactoryDouble struct {
	versions int
	tasks    int
}

func (double *identityFactoryDouble) NextSubmissionVersionID(_ context.Context) (domain.SubmissionVersionID, error) {
	double.versions++
	return domain.NewSubmissionVersionID("version-" + strconv.Itoa(double.versions))
}

func (double *identityFactoryDouble) NextAcceptanceDecisionTaskID(_ context.Context) (domain.AcceptanceDecisionTaskID, error) {
	double.tasks++
	return domain.NewAcceptanceDecisionTaskID("task-" + strconv.Itoa(double.tasks))
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

type stringValue interface {
	String() string
}

func mustValue[T stringValue](t *testing.T, constructor func(string) (T, error), value string) T {
	t.Helper()
	got, err := constructor(value)
	if err != nil {
		t.Fatalf("construct %q: %v", value, err)
	}
	return got
}

func validity(t *testing.T) domain.OwnershipValidityInterval {
	t.Helper()
	interval, err := domain.NewOwnershipValidityInterval(
		time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("new validity interval: %v", err)
	}
	return interval
}

var (
	_ shipmenthttp.SubmissionIntake      = (*intakeDouble)(nil)
	_ ports.SourceSubmissionRepository   = (*sourceRepositoryDouble)(nil)
	_ ports.ShipmentRequestRepository    = (*shipmentRequestRepositoryDouble)(nil)
	_ ports.ProductionOwnershipAuthority = (*ownershipAuthorityDouble)(nil)
	_ ports.SubmissionIdentityFactory    = (*identityFactoryDouble)(nil)
	_ ports.Clock                        = fixedClock{}
)

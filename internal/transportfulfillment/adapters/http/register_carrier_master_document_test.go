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

// 本文件证总单登记两个端点（ADR-0113 决定五）的传输层纪律：首登与新版本各新落一版 201、其余答案 200 且
// `outcome` 区分、未决带续办引用、登记内容与关联集原样透出（零关联是 `[]`）、405/400/500 分法、未配置 Intake
// 两口同堵不读体。编排是真处理器接内存登记册替身，不构造结果。

// ---- 登记册替身（只插不改，当前版按回指派生）----

type masterDocumentRegistryDouble struct {
	records []ports.MasterDocumentRecord
	findErr error
}

func (double *masterDocumentRegistryDouble) FindByKey(
	_ context.Context,
	key ports.MasterDocumentKey,
) (ports.MasterDocumentRecord, bool, error) {
	if double.findErr != nil {
		return ports.MasterDocumentRecord{}, false, double.findErr
	}
	for _, record := range double.records {
		if record.Key == key {
			return record, true, nil
		}
	}
	return ports.MasterDocumentRecord{}, false, nil
}

func (double *masterDocumentRegistryDouble) FindCurrent(
	_ context.Context,
	tenant domain.TenantID,
	document domain.MasterDocumentReference,
) (ports.MasterDocumentRecord, bool, error) {
	if double.findErr != nil {
		return ports.MasterDocumentRecord{}, false, double.findErr
	}
	superseded := map[domain.MasterDocumentVersion]bool{}
	for _, record := range double.records {
		if record.Key.TenantID == tenant && record.Key.Document == document {
			if prior, has := record.Document.Supersedes(); has {
				superseded[prior] = true
			}
		}
	}
	for _, record := range double.records {
		if record.Key.TenantID == tenant && record.Key.Document == document && !superseded[record.Key.Version] {
			return record, true, nil
		}
	}
	return ports.MasterDocumentRecord{}, false, nil
}

func (double *masterDocumentRegistryDouble) ListVersions(
	context.Context,
	domain.TenantID,
	domain.MasterDocumentReference,
) ([]ports.MasterDocumentRecord, error) {
	return double.records, nil
}

func (double *masterDocumentRegistryDouble) Save(
	_ context.Context,
	record ports.MasterDocumentRecord,
) (ports.MasterDocumentSaveOutcome, error) {
	for _, existing := range double.records {
		if existing.Key == record.Key {
			return ports.MasterDocumentAlreadyRegistered, nil
		}
	}
	double.records = append(double.records, record)
	return ports.MasterDocumentSaved, nil
}

// ---- 请求体与 intake 替身（信封固定、字段从体收）----

type masterDocumentAssociationInput struct {
	Kind      string `json:"kind"`
	Reference string `json:"reference"`
}

type masterDocumentBody struct {
	Document     string                           `json:"document"`
	Version      string                           `json:"version"`
	Issuer       string                           `json:"issuer"`
	Scope        string                           `json:"scope"`
	Commission   string                           `json:"commission"`
	Booking      string                           `json:"booking"`
	Associations []masterDocumentAssociationInput `json:"associations"`
}

type masterDocumentRevisionBody struct {
	Document     string                           `json:"document"`
	Revision     string                           `json:"revision"`
	At           string                           `json:"at"`
	NewVersion   string                           `json:"newVersion"`
	Replacement  string                           `json:"replacement"`
	Associations []masterDocumentAssociationInput `json:"associations"`
}

type masterDocumentIntakeDouble struct {
	tenant domain.TenantID
	err    error
}

func associationInputs(inputs []masterDocumentAssociationInput) []application.MasterDocumentAssociationInput {
	converted := make([]application.MasterDocumentAssociationInput, 0, len(inputs))
	for _, input := range inputs {
		converted = append(converted, application.MasterDocumentAssociationInput{Kind: input.Kind, Reference: input.Reference})
	}
	return converted
}

func (intake *masterDocumentIntakeDouble) IntakeMasterDocumentRegistration(
	_ context.Context,
	request *http.Request,
) (application.RegisterMasterDocumentCommand, error) {
	if intake.err != nil {
		return application.RegisterMasterDocumentCommand{}, intake.err
	}
	var body masterDocumentBody
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		return application.RegisterMasterDocumentCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	return application.RegisterMasterDocumentCommand{
		TenantID:     intake.tenant,
		Document:     body.Document,
		Version:      body.Version,
		Issuer:       body.Issuer,
		Scope:        body.Scope,
		Commission:   body.Commission,
		Booking:      body.Booking,
		Associations: associationInputs(body.Associations),
	}, nil
}

func (intake *masterDocumentIntakeDouble) IntakeMasterDocumentRevision(
	_ context.Context,
	request *http.Request,
) (application.ReviseMasterDocumentCommand, error) {
	if intake.err != nil {
		return application.ReviseMasterDocumentCommand{}, intake.err
	}
	var body masterDocumentRevisionBody
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		return application.ReviseMasterDocumentCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	at, err := parseOptionalTime(body.At)
	if err != nil {
		return application.ReviseMasterDocumentCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	// 真渠道 Intake 就位前，把请求体里的改变词认回封闭集合是替身自己的事；词不在集合内留零值，让编排答未受理。
	revision := domain.MasterDocumentRevisionInvalid
	for _, candidate := range []domain.MasterDocumentRevision{domain.MasterDocumentRevocation, domain.MasterDocumentSupersession, domain.MasterDocumentAssociationRestatement} {
		if candidate.String() == body.Revision {
			revision = candidate
		}
	}
	return application.ReviseMasterDocumentCommand{
		TenantID:     intake.tenant,
		Document:     body.Document,
		Revision:     revision,
		At:           at,
		NewVersion:   body.NewVersion,
		Replacement:  body.Replacement,
		Associations: associationInputs(body.Associations),
	}, nil
}

type masterDocumentFixture struct {
	intake   *masterDocumentIntakeDouble
	registry *masterDocumentRegistryDouble
	register http.Handler
	revise   http.Handler
}

func newMasterDocumentFixture(t *testing.T) *masterDocumentFixture {
	t.Helper()
	fixture := &masterDocumentFixture{
		intake:   &masterDocumentIntakeDouble{tenant: httpValue(t, domain.NewTenantID, "tenant-1")},
		registry: &masterDocumentRegistryDouble{},
	}
	handler := application.NewRegisterMasterDocumentHandler(application.RegisterMasterDocumentDeps{
		Documents: fixture.registry,
		Clock:     tfClock{at: time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)},
	})
	fixture.register = tfhttp.NewRegisterCarrierMasterDocumentEndpoint(fixture.intake, handler)
	fixture.revise = tfhttp.NewReviseCarrierMasterDocumentEndpoint(fixture.intake, handler)
	return fixture
}

func defaultMasterDocumentBody() masterDocumentBody {
	return masterDocumentBody{
		Document: "SYN-MAWB-1",
		Version:  "MDV-1",
		Issuer:   "party/carrier-x",
		Scope:    "SYN-LANE-1",
		Associations: []masterDocumentAssociationInput{
			{Kind: "PARCEL", Reference: "PCL-1"},
			{Kind: "CONSOLIDATION_UNIT", Reference: "CU-1"},
		},
	}
}

type masterDocumentView struct {
	Outcome               string                           `json:"outcome"`
	UndecidedReason       string                           `json:"undecidedReason"`
	ContinuationReference string                           `json:"continuationReference"`
	Document              string                           `json:"document"`
	Version               string                           `json:"version"`
	Issuer                string                           `json:"issuer"`
	Scope                 string                           `json:"scope"`
	Commission            string                           `json:"commission"`
	Booking               string                           `json:"booking"`
	Standing              string                           `json:"standing"`
	ChangedAt             string                           `json:"changedAt"`
	Supersedes            string                           `json:"supersedes"`
	ReplacedBy            string                           `json:"replacedBy"`
	Associations          []masterDocumentAssociationInput `json:"associations"`
}

func decodeMasterDocument(t *testing.T, recorder *httptest.ResponseRecorder) masterDocumentView {
	t.Helper()
	var view masterDocumentView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return view
}

const (
	masterDocumentRegisterPath = "/transport-fulfillment-carrier-master-document-registrations"
	masterDocumentRevisePath   = "/transport-fulfillment-carrier-master-document-revisions"
)

// Covers: ADR-0022 首登新落一版 201；登记内容原样透出；关联按（类别，引用）整理；没给的可缺引用与改变才有的字段
// 都不出现。
func TestRegisterMasterDocumentReportsCreatedWithTheRegisteredThings(t *testing.T) {
	fixture := newMasterDocumentFixture(t)
	body := defaultMasterDocumentBody()
	body.Commission = "COMM-1"

	response := postTo(t, fixture.register, masterDocumentRegisterPath, body)

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", response.Code, response.Body.String())
	}
	view := decodeMasterDocument(t, response)
	if view.Outcome != "MASTER_DOCUMENT_REGISTERED" || view.Document != "SYN-MAWB-1" || view.Version != "MDV-1" {
		t.Fatalf("view = %+v", view)
	}
	if view.Issuer != "party/carrier-x" || view.Scope != "SYN-LANE-1" || view.Commission != "COMM-1" || view.Booking != "" {
		t.Fatalf("签发方、范围与可缺引用没有照实透出：%+v", view)
	}
	if view.Standing != "IN_FORCE" || view.Supersedes != "" || view.ReplacedBy != "" || view.ChangedAt != "" {
		t.Fatalf("首版透出了改变才有的字段：%+v", view)
	}
	if len(view.Associations) != 2 || view.Associations[0].Kind != "CONSOLIDATION_UNIT" || view.Associations[1].Reference != "PCL-1" {
		t.Fatalf("关联集没有按（类别，引用）整理透出：%+v", view.Associations)
	}
}

// Covers: 零关联是内容——`associations` 是 `[]` 不是缺席也不是 null。
func TestAMasterDocumentWithNoAssociationsAnswersAnEmptyArray(t *testing.T) {
	fixture := newMasterDocumentFixture(t)
	body := defaultMasterDocumentBody()
	body.Associations = nil

	response := postTo(t, fixture.register, masterDocumentRegisterPath, body)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if raw := arrayAt(t, response, "associations"); raw != "[]" {
		t.Fatalf("associations = %s, want []", raw)
	}
}

// Covers: 形成新版本新落一版 201，回指前版、改变时间与替代者透出；对已不适用的再改是 200 答案带回当前版。
func TestReviseMasterDocumentReportsCreatedAndExposesTheChain(t *testing.T) {
	fixture := newMasterDocumentFixture(t)
	if seed := postTo(t, fixture.register, masterDocumentRegisterPath, defaultMasterDocumentBody()); seed.Code != http.StatusCreated {
		t.Fatalf("seed status = %d body = %s", seed.Code, seed.Body.String())
	}

	restated := postTo(t, fixture.revise, masterDocumentRevisePath, masterDocumentRevisionBody{
		Document:     "SYN-MAWB-1",
		Revision:     "RESTATE_ASSOCIATIONS",
		At:           "2026-09-06T08:00:00Z",
		NewVersion:   "MDV-2",
		Associations: []masterDocumentAssociationInput{{Kind: "FULFILLMENT_SEGMENT", Reference: "SEG-1"}},
	})
	if restated.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201（关联重述也新落了一版）; body = %s", restated.Code, restated.Body.String())
	}
	view := decodeMasterDocument(t, restated)
	if view.Outcome != "MASTER_DOCUMENT_REVISED" || view.Version != "MDV-2" || view.Supersedes != "MDV-1" || view.Standing != "IN_FORCE" {
		t.Fatalf("view = %+v", view)
	}
	if view.ChangedAt != "2026-09-06T08:00:00Z" || len(view.Associations) != 1 || view.Associations[0].Kind != "FULFILLMENT_SEGMENT" {
		t.Fatalf("改变时间或重述后的关联集走样：%+v", view)
	}

	superseded := postTo(t, fixture.revise, masterDocumentRevisePath, masterDocumentRevisionBody{
		Document: "SYN-MAWB-1", Revision: "SUPERSEDE", At: "2026-09-06T09:00:00Z", NewVersion: "MDV-3", Replacement: "SYN-MAWB-1B",
	})
	if superseded.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", superseded.Code, superseded.Body.String())
	}
	if view := decodeMasterDocument(t, superseded); view.Standing != "SUPERSEDED" || view.ReplacedBy != "SYN-MAWB-1B" || view.Supersedes != "MDV-2" {
		t.Fatalf("view = %+v", view)
	}

	t.Run("revising an already superseded document is an answer with the current version", func(t *testing.T) {
		again := postTo(t, fixture.revise, masterDocumentRevisePath, masterDocumentRevisionBody{
			Document: "SYN-MAWB-1", Revision: "REVOKE", At: "2026-09-06T10:00:00Z", NewVersion: "MDV-4",
		})
		if again.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", again.Code)
		}
		if view := decodeMasterDocument(t, again); view.Outcome != "NO_LONGER_IN_FORCE" || view.Version != "MDV-3" {
			t.Fatalf("view = %+v", view)
		}
	})
}

// Covers: ADR-0022「业务判别一律进响应体」——重放、冲突、已有版本链、未登记、未受理、未决都是形成了的答案，
// 都是 200，`outcome` 区分；`未决`带 `continuationReference` 与 `undecidedReason`。
func TestMasterDocumentEndpointsReportOKForEveryAnswerThatRegisteredNothing(t *testing.T) {
	cases := map[string]struct {
		arrange    func(*masterDocumentFixture) (http.Handler, any)
		want       string
		wantReason string
	}{
		"replay returns the existing version": {
			arrange: func(fixture *masterDocumentFixture) (http.Handler, any) {
				postTo(t, fixture.register, "/x", defaultMasterDocumentBody())
				return fixture.register, defaultMasterDocumentBody()
			},
			want: "EXISTING_VERSION",
		},
		"a different scope under the same version is a conflict": {
			arrange: func(fixture *masterDocumentFixture) (http.Handler, any) {
				postTo(t, fixture.register, "/x", defaultMasterDocumentBody())
				body := defaultMasterDocumentBody()
				body.Scope = "SYN-LANE-2"
				return fixture.register, body
			},
			want: "CONTENT_CONFLICT",
		},
		"a second first version for a registered document is refused": {
			arrange: func(fixture *masterDocumentFixture) (http.Handler, any) {
				postTo(t, fixture.register, "/x", defaultMasterDocumentBody())
				body := defaultMasterDocumentBody()
				body.Version = "MDV-1B"
				return fixture.register, body
			},
			want: "MASTER_DOCUMENT_ALREADY_REGISTERED",
		},
		"an unknown association kind is not accepted": {
			arrange: func(fixture *masterDocumentFixture) (http.Handler, any) {
				body := defaultMasterDocumentBody()
				body.Associations = []masterDocumentAssociationInput{{Kind: "BAG", Reference: "B-1"}}
				return fixture.register, body
			},
			want: "INPUT_NOT_ACCEPTED",
		},
		"revising a never registered document": {
			arrange: func(fixture *masterDocumentFixture) (http.Handler, any) {
				return fixture.revise, masterDocumentRevisionBody{Document: "SYN-MAWB-never", Revision: "REVOKE", At: "2026-09-06T08:00:00Z", NewVersion: "MDV-2"}
			},
			want: "MASTER_DOCUMENT_NOT_REGISTERED",
		},
		"a registry failure is undecided with a continuation": {
			arrange: func(fixture *masterDocumentFixture) (http.Handler, any) {
				fixture.registry.findErr = errors.New("registry down")
				return fixture.register, defaultMasterDocumentBody()
			},
			want:       "REGISTRATION_UNDECIDED",
			wantReason: "MASTER_DOCUMENT_REGISTRY_UNAVAILABLE",
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newMasterDocumentFixture(t)
			endpoint, body := testCase.arrange(fixture)
			response := postTo(t, endpoint, "/x", body)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200（未决也是答案）; body = %s", response.Code, response.Body.String())
			}
			view := decodeMasterDocument(t, response)
			if view.Outcome != testCase.want {
				t.Fatalf("outcome = %q, want %q", view.Outcome, testCase.want)
			}
			if view.UndecidedReason != testCase.wantReason {
				t.Fatalf("undecidedReason = %q, want %q", view.UndecidedReason, testCase.wantReason)
			}
			if testCase.wantReason != "" && view.ContinuationReference == "" {
				t.Fatal("未决没有带 continuationReference，调用方无从续办")
			}
		})
	}
}

// 传输层分法：405 带 Allow；构造不出命令是 4xx；接入层其他失败与编排错误是 5xx，且 5xx 体里不带 outcome；
// 无名 outcome 是 500。错误体只有稳定 code 不回显请求（ADR-0029）。
func TestMasterDocumentTransportProblemsCarryOnlyStableCodes(t *testing.T) {
	t.Run("method not allowed", func(t *testing.T) {
		fixture := newMasterDocumentFixture(t)
		for name, endpoint := range map[string]http.Handler{"register": fixture.register, "revise": fixture.revise} {
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/x", nil))
			if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodPost {
				t.Fatalf("%s: status = %d allow = %q", name, recorder.Code, recorder.Header().Get("Allow"))
			}
		}
	})

	t.Run("a malformed request is 400 and does not echo the body", func(t *testing.T) {
		fixture := newMasterDocumentFixture(t)
		recorder := httptest.NewRecorder()
		fixture.revise.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("secret-not-json")))
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "MALFORMED_REQUEST") {
			t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
		}
		if strings.Contains(recorder.Body.String(), "secret-not-json") {
			t.Fatal("错误体回显了请求内容")
		}
	})

	t.Run("an intake infrastructure failure is 500", func(t *testing.T) {
		fixture := newMasterDocumentFixture(t)
		fixture.intake.err = errors.New("session store down")
		response := postTo(t, fixture.register, "/x", defaultMasterDocumentBody())
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "INTAKE_FAILED") {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
	})

	t.Run("an orchestration error is 500 NO_ANSWER_FORMED without an outcome", func(t *testing.T) {
		fixture := newMasterDocumentFixture(t)
		endpoint := tfhttp.NewRegisterCarrierMasterDocumentEndpoint(fixture.intake, failingMasterDocumentRegistrar{})
		response := postTo(t, endpoint, "/x", defaultMasterDocumentBody())
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "NO_ANSWER_FORMED") {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "outcome") {
			t.Fatal("5xx 响应不得携带 outcome（ADR-0022）")
		}
	})

	t.Run("an unnamed outcome is a 500, not a new business answer", func(t *testing.T) {
		fixture := newMasterDocumentFixture(t)
		endpoint := tfhttp.NewReviseCarrierMasterDocumentEndpoint(fixture.intake, unnamedMasterDocumentRegistrar{})
		response := postTo(t, endpoint, "/x", masterDocumentRevisionBody{At: "2026-09-06T08:00:00Z"})
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "UNNAMED_OUTCOME") {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
	})
}

// Covers: ADR-0055「未配置即拒、不读内容」——两口同堵；编排是被调即失败的替身。
func TestUnconfiguredMasterDocumentIntakeRefusesBothDoorsWithoutReadingTheBody(t *testing.T) {
	registrar := unreachableMasterDocumentRegistrar{t: t}
	for name, endpoint := range map[string]http.Handler{
		"register": tfhttp.NewRegisterCarrierMasterDocumentEndpoint(tfhttp.UnconfiguredIntake{}, registrar),
		"revise":   tfhttp.NewReviseCarrierMasterDocumentEndpoint(tfhttp.UnconfiguredIntake{}, registrar),
	} {
		t.Run(name, func(t *testing.T) {
			probe := &readProbe{}
			response := httptest.NewRecorder()
			endpoint.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/x", probe))
			if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
				t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
			}
			assertNoOutcome(t, response)
			if probe.read {
				t.Fatal("an unconfigured intake read the business content")
			}
		})
	}
}

type failingMasterDocumentRegistrar struct{}

func (failingMasterDocumentRegistrar) Register(
	context.Context,
	application.RegisterMasterDocumentCommand,
) (application.RegisterMasterDocumentResult, error) {
	return application.RegisterMasterDocumentResult{}, errors.New("orchestration exploded")
}

func (failingMasterDocumentRegistrar) Revise(
	context.Context,
	application.ReviseMasterDocumentCommand,
) (application.RegisterMasterDocumentResult, error) {
	return application.RegisterMasterDocumentResult{}, errors.New("orchestration exploded")
}

type unnamedMasterDocumentRegistrar struct{}

func (unnamedMasterDocumentRegistrar) Register(
	context.Context,
	application.RegisterMasterDocumentCommand,
) (application.RegisterMasterDocumentResult, error) {
	return application.RegisterMasterDocumentResult{}, nil
}

func (unnamedMasterDocumentRegistrar) Revise(
	context.Context,
	application.ReviseMasterDocumentCommand,
) (application.RegisterMasterDocumentResult, error) {
	return application.RegisterMasterDocumentResult{}, nil
}

type unreachableMasterDocumentRegistrar struct{ t *testing.T }

func (registrar unreachableMasterDocumentRegistrar) Register(
	context.Context,
	application.RegisterMasterDocumentCommand,
) (application.RegisterMasterDocumentResult, error) {
	registrar.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.RegisterMasterDocumentResult{}, nil
}

func (registrar unreachableMasterDocumentRegistrar) Revise(
	context.Context,
	application.ReviseMasterDocumentCommand,
) (application.RegisterMasterDocumentResult, error) {
	registrar.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.RegisterMasterDocumentResult{}, nil
}

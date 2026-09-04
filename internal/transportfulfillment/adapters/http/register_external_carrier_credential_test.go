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

// 本文件证凭证登记两个端点（label-channel/18）的传输层纪律：首登与改变新落一版 201、其余答案 200 且
// `outcome` 区分、未决带续办引用、五件事原样透出、405/400/500 分法、未配置 Intake 两口同堵不读体。
// 编排是真处理器接内存登记册替身，不构造结果。

// ---- 登记册替身（只插不改，当前版按回指派生）----

type credentialRegistryDouble struct {
	records []ports.ExternalCarrierCredentialRecord
	findErr error
}

func (double *credentialRegistryDouble) FindByKey(
	_ context.Context,
	key ports.ExternalCarrierCredentialKey,
) (ports.ExternalCarrierCredentialRecord, bool, error) {
	if double.findErr != nil {
		return ports.ExternalCarrierCredentialRecord{}, false, double.findErr
	}
	for _, record := range double.records {
		if record.Key == key {
			return record, true, nil
		}
	}
	return ports.ExternalCarrierCredentialRecord{}, false, nil
}

func (double *credentialRegistryDouble) FindCurrent(
	_ context.Context,
	tenant domain.TenantID,
	credential domain.ExternalCarrierCredentialReference,
) (ports.ExternalCarrierCredentialRecord, bool, error) {
	if double.findErr != nil {
		return ports.ExternalCarrierCredentialRecord{}, false, double.findErr
	}
	superseded := map[domain.ExternalCarrierCredentialVersion]bool{}
	for _, record := range double.records {
		if record.Key.TenantID == tenant && record.Key.Credential == credential {
			if prior, has := record.Credential.Supersedes(); has {
				superseded[prior] = true
			}
		}
	}
	for _, record := range double.records {
		if record.Key.TenantID == tenant && record.Key.Credential == credential && !superseded[record.Key.Version] {
			return record, true, nil
		}
	}
	return ports.ExternalCarrierCredentialRecord{}, false, nil
}

func (double *credentialRegistryDouble) ListVersions(
	context.Context,
	domain.TenantID,
	domain.ExternalCarrierCredentialReference,
) ([]ports.ExternalCarrierCredentialRecord, error) {
	return double.records, nil
}

func (double *credentialRegistryDouble) Save(
	_ context.Context,
	record ports.ExternalCarrierCredentialRecord,
) (ports.ExternalCarrierCredentialSaveOutcome, error) {
	for _, existing := range double.records {
		if existing.Key == record.Key {
			return ports.ExternalCarrierCredentialAlreadyRegistered, nil
		}
	}
	double.records = append(double.records, record)
	return ports.ExternalCarrierCredentialSaved, nil
}

// ---- 请求体与 intake 替身（信封固定、字段从体收）----

type credentialBody struct {
	Credential     string `json:"credential"`
	Version        string `json:"version"`
	Assigner       string `json:"assigner"`
	IdentifiedKind string `json:"identifiedKind"`
	IdentifiedRef  string `json:"identifiedRef"`
	EffectiveFrom  string `json:"effectiveFrom"`
	EffectiveUntil string `json:"effectiveUntil"`
}

type credentialChangeBody struct {
	Credential  string `json:"credential"`
	Change      string `json:"change"`
	At          string `json:"at"`
	NewVersion  string `json:"newVersion"`
	Replacement string `json:"replacement"`
}

type credentialIntakeDouble struct {
	tenant domain.TenantID
	err    error
}

func parseOptionalTime(raw string) (time.Time, error) {
	if raw == "" {
		return time.Time{}, nil
	}
	return time.Parse(time.RFC3339, raw)
}

func (intake *credentialIntakeDouble) IntakeCredentialRegistration(
	_ context.Context,
	request *http.Request,
) (application.RegisterExternalCarrierCredentialCommand, error) {
	if intake.err != nil {
		return application.RegisterExternalCarrierCredentialCommand{}, intake.err
	}
	var body credentialBody
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		return application.RegisterExternalCarrierCredentialCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	from, err := parseOptionalTime(body.EffectiveFrom)
	if err != nil {
		return application.RegisterExternalCarrierCredentialCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	until, err := parseOptionalTime(body.EffectiveUntil)
	if err != nil {
		return application.RegisterExternalCarrierCredentialCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	return application.RegisterExternalCarrierCredentialCommand{
		TenantID:       intake.tenant,
		Credential:     body.Credential,
		Version:        body.Version,
		Assigner:       body.Assigner,
		IdentifiedKind: body.IdentifiedKind,
		IdentifiedRef:  body.IdentifiedRef,
		EffectiveFrom:  from,
		EffectiveUntil: until,
	}, nil
}

func (intake *credentialIntakeDouble) IntakeCredentialApplicabilityChange(
	_ context.Context,
	request *http.Request,
) (application.ChangeCredentialApplicabilityCommand, error) {
	if intake.err != nil {
		return application.ChangeCredentialApplicabilityCommand{}, intake.err
	}
	var body credentialChangeBody
	if err := json.NewDecoder(request.Body).Decode(&body); err != nil {
		return application.ChangeCredentialApplicabilityCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	at, err := parseOptionalTime(body.At)
	if err != nil {
		return application.ChangeCredentialApplicabilityCommand{}, fmt.Errorf("%w: %v", tfhttp.ErrMalformedRequest, err)
	}
	change, _ := domain.ParseCredentialStanding(body.Change)
	return application.ChangeCredentialApplicabilityCommand{
		TenantID:    intake.tenant,
		Credential:  body.Credential,
		Change:      change,
		At:          at,
		NewVersion:  body.NewVersion,
		Replacement: body.Replacement,
	}, nil
}

type credentialFixture struct {
	intake   *credentialIntakeDouble
	registry *credentialRegistryDouble
	register http.Handler
	change   http.Handler
}

func newCredentialFixture(t *testing.T) *credentialFixture {
	t.Helper()
	fixture := &credentialFixture{
		intake:   &credentialIntakeDouble{tenant: httpValue(t, domain.NewTenantID, "tenant-1")},
		registry: &credentialRegistryDouble{},
	}
	handler := application.NewRegisterExternalCarrierCredentialHandler(application.RegisterExternalCarrierCredentialDeps{
		Credentials: fixture.registry,
		Clock:       tfClock{at: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)},
	})
	fixture.register = tfhttp.NewRegisterExternalCarrierCredentialEndpoint(fixture.intake, handler)
	fixture.change = tfhttp.NewChangeExternalCarrierCredentialApplicabilityEndpoint(fixture.intake, handler)
	return fixture
}

func defaultCredentialBody() credentialBody {
	return credentialBody{
		Credential:     "carrier-x/1Z001",
		Version:        "ECV-1",
		Assigner:       "carrier-x",
		IdentifiedKind: "CARRIED_OBJECT",
		IdentifiedRef:  "PCL-1",
		EffectiveFrom:  "2026-09-01T00:00:00Z",
	}
}

type credentialView struct {
	Outcome               string `json:"outcome"`
	UndecidedReason       string `json:"undecidedReason"`
	ContinuationReference string `json:"continuationReference"`
	Credential            string `json:"credential"`
	Version               string `json:"version"`
	Assigner              string `json:"assigner"`
	IdentifiedKind        string `json:"identifiedKind"`
	IdentifiedRef         string `json:"identifiedRef"`
	EffectiveFrom         string `json:"effectiveFrom"`
	EffectiveUntil        string `json:"effectiveUntil"`
	Standing              string `json:"standing"`
	ChangedAt             string `json:"changedAt"`
	Supersedes            string `json:"supersedes"`
	ReplacedBy            string `json:"replacedBy"`
}

func decodeCredential(t *testing.T, recorder *httptest.ResponseRecorder) credentialView {
	t.Helper()
	var view credentialView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return view
}

// Covers: ADR-0022 首登新落一版 201；五件事原样透出；开放的适用范围不带终点。
func TestRegisterCredentialReportsCreatedWithTheFiveRegisteredThings(t *testing.T) {
	fixture := newCredentialFixture(t)

	response := postTo(t, fixture.register, "/transport-fulfillment-external-carrier-credential-registrations", defaultCredentialBody())

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", response.Code, response.Body.String())
	}
	view := decodeCredential(t, response)
	if view.Outcome != "CREDENTIAL_REGISTERED" || view.Credential != "carrier-x/1Z001" || view.Version != "ECV-1" {
		t.Fatalf("view = %+v", view)
	}
	if view.Assigner != "carrier-x" || view.IdentifiedKind != "CARRIED_OBJECT" || view.IdentifiedRef != "PCL-1" {
		t.Fatalf("分配方与标识对象没有透出：%+v", view)
	}
	if view.EffectiveFrom != "2026-09-01T00:00:00Z" || view.EffectiveUntil != "" || view.Standing != "APPLICABLE" {
		t.Fatalf("适用范围与状态没有透出：%+v", view)
	}
	if view.Supersedes != "" || view.ReplacedBy != "" || view.ChangedAt != "" {
		t.Fatalf("首版透出了改变才有的字段：%+v", view)
	}
}

// Covers: 改变适用关系新落一版 201，回指前版、终点与替代者透出；对已不适用的再改是 200 答案。
func TestChangeApplicabilityReportsCreatedAndExposesTheChain(t *testing.T) {
	fixture := newCredentialFixture(t)
	if seed := postTo(t, fixture.register, "/transport-fulfillment-external-carrier-credential-registrations", defaultCredentialBody()); seed.Code != http.StatusCreated {
		t.Fatalf("seed status = %d body = %s", seed.Code, seed.Body.String())
	}

	response := postTo(t, fixture.change, "/transport-fulfillment-external-carrier-credential-applicability-changes", credentialChangeBody{
		Credential:  "carrier-x/1Z001",
		Change:      "SUPERSEDED",
		At:          "2026-09-03T09:00:00Z",
		NewVersion:  "ECV-2",
		Replacement: "carrier-x/1Z001-B",
	})

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201（改变也新落了一版）; body = %s", response.Code, response.Body.String())
	}
	view := decodeCredential(t, response)
	if view.Outcome != "APPLICABILITY_CHANGED" || view.Version != "ECV-2" || view.Supersedes != "ECV-1" {
		t.Fatalf("view = %+v", view)
	}
	if view.Standing != "SUPERSEDED" || view.ReplacedBy != "carrier-x/1Z001-B" {
		t.Fatalf("view = %+v", view)
	}
	if view.EffectiveUntil != "2026-09-03T09:00:00Z" || view.ChangedAt != "2026-09-03T09:00:00Z" {
		t.Fatalf("终点与改变时间应落在改变时刻：%+v", view)
	}

	t.Run("changing an already superseded credential is an answer with the current version", func(t *testing.T) {
		again := postTo(t, fixture.change, "/transport-fulfillment-external-carrier-credential-applicability-changes", credentialChangeBody{
			Credential: "carrier-x/1Z001", Change: "REVOKED", At: "2026-09-04T09:00:00Z", NewVersion: "ECV-3",
		})
		if again.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200", again.Code)
		}
		if view := decodeCredential(t, again); view.Outcome != "NO_LONGER_APPLICABLE" || view.Version != "ECV-2" {
			t.Fatalf("view = %+v", view)
		}
	})
}

// Covers: ADR-0022「业务判别一律进响应体」——重放、冲突、已有版本链、未登记、未受理、未决都是形成了的
// 答案，都是 200，`outcome` 区分；`未决`带 `continuationReference` 与 `undecidedReason`。
func TestCredentialEndpointsReportOKForEveryAnswerThatRegisteredNothing(t *testing.T) {
	cases := map[string]struct {
		arrange    func(*credentialFixture) (http.Handler, any)
		want       string
		wantReason string
	}{
		"replay returns the existing version": {
			arrange: func(fixture *credentialFixture) (http.Handler, any) {
				postTo(t, fixture.register, "/x", defaultCredentialBody())
				return fixture.register, defaultCredentialBody()
			},
			want: "EXISTING_VERSION",
		},
		"a different object under the same version is a conflict": {
			arrange: func(fixture *credentialFixture) (http.Handler, any) {
				postTo(t, fixture.register, "/x", defaultCredentialBody())
				body := defaultCredentialBody()
				body.IdentifiedRef = "PCL-2"
				return fixture.register, body
			},
			want: "CONTENT_CONFLICT",
		},
		"a second first version for a registered credential is refused": {
			arrange: func(fixture *credentialFixture) (http.Handler, any) {
				postTo(t, fixture.register, "/x", defaultCredentialBody())
				body := defaultCredentialBody()
				body.Version = "ECV-2"
				return fixture.register, body
			},
			want: "CREDENTIAL_ALREADY_REGISTERED",
		},
		"an unknown identified kind is not accepted": {
			arrange: func(fixture *credentialFixture) (http.Handler, any) {
				body := defaultCredentialBody()
				body.IdentifiedKind = "TRACKING_NUMBER"
				return fixture.register, body
			},
			want: "INPUT_NOT_ACCEPTED",
		},
		"changing a never registered credential": {
			arrange: func(fixture *credentialFixture) (http.Handler, any) {
				return fixture.change, credentialChangeBody{Credential: "carrier-x/never", Change: "REVOKED", At: "2026-09-03T09:00:00Z", NewVersion: "ECV-2"}
			},
			want: "CREDENTIAL_NOT_REGISTERED",
		},
		"a registry failure is undecided with a continuation": {
			arrange: func(fixture *credentialFixture) (http.Handler, any) {
				fixture.registry.findErr = errors.New("registry down")
				return fixture.register, defaultCredentialBody()
			},
			want:       "REGISTRATION_UNDECIDED",
			wantReason: "CREDENTIAL_REGISTRY_UNAVAILABLE",
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newCredentialFixture(t)
			endpoint, body := testCase.arrange(fixture)
			response := postTo(t, endpoint, "/x", body)
			if response.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200（未决也是答案）; body = %s", response.Code, response.Body.String())
			}
			view := decodeCredential(t, response)
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

// 传输层分法：405 带 Allow；构造不出命令是 4xx；接入层其他失败与编排错误是 5xx，且 5xx 体里
// 不带 outcome；无名 outcome 是 500。错误体只有稳定 code 不回显请求（ADR-0029）。
func TestCredentialTransportProblemsCarryOnlyStableCodes(t *testing.T) {
	t.Run("method not allowed", func(t *testing.T) {
		fixture := newCredentialFixture(t)
		for name, endpoint := range map[string]http.Handler{"register": fixture.register, "change": fixture.change} {
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/x", nil))
			if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodPost {
				t.Fatalf("%s: status = %d allow = %q", name, recorder.Code, recorder.Header().Get("Allow"))
			}
		}
	})

	t.Run("a malformed request is 400 and does not echo the body", func(t *testing.T) {
		fixture := newCredentialFixture(t)
		recorder := httptest.NewRecorder()
		fixture.change.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("secret-not-json")))
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "MALFORMED_REQUEST") {
			t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
		}
		if strings.Contains(recorder.Body.String(), "secret-not-json") {
			t.Fatal("错误体回显了请求内容")
		}
	})

	t.Run("an intake infrastructure failure is 500", func(t *testing.T) {
		fixture := newCredentialFixture(t)
		fixture.intake.err = errors.New("session store down")
		response := postTo(t, fixture.register, "/x", defaultCredentialBody())
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "INTAKE_FAILED") {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
	})

	t.Run("an orchestration error is 500 NO_ANSWER_FORMED without an outcome", func(t *testing.T) {
		fixture := newCredentialFixture(t)
		endpoint := tfhttp.NewRegisterExternalCarrierCredentialEndpoint(fixture.intake, failingCredentialRegistrar{})
		response := postTo(t, endpoint, "/x", defaultCredentialBody())
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "NO_ANSWER_FORMED") {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "outcome") {
			t.Fatal("5xx 响应不得携带 outcome（ADR-0022）")
		}
	})

	t.Run("an unnamed outcome is a 500, not a new business answer", func(t *testing.T) {
		fixture := newCredentialFixture(t)
		endpoint := tfhttp.NewChangeExternalCarrierCredentialApplicabilityEndpoint(fixture.intake, unnamedCredentialRegistrar{})
		response := postTo(t, endpoint, "/x", credentialChangeBody{At: "2026-09-03T09:00:00Z"})
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "UNNAMED_OUTCOME") {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
	})
}

// Covers: ADR-0055「未配置即拒、不读内容」——两口同堵；编排是被调即失败的替身。
func TestUnconfiguredCredentialIntakeRefusesBothDoorsWithoutReadingTheBody(t *testing.T) {
	registrar := unreachableCredentialRegistrar{t: t}
	for name, endpoint := range map[string]http.Handler{
		"register": tfhttp.NewRegisterExternalCarrierCredentialEndpoint(tfhttp.UnconfiguredIntake{}, registrar),
		"change":   tfhttp.NewChangeExternalCarrierCredentialApplicabilityEndpoint(tfhttp.UnconfiguredIntake{}, registrar),
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

type failingCredentialRegistrar struct{}

func (failingCredentialRegistrar) Register(
	context.Context,
	application.RegisterExternalCarrierCredentialCommand,
) (application.RegisterExternalCarrierCredentialResult, error) {
	return application.RegisterExternalCarrierCredentialResult{}, errors.New("orchestration exploded")
}

func (failingCredentialRegistrar) ChangeApplicability(
	context.Context,
	application.ChangeCredentialApplicabilityCommand,
) (application.RegisterExternalCarrierCredentialResult, error) {
	return application.RegisterExternalCarrierCredentialResult{}, errors.New("orchestration exploded")
}

type unnamedCredentialRegistrar struct{}

func (unnamedCredentialRegistrar) Register(
	context.Context,
	application.RegisterExternalCarrierCredentialCommand,
) (application.RegisterExternalCarrierCredentialResult, error) {
	return application.RegisterExternalCarrierCredentialResult{}, nil
}

func (unnamedCredentialRegistrar) ChangeApplicability(
	context.Context,
	application.ChangeCredentialApplicabilityCommand,
) (application.RegisterExternalCarrierCredentialResult, error) {
	return application.RegisterExternalCarrierCredentialResult{}, nil
}

type unreachableCredentialRegistrar struct{ t *testing.T }

func (registrar unreachableCredentialRegistrar) Register(
	context.Context,
	application.RegisterExternalCarrierCredentialCommand,
) (application.RegisterExternalCarrierCredentialResult, error) {
	registrar.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.RegisterExternalCarrierCredentialResult{}, nil
}

func (registrar unreachableCredentialRegistrar) ChangeApplicability(
	context.Context,
	application.ChangeCredentialApplicabilityCommand,
) (application.RegisterExternalCarrierCredentialResult, error) {
	registrar.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.RegisterExternalCarrierCredentialResult{}, nil
}

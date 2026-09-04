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

// 本文件证有效时间规则登记端点（label-channel/19）的传输层纪律：首登与换版新落一版 201、其余答案 200
// 且 `outcome` 区分、未决带续办引用、三件正文与前版原样透出、载荷解码严格（未知键拒、身份键拒）、
// 405/400/500 分法、未配置 Intake 不读体。编排是真处理器接内存目录替身，不构造结果。

// ---- 目录替身（只插不改，当前版按回指派生）----

type ruleRegistryDouble struct {
	records []ports.EffectiveTimeRuleRecord
	findErr error
}

func (double *ruleRegistryDouble) FindByKey(
	_ context.Context,
	key ports.EffectiveTimeRuleKey,
) (ports.EffectiveTimeRuleRecord, bool, error) {
	if double.findErr != nil {
		return ports.EffectiveTimeRuleRecord{}, false, double.findErr
	}
	for _, record := range double.records {
		if record.Key == key {
			return record, true, nil
		}
	}
	return ports.EffectiveTimeRuleRecord{}, false, nil
}

func (double *ruleRegistryDouble) FindCurrent(
	_ context.Context,
	tenant domain.TenantID,
	source domain.TrackingSourceReference,
) (ports.EffectiveTimeRuleRecord, bool, error) {
	if double.findErr != nil {
		return ports.EffectiveTimeRuleRecord{}, false, double.findErr
	}
	superseded := map[domain.EffectiveTimeRuleVersion]bool{}
	for _, record := range double.records {
		if record.Key.TenantID == tenant && record.Key.Source == source {
			if prior, has := record.Rule.Supersedes(); has {
				superseded[prior] = true
			}
		}
	}
	for _, record := range double.records {
		if record.Key.TenantID == tenant && record.Key.Source == source && !superseded[record.Key.Version] {
			return record, true, nil
		}
	}
	return ports.EffectiveTimeRuleRecord{}, false, nil
}

func (double *ruleRegistryDouble) ListVersions(
	context.Context,
	domain.TenantID,
	domain.TrackingSourceReference,
) ([]ports.EffectiveTimeRuleRecord, error) {
	return double.records, nil
}

func (double *ruleRegistryDouble) Save(
	_ context.Context,
	record ports.EffectiveTimeRuleRecord,
) (ports.EffectiveTimeRuleSaveOutcome, error) {
	for _, existing := range double.records {
		if existing.Key == record.Key {
			return ports.EffectiveTimeRuleAlreadyRegistered, nil
		}
	}
	double.records = append(double.records, record)
	return ports.EffectiveTimeRuleSaved, nil
}

// ---- intake 替身：信封固定为一个租户，正文经产品定义的载荷解码器翻译 ----

type ruleIntakeDouble struct {
	tenant domain.TenantID
	err    error
}

func (intake *ruleIntakeDouble) IntakeEffectiveTimeRuleRegistration(
	_ context.Context,
	request *http.Request,
) (application.RegisterEffectiveTimeRuleCommand, error) {
	if intake.err != nil {
		return application.RegisterEffectiveTimeRuleCommand{}, intake.err
	}
	payload, err := tfhttp.DecodeEffectiveTimeRuleRegistrationPayload(request.Body)
	if err != nil {
		return application.RegisterEffectiveTimeRuleCommand{}, err
	}
	return payload.Command(intake.tenant)
}

type ruleFixture struct {
	intake   *ruleIntakeDouble
	registry *ruleRegistryDouble
	register http.Handler
}

func newRuleFixture(t *testing.T) *ruleFixture {
	t.Helper()
	fixture := &ruleFixture{
		intake:   &ruleIntakeDouble{tenant: httpValue(t, domain.NewTenantID, "tenant-1")},
		registry: &ruleRegistryDouble{},
	}
	handler := application.NewRegisterEffectiveTimeRuleHandler(application.RegisterEffectiveTimeRuleDeps{
		Rules: fixture.registry,
		Clock: tfClock{at: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)},
	})
	fixture.register = tfhttp.NewRegisterEffectiveTimeRuleEndpoint(fixture.intake, handler)
	return fixture
}

func defaultRuleBody() tfhttp.EffectiveTimeRuleRegistrationPayload {
	return tfhttp.EffectiveTimeRuleRegistrationPayload{
		Source:            "aggregator-a",
		Version:           "ETR-1",
		SourceTimeMeaning: "EVENT_OCCURRENCE",
		Anchor:            "OCCURRED_AT",
		OffsetSeconds:     0,
	}
}

type ruleView struct {
	Outcome               string `json:"outcome"`
	UndecidedReason       string `json:"undecidedReason"`
	ContinuationReference string `json:"continuationReference"`
	Source                string `json:"source"`
	Version               string `json:"version"`
	SourceTimeMeaning     string `json:"sourceTimeMeaning"`
	Anchor                string `json:"anchor"`
	OffsetSeconds         *int64 `json:"offsetSeconds"`
	Supersedes            string `json:"supersedes"`
	RecordedAt            string `json:"recordedAt"`
}

func decodeRule(t *testing.T, recorder *httptest.ResponseRecorder) ruleView {
	t.Helper()
	var view ruleView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v", err)
	}
	return view
}

// Covers: ADR-0022 首登新落一版 201；三件正文原样透出；首版不带前版；偏移即便为零也在场（它是登记出来的值）。
func TestRegisterRuleReportsCreatedWithTheRegisteredContent(t *testing.T) {
	fixture := newRuleFixture(t)

	response := postTo(t, fixture.register, "/transport-fulfillment-effective-time-rule-registrations", defaultRuleBody())

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", response.Code, response.Body.String())
	}
	view := decodeRule(t, response)
	if view.Outcome != "RULE_REGISTERED" || view.Source != "aggregator-a" || view.Version != "ETR-1" {
		t.Fatalf("view = %+v", view)
	}
	if view.SourceTimeMeaning != "EVENT_OCCURRENCE" || view.Anchor != "OCCURRED_AT" {
		t.Fatalf("正文没有透出：%+v", view)
	}
	if view.OffsetSeconds == nil || *view.OffsetSeconds != 0 {
		t.Fatalf("零偏移是登记出来的值，必须在场：%+v", view.OffsetSeconds)
	}
	if view.Supersedes != "" || view.RecordedAt != "2026-09-05T12:00:00Z" {
		t.Fatalf("首版透出了前版或登记时刻不对：%+v", view)
	}
}

// Covers: 换版新落一版 201 并透出回指的前版与负偏移；重放、冲突、未受理、未决都是 200 答案。
func TestRegisterRuleRevisesAndReportsEveryOtherAnswerAsOK(t *testing.T) {
	fixture := newRuleFixture(t)
	if seed := postTo(t, fixture.register, "/x", defaultRuleBody()); seed.Code != http.StatusCreated {
		t.Fatalf("seed status = %d body = %s", seed.Code, seed.Body.String())
	}

	revision := defaultRuleBody()
	revision.Version = "ETR-2"
	revision.SourceTimeMeaning = "SOURCE_PROCESSING"
	revision.Anchor = "RECEIVED_AT"
	revision.OffsetSeconds = -900
	response := postTo(t, fixture.register, "/x", revision)
	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201（换版也新落了一版）; body = %s", response.Code, response.Body.String())
	}
	view := decodeRule(t, response)
	if view.Outcome != "RULE_REVISED" || view.Version != "ETR-2" || view.Supersedes != "ETR-1" {
		t.Fatalf("view = %+v", view)
	}
	if view.Anchor != "RECEIVED_AT" || view.OffsetSeconds == nil || *view.OffsetSeconds != -900 {
		t.Fatalf("新正文没有透出：%+v", view)
	}

	cases := map[string]struct {
		arrange    func(*ruleFixture) any
		want       string
		wantReason string
	}{
		"replay returns the existing version": {
			arrange: func(*ruleFixture) any { return revision },
			want:    "EXISTING_VERSION",
		},
		"a different offset under the same version is a conflict": {
			arrange: func(*ruleFixture) any {
				body := revision
				body.OffsetSeconds = -60
				return body
			},
			want: "CONTENT_CONFLICT",
		},
		"an anchor outside the closed set is not accepted": {
			arrange: func(*ruleFixture) any {
				body := defaultRuleBody()
				body.Version = "ETR-3"
				body.Anchor = "NOW"
				return body
			},
			want: "INPUT_NOT_ACCEPTED",
		},
		"a catalogue failure is undecided with a continuation": {
			arrange: func(fixture *ruleFixture) any {
				fixture.registry.findErr = errors.New("catalogue down")
				return defaultRuleBody()
			},
			want:       "REGISTRATION_UNDECIDED",
			wantReason: "EFFECTIVE_TIME_RULE_REGISTRY_UNAVAILABLE",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			body := testCase.arrange(fixture)
			answer := postTo(t, fixture.register, "/x", body)
			if answer.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200（未决也是答案）; body = %s", answer.Code, answer.Body.String())
			}
			view := decodeRule(t, answer)
			if view.Outcome != testCase.want || view.UndecidedReason != testCase.wantReason {
				t.Fatalf("view = %+v, want %s / %s", view, testCase.want, testCase.wantReason)
			}
			if testCase.wantReason != "" && view.ContinuationReference == "" {
				t.Fatal("未决没有带 continuationReference，调用方无从续办")
			}
		})
	}
}

// Covers: 载荷由产品定义（ADR-0101 决定一）且只有内容没有身份——未知键拒、`tenant` 键拒、缺身份的翻译拒；
// 解码只判结构，词对不对留给领域构造门（集外的词到编排答未受理，不在这里 400）。
func TestTheRulePayloadDecoderIsStrictAndCarriesNoIdentity(t *testing.T) {
	t.Run("unknown keys are refused", func(t *testing.T) {
		_, err := tfhttp.DecodeEffectiveTimeRuleRegistrationPayload(strings.NewReader(`{"source":"a","version":"v","sourceTimeMeaning":"EVENT_OCCURRENCE","anchor":"OCCURRED_AT","offsetSeconds":0,"tenant":"T-9"}`))
		if !errors.Is(err, tfhttp.ErrMalformedRequest) {
			t.Fatalf("自报租户的键应被拒：%v", err)
		}
	})
	t.Run("a payload without an operator tenant cannot become a command", func(t *testing.T) {
		payload := defaultRuleBody()
		if _, err := payload.Command(domain.TenantID{}); !errors.Is(err, tfhttp.ErrOperatorIdentityMissing) {
			t.Fatalf("没有信封给的租户不得铸命令：%v", err)
		}
	})
	t.Run("the offset is carried as whole seconds", func(t *testing.T) {
		payload := defaultRuleBody()
		payload.OffsetSeconds = -900
		command, err := payload.Command(httpValue(t, domain.NewTenantID, "tenant-1"))
		if err != nil || command.Offset != -15*time.Minute || command.Anchor != "OCCURRED_AT" {
			t.Fatalf("command = %+v err = %v", command, err)
		}
	})
}

func TestRuleTransportProblemsCarryOnlyStableCodes(t *testing.T) {
	t.Run("method not allowed", func(t *testing.T) {
		fixture := newRuleFixture(t)
		recorder := httptest.NewRecorder()
		fixture.register.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/x", nil))
		if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodPost {
			t.Fatalf("status = %d allow = %q", recorder.Code, recorder.Header().Get("Allow"))
		}
	})
	t.Run("a malformed request is 400 and does not echo the body", func(t *testing.T) {
		fixture := newRuleFixture(t)
		recorder := httptest.NewRecorder()
		fixture.register.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("secret-not-json")))
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "MALFORMED_REQUEST") {
			t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
		}
		if strings.Contains(recorder.Body.String(), "secret-not-json") {
			t.Fatal("错误体回显了请求内容")
		}
	})
	t.Run("an intake infrastructure failure is 500", func(t *testing.T) {
		fixture := newRuleFixture(t)
		fixture.intake.err = errors.New("session store down")
		response := postTo(t, fixture.register, "/x", defaultRuleBody())
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "INTAKE_FAILED") {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
	})
	t.Run("an orchestration error is 500 NO_ANSWER_FORMED without an outcome", func(t *testing.T) {
		fixture := newRuleFixture(t)
		endpoint := tfhttp.NewRegisterEffectiveTimeRuleEndpoint(fixture.intake, failingRuleRegistrar{})
		response := postTo(t, endpoint, "/x", defaultRuleBody())
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "NO_ANSWER_FORMED") {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
		if strings.Contains(response.Body.String(), "outcome") {
			t.Fatal("5xx 响应不得携带 outcome（ADR-0022）")
		}
	})
	t.Run("an unnamed outcome is a 500, not a new business answer", func(t *testing.T) {
		fixture := newRuleFixture(t)
		endpoint := tfhttp.NewRegisterEffectiveTimeRuleEndpoint(fixture.intake, unnamedRuleRegistrar{})
		response := postTo(t, endpoint, "/x", defaultRuleBody())
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "UNNAMED_OUTCOME") {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
	})
}

// Covers: ADR-0055「未配置即拒、不读内容」；编排是被调即失败的替身。
func TestUnconfiguredRuleIntakeRefusesWithoutReadingTheBody(t *testing.T) {
	endpoint := tfhttp.NewRegisterEffectiveTimeRuleEndpoint(tfhttp.UnconfiguredIntake{}, unreachableRuleRegistrar{t: t})
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
}

type failingRuleRegistrar struct{}

func (failingRuleRegistrar) Register(
	context.Context,
	application.RegisterEffectiveTimeRuleCommand,
) (application.RegisterEffectiveTimeRuleResult, error) {
	return application.RegisterEffectiveTimeRuleResult{}, errors.New("orchestration exploded")
}

type unnamedRuleRegistrar struct{}

func (unnamedRuleRegistrar) Register(
	context.Context,
	application.RegisterEffectiveTimeRuleCommand,
) (application.RegisterEffectiveTimeRuleResult, error) {
	return application.RegisterEffectiveTimeRuleResult{}, nil
}

type unreachableRuleRegistrar struct{ t *testing.T }

func (registrar unreachableRuleRegistrar) Register(
	context.Context,
	application.RegisterEffectiveTimeRuleCommand,
) (application.RegisterEffectiveTimeRuleResult, error) {
	registrar.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.RegisterEffectiveTimeRuleResult{}, nil
}

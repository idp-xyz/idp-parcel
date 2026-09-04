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

// 本文件证有效时间显式判断端点（POST /transport-fulfillment-effective-time-judgments，label-channel/21）的
// 传输层纪律：新落一版 201、其余三格 200 且 `outcome` 区分、判断版本带依据与前版、「已按同值判过」把既有
// 版本的依据摆出来、未决带续办引用、意图没交出去时带 handoffReference、载荷严格解码（未知键拒、身份键拒、
// 时刻按 RFC 3339）、405/400/500 分法、未配置 Intake 不读体。编排是真处理器接内存登记册替身，不构造结果。

// ---- 登记册替身（只插不改，当前版按回指派生）----

type judgmentFactsDouble struct {
	records []ports.ExternalTrackingFactRecord
	findErr error
	saveErr error
}

func (double *judgmentFactsDouble) FindByKey(_ context.Context, key ports.ExternalTrackingFactKey) (ports.ExternalTrackingFactRecord, bool, error) {
	for _, record := range double.records {
		if record.Key == key {
			return record, true, nil
		}
	}
	return ports.ExternalTrackingFactRecord{}, false, nil
}

func (double *judgmentFactsDouble) FindBySourceEvent(
	context.Context, domain.TenantID, domain.TrackingSourceReference, domain.SourceEventReference,
) (ports.ExternalTrackingFactRecord, bool, error) {
	return ports.ExternalTrackingFactRecord{}, false, nil
}

func (double *judgmentFactsDouble) FindCurrent(
	_ context.Context,
	tenant domain.TenantID,
	fact domain.ExternalTrackingFactReference,
) (ports.ExternalTrackingFactRecord, bool, error) {
	if double.findErr != nil {
		return ports.ExternalTrackingFactRecord{}, false, double.findErr
	}
	superseded := map[domain.ExternalTrackingFactVersion]bool{}
	for _, record := range double.records {
		if record.Key.TenantID == tenant && record.Key.Fact == fact {
			if prior, has := record.Fact.Supersedes(); has {
				superseded[prior] = true
			}
		}
	}
	for _, record := range double.records {
		if record.Key.TenantID == tenant && record.Key.Fact == fact && !superseded[record.Key.Version] {
			return record, true, nil
		}
	}
	return ports.ExternalTrackingFactRecord{}, false, nil
}

func (double *judgmentFactsDouble) Save(_ context.Context, record ports.ExternalTrackingFactRecord) (ports.ExternalTrackingFactSaveOutcome, error) {
	if double.saveErr != nil {
		return ports.ExternalTrackingFactSaveOutcomeInvalid, double.saveErr
	}
	for _, existing := range double.records {
		if existing.Key == record.Key {
			return ports.ExternalTrackingFactAlreadyRegistered, nil
		}
	}
	double.records = append(double.records, record)
	return ports.ExternalTrackingFactSaved, nil
}

type judgmentIdentitiesDouble struct{ next int }

func (double *judgmentIdentitiesDouble) NextExternalTrackingFactReference(context.Context) (domain.ExternalTrackingFactReference, error) {
	return domain.ExternalTrackingFactReference{}, errors.New("a judgment never mints a fact reference")
}

func (double *judgmentIdentitiesDouble) NextExternalTrackingFactVersion(context.Context) (domain.ExternalTrackingFactVersion, error) {
	double.next++
	return domain.NewExternalTrackingFactVersion(fmt.Sprintf("EXTV-J%d", double.next))
}

type judgmentHandoffDouble struct {
	intents []ports.ExternalTrackingFactHandoffIntent
	err     error
}

func (double *judgmentHandoffDouble) HandOffExternalTrackingFact(_ context.Context, intent ports.ExternalTrackingFactHandoffIntent) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

// ---- intake 替身：信封固定为一个租户，正文经产品定义的载荷解码器翻译 ----

type judgmentIntakeDouble struct {
	tenant domain.TenantID
	err    error
}

func (intake *judgmentIntakeDouble) IntakeEffectiveTimeJudgment(
	_ context.Context,
	request *http.Request,
) (application.JudgeEffectiveTimeCommand, error) {
	if intake.err != nil {
		return application.JudgeEffectiveTimeCommand{}, intake.err
	}
	payload, err := tfhttp.DecodeEffectiveTimeJudgmentPayload(request.Body)
	if err != nil {
		return application.JudgeEffectiveTimeCommand{}, err
	}
	return payload.Command(intake.tenant)
}

var judgmentOccurredAt = time.Date(2026, 9, 4, 6, 0, 0, 0, time.UTC)

// pendingFactRecord 是一条已认领、有效时间待判断的事实（源未登规则时收编执行器留下的那种版本）。
func pendingFactRecord(t *testing.T, tenant domain.TenantID, fact string) ports.ExternalTrackingFactRecord {
	t.Helper()
	spec := domain.ExternalTrackingFactSpec{
		TenantID:    tenant,
		Fact:        httpValue(t, domain.NewExternalTrackingFactReference, fact),
		Version:     httpValue(t, domain.NewExternalTrackingFactVersion, fact+"/v1"),
		Source:      httpValue(t, domain.NewTrackingSourceReference, "aggregator-a"),
		Credential:  httpValue(t, domain.NewExternalCarrierCredentialReference, "carrier-x/1Z999"),
		Object:      httpValue(t, domain.NewCarriedObjectReference, "PCL-1"),
		SourceEvent: httpValue(t, domain.NewSourceEventReference, "evt-"+fact),
		Status:      httpValue(t, domain.NewRawStatusReference, "DELIVERED"),
		OccurredAt:  judgmentOccurredAt,
		ReceivedAt:  judgmentOccurredAt.Add(90 * time.Second),
		Effective:   domain.PendingEffectiveTime(),
	}
	adopted, err := domain.AdoptExternalCarrierTracking(spec)
	if err != nil {
		t.Fatalf("形成待判断事实：%v", err)
	}
	return ports.ExternalTrackingFactRecord{
		Key:        ports.ExternalTrackingFactKey{TenantID: tenant, Fact: spec.Fact, Version: spec.Version},
		Fact:       adopted,
		RecordedAt: spec.ReceivedAt.Add(time.Second),
	}
}

type judgmentFixture struct {
	intake  *judgmentIntakeDouble
	facts   *judgmentFactsDouble
	handoff *judgmentHandoffDouble
	judge   http.Handler
}

func newJudgmentFixture(t *testing.T) *judgmentFixture {
	t.Helper()
	tenant := httpValue(t, domain.NewTenantID, "tenant-1")
	fixture := &judgmentFixture{
		intake:  &judgmentIntakeDouble{tenant: tenant},
		facts:   &judgmentFactsDouble{records: []ports.ExternalTrackingFactRecord{pendingFactRecord(t, tenant, "EXTF-1")}},
		handoff: &judgmentHandoffDouble{},
	}
	handler := application.NewJudgeEffectiveTimeHandler(application.JudgeEffectiveTimeDeps{
		Facts:      fixture.facts,
		Identities: &judgmentIdentitiesDouble{},
		Downstream: fixture.handoff,
		Clock:      tfClock{at: time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC)},
	})
	fixture.judge = tfhttp.NewJudgeEffectiveTimeEndpoint(fixture.intake, handler)
	return fixture
}

func defaultJudgmentBody() tfhttp.EffectiveTimeJudgmentPayload {
	return tfhttp.EffectiveTimeJudgmentPayload{Fact: "EXTF-1", EffectiveAt: "2026-09-04T06:05:00Z"}
}

type judgmentView struct {
	Outcome               string  `json:"outcome"`
	UndecidedReason       string  `json:"undecidedReason"`
	ContinuationReference string  `json:"continuationReference"`
	HandoffReference      string  `json:"handoffReference"`
	Fact                  string  `json:"fact"`
	Version               string  `json:"version"`
	Source                string  `json:"source"`
	Object                string  `json:"object"`
	Status                string  `json:"status"`
	OccurredAt            string  `json:"occurredAt"`
	ReceivedAt            string  `json:"receivedAt"`
	EffectiveBasis        string  `json:"effectiveBasis"`
	EffectiveAt           *string `json:"effectiveAt"`
	EffectiveRule         *string `json:"effectiveRule"`
	EffectiveRuleVersion  *string `json:"effectiveRuleVersion"`
	Supersedes            *string `json:"supersedes"`
	RecordedAt            string  `json:"recordedAt"`
}

func decodeJudgment(t *testing.T, recorder *httptest.ResponseRecorder) judgmentView {
	t.Helper()
	var view judgmentView
	if err := json.Unmarshal(recorder.Body.Bytes(), &view); err != nil {
		t.Fatalf("decode: %v body=%s", err, recorder.Body.String())
	}
	return view
}

// Covers: ADR-0022 新落一版 201；判断版本回指被判断的那一版、依据是 JUDGED_EXPLICITLY、有效时间是所有者给的
// 那一个而不是发生时间；源给的内容一字不动地透出；意图已交出去时 handoffReference 缺席。
func TestJudgeReportsCreatedWithTheJudgedVersionSupersedingTheCurrentOne(t *testing.T) {
	fixture := newJudgmentFixture(t)

	response := postTo(t, fixture.judge, "/transport-fulfillment-effective-time-judgments", defaultJudgmentBody())

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body = %s", response.Code, response.Body.String())
	}
	view := decodeJudgment(t, response)
	if view.Outcome != "EFFECTIVE_TIME_JUDGED" || view.Fact != "EXTF-1" || view.Version != "EXTV-J1" {
		t.Fatalf("view = %+v", view)
	}
	if view.Supersedes == nil || *view.Supersedes != "EXTF-1/v1" {
		t.Fatalf("判断版本没有回指被判断的那一版：%+v", view.Supersedes)
	}
	if view.EffectiveBasis != "JUDGED_EXPLICITLY" || view.EffectiveAt == nil || *view.EffectiveAt != "2026-09-04T06:05:00Z" {
		t.Fatalf("依据或有效时间不对：%+v", view)
	}
	if view.EffectiveRule != nil || view.EffectiveRuleVersion != nil {
		t.Fatalf("显式判断不带规则版本：%+v", view)
	}
	if view.Source != "aggregator-a" || view.Object != "PCL-1" || view.Status != "DELIVERED" ||
		view.OccurredAt != "2026-09-04T06:00:00Z" || view.ReceivedAt != "2026-09-04T06:01:30Z" || view.RecordedAt != "2026-09-05T12:00:00Z" {
		t.Fatalf("源给的内容与两个时刻没有原样透出：%+v", view)
	}
	if view.HandoffReference != "" || len(fixture.handoff.intents) != 1 {
		t.Fatalf("意图应已交出且不带续办引用：ref=%q intents=%d", view.HandoffReference, len(fixture.handoff.intents))
	}
}

// Covers: 其余三格都是 200 答案——同值重放答「已按同值判过」并把既有版本的依据摆出来（票 21 红线）；判断一条
// 不存在的事实是未受理；登记册读不回是未决带续办引用。改值再判是新一版（201），回指上一判断版本。
func TestJudgeReportsEveryOtherAnswerAsOKAndRejudgesByANewVersion(t *testing.T) {
	fixture := newJudgmentFixture(t)
	if seed := postTo(t, fixture.judge, "/x", defaultJudgmentBody()); seed.Code != http.StatusCreated {
		t.Fatalf("seed status = %d body = %s", seed.Code, seed.Body.String())
	}

	replay := postTo(t, fixture.judge, "/x", defaultJudgmentBody())
	if replay.Code != http.StatusOK {
		t.Fatalf("重放 status = %d, want 200; body = %s", replay.Code, replay.Body.String())
	}
	view := decodeJudgment(t, replay)
	if view.Outcome != "ALREADY_JUDGED_AS_GIVEN" || view.Version != "EXTV-J1" || view.EffectiveBasis != "JUDGED_EXPLICITLY" ||
		view.EffectiveAt == nil || *view.EffectiveAt != "2026-09-04T06:05:00Z" {
		t.Fatalf("同值重放应交回既有判断版本及其依据：%+v", view)
	}
	if len(fixture.handoff.intents) != 2 {
		t.Fatalf("重放应重发同一份意图（无害）：%d", len(fixture.handoff.intents))
	}

	rejudged := defaultJudgmentBody()
	rejudged.EffectiveAt = "2026-09-04T06:30:00Z"
	response := postTo(t, fixture.judge, "/x", rejudged)
	if response.Code != http.StatusCreated {
		t.Fatalf("改值再判 status = %d, want 201; body = %s", response.Code, response.Body.String())
	}
	view = decodeJudgment(t, response)
	if view.Outcome != "EFFECTIVE_TIME_JUDGED" || view.Version != "EXTV-J2" || view.Supersedes == nil || *view.Supersedes != "EXTV-J1" {
		t.Fatalf("再判应形成回指上一判断版本的新版本：%+v", view)
	}

	cases := map[string]struct {
		arrange    func(*judgmentFixture) any
		want       string
		wantReason string
	}{
		"an unknown fact is not accepted": {
			arrange: func(*judgmentFixture) any {
				body := defaultJudgmentBody()
				body.Fact = "EXTF-nobody"
				return body
			},
			want: "INPUT_NOT_ACCEPTED",
		},
		"a blank effective time is not accepted": {
			arrange: func(*judgmentFixture) any {
				body := defaultJudgmentBody()
				body.EffectiveAt = ""
				return body
			},
			want: "INPUT_NOT_ACCEPTED",
		},
		"a registry failure is undecided with a continuation": {
			arrange: func(fixture *judgmentFixture) any {
				fixture.facts.findErr = errors.New("registry down")
				return defaultJudgmentBody()
			},
			want:       "JUDGMENT_UNDECIDED",
			wantReason: "TRACKING_FACT_REGISTRY_UNAVAILABLE",
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			// 各格共用一只登记册替身，而 map 遍历无序：「登记册故障」那格置下的 findErr 要在每格开头清掉。
			fixture.facts.findErr = nil
			body := testCase.arrange(fixture)
			answer := postTo(t, fixture.judge, "/x", body)
			if answer.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200（未决也是答案）; body = %s", answer.Code, answer.Body.String())
			}
			view := decodeJudgment(t, answer)
			if view.Outcome != testCase.want || view.UndecidedReason != testCase.wantReason {
				t.Fatalf("view = %+v, want %s / %s", view, testCase.want, testCase.wantReason)
			}
			if testCase.wantReason != "" && view.ContinuationReference == "" {
				t.Fatal("未决没有带 continuationReference，调用方无从续办")
			}
			// 这三格都没有记录可带：未受理无中生有不出事实，未决连当前版都没读回来。
			if view.Fact != "" || view.Version != "" || view.EffectiveBasis != "" {
				t.Fatalf("没有记录的答案不得带记录字段：%+v", view)
			}
		})
	}
}

// Covers: 新版本已登记但意图没交出去——判断照样是 201 答案，handoffReference 非空让调用方知道 VE 那一半
// 还欠着，重放会重发。
func TestJudgeCarriesAHandoffReferenceWhenTheIntentCouldNotBeHandedOff(t *testing.T) {
	fixture := newJudgmentFixture(t)
	fixture.handoff.err = errors.New("outbox down")

	response := postTo(t, fixture.judge, "/x", defaultJudgmentBody())

	if response.Code != http.StatusCreated {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	view := decodeJudgment(t, response)
	if view.Outcome != "EFFECTIVE_TIME_JUDGED" || view.HandoffReference == "" {
		t.Fatalf("意图没交出去时应带 handoffReference：%+v", view)
	}
}

// Covers: 载荷由产品定义（ADR-0101 决定一）且只有内容没有身份——未知键拒、`tenant` 键拒、缺身份的翻译拒；
// 时刻按 RFC 3339 解，解不出是坏报文（400）而不是业务未受理（改的是报文不是值）。
func TestTheJudgmentPayloadDecoderIsStrictAndCarriesNoIdentity(t *testing.T) {
	t.Run("unknown keys are refused", func(t *testing.T) {
		_, err := tfhttp.DecodeEffectiveTimeJudgmentPayload(strings.NewReader(`{"fact":"EXTF-1","effectiveAt":"2026-09-04T06:05:00Z","tenant":"T-9"}`))
		if !errors.Is(err, tfhttp.ErrMalformedRequest) {
			t.Fatalf("自报租户的键应被拒：%v", err)
		}
	})
	t.Run("a payload without an operator tenant cannot become a command", func(t *testing.T) {
		if _, err := defaultJudgmentBody().Command(domain.TenantID{}); !errors.Is(err, tfhttp.ErrOperatorIdentityMissing) {
			t.Fatalf("没有信封给的租户不得铸命令：%v", err)
		}
	})
	t.Run("the effective time is carried as an instant in UTC", func(t *testing.T) {
		payload := defaultJudgmentBody()
		payload.EffectiveAt = "2026-09-04T14:05:00+08:00"
		command, err := payload.Command(httpValue(t, domain.NewTenantID, "tenant-1"))
		if err != nil || !command.EffectiveAt.Equal(time.Date(2026, 9, 4, 6, 5, 0, 0, time.UTC)) || command.Fact != "EXTF-1" {
			t.Fatalf("command = %+v err = %v", command, err)
		}
	})
	t.Run("an unparsable instant is a malformed request", func(t *testing.T) {
		payload := defaultJudgmentBody()
		payload.EffectiveAt = "yesterday"
		if _, err := payload.Command(httpValue(t, domain.NewTenantID, "tenant-1")); !errors.Is(err, tfhttp.ErrMalformedRequest) {
			t.Fatalf("解不出的时刻应是坏报文：%v", err)
		}
	})
	t.Run("a blank instant is left to the orchestration", func(t *testing.T) {
		payload := defaultJudgmentBody()
		payload.EffectiveAt = ""
		command, err := payload.Command(httpValue(t, domain.NewTenantID, "tenant-1"))
		if err != nil || !command.EffectiveAt.IsZero() {
			t.Fatalf("空时刻应交给编排答未受理，不在传输层 400：%+v %v", command, err)
		}
	})
}

func TestJudgmentTransportProblemsCarryOnlyStableCodes(t *testing.T) {
	t.Run("method not allowed", func(t *testing.T) {
		fixture := newJudgmentFixture(t)
		recorder := httptest.NewRecorder()
		fixture.judge.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/x", nil))
		if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodPost {
			t.Fatalf("status = %d allow = %q", recorder.Code, recorder.Header().Get("Allow"))
		}
	})
	t.Run("a malformed request is 400 and does not echo the body", func(t *testing.T) {
		fixture := newJudgmentFixture(t)
		recorder := httptest.NewRecorder()
		fixture.judge.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("secret-not-json")))
		if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "MALFORMED_REQUEST") {
			t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body.String())
		}
		if strings.Contains(recorder.Body.String(), "secret-not-json") {
			t.Fatal("错误体回显了请求内容")
		}
	})
	t.Run("an intake infrastructure failure is 500", func(t *testing.T) {
		fixture := newJudgmentFixture(t)
		fixture.intake.err = errors.New("session store down")
		response := postTo(t, fixture.judge, "/x", defaultJudgmentBody())
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "INTAKE_FAILED") {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
	})
	t.Run("an orchestration error is 500 NO_ANSWER_FORMED without an outcome", func(t *testing.T) {
		fixture := newJudgmentFixture(t)
		endpoint := tfhttp.NewJudgeEffectiveTimeEndpoint(fixture.intake, failingJudge{})
		response := postTo(t, endpoint, "/x", defaultJudgmentBody())
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "NO_ANSWER_FORMED") {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
		assertNoOutcome(t, response)
	})
	t.Run("an unnamed outcome is a 500, not a new business answer", func(t *testing.T) {
		fixture := newJudgmentFixture(t)
		endpoint := tfhttp.NewJudgeEffectiveTimeEndpoint(fixture.intake, unnamedJudge{})
		response := postTo(t, endpoint, "/x", defaultJudgmentBody())
		if response.Code != http.StatusInternalServerError || !strings.Contains(response.Body.String(), "UNNAMED_OUTCOME") {
			t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
		}
	})
}

// Covers: ADR-0055「未配置即拒、不读内容」；编排是被调即失败的替身。
func TestUnconfiguredJudgmentIntakeRefusesWithoutReadingTheBody(t *testing.T) {
	endpoint := tfhttp.NewJudgeEffectiveTimeEndpoint(tfhttp.UnconfiguredIntake{}, unreachableJudge{t: t})
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

type failingJudge struct{}

func (failingJudge) Judge(context.Context, application.JudgeEffectiveTimeCommand) (application.JudgeEffectiveTimeResult, error) {
	return application.JudgeEffectiveTimeResult{}, errors.New("orchestration exploded")
}

type unnamedJudge struct{}

func (unnamedJudge) Judge(context.Context, application.JudgeEffectiveTimeCommand) (application.JudgeEffectiveTimeResult, error) {
	return application.JudgeEffectiveTimeResult{}, nil
}

type unreachableJudge struct{ t *testing.T }

func (judge unreachableJudge) Judge(context.Context, application.JudgeEffectiveTimeCommand) (application.JudgeEffectiveTimeResult, error) {
	judge.t.Fatal("a request passed the unconfigured intake and reached the orchestration")
	return application.JudgeEffectiveTimeResult{}, nil
}

package settlementhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	settlementhttp "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/http"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件对外部资金事实采用 / 更正两个在线登记端点（票 sa-cc/31，27 裁决 2 第二步）证传输面：只收 POST、
// 未配置 403 且不读内容、Intake 失败分流、答案逐名转写、响应形封闭。答案代数是 application.FundsOutcome
// 那一族，转写的判据与受控 CLI 的 fundsAnswer 同一条（按恢复动作归格）：治理答案 200 原名、采用 201；
// `未决`今天只有依赖故障一种来路，折成「没形成答案」；「行已落、信封未出」不与已采用同格。

// fundsIntakeDouble 交回测试预先备好的命令，对请求零读取。
type fundsIntakeDouble struct {
	adopt      application.AdoptFundsFactCommand
	correction application.CorrectFundsFactCommand
	err        error
}

func (double fundsIntakeDouble) IntakeExternalFundsFactRegistration(
	context.Context, *http.Request,
) (application.AdoptFundsFactCommand, error) {
	return double.adopt, double.err
}

func (double fundsIntakeDouble) IntakeExternalFundsFactCorrectionRegistration(
	context.Context, *http.Request,
) (application.CorrectFundsFactCommand, error) {
	return double.correction, double.err
}

// fundsRegistrarDouble 顶替采用编排：两个方法长在一个类型上，端点各自只消费其中一个接口，接错编译期就红。
// 它只用于「编排返错」与「零值答案」两格——FundsResult 的字段不导出，其余各格只有真用例造得出。
type fundsRegistrarDouble struct {
	result application.FundsResult
	err    error
	called bool
}

func (double *fundsRegistrarDouble) AdoptFact(
	context.Context, application.AdoptFundsFactCommand,
) (application.FundsResult, error) {
	double.called = true
	return double.result, double.err
}

func (double *fundsRegistrarDouble) CorrectFact(
	context.Context, application.CorrectFundsFactCommand,
) (application.FundsResult, error) {
	double.called = true
	return double.result, double.err
}

type unreachableFundsRegistrar struct{ t *testing.T }

func (registrar unreachableFundsRegistrar) AdoptFact(
	context.Context, application.AdoptFundsFactCommand,
) (application.FundsResult, error) {
	registrar.t.Fatal("a request passed the unconfigured intake and reached the adoption orchestration")
	return application.FundsResult{}, nil
}

func (registrar unreachableFundsRegistrar) CorrectFact(
	context.Context, application.CorrectFundsFactCommand,
) (application.FundsResult, error) {
	registrar.t.Fatal("a request passed the unconfigured intake and reached the correction orchestration")
	return application.FundsResult{}, nil
}

// fundsEndpoints 遍历两个登记端点，各配「被调即失败」的编排替身与生产侧的未配置 Intake。
func fundsEndpoints(t *testing.T) map[string]http.Handler {
	t.Helper()
	unconfigured := settlementhttp.UnconfiguredIntake{}
	return map[string]http.Handler{
		"采用": settlementhttp.NewRegisterExternalFundsFactEndpoint(unconfigured, unreachableFundsRegistrar{t: t}),
		"更正": settlementhttp.NewRegisterExternalFundsFactCorrectionEndpoint(unconfigured, unreachableFundsRegistrar{t: t}),
	}
}

// readProbe 记录报文有没有被读过。用旗标而不是当场失败，是为了把「读了」报成一次明确断言。
type readProbe struct{ read bool }

func (probe *readProbe) Read([]byte) (int, error) {
	probe.read = true
	return 0, io.EOF
}

func TestExternalFundsFactRegistrationEndpointsOnlyAcceptPost(t *testing.T) {
	for name, endpoint := range fundsEndpoints(t) {
		t.Run(name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/probe", nil))
			if recorder.Code != http.StatusMethodNotAllowed {
				t.Fatalf("GET 答 %d, want 405", recorder.Code)
			}
			if allow := recorder.Header().Get("Allow"); allow != http.MethodPost {
				t.Fatalf("Allow = %q, want POST", allow)
			}
		})
	}
}

// Covers: ADR-0055「未配置即拒、不读内容」；ADR-0085 决定二 写准入不另立形（判据 2 / 3 的 403 一格）。
func TestUnconfiguredExternalFundsFactRegistrationRefusesWithoutReadingTheBody(t *testing.T) {
	for name, endpoint := range fundsEndpoints(t) {
		t.Run(name, func(t *testing.T) {
			probe := &readProbe{}
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", probe))
			if recorder.Code != http.StatusForbidden {
				t.Fatalf("未配置答 %d, want 403", recorder.Code)
			}
			if code := problemCode(t, recorder); code != "ACCESS_CHANNEL_NOT_CONFIGURED" {
				t.Fatalf("错误码 = %q", code)
			}
			assertNoOutcome(t, recorder)
			if probe.read {
				t.Fatal("未配置 Intake 读了登记载荷")
			}
		})
	}
}

// Covers: ADR-0055「答复对一切请求内容与自报身份一致」——载荷里的 tenantId 是这一面最危险的自报身份。
func TestUnconfiguredExternalFundsFactRegistrationAnswersEveryRequestIdentically(t *testing.T) {
	for name, endpoint := range fundsEndpoints(t) {
		t.Run(name, func(t *testing.T) {
			baseline := httptest.NewRecorder()
			endpoint.ServeHTTP(baseline, httptest.NewRequest(http.MethodPost, "/probe", nil))
			variants := map[string]*http.Request{
				"登记快照": httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(`{"tenantId":"SYN-T-SA31","factRef":"SYN-FACT-1"}`)),
				"畸形载荷": httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader("!!not-json!!")),
				"另一租户": httptest.NewRequest(http.MethodPost, "/probe", strings.NewReader(`{"tenantId":"SYN-T-OTHER"}`)),
			}
			reported := httptest.NewRequest(http.MethodPost, "/probe", nil)
			reported.Header.Set("X-Reported-Tenant", "SYN-T-SA31")
			variants["自报身份头部"] = reported
			for variant, request := range variants {
				recorder := httptest.NewRecorder()
				endpoint.ServeHTTP(recorder, request)
				if recorder.Code != baseline.Code || recorder.Body.String() != baseline.Body.String() {
					t.Fatalf("%s：答复与基线不同：%d %s vs %d %s",
						variant, recorder.Code, recorder.Body.String(), baseline.Code, baseline.Body.String())
				}
			}
		})
	}
}

// Covers: 判据 3「Intake 拒 → 4xx 带 problem」；三格（未配置 / 畸形 / 故障）是接入渠道这一层的状态，
// 读面与写面同一套映射。
func TestExternalFundsFactRegistrationMapsIntakeFailures(t *testing.T) {
	cases := []struct {
		name       string
		err        error
		wantStatus int
		wantCode   string
	}{
		{"畸形", fmt.Errorf("登记载荷缺格: %w", settlementhttp.ErrMalformedRequest), http.StatusBadRequest, "MALFORMED_REQUEST"},
		{"Intake 故障", errors.New("认证后端寄了"), http.StatusInternalServerError, "INTAKE_FAILED"},
	}
	for _, spec := range cases {
		t.Run(spec.name, func(t *testing.T) {
			registrar := &fundsRegistrarDouble{}
			endpoint := settlementhttp.NewRegisterExternalFundsFactEndpoint(fundsIntakeDouble{err: spec.err}, registrar)
			recorder := httptest.NewRecorder()
			endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))
			if recorder.Code != spec.wantStatus {
				t.Fatalf("答 %d, want %d", recorder.Code, spec.wantStatus)
			}
			if code := problemCode(t, recorder); code != spec.wantCode {
				t.Fatalf("错误码 = %q, want %s", code, spec.wantCode)
			}
			if registrar.called {
				t.Fatal("Intake 没交出命令，编排不该被调到")
			}
		})
	}
}

// —— 转写断言走真编排 ——
//
// FundsResult 的字段不导出、也没有构造函数，本文件不伪造它：采用四格与更正的链头判断全在用例里，每一格
// 都用真 handler 接存储替身造出来——伪造得出的那份代数与生产的是不是同一份，正是这几条断言要证的东西
// （判据同 customshttp 的 register_credential_and_duty_test）。只有「零值答案」与「编排返错」两格用替身。

// fundsStoreStub 是采用编排六口一体的替身：资金事实库三口按格给定，映射 / 核销 / 核销交接三口不在采用与更正
// 路径上、只为过构造门；资金事实交接口默认成功，handoffErr 非空即「行已落、信封未出」那一格。
type fundsStoreStub struct {
	findVersionErr error
	// head 是链头那一版：FindByKey 找得到时交回它。headFound 为真即事实已有版本链。
	head      ports.FundsFactRecord
	headFound bool
	// saveOutcome 是 Save 的答复；saveErr 非空即资金事实库不可用。
	saveOutcome ports.FundsFactSaveOutcome
	saveErr     error
	// headAfterSave 为真时链头只在 Save 之后才找得到——「Save 答已采用、读回赢家」那条重放路要这一形。
	headAfterSave bool
	saved         bool

	handoffErr error
}

func (stub *fundsStoreStub) FindByKey(context.Context, ports.FundsFactKey) (ports.FundsFactRecord, bool, error) {
	if stub.headAfterSave && !stub.saved {
		return ports.FundsFactRecord{}, false, nil
	}
	return stub.head, stub.headFound, nil
}

func (stub *fundsStoreStub) FindVersion(
	context.Context, ports.FundsFactKey, domain.FundsFactVersion,
) (ports.FundsFactRecord, bool, error) {
	return ports.FundsFactRecord{}, false, stub.findVersionErr
}

func (stub *fundsStoreStub) Save(context.Context, ports.FundsFactRecord) (ports.FundsFactSaveOutcome, error) {
	stub.saved = true
	return stub.saveOutcome, stub.saveErr
}

func (stub *fundsStoreStub) HandOffExternalFundsFact(context.Context, ports.ExternalFundsFactIntent) error {
	return stub.handoffErr
}

// 以下三口不在采用 / 更正路径上；被调到就是端点接错了编排的方法。
type unreachableFundsSideStores struct{ t *testing.T }

func (stores unreachableFundsSideStores) fail() {
	stores.t.Fatal("采用 / 更正端点碰到了映射或核销那几口")
}

func (stores unreachableFundsSideStores) FindByKey(
	context.Context, ports.FundsMappingKey,
) (ports.FundsMappingRecord, bool, error) {
	stores.fail()
	return ports.FundsMappingRecord{}, false, nil
}

func (stores unreachableFundsSideStores) Save(
	context.Context, ports.FundsMappingRecord,
) (ports.FundsMappingSaveOutcome, error) {
	stores.fail()
	return ports.FundsMappingSaveOutcomeInvalid, nil
}

type unreachableApplicationStore struct{ t *testing.T }

func (store unreachableApplicationStore) FindByKey(
	context.Context, ports.SettlementApplicationKey,
) (ports.SettlementApplicationRecord, bool, error) {
	store.t.Fatal("采用 / 更正端点碰到了核销库")
	return ports.SettlementApplicationRecord{}, false, nil
}

func (store unreachableApplicationStore) Save(
	context.Context, ports.SettlementApplicationRecord,
) (ports.SettlementApplicationSaveOutcome, error) {
	store.t.Fatal("采用 / 更正端点碰到了核销库")
	return ports.SettlementApplicationSaveOutcomeInvalid, nil
}

func (store unreachableApplicationStore) Replace(context.Context, ports.SettlementApplicationRecord) (bool, error) {
	store.t.Fatal("采用 / 更正端点碰到了核销库")
	return false, nil
}

func (store unreachableApplicationStore) HandOffSettlementApplication(
	context.Context, ports.SettlementApplicationIntent,
) error {
	store.t.Fatal("采用 / 更正端点碰到了核销交接口")
	return nil
}

type fundsTestClock struct{}

func (fundsTestClock) Now() time.Time { return time.Date(2026, 9, 15, 7, 0, 0, 0, time.UTC) }

func fundsValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return value
}

// fundsHandlerOver 把真编排装在替身上；它本身就满足两个 Registrar 契约（方法名即用例方法名）。
func fundsHandlerOver(t *testing.T, stub *fundsStoreStub) *application.MapExternalFundsHandler {
	t.Helper()
	handler, err := application.NewMapExternalFundsHandler(application.MapExternalFundsDeps{
		Facts:        stub,
		Mappings:     unreachableFundsSideStores{t: t},
		Applications: unreachableApplicationStore{t: t},
		Downstream:   unreachableApplicationStore{t: t},
		FactHandoff:  stub,
		Clock:        fundsTestClock{},
	})
	if err != nil {
		t.Fatalf("构造编排：%v", err)
	}
	return handler
}

// adoptCommand 是一份立得住的首版采用（身份全取 SYN- 前缀；币种取测试码 XTS）。
func adoptCommand(t *testing.T) application.AdoptFundsFactCommand {
	t.Helper()
	return application.AdoptFundsFactCommand{
		TenantID:    fundsValue(t, domain.NewTenantID, "SYN-T-SA31"),
		Fact:        "SYN-FACT-1",
		Source:      "SYN-SOURCE-BANK-1",
		Payer:       "SYN-PAYER-1",
		Kind:        domain.FundsReceiptConfirmed,
		Currency:    "XTS",
		AmountMinor: 8000,
		Version:     "SYN-FACT-1/v1",
		OccurredAt:  time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC),
	}
}

// headRecord 造链头那一版（与 adoptCommand 同一条事实的 v1），供更正回指。
func headRecord(t *testing.T) ports.FundsFactRecord {
	t.Helper()
	fact, err := domain.AdoptExternalFundsFact(domain.ExternalFundsFactSpec{
		Fact:        fundsValue(t, domain.NewFundsFactReference, "SYN-FACT-1"),
		Source:      fundsValue(t, domain.NewFundsSourceRegistrationReference, "SYN-SOURCE-BANK-1"),
		Kind:        domain.FundsReceiptConfirmed,
		Currency:    fundsValue(t, domain.NewCurrencyCode, "XTS"),
		AmountMinor: 8000,
		Version:     fundsValue(t, domain.NewFundsFactVersion, "SYN-FACT-1/v1"),
		OccurredAt:  time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("造链头：%v", err)
	}
	return ports.FundsFactRecord{
		Key: ports.FundsFactKey{
			TenantID: fundsValue(t, domain.NewTenantID, "SYN-T-SA31"),
			Fact:     fundsValue(t, domain.NewFundsFactReference, "SYN-FACT-1"),
		},
		ContentDigest: "SYN-DIGEST-V1",
		Fact:          fact,
		RecordedAt:    fundsTestClock{}.Now().Add(-time.Hour),
	}
}

func correctionCommand(t *testing.T) application.CorrectFundsFactCommand {
	t.Helper()
	return application.CorrectFundsFactCommand{
		TenantID:    fundsValue(t, domain.NewTenantID, "SYN-T-SA31"),
		Fact:        "SYN-FACT-1",
		Corrects:    "SYN-FACT-1/v1",
		Version:     "SYN-FACT-1/v2",
		AmountMinor: 9000,
		CorrectedAt: time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC),
	}
}

type fundsAnswerCase struct {
	name       string
	wantStatus int
	wantAnswer string
}

// Covers: 判据 3——采用端点逐名过线：已采用 201；已存在 / 内容冲突 / 未受理 200 原名（治理答案不是失败，
// ADR-0022 否决的正是 409 / 422 那条路）；资金事实库不可用的未决 5xx（没形成答案，不带 outcome）。
func TestExternalFundsFactRegistrationTranscribesTheAnswerAlgebra(t *testing.T) {
	command := adoptCommand(t)
	blankFact := command
	blankFact.Fact = ""

	cases := []struct {
		fundsAnswerCase
		command application.AdoptFundsFactCommand
		stub    *fundsStoreStub
	}{
		{fundsAnswerCase{"已采用", http.StatusCreated, "FUNDS_FACT_ADOPTED"}, command,
			&fundsStoreStub{saveOutcome: ports.FundsFactSaved}},
		{fundsAnswerCase{"已存在（并发先落、读回赢家）", http.StatusOK, "EXISTING_FUNDS_FACT"}, command,
			&fundsStoreStub{saveOutcome: ports.FundsFactAlreadyAdopted, head: headRecord(t), headFound: true, headAfterSave: true}},
		{fundsAnswerCase{"内容冲突（同事实第二个首版）", http.StatusOK, "FUNDS_FACT_CONFLICT"}, command,
			&fundsStoreStub{head: headRecord(t), headFound: true}},
		{fundsAnswerCase{"未受理", http.StatusOK, "SOURCE_NOT_ACCEPTED"}, blankFact, &fundsStoreStub{}},
	}
	for _, spec := range cases {
		t.Run(spec.name, func(t *testing.T) {
			endpoint := settlementhttp.NewRegisterExternalFundsFactEndpoint(
				fundsIntakeDouble{adopt: spec.command}, fundsHandlerOver(t, spec.stub))
			assertFundsAnswer(t, endpoint, spec.fundsAnswerCase)
		})
	}

	t.Run("资金事实库不可用", func(t *testing.T) {
		endpoint := settlementhttp.NewRegisterExternalFundsFactEndpoint(
			fundsIntakeDouble{adopt: command},
			fundsHandlerOver(t, &fundsStoreStub{findVersionErr: errors.New("funds fact store unavailable")}))
		assertNoAnswerFormed(t, endpoint, "NO_ANSWER_FORMED")
	})
	t.Run("编排返错", func(t *testing.T) {
		endpoint := settlementhttp.NewRegisterExternalFundsFactEndpoint(
			fundsIntakeDouble{adopt: command}, &fundsRegistrarDouble{err: errors.New("事务壳寄了")})
		assertNoAnswerFormed(t, endpoint, "NO_ANSWER_FORMED")
	})
	t.Run("没有名字的答案不上线", func(t *testing.T) {
		endpoint := settlementhttp.NewRegisterExternalFundsFactEndpoint(
			fundsIntakeDouble{adopt: command}, &fundsRegistrarDouble{})
		assertNoAnswerFormed(t, endpoint, "UNNAMED_OUTCOME")
	})
}

// Covers: 判据 3「续办引用那一格单独一例」——行已落、信封未出（FundsHandoffReference 非空）不与已采用同格：
// 折成 5xx 且带专名错误码，不带 outcome。判据与 fundsAnswer 同一条：它需要人重发同一份补发同一封，而 5xx 正是
// 「重发同一份」的恢复动作；答 2xx 带原名会让一个只看状态与 outcome 的调用方把 CC 永远等不到的那封当成已出。
func TestAnAdoptedFactWhoseHandoffDidNotLeaveIsNotReportedAsAdopted(t *testing.T) {
	for name, stub := range map[string]*fundsStoreStub{
		"首版采用、信封未出": {saveOutcome: ports.FundsFactSaved, handoffErr: errors.New("outbox unavailable")},
		"重放已存在、补发仍未出": {saveOutcome: ports.FundsFactAlreadyAdopted, head: headRecord(t), headFound: true,
			headAfterSave: true, handoffErr: errors.New("outbox unavailable")},
	} {
		t.Run(name, func(t *testing.T) {
			endpoint := settlementhttp.NewRegisterExternalFundsFactEndpoint(
				fundsIntakeDouble{adopt: adoptCommand(t)}, fundsHandlerOver(t, stub))
			assertNoAnswerFormed(t, endpoint, "HANDOFF_NOT_SENT")
		})
	}
}

// Covers: 判据 3——更正端点逐名过线：回指链头的更正 201 已采用；回指自己 / 回指非链头 / 更正未采用的事实都是
// `未受理` 200 原名（提交矛盾，改内容再来，不是重试）；库不可用 5xx。
func TestExternalFundsFactCorrectionRegistrationTranscribesTheAnswerAlgebra(t *testing.T) {
	command := correctionCommand(t)
	selfReferencing := command
	selfReferencing.Corrects = command.Version
	staleHead := command
	staleHead.Corrects = "SYN-FACT-1/v0"

	cases := []struct {
		fundsAnswerCase
		command application.CorrectFundsFactCommand
		stub    *fundsStoreStub
	}{
		{fundsAnswerCase{"更正已采用", http.StatusCreated, "FUNDS_FACT_ADOPTED"}, command,
			&fundsStoreStub{head: headRecord(t), headFound: true, saveOutcome: ports.FundsFactSaved}},
		{fundsAnswerCase{"回指自己", http.StatusOK, "SOURCE_NOT_ACCEPTED"}, selfReferencing, &fundsStoreStub{}},
		{fundsAnswerCase{"回指非链头", http.StatusOK, "SOURCE_NOT_ACCEPTED"}, staleHead,
			&fundsStoreStub{head: headRecord(t), headFound: true}},
		{fundsAnswerCase{"更正未采用的事实", http.StatusOK, "SOURCE_NOT_ACCEPTED"}, command, &fundsStoreStub{}},
	}
	for _, spec := range cases {
		t.Run(spec.name, func(t *testing.T) {
			endpoint := settlementhttp.NewRegisterExternalFundsFactCorrectionEndpoint(
				fundsIntakeDouble{correction: spec.command}, fundsHandlerOver(t, spec.stub))
			assertFundsAnswer(t, endpoint, spec.fundsAnswerCase)
		})
	}

	t.Run("资金事实库不可用", func(t *testing.T) {
		endpoint := settlementhttp.NewRegisterExternalFundsFactCorrectionEndpoint(
			fundsIntakeDouble{correction: command},
			fundsHandlerOver(t, &fundsStoreStub{findVersionErr: errors.New("funds fact store unavailable")}))
		assertNoAnswerFormed(t, endpoint, "NO_ANSWER_FORMED")
	})
	t.Run("编排返错", func(t *testing.T) {
		endpoint := settlementhttp.NewRegisterExternalFundsFactCorrectionEndpoint(
			fundsIntakeDouble{correction: command}, &fundsRegistrarDouble{err: errors.New("事务壳寄了")})
		assertNoAnswerFormed(t, endpoint, "NO_ANSWER_FORMED")
	})
}

// TestOnlineExternalFundsFactRegistrationTakesTheSameSnapshotShapeAsTheCLI 是一条编译期断言：两个 Intake 契约要
// 交出的命令，正是 registrationjson 从登记输入译出的那一个——谁在端点侧另写一份译装、或让某一口收起了与 CLI
// -input 不同的形状，这里就编译不过（红线「译装只用 adapters/registrationjson 那一份」）。
func TestOnlineExternalFundsFactRegistrationTakesTheSameSnapshotShapeAsTheCLI(t *testing.T) {
	intake := settlementhttp.UnconfiguredIntake{}
	sameSnapshotShape(intake.IntakeExternalFundsFactRegistration, registrationjson.ExternalFundsFactFromJSON)
	sameSnapshotShape(intake.IntakeExternalFundsFactCorrectionRegistration, registrationjson.ExternalFundsFactCorrectionFromJSON)
}

// sameSnapshotShape 两个参数都不使用：它表达的是类型相等，不是一次调用。
func sameSnapshotShape[Command any](
	_ func(context.Context, *http.Request) (Command, error),
	_ func([]byte) (Command, error),
) {
}

// Covers: 判据 2——隔离读放行（ADR-0078）装不进两个命令口：IsolatedOperationsReadIntake 不满足任何一个登记
// 命令 Intake 接口，编译期排除保持（ADR-0085 决定二维持的原判）。
func TestIsolatedReadIntakeCannotServeExternalFundsFactRegistration(t *testing.T) {
	var intake any = settlementhttp.IsolatedOperationsReadIntake{}
	if _, ok := intake.(settlementhttp.ExternalFundsFactRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进资金事实采用口")
	}
	if _, ok := intake.(settlementhttp.ExternalFundsFactCorrectionRegistrationIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进资金事实更正口")
	}
}

func assertFundsAnswer(t *testing.T, endpoint http.Handler, spec fundsAnswerCase) {
	t.Helper()
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))
	if recorder.Code != spec.wantStatus {
		t.Fatalf("答 %d（%s）, want %d", recorder.Code, recorder.Body.String(), spec.wantStatus)
	}
	body := decodeRegistrationBody(t, recorder)
	if body["outcome"] != spec.wantAnswer {
		t.Fatalf("outcome = %v, want %s", body["outcome"], spec.wantAnswer)
	}
	assertKeys(t, body, "outcome")
}

func assertNoAnswerFormed(t *testing.T, endpoint http.Handler, wantCode string) {
	t.Helper()
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/probe", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("没形成答案却答 %d（%s）, want 500", recorder.Code, recorder.Body.String())
	}
	if code := problemCode(t, recorder); code != wantCode {
		t.Fatalf("错误码 = %q, want %s", code, wantCode)
	}
	assertNoOutcome(t, recorder)
}

func decodeRegistrationBody(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", recorder.Body.Bytes(), err)
	}
	return body
}

// assertKeys 钉响应形封闭（ADR-0022）：键集恰好是列出的那几个，多一个少一个都红。
func assertKeys(t *testing.T, body map[string]any, want ...string) {
	t.Helper()
	got := make([]string, 0, len(body))
	for key := range body {
		got = append(got, key)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("响应键集 = %v, want %v", got, want)
	}
}

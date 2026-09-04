package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证受控喂价口的路由与退出码翻译：两种运行种类各归各的用例；喂价的每一格答案译成 0/1/2/3/4
// 五个码，来源失败与治理答案分开；绑定文档只做形状翻译，声明齐不齐由领域构造门在入库前拒。事务
// 替身只透传闭包：事务纪律由适配器真库用例证，本工具的职责只是把整条链包进一个事务。

type passthroughTransactor struct{}

func (passthroughTransactor) WithinTransaction(ctx context.Context, fn bentoapp.TxFunc) error {
	return fn(ctx)
}

type feederDouble struct {
	result application.FeedReferenceSeriesResult
	err    error
	calls  int
	last   application.FeedReferenceSeriesCommand
}

func (double *feederDouble) Handle(_ context.Context, command application.FeedReferenceSeriesCommand) (application.FeedReferenceSeriesResult, error) {
	double.calls++
	double.last = command
	return double.result, double.err
}

type bindingRegistrarDouble struct {
	outcome application.RegisterSourceConnectorBindingOutcome
	err     error
	calls   int
	last    application.RegisterSourceConnectorBindingCommand
}

func (double *bindingRegistrarDouble) Handle(_ context.Context, command application.RegisterSourceConnectorBindingCommand) (application.RegisterSourceConnectorBindingOutcome, error) {
	double.calls++
	double.last = command
	return double.outcome, double.err
}

const bindingJSON = `{"tenant":"tenant-1","seriesId":"SYN-PRC-USD-CNY","bindingVersion":"b1","connectorKind":"FILE",
	"sourceIdentifier":"SYN-SOURCE/usd-cny-daily","sourceLocator":"rates/usd-cny/latest.json","seriesKind":"EXCHANGE_RATE",
	"quoteBasis":{"policyId":"SYN-PRC-FX-POLICY","policyVersion":"v1","digest":"sha256:syn-fx-policy"},
	"registrant":"SYN-PRC-SERIES-REGISTRAR","cadence":"DAILY","reviewExemption":"EXEMPT"}`

func feedInput(tenant, series string) executeInput {
	return executeInput{kind: kindFeed, tenant: tenant, seriesID: series}
}

func reference(t *testing.T) domain.VersionReference {
	t.Helper()
	built, err := domain.NewVersionReference(domain.ArtifactReferenceSeries, "SYN-PRC-USD-CNY", "2026-09-04", "sha256:syn")
	if err != nil {
		t.Fatalf("构造引用：%v", err)
	}
	return built
}

// TestExecuteTranslatesFeedAnswersIntoExitCodes 证喂价答案逐格译码：登了/重放 0；登了但免复核代写
// 的复核被拒 2；无绑定/连接器未装配/登记册要人裁 2；来源失败 4；命令不合法 1；未决 3。
func TestExecuteTranslatesFeedAnswersIntoExitCodes(t *testing.T) {
	registered := application.FeedReferenceSeriesResult{Outcome: application.FeedVersionRegistered, Reference: reference(t), Registration: application.ReferenceSeriesRecorded}
	reviewed := registered
	reviewed.ReviewRequested, reviewed.Review = true, application.SeriesReviewRecorded
	refused := registered
	refused.ReviewRequested, refused.Review = true, application.SeriesReviewNeedsAnotherReviewer

	cases := map[string]struct {
		result application.FeedReferenceSeriesResult
		code   int
		text   string
	}{
		"登了未声明免复核": {result: registered, code: exitRegistered, text: "REGISTERED SYN-PRC-USD-CNY@2026-09-04"},
		"登了且代写复核":  {result: reviewed, code: exitRegistered, text: "review=RECORDED"},
		"登了但四眼门拒":  {result: refused, code: exitGovernance, text: "review=NEEDS_ANOTHER_REVIEWER"},
		"幂等重放":     {result: application.FeedReferenceSeriesResult{Outcome: application.FeedVersionReplayed, Reference: reference(t)}, code: exitRegistered, text: "REPLAYED"},
		"无绑定":      {result: application.FeedReferenceSeriesResult{Outcome: application.FeedBindingUnknown}, code: exitGovernance, text: "BINDING_UNKNOWN"},
		"连接器未装配":   {result: application.FeedReferenceSeriesResult{Outcome: application.FeedConnectorUnavailable}, code: exitGovernance, text: "CONNECTOR_UNAVAILABLE"},
		"登记册要人裁":   {result: application.FeedReferenceSeriesResult{Outcome: application.FeedRegistrationRefused, Registration: application.ReferenceSeriesRegistrationConflict}, code: exitGovernance, text: "REGISTRATION_REFUSED"},
		"抓取失败":     {result: application.FeedReferenceSeriesResult{Outcome: application.FeedFetchFailed}, code: exitSourceFailure, text: "FETCH_FAILED"},
		"转录被拒":     {result: application.FeedReferenceSeriesResult{Outcome: application.FeedTranscriptionRefused}, code: exitSourceFailure, text: "TRANSCRIPTION_REFUSED"},
		"命令不合法":    {result: application.FeedReferenceSeriesResult{Outcome: application.FeedNotAccepted}, code: exitUsage, text: "NOT_ACCEPTED"},
		"代数外答案按未决": {result: application.FeedReferenceSeriesResult{Outcome: application.FeedReferenceSeriesOutcome(99)}, code: exitUndecided, text: "feed:"},
	}
	for label, test := range cases {
		feed := &feederDouble{result: test.result}
		message, code := execute(t.Context(), feedInput("tenant-1", "SYN-PRC-USD-CNY"), feed, &bindingRegistrarDouble{}, passthroughTransactor{})
		if code != test.code || !strings.Contains(message, test.text) {
			t.Fatalf("%s：message=%q code=%d，想要含 %q 且码 %d", label, message, code, test.text, test.code)
		}
		if feed.calls != 1 || feed.last.Tenant.String() != "tenant-1" || feed.last.SeriesID != "SYN-PRC-USD-CNY" {
			t.Fatalf("%s：命令翻译走样：%+v", label, feed.last)
		}
	}

	boom := errors.New("registers are down")
	message, code := execute(t.Context(), feedInput("tenant-1", "SYN-PRC-USD-CNY"), &feederDouble{err: boom, result: application.FeedReferenceSeriesResult{Outcome: application.FeedUndecided}}, &bindingRegistrarDouble{}, passthroughTransactor{})
	if code != exitUndecided || !strings.Contains(message, "registers are down") {
		t.Fatalf("未决：message=%q code=%d", message, code)
	}

	untouched := &feederDouble{result: registered}
	if _, code := execute(t.Context(), feedInput("", "SYN-PRC-USD-CNY"), untouched, &bindingRegistrarDouble{}, passthroughTransactor{}); code != exitUsage || untouched.calls != 0 {
		t.Fatalf("缺租户 code=%d calls=%d", code, untouched.calls)
	}
	if _, code := execute(t.Context(), feedInput("tenant-1", ""), untouched, &bindingRegistrarDouble{}, passthroughTransactor{}); code != exitUsage || untouched.calls != 0 {
		t.Fatalf("缺序列 code=%d calls=%d", code, untouched.calls)
	}
	if _, code := execute(t.Context(), executeInput{kind: "moon-phase"}, untouched, &bindingRegistrarDouble{}, passthroughTransactor{}); code != exitUsage {
		t.Fatalf("未知种类 code=%d", code)
	}
}

// TestExecuteRoutesBindingDocumentsToTheRegistrar 证绑定文档译成领域绑定（口径三件、节律、免复核原样
// 带到），三格答案译码；缺免复核声明、裸汇率、口径缺 digest、坏 JSON 都在入库前拒成 1 且不到达用例。
func TestExecuteRoutesBindingDocumentsToTheRegistrar(t *testing.T) {
	registrar := &bindingRegistrarDouble{outcome: application.SourceConnectorBindingRecorded}
	message, code := execute(t.Context(), executeInput{kind: kindBinding, raw: []byte(bindingJSON)}, &feederDouble{}, registrar, passthroughTransactor{})
	if code != exitRegistered || !strings.Contains(message, "RECORDED") || registrar.calls != 1 {
		t.Fatalf("message=%q code=%d calls=%d", message, code, registrar.calls)
	}
	binding := registrar.last.Binding
	basis, declared := binding.QuoteBasis()
	cadence, scheduled := binding.Cadence()
	if binding.Tenant().String() != "tenant-1" || binding.SeriesID() != "SYN-PRC-USD-CNY" || binding.Version() != "b1" ||
		binding.ConnectorKind() != "FILE" || binding.SourceLocator() != "rates/usd-cny/latest.json" ||
		binding.SeriesKind() != domain.ReferenceSeriesExchangeRate || !declared || basis.Digest() != "sha256:syn-fx-policy" ||
		binding.Registrant() != "SYN-PRC-SERIES-REGISTRAR" || !scheduled || cadence != "DAILY" ||
		binding.ReviewExemption() != domain.ReviewExemptionGranted {
		t.Fatalf("绑定翻译走样：%+v", binding.Spec())
	}

	for label, outcome := range map[string]struct {
		outcome application.RegisterSourceConnectorBindingOutcome
		code    int
	}{
		"幂等重放":   {application.SourceConnectorBindingAlreadyOnRegister, exitRegistered},
		"同版本异声明": {application.SourceConnectorBindingRegistrationConflict, exitGovernance},
		"不受理":    {application.SourceConnectorBindingNotAccepted, exitUsage},
		"未决":     {application.SourceConnectorBindingUndecided, exitUndecided},
	} {
		_, code := execute(t.Context(), executeInput{kind: kindBinding, raw: []byte(bindingJSON)}, &feederDouble{}, &bindingRegistrarDouble{outcome: outcome.outcome}, passthroughTransactor{})
		if code != outcome.code {
			t.Fatalf("%s：code=%d，想要 %d", label, code, outcome.code)
		}
	}

	untouched := &bindingRegistrarDouble{outcome: application.SourceConnectorBindingRecorded}
	for label, raw := range map[string]string{
		"坏 JSON":     `not json`,
		"缺免复核声明":     strings.Replace(bindingJSON, `,"reviewExemption":"EXEMPT"`, ``, 1),
		"免复核不在封闭集":   strings.Replace(bindingJSON, `"reviewExemption":"EXEMPT"`, `"reviewExemption":"YES"`, 1),
		"裸汇率":        strings.Replace(bindingJSON, `"quoteBasis":{"policyId":"SYN-PRC-FX-POLICY","policyVersion":"v1","digest":"sha256:syn-fx-policy"},`, ``, 1),
		"口径缺 digest": strings.Replace(bindingJSON, `,"digest":"sha256:syn-fx-policy"`, ``, 1),
		"缺租户":        strings.Replace(bindingJSON, `"tenant":"tenant-1"`, `"tenant":""`, 1),
	} {
		message, code := execute(t.Context(), executeInput{kind: kindBinding, raw: []byte(raw)}, &feederDouble{}, untouched, passthroughTransactor{})
		if code != exitUsage || untouched.calls != 0 {
			t.Fatalf("%s：message=%q code=%d calls=%d，想要 1 且不到达用例", label, message, code, untouched.calls)
		}
	}
}

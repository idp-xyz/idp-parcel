package pricinghttp_test

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证登记前预览端点（票 pricing-reference-series-operations/08 件②）：只收 POST、未配置
// 403 且不构造命令、畸形 400、预览答复逐字段透出（等级、规范化版本、摘要、对照结果与逐期
// 差异）、未决 500。预览是命令形状的读——要操作者信封才知道租户与登记责任方——所以它的
// Intake 与登记口同族，隔离读放行装不进来（编译期）。

type previewIntakeDouble struct {
	command application.PreviewReferenceSeriesCommand
	err     error
}

func (double previewIntakeDouble) IntakeReferenceSeriesPreview(
	context.Context,
	*http.Request,
) (application.PreviewReferenceSeriesCommand, error) {
	if double.err != nil {
		return application.PreviewReferenceSeriesCommand{}, double.err
	}
	return double.command, nil
}

type previewerDouble struct {
	preview application.ReferenceSeriesPreview
	err     error
	called  bool
}

func (double *previewerDouble) Handle(
	context.Context,
	application.PreviewReferenceSeriesCommand,
) (application.ReferenceSeriesPreview, error) {
	double.called = true
	return double.preview, double.err
}

func previewPeriod(t *testing.T, from, to time.Time, value, evidence string) domain.SeriesPeriodValue {
	t.Helper()
	period, err := domain.NewSeriesPeriodValue(from, to, mustDecimal(t, value), evidence)
	if err != nil {
		t.Fatalf("构造期次：%v", err)
	}
	return period
}

func mustDecimal(t *testing.T, value string) domain.Decimal {
	t.Helper()
	decimal, err := domain.ParseDecimal(value)
	if err != nil {
		t.Fatalf("解析十进制 %s：%v", value, err)
	}
	return decimal
}

func previewRegistration(t *testing.T, version string, periods ...domain.SeriesPeriodValue) domain.ReferenceSeriesRegistration {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("租户：%v", err)
	}
	reference, err := domain.NewVersionReference(domain.ArtifactReferenceSeries, "SYN-FUEL", version, "sha256:syn-fuel-"+version)
	if err != nil {
		t.Fatalf("引用：%v", err)
	}
	registration, err := domain.NewReferenceSeriesRegistration(domain.ReferenceSeriesRegistrationSpec{
		Tenant:           tenant,
		Kind:             domain.ReferenceSeriesFuelRate,
		Reference:        reference,
		SourceIdentifier: "SYN-CARRIER/fuel",
		Registrant:       "SYN-REGISTRAR",
		Periods:          periods,
	})
	if err != nil {
		t.Fatalf("构造登记：%v", err)
	}
	return registration
}

func TestPreviewReferenceSeriesEndpointOnlyAcceptsPost(t *testing.T) {
	endpoint := pricinghttp.NewPreviewReferenceSeriesEndpoint(previewIntakeDouble{}, &previewerDouble{})
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/pricing-reference-series-previews", nil))

	if recorder.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET 答 %d, want 405", recorder.Code)
	}
	if allow := recorder.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("Allow = %q, want POST", allow)
	}
}

func TestPreviewReferenceSeriesEndpointAnswersForbiddenWhenIntakeUnconfigured(t *testing.T) {
	previewer := &previewerDouble{}
	endpoint := pricinghttp.NewPreviewReferenceSeriesEndpoint(pricinghttp.UnconfiguredIntake{}, previewer)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-reference-series-previews", nil))

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("未配置 Intake 答 %d, want 403", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("错误码 = %q", code)
	}
	if previewer.called {
		t.Fatal("未配置即拒不构造命令，预览用例不该被调到")
	}
}

func TestPreviewReferenceSeriesEndpointMapsMalformedIntakeToBadRequest(t *testing.T) {
	endpoint := pricinghttp.NewPreviewReferenceSeriesEndpoint(
		previewIntakeDouble{err: fmt.Errorf("载荷缺格: %w", pricinghttp.ErrMalformedRequest)},
		&previewerDouble{},
	)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-reference-series-previews", nil))

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("畸形请求答 %d, want 400", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "MALFORMED_REQUEST" {
		t.Fatalf("错误码 = %q", code)
	}
}

// TestPreviewReferenceSeriesEndpointTranscribesThePreview 证答复逐字段透出：等级与摘要原样、
// 对照结果原名、逐期差异每条带种类、三个细项与两侧期次（缺席侧不给键）。
func TestPreviewReferenceSeriesEndpointTranscribesThePreview(t *testing.T) {
	weekOne := time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC)
	weekTwo := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	weekThree := time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)
	base := previewRegistration(t, "v1",
		previewPeriod(t, weekOne, weekTwo, "0.22", "SYN-EVIDENCE/fuel-W32"),
		previewPeriod(t, weekTwo, time.Time{}, "0.24", "SYN-EVIDENCE/fuel-W33"))
	proposed := previewRegistration(t, "v2",
		previewPeriod(t, weekOne, weekTwo, "0.23", "SYN-EVIDENCE/fuel-W32"),
		previewPeriod(t, weekThree, time.Time{}, "0.26", ""))
	changes, err := domain.DiffSeriesPeriods(base, proposed)
	if err != nil {
		t.Fatalf("比对：%v", err)
	}
	previewer := &previewerDouble{preview: application.ReferenceSeriesPreview{
		Outcome:          application.ReferenceSeriesPreviewed,
		EvidenceGrade:    domain.SeriesEvidenceAsserted,
		Canonicalization: proposed.Canonicalization(),
		ContentDigest:    proposed.ContentDigest(),
		Comparison:       application.SeriesComparisonCompared,
		BaseVersion:      "v1",
		Changes:          changes,
	}}
	endpoint := pricinghttp.NewPreviewReferenceSeriesEndpoint(previewIntakeDouble{}, previewer)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-reference-series-previews", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	body := decodeBody(t, recorder)
	if body["outcome"] != "PREVIEWED" || body["evidenceGrade"] != "ASSERTED" ||
		body["canonicalization"] != proposed.Canonicalization() || body["contentDigest"] != proposed.ContentDigest() {
		t.Fatalf("预览头部变形：%v", body)
	}
	comparison, ok := body["comparison"].(map[string]any)
	if !ok || comparison["outcome"] != "COMPARED" || comparison["baseVersion"] != "v1" {
		t.Fatalf("对照结果变形：%v", body["comparison"])
	}
	listed, ok := comparison["changes"].([]any)
	if !ok || len(listed) != 3 {
		t.Fatalf("changes = %v", comparison["changes"])
	}

	changed := listed[0].(map[string]any)
	if changed["kind"] != "CHANGED" || changed["startsAt"] != weekOne.Format(time.RFC3339Nano) ||
		changed["valueChanged"] != true || changed["endChanged"] != false || changed["evidenceChanged"] != false {
		t.Fatalf("首期差异变形：%v", changed)
	}
	if changed["base"].(map[string]any)["value"] != "0.22" || changed["proposed"].(map[string]any)["value"] != "0.23" {
		t.Fatalf("首期两侧取值变形：%v", changed)
	}

	removed := listed[1].(map[string]any)
	if removed["kind"] != "REMOVED" {
		t.Fatalf("第二期应报 REMOVED：%v", removed)
	}
	if _, has := removed["proposed"]; has {
		t.Fatal("被移除的期次长出了 proposed 键")
	}
	if _, has := removed["base"].(map[string]any)["endsAt"]; has {
		t.Fatal("无上界的基准期次长出了 endsAt 键")
	}

	added := listed[2].(map[string]any)
	if added["kind"] != "ADDED" {
		t.Fatalf("第三期应报 ADDED：%v", added)
	}
	if _, has := added["base"]; has {
		t.Fatal("新增的期次长出了 base 键")
	}
	if _, has := added["proposed"].(map[string]any)["evidenceRef"]; has {
		t.Fatal("缺凭证的拟登期次长出了 evidenceRef 键")
	}
}

// TestPreviewReferenceSeriesEndpointAnswersWithoutComparisonWhenNoneRequested 证没要求对照时
// comparison 只有原名 NOT_REQUESTED，不带 baseVersion，changes 是空数组不是 null。
func TestPreviewReferenceSeriesEndpointAnswersWithoutComparisonWhenNoneRequested(t *testing.T) {
	previewer := &previewerDouble{preview: application.ReferenceSeriesPreview{
		Outcome:          application.ReferenceSeriesPreviewed,
		EvidenceGrade:    domain.SeriesEvidenceVerifiable,
		Canonicalization: "PRS-1",
		ContentDigest:    "digest-1",
		Comparison:       application.SeriesComparisonNotRequested,
	}}
	endpoint := pricinghttp.NewPreviewReferenceSeriesEndpoint(previewIntakeDouble{}, previewer)
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-reference-series-previews", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	comparison := decodeBody(t, recorder)["comparison"].(map[string]any)
	if comparison["outcome"] != "NOT_REQUESTED" {
		t.Fatalf("对照结果 = %v", comparison["outcome"])
	}
	if _, has := comparison["baseVersion"]; has {
		t.Fatal("没要求对照却带了 baseVersion")
	}
	if changes, ok := comparison["changes"].([]any); !ok || len(changes) != 0 {
		t.Fatalf("changes 应为空数组：%v", comparison["changes"])
	}
}

// TestPreviewReferenceSeriesEndpointAnswersNotAcceptedAndUndecided 证不受理只带 outcome（没有
// 摘要可透），未决答 500 NO_ANSWER_FORMED 不带 outcome。
func TestPreviewReferenceSeriesEndpointAnswersNotAcceptedAndUndecided(t *testing.T) {
	rejected := pricinghttp.NewPreviewReferenceSeriesEndpoint(previewIntakeDouble{}, &previewerDouble{
		preview: application.ReferenceSeriesPreview{Outcome: application.ReferenceSeriesPreviewNotAccepted},
	})
	recorder := httptest.NewRecorder()
	rejected.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-reference-series-previews", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("不受理答 %d, want 200", recorder.Code)
	}
	body := decodeBody(t, recorder)
	if body["outcome"] != "NOT_ACCEPTED" {
		t.Fatalf("outcome = %v", body["outcome"])
	}
	for _, forbidden := range []string{"contentDigest", "evidenceGrade", "comparison"} {
		if _, has := body[forbidden]; has {
			t.Fatalf("不受理却带了 %s", forbidden)
		}
	}

	undecided := pricinghttp.NewPreviewReferenceSeriesEndpoint(previewIntakeDouble{}, &previewerDouble{
		preview: application.ReferenceSeriesPreview{Outcome: application.ReferenceSeriesPreviewUndecided},
		err:     errors.New("库连不上"),
	})
	recorder = httptest.NewRecorder()
	undecided.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-reference-series-previews", nil))
	if recorder.Code != http.StatusInternalServerError {
		t.Fatalf("未决答 %d, want 500", recorder.Code)
	}
	if code := errorCode(t, recorder); code != "NO_ANSWER_FORMED" {
		t.Fatalf("错误码 = %q", code)
	}
}

// TestIsolatedReadIntakeCannotServePreview 钉住隔离读放行装不进预览口：预览要的是操作者信封
// （租户 + 登记责任方），注入的合成作用域没有登记责任方可给，装进来就是替人铸身份。
func TestIsolatedReadIntakeCannotServePreview(t *testing.T) {
	var intake any = pricinghttp.IsolatedOperationsReadIntake{}
	if _, ok := intake.(pricinghttp.ReferenceSeriesPreviewIntake); ok {
		t.Fatal("隔离读 Intake 不该装得进序列预览口")
	}
}

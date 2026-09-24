package pricinghttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

const priceCardPreviewTarget = "/pricing-price-card-previews"

type priceCardPreviewIntakeDouble struct{ err error }

func (double priceCardPreviewIntakeDouble) IntakePriceCardPreview(context.Context, *http.Request) (application.PreviewPriceCardImportCommand, error) {
	return application.PreviewPriceCardImportCommand{}, double.err
}

type priceCardPreviewerDouble struct {
	preview application.PriceCardImportPreview
	err     error
}

func (double priceCardPreviewerDouble) Handle(context.Context, application.PreviewPriceCardImportCommand) (application.PriceCardImportPreview, error) {
	return double.preview, double.err
}

func postPriceCardPreview(intake pricinghttp.PriceCardPreviewIntake, previewer pricinghttp.PriceCardImportPreviewer) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	pricinghttp.NewPreviewPriceCardImportEndpoint(intake, previewer).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, priceCardPreviewTarget, strings.NewReader("")))
	return recorder
}

// 已校验：带源文件身份、方案引用与方案概要（摘要、价表、计重、方向授权）；问题清单是空数组不是缺席。
func TestPriceCardPreviewAnswersValidatedWithTheContentSummary(t *testing.T) {
	plan := estimatePlan(t)
	source, err := domain.NewSourceFileIdentity("card.xlsx", strings.Repeat("a", 64))
	if err != nil {
		t.Fatal(err)
	}
	authorization := pptest.IdentityReference(t, domain.ArtifactCommercialAuthorization, "SYN-AUTH", "v1")
	recorder := postPriceCardPreview(priceCardPreviewIntakeDouble{}, priceCardPreviewerDouble{preview: application.PriceCardImportPreview{
		Outcome: application.PriceCardImportValidated, TemplateVersion: "PPT-1", SourceFile: source, Plan: plan.Reference(),
		Content: &ports.PriceCardTemplateContent{Plan: plan, DirectionAuthorization: authorization},
	}})
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body)
	}
	var body struct {
		Outcome         string            `json:"outcome"`
		TemplateVersion string            `json:"templateVersion"`
		SourceFile      map[string]string `json:"sourceFile"`
		Plan            map[string]string `json:"plan"`
		Content         struct {
			ContentDigest          string              `json:"contentDigest"`
			Canonicalization       string              `json:"canonicalization"`
			Direction              string              `json:"direction"`
			RateTable              map[string]any      `json:"rateTable"`
			WeightPolicy           map[string]string   `json:"weightPolicy"`
			Manifest               []map[string]string `json:"manifest"`
			DirectionAuthorization map[string]string   `json:"directionAuthorization"`
		} `json:"content"`
		Problems []any `json:"problems"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.Outcome != "VALIDATED" || body.TemplateVersion != "PPT-1" || body.SourceFile["name"] != "card.xlsx" || body.Plan["id"] != "SYN-PLAN-EST" {
		t.Fatalf("body = %s", recorder.Body)
	}
	if body.Content.ContentDigest != plan.ContentDigest() || body.Content.Canonicalization != plan.CanonicalizationVersion() ||
		body.Content.Direction != "SELL" || body.Content.RateTable["id"] != "SYN-TABLE-EST" || body.Content.RateTable["rows"] != float64(1) ||
		body.Content.WeightPolicy["id"] != "SYN-WEIGHT-EST" || body.Content.DirectionAuthorization["id"] != "SYN-AUTH" || len(body.Content.Manifest) == 0 {
		t.Fatalf("content = %+v", body.Content)
	}
	if body.Problems == nil || len(body.Problems) != 0 {
		t.Fatalf("problems = %v, want an empty array", body.Problems)
	}
}

// 带问题：逐格坐标原样交回，没有 content；未受理且命令没立住时连源文件身份也没有。
func TestPriceCardPreviewAnswersProblemsWithCoordinates(t *testing.T) {
	plan := estimatePlan(t)
	recorder := postPriceCardPreview(priceCardPreviewIntakeDouble{}, priceCardPreviewerDouble{preview: application.PriceCardImportPreview{
		Outcome: application.PriceCardImportHasProblems, TemplateVersion: "PPT-1", Plan: plan.Reference(),
		Problems: []ports.PriceCardTemplateProblem{{Sheet: "tables", Row: 2, Column: "currency", Code: ports.TemplateCellInvalid, Message: "币种立不住"}},
	}})
	var body struct {
		Outcome  string           `json:"outcome"`
		Content  *json.RawMessage `json:"content"`
		Problems []struct {
			Sheet, Column, Code, Message string
			Row                          int
		} `json:"problems"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if recorder.Code != http.StatusOK || body.Outcome != "HAS_PROBLEMS" || body.Content != nil || len(body.Problems) != 1 ||
		body.Problems[0].Sheet != "tables" || body.Problems[0].Row != 2 || body.Problems[0].Column != "currency" || body.Problems[0].Code != "CELL_INVALID" {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body)
	}

	recorder = postPriceCardPreview(priceCardPreviewIntakeDouble{}, priceCardPreviewerDouble{preview: application.PriceCardImportPreview{
		Outcome: application.PriceCardImportNotAccepted,
	}})
	if recorder.Code != http.StatusOK || strings.Contains(recorder.Body.String(), "sourceFile") || !strings.Contains(recorder.Body.String(), `"NOT_ACCEPTED"`) {
		t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body)
	}
}

func TestPriceCardPreviewTransportAnswers(t *testing.T) {
	recorder := postPriceCardPreview(pricinghttp.UnconfiguredIntake{}, priceCardPreviewerDouble{})
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "ACCESS_CHANNEL_NOT_CONFIGURED") {
		t.Fatalf("unconfigured: status = %d body = %s", recorder.Code, recorder.Body)
	}
	recorder = postPriceCardPreview(priceCardPreviewIntakeDouble{}, priceCardPreviewerDouble{err: errors.New("boom")})
	if recorder.Code != http.StatusInternalServerError || !strings.Contains(recorder.Body.String(), "NO_ANSWER_FORMED") {
		t.Fatalf("handler error: status = %d body = %s", recorder.Code, recorder.Body)
	}
	recorder = httptest.NewRecorder()
	pricinghttp.NewPreviewPriceCardImportEndpoint(priceCardPreviewIntakeDouble{}, priceCardPreviewerDouble{}).
		ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, priceCardPreviewTarget, nil))
	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodPost {
		t.Fatalf("GET: status = %d allow = %q", recorder.Code, recorder.Header().Get("Allow"))
	}
}

func uploadRequest(t *testing.T, build func(*multipart.Writer)) *http.Request {
	t.Helper()
	var buffer bytes.Buffer
	writer := multipart.NewWriter(&buffer)
	build(writer)
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, priceCardPreviewTarget, &buffer)
	request.Header.Set("Content-Type", writer.FormDataContentType())
	return request
}

// 上传解码：恰一个文件格照收；缺文件、多出别的格、不是表单都按请求形状不对拒。
func TestDecodePriceCardUploadIsStrict(t *testing.T) {
	file := func(writer *multipart.Writer, field, name, content string) {
		part, err := writer.CreateFormFile(field, name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write([]byte(content))
	}
	upload, err := pricinghttp.DecodePriceCardUpload(uploadRequest(t, func(writer *multipart.Writer) { file(writer, "file", "card.xlsx", "bytes") }))
	if err != nil || upload.FileName != "card.xlsx" || string(upload.Raw) != "bytes" {
		t.Fatalf("upload = %+v err = %v", upload, err)
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	if command := upload.PreviewCommand(tenant); command.Tenant != tenant || command.FileName != "card.xlsx" || string(command.Raw) != "bytes" {
		t.Fatalf("command = %+v", command)
	}

	refused := map[string]*http.Request{
		"缺文件格": uploadRequest(t, func(writer *multipart.Writer) { _ = writer.WriteField("tenant", "SYN-TENANT-01") }),
		"多出别的格": uploadRequest(t, func(writer *multipart.Writer) {
			file(writer, "file", "card.xlsx", "bytes")
			_ = writer.WriteField("tenant", "SYN-TENANT-02")
		}),
		"文件格名不对": uploadRequest(t, func(writer *multipart.Writer) { file(writer, "upload", "card.xlsx", "bytes") }),
		"两个文件": uploadRequest(t, func(writer *multipart.Writer) {
			file(writer, "file", "a.xlsx", "a")
			file(writer, "file", "b.xlsx", "b")
		}),
		"不是表单": httptest.NewRequest(http.MethodPost, priceCardPreviewTarget, strings.NewReader(`{"file":"x"}`)),
	}
	for name, request := range refused {
		if _, err := pricinghttp.DecodePriceCardUpload(request); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
			t.Errorf("%s: err = %v, want ErrMalformedRequest", name, err)
		}
	}
}

// 操作者渠道：租户取认证出的身份，上传照严格解码；没令牌答凭据被拒，令牌有效而上传不合格答请求形状不对。
func TestOperatorIntakeTakesTheTenantFromTheAuthenticatedOperator(t *testing.T) {
	intake, err := pricinghttp.NewOperatorRegistryIntake(registryAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	upload := func(token string, build func(*multipart.Writer)) *http.Request {
		request := uploadRequest(t, build)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		return request
	}
	card := func(writer *multipart.Writer) {
		part, err := writer.CreateFormFile("file", "card.xlsx")
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write([]byte("bytes"))
	}
	command, err := intake.IntakePriceCardPreview(context.Background(), upload("token", card))
	if err != nil || command.Tenant.String() != "SYN-TENANT-01" || command.FileName != "card.xlsx" || string(command.Raw) != "bytes" {
		t.Fatalf("command = %+v err = %v", command, err)
	}
	if _, err := intake.IntakePriceCardPreview(context.Background(), upload("", card)); !errors.Is(err, pricinghttp.ErrOperatorCredentialRejected) {
		t.Fatalf("no token: err = %v", err)
	}
	extra := func(writer *multipart.Writer) {
		card(writer)
		_ = writer.WriteField("tenant", "SYN-TENANT-02")
	}
	if _, err := intake.IntakePriceCardPreview(context.Background(), upload("token", extra)); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
		t.Fatalf("self-reported tenant field: err = %v", err)
	}
	recorder := httptest.NewRecorder()
	pricinghttp.NewPreviewPriceCardImportEndpoint(intake, priceCardPreviewerDouble{}).ServeHTTP(recorder, upload("", card))
	if recorder.Code != http.StatusUnauthorized || !strings.Contains(recorder.Body.String(), "OPERATOR_CREDENTIAL_REJECTED") {
		t.Fatalf("endpoint without token: status = %d body = %s", recorder.Code, recorder.Body)
	}
}

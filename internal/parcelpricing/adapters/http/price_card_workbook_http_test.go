package pricinghttp_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件证价卡导入的第二种载体与模板规格口的传输面（ADR-0349）：Content-Type 为 application/json 时收页面表单提交的结构化工作簿，
// 其余类型照旧按上传解、行为不变；草稿读口对来自页面表单的草稿多标一格来源；规格口先认证、查询串封闭、答的就是读口交来的规格。

type specSourceDouble struct {
	spec  ports.PriceCardTemplateSpec
	err   error
	calls int
}

func (double *specSourceDouble) ReadPriceCardTemplateSpec(context.Context) (ports.PriceCardTemplateSpec, error) {
	double.calls++
	return double.spec, double.err
}

func sampleSpec() ports.PriceCardTemplateSpec {
	return ports.PriceCardTemplateSpec{
		TemplateVersion:          "PPT-2",
		AcceptedTemplateVersions: []string{"PPT-1", "PPT-2"},
		Sheets: []ports.PriceCardTemplateSheetSpec{
			{Name: "card", Title: "方案头与登记件", Purpose: "每个字段一行", Layout: "FIELDS", Repeatable: false, Columns: []ports.PriceCardTemplateColumnSpec{
				{Key: "direction", Title: "价格方向", Requirement: "always", Description: "价格方向", AllowedValues: []string{"BUY", "SELL"}},
				{Key: "amountRoundingPoints", Title: "金额取整应用点", Requirement: "conditional", Description: "分号分隔", AllowedValues: []string{"TOTAL"}, MultiValued: true},
				{Key: "planId", Title: "方案标识", Requirement: "always", Description: "方案标识"},
			}},
			{Name: "total_bounds", Title: "最低 / 最高收费", Purpose: "对整票合计声明下限与上限", Layout: "ROWS", Repeatable: false, Since: "PPT-2", Columns: []ports.PriceCardTemplateColumnSpec{
				{Key: "minimum", Title: "最低收费", Requirement: "conditional", Description: "下限"},
			}},
			{Name: "conditions", Title: "条件树", Purpose: "条件树", Layout: "ROWS", Repeatable: true, Columns: []ports.PriceCardTemplateColumnSpec{
				{Key: "category", Title: "类别值", Requirement: "conditional", Description: "类别值", AllowedValuesWhen: []ports.PriceCardTemplateValuesWhen{
					{Column: "source", Equals: "ADDRESS_NATURE", Values: []string{"COMMERCIAL", "RESIDENTIAL"}},
				}},
			}},
		},
	}
}

func getSpec(endpoint http.Handler, target string) *httptest.ResponseRecorder {
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, target, nil))
	return recorder
}

// Covers: `AT-PP-047`，ADR-0349 决定五——规格口只收 GET；接入拒绝先于一切，规格源一次都不被调到；查询串封闭，带任何键都是坏请求。
func TestTemplateSpecEndpointGatesBeforeAnsweringAndTakesNoQuery(t *testing.T) {
	source := &specSourceDouble{spec: sampleSpec()}
	endpoint := pricinghttp.NewQueryPriceCardTemplateSpecEndpoint(intakeDouble{query: catalogueQuery(t)}, source)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-price-card-template/spec", nil))
	if recorder.Code != http.StatusMethodNotAllowed || recorder.Header().Get("Allow") != http.MethodGet {
		t.Fatalf("POST：%d allow=%q", recorder.Code, recorder.Header().Get("Allow"))
	}

	for _, refusal := range []struct {
		err    error
		status int
		code   string
	}{
		{pricinghttp.ErrAccessChannelNotConfigured, http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		{pricinghttp.ErrOperatorCredentialRejected, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		{pricinghttp.ErrOperatorNotGranted, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		{pricinghttp.ErrIdentityDependencyUnavailable, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
	} {
		denied := pricinghttp.NewQueryPriceCardTemplateSpecEndpoint(intakeDouble{err: refusal.err}, source)
		// 查询串里带键也先答接入拒绝，不泄露本口认哪些键。
		recorder := getSpec(denied, "/pricing-price-card-template/spec?tenant=SYN-TENANT-02")
		if recorder.Code != refusal.status || errorCode(t, recorder) != refusal.code {
			t.Errorf("%v：%d %s", refusal.err, recorder.Code, recorder.Body)
		}
	}
	if recorder := getSpec(pricinghttp.NewQueryPriceCardTemplateSpecEndpoint(pricinghttp.UnconfiguredIntake{}, source), "/pricing-price-card-template/spec"); recorder.Code != http.StatusForbidden {
		t.Errorf("未配置即拒：%d", recorder.Code)
	}
	if source.calls != 0 {
		t.Fatalf("被拒的请求调到了规格源 %d 次", source.calls)
	}

	for _, target := range []string{
		"/pricing-price-card-template/spec?tenant=SYN-TENANT-01",
		"/pricing-price-card-template/spec?version=PPT-2",
		"/pricing-price-card-template/spec?x",
	} {
		if recorder := getSpec(endpoint, target); recorder.Code != http.StatusBadRequest || errorCode(t, recorder) != "MALFORMED_REQUEST" {
			t.Errorf("%s：%d %s", target, recorder.Code, recorder.Body)
		}
	}
	if source.calls != 0 {
		t.Fatalf("坏请求调到了规格源 %d 次", source.calls)
	}
}

func TestTemplateSpecEndpointSaysSoWhenItCannotAnswer(t *testing.T) {
	source := &specSourceDouble{err: errors.New("boom")}
	recorder := getSpec(pricinghttp.NewQueryPriceCardTemplateSpecEndpoint(intakeDouble{query: catalogueQuery(t)}, source), "/pricing-price-card-template/spec")
	if recorder.Code != http.StatusInternalServerError || errorCode(t, recorder) != "NO_ANSWER_FORMED" {
		t.Fatalf("读口失败：%d %s", recorder.Code, recorder.Body)
	}
}

func TestTemplateSpecEndpointAnswersTheSourceSpecInItsDocumentedShape(t *testing.T) {
	source := &specSourceDouble{spec: sampleSpec()}
	recorder := getSpec(pricinghttp.NewQueryPriceCardTemplateSpecEndpoint(intakeDouble{query: catalogueQuery(t)}, source), "/pricing-price-card-template/spec")
	if recorder.Code != http.StatusOK || source.calls != 1 {
		t.Fatalf("status = %d calls = %d body = %s", recorder.Code, source.calls, recorder.Body)
	}
	var body struct {
		TemplateVersion          string   `json:"templateVersion"`
		AcceptedTemplateVersions []string `json:"acceptedTemplateVersions"`
		Sheets                   []struct {
			Name       string           `json:"name"`
			Title      string           `json:"title"`
			Purpose    string           `json:"purpose"`
			Layout     string           `json:"layout"`
			Repeatable bool             `json:"repeatable"`
			Since      string           `json:"since"`
			Columns    []map[string]any `json:"columns"`
		} `json:"sheets"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body.TemplateVersion != "PPT-2" || strings.Join(body.AcceptedTemplateVersions, ",") != "PPT-1,PPT-2" || len(body.Sheets) != 3 {
		t.Fatalf("body = %s", recorder.Body)
	}
	card, bounds := body.Sheets[0], body.Sheets[1]
	if card.Name != "card" || card.Layout != "FIELDS" || card.Repeatable || card.Since != "" || len(card.Columns) != 3 {
		t.Fatalf("card = %+v", card)
	}
	if bounds.Name != "total_bounds" || bounds.Since != "PPT-2" || bounds.Repeatable {
		t.Fatalf("total_bounds = %+v", bounds)
	}
	direction := card.Columns[0]
	if direction["key"] != "direction" || direction["title"] != "价格方向" || direction["requirement"] != "always" ||
		direction["description"] != "价格方向" || len(direction["allowedValues"].([]any)) != 2 {
		t.Fatalf("direction = %v", direction)
	}
	if card.Columns[1]["multiValued"] != true {
		t.Fatalf("amountRoundingPoints = %v", card.Columns[1])
	}
	// 没有封闭取值的列不带 allowedValues、multiValued 与 allowedValuesWhen：缺席即「开放词汇」。
	for _, key := range []string{"allowedValues", "multiValued", "allowedValuesWhen"} {
		if _, present := card.Columns[2][key]; present {
			t.Errorf("planId 不该带 %s：%v", key, card.Columns[2])
		}
	}
	// 条件封闭的列：不带 allowedValues，条件与取值在 allowedValuesWhen 里。
	category := body.Sheets[2].Columns[0]
	when, _ := category["allowedValuesWhen"].([]any)
	if _, closed := category["allowedValues"]; closed || len(when) != 1 {
		t.Fatalf("category = %v", category)
	}
	first, _ := when[0].(map[string]any)
	if first["column"] != "source" || first["equals"] != "ADDRESS_NATURE" || len(first["allowedValues"].([]any)) != 2 {
		t.Fatalf("allowedValuesWhen = %v", first)
	}
	if !strings.Contains(recorder.Body.String(), `"sheets":[`) || strings.Contains(recorder.Body.String(), `"since":""`) {
		t.Errorf("无 since 的表不该写出空 since：%s", recorder.Body)
	}
}

const workbookJSONBody = `{"templateVersion":"PPT-2","sheets":{"card":[{"field":"planId","value":"P"}]}}`

func workbookRequest(target, contentType, token, body string) *http.Request {
	request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	return request
}

// Covers: `AT-PP-046`，ADR-0349 决定四——Content-Type 为 application/json 时（带不带 charset 都算）预览与录入口把请求体原样当作结构化工作簿
// 交给应用层，租户与录入者仍只取认证出的身份；先认证、后读体。别的 Content-Type 与没有 Content-Type 照旧走上传解码，
// 答原有的坏请求，不因这一条多出任何新答案。
func TestOperatorIntakeTakesAJSONWorkbookAndLeavesOtherContentTypesToTheUploadPath(t *testing.T) {
	intake, err := pricinghttp.NewOperatorRegistryIntake(registryAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	for _, contentType := range []string{"application/json", "application/json; charset=utf-8", "Application/JSON"} {
		preview, err := intake.IntakePriceCardPreview(context.Background(), workbookRequest(priceCardPreviewTarget, contentType, "token", workbookJSONBody))
		if err != nil || !preview.FromForm || preview.Tenant.String() != "SYN-TENANT-01" || preview.FileName != "" || string(preview.Raw) != workbookJSONBody {
			t.Fatalf("%s 预览：%+v err=%v", contentType, preview, err)
		}
		draft, err := intake.IntakePriceCardDraftSubmission(context.Background(), workbookRequest(priceCardDraftTarget, contentType, "token", workbookJSONBody))
		if err != nil || !draft.FromForm || draft.Tenant.String() != "SYN-TENANT-01" || draft.Submitter != "https://id.syn.example/dex#SYN-OPERATOR-02" ||
			draft.FileName != "" || string(draft.Raw) != workbookJSONBody {
			t.Fatalf("%s 录入：%+v err=%v", contentType, draft, err)
		}
	}

	// 先认证：没令牌答凭据被拒，请求体一个字也不读（体里是什么都无所谓）。
	if _, err := intake.IntakePriceCardPreview(context.Background(), workbookRequest(priceCardPreviewTarget, "application/json", "", workbookJSONBody)); !errors.Is(err, pricinghttp.ErrOperatorCredentialRejected) {
		t.Fatalf("无令牌：err = %v", err)
	}
	if _, err := intake.IntakePriceCardDraftSubmission(context.Background(), workbookRequest(priceCardDraftTarget, "application/json", "", workbookJSONBody)); !errors.Is(err, pricinghttp.ErrOperatorCredentialRejected) {
		t.Fatalf("无令牌：err = %v", err)
	}

	for _, contentType := range []string{"", "text/plain", "text/json", "application/jsonx", "application/xml", "multipart/form-data"} {
		if _, err := intake.IntakePriceCardPreview(context.Background(), workbookRequest(priceCardPreviewTarget, contentType, "token", workbookJSONBody)); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
			t.Errorf("预览 Content-Type %q：err = %v，应照旧答坏请求", contentType, err)
		}
		if _, err := intake.IntakePriceCardDraftSubmission(context.Background(), workbookRequest(priceCardDraftTarget, contentType, "token", workbookJSONBody)); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
			t.Errorf("录入 Content-Type %q：err = %v，应照旧答坏请求", contentType, err)
		}
	}

	// 请求体超过上传的同一个上限：坏请求，不无界读入。
	oversize := strings.Repeat("x", 12<<20)
	if _, err := intake.IntakePriceCardPreview(context.Background(), workbookRequest(priceCardPreviewTarget, "application/json", "token", oversize)); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
		t.Fatalf("超限：err = %v", err)
	}

	// 端点端到端：令牌有效、Content-Type 正确，租户从信封来，应用层收到的是原样的工作簿。
	received := application.PreviewPriceCardImportCommand{}
	endpoint := pricinghttp.NewPreviewPriceCardImportEndpoint(intake, previewerRecording{record: &received})
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, workbookRequest(priceCardPreviewTarget, "application/json", "token", workbookJSONBody))
	if recorder.Code != http.StatusOK || !received.FromForm || string(received.Raw) != workbookJSONBody {
		t.Fatalf("status = %d received = %+v", recorder.Code, received)
	}
}

type previewerRecording struct {
	record *application.PreviewPriceCardImportCommand
}

func (double previewerRecording) Handle(_ context.Context, command application.PreviewPriceCardImportCommand) (application.PriceCardImportPreview, error) {
	*double.record = command
	return application.PriceCardImportPreview{Outcome: application.PriceCardImportNotAccepted}, nil
}

// Covers: `AT-PP-045`，ADR-0349 决定四——来自页面表单的草稿，答复里多一格 sourceOrigin: FORM（录入口的 draft 与草稿查阅读口同形）；
// 上传文件的草稿没有这一格，答复与此前一字不差；一个碰巧叫 form:xxxx 的上传文件名对不上自己字节的散列，不会被当成表单。
func TestADraftFromAFormSaysSoAndAFileDraftSaysNothingNew(t *testing.T) {
	digest := strings.Repeat("c", 64)
	formSource, err := domain.NewFormSourceFileIdentity(digest)
	if err != nil {
		t.Fatal(err)
	}
	if formSource.Name() != "form:cccccccccccc" || formSource.SHA256() != digest || !formSource.FromForm() {
		t.Fatalf("form identity = %q / %s / %v", formSource.Name(), formSource.SHA256(), formSource.FromForm())
	}
	spoof, err := domain.NewSourceFileIdentity("form:cccccccccccc", strings.Repeat("d", 64))
	if err != nil || spoof.FromForm() {
		t.Fatalf("名叫 form: 的上传文件不是表单：%v %v", spoof.FromForm(), err)
	}

	answer := func(source domain.SourceFileIdentity) string {
		preview, draft := validatedDraft(t)
		draft = redraftFrom(t, draft, source)
		preview.SourceFile = source
		recorder := postPriceCardDraft(priceCardDraftSubmissionIntakeDouble{}, priceCardDraftSubmitterDouble{result: application.SubmitPriceCardDraftResult{
			Outcome: application.PriceCardDraftSubmitted, Reading: preview, Draft: draft, HasDraft: true,
		}})
		if recorder.Code != http.StatusCreated {
			t.Fatalf("status = %d body = %s", recorder.Code, recorder.Body)
		}
		return recorder.Body.String()
	}

	fromForm := answer(formSource)
	var body struct {
		Draft map[string]any `json:"draft"`
	}
	if err := json.Unmarshal([]byte(fromForm), &body); err != nil {
		t.Fatal(err)
	}
	if body.Draft["sourceOrigin"] != "FORM" || body.Draft["sourceFile"].(map[string]any)["name"] != "form:cccccccccccc" {
		t.Fatalf("表单草稿：%v", body.Draft)
	}

	if fromFile := answer(draftSourceFile(t)); strings.Contains(fromFile, "sourceOrigin") {
		t.Fatalf("上传文件的草稿不该多出来源格：%s", fromFile)
	}
	if spoofed := answer(spoof); strings.Contains(spoofed, "sourceOrigin") {
		t.Fatalf("冒名的上传文件不该被标成表单：%s", spoofed)
	}
	if !bytes.Contains([]byte(fromForm), []byte(`"sourceFile":{"name":"form:cccccccccccc"`)) {
		t.Errorf("sourceFile 仍是名称加散列：%s", fromForm)
	}
}

// redraftFrom 用同一份内容、换一个源文件身份重立草稿。
func redraftFrom(t *testing.T, draft domain.PriceCardDraft, source domain.SourceFileIdentity) domain.PriceCardDraft {
	t.Helper()
	content, ok := draft.Content()
	if !ok {
		t.Fatal("草稿没有内容")
	}
	rebuilt, err := domain.SubmitPriceCardDraft(domain.PriceCardDraftSubmission{
		Tenant: draft.Tenant(), Plan: draft.Plan(), Source: source,
		Content:   &domain.PriceCardDraftContent{Plan: content.Plan, DirectionAuthorization: content.DirectionAuthorization},
		Submitter: draft.Submitter(), SubmittedAt: draft.SubmittedAt(),
	})
	if err != nil {
		t.Fatal(err)
	}
	return rebuilt
}

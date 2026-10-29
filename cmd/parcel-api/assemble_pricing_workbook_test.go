package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/pricecardtemplate"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

const (
	templateTestdata = "../../internal/parcelpricing/adapters/pricecardtemplate/testdata/"
	full03WorkbookJS = templateTestdata + "price-card-cn-sg-full-03-workbook-PPT-2.json"
	full03XLSX       = templateTestdata + "price-card-cn-sg-full-03-PPT-2.xlsx"
)

// otherTenantReaderRegister 与 operationsReaderRegister 同形（持主数据与运营查阅读），主体绑在另一个租户上。
type otherTenantReaderRegister struct{}

func (otherTenantReaderRegister) FindOperator(_ context.Context, subject accessidentity.OperatorSubject) (accessidentity.OperatorStanding, bool, error) {
	binding, err := accessidentity.NewOperatorBinding(subject, "SYN-TENANT-02", "SYN-BASIS-BINDING")
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	grant, err := accessidentity.NewOperatorGrant("SYN-TENANT-02", "SYN-GRANT-READ", subject, accessidentity.CapabilityMasterDataAndOperationsRead, interval, "SYN-BASIS-GRANT")
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	recorded, err := accessidentity.NewRecordedGrant(grant, nil)
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	standing, err := accessidentity.NewOperatorStanding(binding, []accessidentity.RecordedGrant{recorded})
	return standing, err == nil, err
}

func registriesFor(t *testing.T, register accessidentity.OperatorRegistry) operatorRegistryIntakes {
	t.Helper()
	minter, err := buildOperatorMinter(acceptingVerifier{}, register, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	intakes, err := buildOperatorRegistryIntakes(minter)
	if err != nil {
		t.Fatal(err)
	}
	return intakes
}

// Covers: `AT-PP-047`，ADR-0349 决定五——模板规格口经操作者渠道走到真规格：持「主数据与运营查阅读」授予的操作者读到最新一版的全部表与列
// （含 PPT-2 新增的 total_bounds 与 TABLE_LOOKUP_BY_REMOTE_TIER）；规格租户无关，两个租户的操作者读到逐字节相同的答复；
// 只持登记写授予答未授予、无令牌答凭据被拒、渠道未配置答未配置；查询串封闭。
func TestPriceCardTemplateSpecEndpointAnswersTheRealSpecThroughTheOperatorChannel(t *testing.T) {
	serve := func(intakes operatorRegistryIntakes, target, token string) *httptest.ResponseRecorder {
		t.Helper()
		var handler http.Handler = pricinghttp.NewQueryPriceCardTemplateSpecEndpoint(pricinghttp.UnconfiguredIntake{}, staticPriceCardTemplateSpec{source: pricecardtemplate.Reader{}})
		if intakes.pricingOperationsRead != nil {
			handler = pricinghttp.NewQueryPriceCardTemplateSpecEndpoint(intakes.pricingOperationsRead, staticPriceCardTemplateSpec{source: pricecardtemplate.Reader{}})
		}
		request := httptest.NewRequest(http.MethodGet, target, nil)
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	const target = "/pricing-price-card-template/spec"
	response := serve(registriesFor(t, operationsReaderRegister{}), target, "presented.operator.token")
	if response.Code != http.StatusOK {
		t.Fatalf("授予齐备：%d %s", response.Code, response.Body)
	}
	var body struct {
		TemplateVersion          string   `json:"templateVersion"`
		AcceptedTemplateVersions []string `json:"acceptedTemplateVersions"`
		Sheets                   []struct {
			Name    string `json:"name"`
			Since   string `json:"since"`
			Columns []struct {
				Key           string   `json:"key"`
				AllowedValues []string `json:"allowedValues"`
			} `json:"columns"`
		} `json:"sheets"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	spec := pricecardtemplate.Reader{}.PriceCardTemplateSpec()
	if body.TemplateVersion != spec.TemplateVersion || len(body.Sheets) != len(spec.Sheets) {
		t.Fatalf("答复与规格源不一致：%s", response.Body)
	}
	var sawBounds, sawRemoteTierMethod bool
	for _, sheet := range body.Sheets {
		if sheet.Name == "total_bounds" && sheet.Since == "PPT-2" {
			sawBounds = true
		}
		for _, column := range sheet.Columns {
			if sheet.Name == "calculations" && column.Key == "method" {
				sawRemoteTierMethod = strings.Contains(strings.Join(column.AllowedValues, ","), "TABLE_LOOKUP_BY_REMOTE_TIER")
			}
		}
	}
	if !sawBounds || !sawRemoteTierMethod || strings.Join(body.AcceptedTemplateVersions, ",") != "PPT-1,PPT-2" {
		t.Fatalf("规格里缺 PPT-2 的新东西：%s", response.Body)
	}

	// 租户无关：别的租户的操作者读到逐字节相同的答复。
	other := serve(registriesFor(t, otherTenantReaderRegister{}), target, "presented.operator.token")
	if other.Code != http.StatusOK || !bytes.Equal(other.Body.Bytes(), response.Body.Bytes()) {
		t.Fatalf("别的租户读到的不同：%d", other.Code)
	}

	if r := serve(registriesFor(t, registryWriterRegister{}), target, "presented.operator.token"); r.Code != http.StatusForbidden || problemCode(t, r) != "OPERATOR_NOT_GRANTED" {
		t.Fatalf("只持登记写授予：%d %s", r.Code, r.Body)
	}
	if r := serve(registriesFor(t, operationsReaderRegister{}), target, ""); r.Code != http.StatusUnauthorized || problemCode(t, r) != "OPERATOR_CREDENTIAL_REJECTED" {
		t.Fatalf("无令牌：%d %s", r.Code, r.Body)
	}
	if r := serve(unconfiguredOperatorRegistries(), target, "presented.operator.token"); r.Code != http.StatusForbidden || problemCode(t, r) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
		t.Fatalf("渠道未配置：%d %s", r.Code, r.Body)
	}
	if r := serve(registriesFor(t, operationsReaderRegister{}), target+"?tenant=SYN-TENANT-02", "presented.operator.token"); r.Code != http.StatusBadRequest {
		t.Fatalf("带租户参数：%d", r.Code)
	}
}

// Covers: `AT-PP-045`、`AT-PP-046`，ADR-0349——页面表单提交的结构化工作簿走到真库：预览与存草稿两口经操作者渠道收 application/json，读通
// FULL-03 的模板写法得到种子卡的内容摘要与规范化版本 PPC-6，与同一份内容写成电子表格上传得同一个摘要；来源标识是 form:<规范化散列
// 前 12 位>，空白与键序不同的同一份内容得同一个来源标识；存草稿后草稿查阅读口标明来自页面表单，上传文件的草稿不带这一格；
// 同一份内容再提交答重放。输入是隔离合成，只记 `S`。
func TestFormWorkbookTravelsTheWiredImportFacesAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	intakes := registriesFor(t, registryWriterRegister{})
	preview, err := buildPriceCardImportPreview()
	if err != nil {
		t.Fatal(err)
	}
	submission, err := buildPriceCardDraftSubmission(db)
	if err != nil {
		t.Fatal(err)
	}
	drafts, err := pppostgres.NewPriceCardDrafts(db)
	if err != nil {
		t.Fatal(err)
	}
	previewFace := pricinghttp.NewPreviewPriceCardImportEndpoint(intakes.pricing, preview)
	draftFace := pricinghttp.NewSubmitPriceCardDraftEndpoint(intakes.pricing, submission)
	viewFace := pricinghttp.NewQueryPriceCardDraftsEndpoint(intakes.pricing, drafts)

	workbook, err := os.ReadFile(full03WorkbookJS)
	if err != nil {
		t.Fatal(err)
	}
	spreadsheetBytes, err := os.ReadFile(full03XLSX)
	if err != nil {
		t.Fatal(err)
	}
	seedRaw, err := os.ReadFile(filepath.Join("..", "..", "scripts", "demo-seeds", "data", "pricing", "price-card-cn-sg-full-03.json"))
	if err != nil {
		t.Fatal(err)
	}
	var seed struct {
		Plan struct {
			ContentDigest string `json:"contentDigest"`
		} `json:"plan"`
	}
	if err := json.Unmarshal(seedRaw, &seed); err != nil {
		t.Fatal(err)
	}

	post := func(face http.Handler, target, contentType string, body []byte) *httptest.ResponseRecorder {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, target, bytes.NewReader(body))
		request.Header.Set("Content-Type", contentType)
		request.Header.Set("Authorization", "Bearer presented.operator.token")
		response := httptest.NewRecorder()
		face.ServeHTTP(response, request)
		return response
	}
	type previewAnswer struct {
		Outcome         string `json:"outcome"`
		TemplateVersion string `json:"templateVersion"`
		SourceFile      struct {
			Name   string `json:"name"`
			SHA256 string `json:"sha256"`
		} `json:"sourceFile"`
		Content struct {
			Canonicalization string `json:"canonicalization"`
			ContentDigest    string `json:"contentDigest"`
		} `json:"content"`
		Problems []map[string]any `json:"problems"`
	}
	decodePreview := func(response *httptest.ResponseRecorder) previewAnswer {
		t.Helper()
		if response.Code != http.StatusOK {
			t.Fatalf("status = %d %s", response.Code, response.Body)
		}
		var answer previewAnswer
		if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
			t.Fatal(err)
		}
		return answer
	}

	fromForm := decodePreview(post(previewFace, "/pricing-price-card-previews", "application/json", workbook))
	if fromForm.Outcome != "VALIDATED" || fromForm.TemplateVersion != "PPT-2" || fromForm.Content.ContentDigest != seed.Plan.ContentDigest ||
		fromForm.Content.Canonicalization != "PPC-6" || len(fromForm.Problems) != 0 {
		t.Fatalf("表单预览 = %+v", fromForm)
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, workbook); err != nil {
		t.Fatal(err)
	}
	again := decodePreview(post(previewFace, "/pricing-price-card-previews", "application/json; charset=utf-8", compact.Bytes()))
	if again.SourceFile != fromForm.SourceFile || again.Content.ContentDigest != fromForm.Content.ContentDigest {
		t.Fatalf("空白不同的同一份内容该得同一个来源标识：%+v vs %+v", again.SourceFile, fromForm.SourceFile)
	}
	if !strings.HasPrefix(fromForm.SourceFile.Name, "form:") || len(fromForm.SourceFile.Name) != len("form:")+12 ||
		!strings.HasPrefix(fromForm.SourceFile.SHA256, fromForm.SourceFile.Name[len("form:"):]) {
		t.Fatalf("来源标识 = %+v", fromForm.SourceFile)
	}
	canonical, err := pricecardtemplate.Reader{}.CanonicalizePriceCardWorkbook(workbook)
	if err != nil {
		t.Fatal(err)
	}
	if sum := sha256.Sum256(canonical); hex.EncodeToString(sum[:]) != fromForm.SourceFile.SHA256 {
		t.Fatal("来源散列不是规范化字节的 SHA-256")
	}

	// 同一份内容写成电子表格上传：摘要相同，来源是文件。
	var upload bytes.Buffer
	writer := multipart.NewWriter(&upload)
	part, err := writer.CreateFormFile("file", "full-03.xlsx")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(spreadsheetBytes)
	_ = writer.Close()
	fromFile := decodePreview(post(previewFace, "/pricing-price-card-previews", writer.FormDataContentType(), upload.Bytes()))
	if fromFile.Outcome != "VALIDATED" || fromFile.Content.ContentDigest != fromForm.Content.ContentDigest || fromFile.SourceFile.Name != "full-03.xlsx" {
		t.Fatalf("文件预览 = %+v", fromFile)
	}

	// 别的 Content-Type 仍是原有的坏请求。
	if r := post(previewFace, "/pricing-price-card-previews", "text/plain", workbook); r.Code != http.StatusBadRequest {
		t.Fatalf("text/plain：%d", r.Code)
	}
	// 形状不对的工作簿整份不收，不是 4xx：读不通是一种读法。
	broken := decodePreview(post(previewFace, "/pricing-price-card-previews", "application/json", []byte(`{"templateVersion":"PPT-2"}`)))
	if broken.Outcome != "NOT_ACCEPTED" || len(broken.Problems) != 1 || broken.Problems[0]["code"] != "FILE_NOT_WORKBOOK" || broken.SourceFile.Name != "" {
		t.Fatalf("坏工作簿 = %+v", broken)
	}

	// 存草稿：首录 201 带 sourceOrigin: FORM；同内容再录（换个空白写法）重放 200。
	first := post(draftFace, "/pricing-price-card-drafts", "application/json", workbook)
	var saved struct {
		Outcome string `json:"outcome"`
		Reading struct {
			Outcome string `json:"outcome"`
		} `json:"reading"`
		Draft struct {
			Status       string `json:"status"`
			SourceOrigin string `json:"sourceOrigin"`
			SourceFile   struct {
				Name string `json:"name"`
			} `json:"sourceFile"`
		} `json:"draft"`
	}
	if first.Code != http.StatusCreated {
		t.Fatalf("首录：%d %s", first.Code, first.Body)
	}
	if err := json.Unmarshal(first.Body.Bytes(), &saved); err != nil {
		t.Fatal(err)
	}
	if saved.Outcome != "DRAFT_SUBMITTED" || saved.Reading.Outcome != "VALIDATED" || saved.Draft.Status != "VALIDATED" ||
		saved.Draft.SourceOrigin != "FORM" || saved.Draft.SourceFile.Name != fromForm.SourceFile.Name {
		t.Fatalf("首录 = %+v", saved)
	}
	replay := post(draftFace, "/pricing-price-card-drafts", "application/json", compact.Bytes())
	if replay.Code != http.StatusOK || !strings.Contains(replay.Body.String(), `"DRAFT_REPLAYED"`) {
		t.Fatalf("重放：%d %s", replay.Code, replay.Body)
	}

	// 草稿查阅读口：来自表单的这一行标 FORM，sourceFile 是 form:<散列前 12 位> 加规范化工作簿的完整散列。
	listDrafts := func() []map[string]any {
		t.Helper()
		view := httptest.NewRecorder()
		viewRequest := httptest.NewRequest(http.MethodGet, "/pricing-price-card-draft-views", nil)
		viewRequest.Header.Set("Authorization", "Bearer presented.operator.token")
		viewFace.ServeHTTP(view, viewRequest)
		if view.Code != http.StatusOK {
			t.Fatalf("查阅：%d %s", view.Code, view.Body)
		}
		var listed struct {
			Drafts []map[string]any `json:"drafts"`
		}
		if err := json.Unmarshal(view.Body.Bytes(), &listed); err != nil {
			t.Fatal(err)
		}
		var rows []map[string]any
		for _, row := range listed.Drafts {
			if row["plan"].(map[string]any)["id"] == "SYN-PLAN-CN-SG-FULL-03" {
				rows = append(rows, row)
			}
		}
		return rows
	}
	rows := listDrafts()
	if len(rows) != 1 || rows[0]["sourceOrigin"] != "FORM" ||
		rows[0]["sourceFile"].(map[string]any)["name"] != fromForm.SourceFile.Name || rows[0]["sourceFile"].(map[string]any)["sha256"] != fromForm.SourceFile.SHA256 {
		t.Fatalf("草稿查阅读口：%v", rows)
	}

	// 同一方案同一版再用上传文件录：源文件身份不同，已批准之前换内容即替换那一行；替换后的行是文件来源，不带来源格。
	revised := post(draftFace, "/pricing-price-card-drafts", writer.FormDataContentType(), upload.Bytes())
	if revised.Code != http.StatusCreated || !strings.Contains(revised.Body.String(), `"DRAFT_REVISED"`) {
		t.Fatalf("文件再录：%d %s", revised.Code, revised.Body)
	}
	rows = listDrafts()
	if len(rows) != 1 || rows[0]["sourceFile"].(map[string]any)["name"] != "full-03.xlsx" {
		t.Fatalf("替换后的草稿：%v", rows)
	}
	if _, marked := rows[0]["sourceOrigin"]; marked {
		t.Fatalf("上传文件的草稿不该带来源格：%v", rows[0])
	}
}

// Covers: `AT-PP-045`，ADR-0349 决定二、四——用到新结构（总额下限、按偏远档位查表）的页面表单草稿走完批准与发布：草稿是 PPC-6 的已校验草稿，
// 批准后发布登进价卡册（RECORDED），册上读回的规范化版本是 PPC-6、摘要等于种子 FULL-03 的摘要。导入这一条路从此能造出 PPC-6 的卡，
// 发布前所有读价卡的进程必须已是认 PPC-6 的构建（ADR-0348 风险点 6）。输入是隔离合成，只记 `S`。
func TestAFormDraftOfTheNewStructuresIsApprovedAndPublishedAsPPC6(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	submission, err := buildPriceCardDraftSubmission(db)
	if err != nil {
		t.Fatal(err)
	}
	approval, err := buildPriceCardDraftApproval(db)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := buildPriceCardDraftPublication(db)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := pppostgres.NewPriceCardApprovalDutyRules(db)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := pppostgres.NewPriceCards(db)
	if err != nil {
		t.Fatal(err)
	}
	workbook, err := os.ReadFile(full03WorkbookJS)
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}

	submitted, err := submission.Handle(t.Context(), pricingapp.SubmitPriceCardDraftCommand{
		Tenant: tenant, Submitter: "https://id.syn.example/dex#SYN-OPERATOR-01", FromForm: true, Raw: workbook,
	})
	if err != nil || submitted.Outcome != pricingapp.PriceCardDraftSubmitted || submitted.Draft.Status() != domain.PriceCardDraftStatusValidated || !submitted.Draft.SourceFile().FromForm() {
		t.Fatalf("录入 = %+v err=%v", submitted, err)
	}
	if content, _ := submitted.Draft.Content(); content.Plan.CanonicalizationVersion() != "PPC-6" {
		t.Fatalf("草稿的规范化版本 = %s，带新结构的方案要 PPC-6", content.Plan.CanonicalizationVersion())
	}

	rule, err := domain.NewPriceCardApprovalDutyRule(tenant, true, domain.OperatorGrant{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, saveErr := rules.SavePriceCardApprovalDutyRule(ctx, rule)
		return saveErr
	}); err != nil {
		t.Fatalf("登规则：%v", err)
	}
	approver, err := domain.NewOperatorSubject("https://id.syn.example/dex#SYN-OPERATOR-02", nil)
	if err != nil {
		t.Fatal(err)
	}
	plan := submitted.Draft.Plan()
	if result, err := approval.Handle(t.Context(), pricingapp.ApprovePriceCardDraftCommand{Tenant: tenant, Plan: plan, Approver: approver}); err != nil || result.Outcome != pricingapp.PriceCardDraftApproved {
		t.Fatalf("批准 = %s err=%v", result.Outcome, err)
	}
	published, err := publication.Handle(t.Context(), pricingapp.PublishPriceCardDraftCommand{Tenant: tenant, Plan: plan})
	if err != nil || published.Outcome != pricingapp.PriceCardDraftPublished || published.Registration != pricingapp.PriceCardRecorded {
		t.Fatalf("发布 = %s / %s err=%v", published.Outcome, published.Registration, err)
	}

	seedRaw, err := os.ReadFile(filepath.Join("..", "..", "scripts", "demo-seeds", "data", "pricing", "price-card-cn-sg-full-03.json"))
	if err != nil {
		t.Fatal(err)
	}
	seed, err := domain.RehydratePriceCardRegistration(seedRaw)
	if err != nil {
		t.Fatal(err)
	}
	registered, found, err := cards.ReadPriceCardContent(t.Context(), tenant, plan)
	if err != nil || !found || registered.Canonicalization != "PPC-6" || registered.ContentDigest != seed.Plan().ContentDigest() {
		t.Fatalf("册上读回 = %+v found=%v err=%v", registered, found, err)
	}
}

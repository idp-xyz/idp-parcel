package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/pricecardtemplate"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	"go.idp.xyz/idp-parcel/internal/platform/spreadsheet"
)

// 装配点接的是真读口：空白模板读得出模板版本、读不出方案身份，答未受理并带那一条问题——替身
// 读口不会知道模板长什么样。
func TestAssembledPriceCardPreviewReadsTheRealTemplate(t *testing.T) {
	preview, err := buildPriceCardImportPreview()
	if err != nil {
		t.Fatal(err)
	}
	var blank bytes.Buffer
	if err := pricecardtemplate.WriteTemplate(&blank); err != nil {
		t.Fatal(err)
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	answer, err := preview.Handle(context.Background(), pricingapp.PreviewPriceCardImportCommand{
		Tenant: tenant, FileName: pricecardtemplate.TemplateFileName, Raw: blank.Bytes(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if answer.Outcome != pricingapp.PriceCardImportNotAccepted || answer.TemplateVersion != pricecardtemplate.TemplateVersion ||
		len(answer.Problems) != 1 || answer.Problems[0].Code != ports.TemplatePlanIdentityUnreadable {
		t.Fatalf("answer = %+v", answer)
	}
}

// Covers: `/pricing-price-card-drafts` 的第二参是真编排——真读口、真库草稿册与装配点的事务包装在真实 PostgreSQL
// 上装得起来。草稿册写口无事务即拒，少了包装首录就答不出落点。只填方案身份的文件落一行`草稿`；同一份字节再录答
// 重放，读得到首行即证首录那笔事务提交了。输入是隔离合成，只记 `S`。
func TestTheWiredPriceCardDraftSubmissionRecordsAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	submission, err := buildPriceCardDraftSubmission(db)
	if err != nil {
		t.Fatalf("装配价卡录入编排：%v", err)
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	command := pricingapp.SubmitPriceCardDraftCommand{
		Tenant: tenant, Submitter: "SYN-OPERATOR-01", FileName: "SYN-PLAN-DRAFT-01.xlsx",
		Raw: templateWithPlanIdentity(t, "SYN-PLAN-DRAFT-01", "v1"),
	}

	submitted, err := submission.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("首录：%v", err)
	}
	if submitted.Outcome != pricingapp.PriceCardDraftSubmitted || submitted.Draft.Status() != domain.PriceCardDraftStatusDraft {
		t.Fatalf("首录 = %s / %s，想要 DRAFT_SUBMITTED / DRAFT", submitted.Outcome, submitted.Draft.Status())
	}
	replayed, err := submission.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("重放：%v", err)
	}
	if replayed.Outcome != pricingapp.PriceCardDraftReplayed {
		t.Fatalf("重放 outcome = %s，想要 DRAFT_REPLAYED——没读到首行说明首录事务没提交", replayed.Outcome)
	}
}

// templateWithPlanIdentity 是空白模板只填上方案身份两格：身份读得出，别的必填格都空，录入因此落一行`草稿`。
// 从空白模板读回再写出，表集合与列键都取模板自己的，不在这里另抄一份。
func templateWithPlanIdentity(t *testing.T, planID, planVersion string) []byte {
	t.Helper()
	var blank bytes.Buffer
	if err := pricecardtemplate.WriteTemplate(&blank); err != nil {
		t.Fatal(err)
	}
	workbook, err := spreadsheet.Read(blank.Bytes(), spreadsheet.Limits{})
	if err != nil {
		t.Fatal(err)
	}
	identity := map[string]string{"planId": planID, "planVersion": planVersion}
	sheets := make([]spreadsheet.WriteSheet, 0, len(workbook.Sheets))
	for _, sheet := range workbook.Sheets {
		header := sheet.Rows[0]
		written := spreadsheet.WriteSheet{Name: sheet.Name, Columns: make([]spreadsheet.WriteColumn, len(header.Cells))}
		for _, cell := range header.Cells {
			written.Columns[cell.Column] = spreadsheet.WriteColumn{Header: cell.Text, Text: true}
		}
		for _, row := range sheet.Rows[1:] {
			values := make([]string, len(written.Columns))
			for _, cell := range row.Cells {
				values[cell.Column] = cell.Text
			}
			if value, filled := identity[values[0]]; filled && sheet.Name == "card" {
				values[1] = value
			}
			written.Rows = append(written.Rows, values)
		}
		sheets = append(sheets, written)
	}
	var filled bytes.Buffer
	if err := spreadsheet.Write(&filled, sheets); err != nil {
		t.Fatal(err)
	}
	return filled.Bytes()
}

// 草稿查阅读口挂在操作者渠道的登记册 Intake 上（ADR-0101 Consequences）。swappedRegistryFaces 那两条只发 POST，
// 这个 GET 口另钉在这里。授予齐备的那一格越过 Intake、撞上未接线的读口答 NO_ANSWER_FORMED；还挂着未配置时，
// 各格一律答 403 ACCESS_CHANNEL_NOT_CONFIGURED。
func TestThePriceCardDraftViewsAnswerFromTheOperatorChannel(t *testing.T) {
	const views = "/pricing-price-card-draft-views"
	cases := map[string]struct {
		register accessidentity.OperatorRegistry
		token    string
		target   string
		status   int
		code     string
	}{
		"no bearer token":  {registryWriterRegister{}, "", views, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		"read grant only":  {readerRegister{}, "presented.operator.token", views, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		"register is down": {failingRegister{}, "presented.operator.token", views, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		// 认证过了才解查询串，自报的租户在那里被拒——租户只从信封来。
		"granted, tenant self-reported": {registryWriterRegister{}, "presented.operator.token", views + "?tenantId=SYN-TENANT-02", http.StatusBadRequest, "MALFORMED_REQUEST"},
		"granted":                       {registryWriterRegister{}, "presented.operator.token", views, http.StatusInternalServerError, "NO_ANSWER_FORMED"},
	}
	for name, testCase := range cases {
		minter, err := buildOperatorMinter(acceptingVerifier{}, testCase.register)
		if err != nil {
			t.Fatal(err)
		}
		registries, err := buildOperatorRegistryIntakes(minter)
		if err != nil {
			t.Fatal(err)
		}
		router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnwiredBusinessEndpointsWithOperatorIntakes(unconfiguredOperatorDecisions(), registries, unconfiguredIntegrationClientIntakes(), nil, nil, nil, nil, nil))
		request := httptest.NewRequest(http.MethodGet, testCase.target, nil)
		if testCase.token != "" {
			request.Header.Set("Authorization", "Bearer "+testCase.token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != testCase.status || problemCode(t, response) != testCase.code {
			t.Fatalf("%s: answer = %d %q, want %d %q", name, response.Code, problemCode(t, response), testCase.status, testCase.code)
		}
	}
}

package main

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
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

// Covers: `/pricing-price-card-draft-approvals` 与 `/pricing-price-card-draft-publications` 的第二参是真编排——真库草稿册、
// 真库审批职责规则册、真库价卡册与装配点的事务包装在真实 PostgreSQL 上走一遍（票 price-card-import/04）：规则没登时批准
// 答`未配置`；登了规则后换人批准转`已批准`；发布把那一版登进价卡册（RECORDED）、草稿转`已发布`；再发布答已发布——读得到
// `已发布`即证发布那一笔事务提交了。推进口与价卡册写口都无事务即拒，少了包装这几步都答不出来。输入是隔离合成，只记 `S`。
func TestTheWiredPriceCardDraftApprovalAndPublicationRecordAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	approval, err := buildPriceCardDraftApproval(db)
	if err != nil {
		t.Fatalf("装配批准编排：%v", err)
	}
	publication, err := buildPriceCardDraftPublication(db)
	if err != nil {
		t.Fatalf("装配发布编排：%v", err)
	}
	drafts, err := pppostgres.NewPriceCardDrafts(db)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := pppostgres.NewPriceCardApprovalDutyRules(db)
	if err != nil {
		t.Fatal(err)
	}

	registration := syntheticPriceCardRegistration(t)
	draft, err := domain.SubmitPriceCardDraft(domain.PriceCardDraftSubmission{
		Tenant: registration.Tenant(), Plan: registration.Plan().Reference(), Source: registration.SourceFile(),
		Content:   &domain.PriceCardDraftContent{Plan: registration.Plan(), DirectionAuthorization: registration.DirectionAuthorization()},
		Submitter: "https://id.syn.example/dex#SYN-OPERATOR-01", SubmittedAt: time.Now().UTC().Add(-time.Hour).Truncate(time.Microsecond),
	})
	if err != nil {
		t.Fatalf("立草稿：%v", err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, submitErr := drafts.SubmitDraft(ctx, draft)
		return submitErr
	}); err != nil {
		t.Fatalf("录入草稿：%v", err)
	}

	approver, err := domain.NewOperatorSubject("https://id.syn.example/dex#SYN-OPERATOR-02", nil)
	if err != nil {
		t.Fatal(err)
	}
	approve := pricingapp.ApprovePriceCardDraftCommand{Tenant: draft.Tenant(), Plan: draft.Plan(), Approver: approver}
	if result, err := approval.Handle(t.Context(), approve); err != nil || result.Outcome != pricingapp.PriceCardApprovalNotConfigured {
		t.Fatalf("规则没登时批准 = %s, err = %v，想要 NOT_CONFIGURED", result.Outcome, err)
	}
	rule, err := domain.NewPriceCardApprovalDutyRule(draft.Tenant(), true, domain.OperatorGrant{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, saveErr := rules.SavePriceCardApprovalDutyRule(ctx, rule)
		return saveErr
	}); err != nil {
		t.Fatalf("登规则：%v", err)
	}
	if result, err := approval.Handle(t.Context(), approve); err != nil || result.Outcome != pricingapp.PriceCardDraftApproved {
		t.Fatalf("换人批准 = %s, err = %v，想要 DRAFT_APPROVED", result.Outcome, err)
	}

	publish := pricingapp.PublishPriceCardDraftCommand{Tenant: draft.Tenant(), Plan: draft.Plan()}
	published, err := publication.Handle(t.Context(), publish)
	if err != nil || published.Outcome != pricingapp.PriceCardDraftPublished || published.Registration != pricingapp.PriceCardRecorded {
		t.Fatalf("发布 = %s / %s, err = %v，想要 DRAFT_PUBLISHED / RECORDED", published.Outcome, published.Registration, err)
	}
	if again, err := publication.Handle(t.Context(), publish); err != nil || again.Outcome != pricingapp.PriceCardPublicationDraftAlreadyPublished {
		t.Fatalf("再发布 = %s, err = %v，想要 DRAFT_ALREADY_PUBLISHED——没读到`已发布`说明发布那笔事务没提交", again.Outcome, err)
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

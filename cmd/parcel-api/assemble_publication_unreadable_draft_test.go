package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: AT-PC-071、AT-PC-072 的装配那一半——真铸造、真操作者 Intake、真库载体编排连起来：库上那一行被改成本构建读不回的样子
// （规范化版本是本构建不认得的 PCC-9，滚动发布时旧构建读新构建写的行正是这个形状），批准口与发布口答 200 加 DRAFT_NOT_READABLE 而不是 5xx，
// 事务不因它回滚成错误；按同一版本重新录入的修订把行换回读得回的内容，之后照常批准。测试输入是隔离合成，只记 `S`。
func TestTheOperatorChannelAnswersAnUnreadableCarrierAsABusinessGradeAgainstARealDatabase(t *testing.T) {
	const submitter, approver = "SYN-OPERATOR-SUBMITTER", "SYN-OPERATOR-APPROVER"
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registration, err := buildCommercialRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配商业登记编排：%v", err)
	}
	minter, err := buildOperatorMinter(tokenSubjects{}, registryWriterRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	registries, err := buildOperatorRegistryIntakes(minter)
	if err != nil {
		t.Fatal(err)
	}
	submit := commercialhttp.NewSubmitPublicationDraftEndpoint(registries.commercial, registration.publicationDrafts)
	approve := commercialhttp.NewApprovePublicationDraftEndpoint(registries.commercial, registration.publicationDrafts)
	publish := commercialhttp.NewPublishPublicationDraftEndpoint(registries.commercial, registration.publicationDrafts)
	post := func(endpoint http.Handler, token, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		endpoint.ServeHTTP(response, request)
		return response
	}

	submitted := post(submit, submitter, operatorCreditPolicyPayload)
	var answer struct {
		ContentDigest string `json:"contentDigest"`
		ShellDigest   string `json:"shellDigest"`
	}
	if err := json.Unmarshal(submitted.Body.Bytes(), &answer); err != nil || submitted.Code != http.StatusCreated {
		t.Fatalf("录入 = %d %s (%v)", submitted.Code, submitted.Body.Bytes(), err)
	}
	if _, err := pool.Exec(t.Context(),
		`UPDATE party_commercial.publication_draft
		    SET content_canonicalization = 'PCC-9', content_digest = 'PCC-9:future-build'
		  WHERE object_id = 'SYN-OC06-CREDIT-1'`); err != nil {
		t.Fatalf("把载体改成本构建读不回的样子：%v", err)
	}
	reference := func(digest string) string {
		return `{"kind": "CREDIT_POLICY", "objectId": "SYN-OC06-CREDIT-1", "version": "v1", "expectedDigest": "` + digest + `", "expectedShellDigest": "` + answer.ShellDigest + `"}`
	}

	for name, endpoint := range map[string]http.Handler{"批准口": approve, "发布口": publish} {
		response := post(endpoint, approver, reference("PCC-9:future-build"))
		var body struct {
			Outcome string `json:"outcome"`
			Status  string `json:"status"`
			Cause   string `json:"cause"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s：答复不是 JSON：%s", name, response.Body.Bytes())
		}
		if response.Code != http.StatusOK || body.Outcome != "DRAFT_NOT_READABLE" || body.Status != "PENDING_APPROVAL" || body.Cause == "" {
			t.Fatalf("%s = %d %s, want 200 DRAFT_NOT_READABLE 带状态与成因", name, response.Code, response.Body.Bytes())
		}
	}

	// 恢复动作：待批准的按同一版本重新录入，行被换成本构建读得回的内容。
	if revised := post(submit, submitter, operatorCreditPolicyPayload); revised.Code != http.StatusCreated || answeredOutcome(t, revised) != "DRAFT_REVISED" {
		t.Fatalf("重新录入 = %d %s, want 201 DRAFT_REVISED", revised.Code, revised.Body.Bytes())
	}
	// 审批职责规则没登：读得回的载体走到规则那一步答未配置，说明批准口已经读得回这一行。
	if approved := post(approve, approver, reference(answer.ContentDigest)); approved.Code != http.StatusOK || answeredOutcome(t, approved) != "NOT_CONFIGURED" {
		t.Fatalf("重新录入之后批准应读得回、走到规则那一步：%d %s", approved.Code, approved.Body.Bytes())
	}
}

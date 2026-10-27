package commercialhttp_test

import (
	"errors"
	"net/http"
	"strings"
	"testing"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// problemsOfPayload 取一份载荷在 Publication 那一步立不住的逐格问题，按 JSON 路径索引。
func problemsOfPayload(t *testing.T, raw string) map[string]commercialhttp.PayloadProblem {
	t.Helper()
	_, _, err := decodePublication(t, raw).Publication(pcNew(t, domain.NewTenantID, pcTenant))
	var problems *commercialhttp.PublicationPayloadProblems
	if !errors.As(err, &problems) {
		t.Fatalf("err = %v (%T), want *PublicationPayloadProblems", err, err)
	}
	byField := map[string]commercialhttp.PayloadProblem{}
	for _, problem := range problems.Problems {
		byField[problem.Field] = problem
	}
	return byField
}

// Covers: AT-PC-070——逐格问题带稳定的问题码与参数：取值不在封闭集答 NOT_IN_SET 并带格名与收到的值；空值答 REQUIRED；时刻格式不对答
// TIME_FORMAT、时刻没填答 REQUIRED；额度的两种互斥各答各的码。原话照旧在 `Problem` 里。
func TestPayloadProblemsCarryStableCodesAndParams(t *testing.T) {
	problems := problemsOfPayload(t, `{
	  "kind": "CREDIT_POLICY", "objectId": "credit-1", "version": "v1", "scope": "SYN-SCOPE",
	  "effectiveStartsAt": "next monday", "effectiveEndsAt": "",
	  "creditPolicy": {
	    "legalEntity": "", "authorityLevel": "L1", "chargeType": "freight",
	    "limitMinor": 100, "ratioBase": "ORDER_AMOUNT",
	    "effectiveStartsAt": ""
	  }
	}`)
	for field, want := range map[string]string{
		"effectiveStartsAt":              commercialhttp.ProblemCodeTimeFormat,
		"creditPolicy.legalEntity":       commercialhttp.ProblemCodeRequired,
		"creditPolicy.effectiveStartsAt": commercialhttp.ProblemCodeRequired,
		"creditPolicy.ratioBase":         commercialhttp.ProblemCodeRatioBaseNotAllowed,
	} {
		got, ok := problems[field]
		if !ok || got.Code != want {
			t.Errorf("%s：code = %q（%+v），want %s", field, got.Code, got, want)
		}
		if ok && got.Problem == "" {
			t.Errorf("%s：原话不该丢", field)
		}
	}

	ratio := problemsOfPayload(t, `{
	  "kind": "CREDIT_POLICY", "objectId": "credit-1", "version": "v1", "scope": "SYN-SCOPE",
	  "effectiveStartsAt": "2026-10-01T00:00:00Z",
	  "creditPolicy": {
	    "legalEntity": "LE-1", "authorityLevel": "L1", "chargeType": "freight",
	    "limitRatioBasisPoints": 500, "ratioBase": "NOT_A_BASE",
	    "effectiveStartsAt": "2026-10-01T00:00:00Z"
	  }
	}`)
	notInSet := ratio["creditPolicy.ratioBase"]
	if notInSet.Code != commercialhttp.ProblemCodeNotInSet || notInSet.Params["what"] != "比例基数" || notInSet.Params["value"] != "NOT_A_BASE" {
		t.Fatalf("集合外的比例基数：%+v", notInSet)
	}

	both := problemsOfPayload(t, `{
	  "kind": "CREDIT_POLICY", "objectId": "credit-1", "version": "v1", "scope": "SYN-SCOPE",
	  "effectiveStartsAt": "2026-10-01T00:00:00Z",
	  "creditPolicy": {
	    "legalEntity": "LE-1", "authorityLevel": "L1", "chargeType": "freight",
	    "limitMinor": 100, "limitRatioBasisPoints": 500,
	    "effectiveStartsAt": "2026-10-01T00:00:00Z"
	  }
	}`)
	if exactly := both["creditPolicy.limit"]; exactly.Code != commercialhttp.ProblemCodeExactlyOne || exactly.Params["subject"] != "额度" {
		t.Fatalf("额度两格都填：%+v", exactly)
	}
	none := problemsOfPayload(t, `{
	  "kind": "CREDIT_POLICY", "objectId": "credit-1", "version": "v1", "scope": "SYN-SCOPE",
	  "effectiveStartsAt": "2026-10-01T00:00:00Z",
	  "creditPolicy": {
	    "legalEntity": "LE-1", "authorityLevel": "L1", "chargeType": "freight",
	    "limitRatioBasisPoints": 500,
	    "effectiveStartsAt": "2026-10-01T00:00:00Z"
	  }
	}`)
	if missing := none["creditPolicy.ratioBase"]; missing.Code != commercialhttp.ProblemCodeRatioBaseRequired {
		t.Fatalf("比例额度缺基数：%+v", missing)
	}
}

// Covers: AT-PC-070——没有更具体的码的问题答兜底码 INVALID，且没有参数：客户端对它退回原话，不会拿到一个错的码。
func TestUncodedPayloadProblemsAnswerTheFallbackCode(t *testing.T) {
	problems := problemsOfPayload(t, `{
	  "kind": "CREDIT_POLICY", "objectId": "credit-1", "version": "v1", "scope": "SYN-SCOPE",
	  "effectiveStartsAt": "2026-10-01T00:00:00Z", "effectiveEndsAt": "2026-09-01T00:00:00Z",
	  "creditPolicy": {
	    "legalEntity": "LE-1", "authorityLevel": "L1", "chargeType": "freight", "limitMinor": 100,
	    "effectiveStartsAt": "2026-10-01T00:00:00Z"
	  }
	}`)
	reversed := problems["effectiveEndsAt"]
	if reversed.Code != commercialhttp.ProblemCodeInvalid || len(reversed.Params) != 0 || reversed.Problem == "" {
		t.Fatalf("区间倒置：%+v", reversed)
	}
}

// Covers: AT-PC-070——预览口把问题码与参数带到答复里：每一条逐格问题都有 code，没有参数的问题不出现 params 键。
func TestPreviewAnswerCarriesTheProblemCodes(t *testing.T) {
	_, _, err := decodePublication(t, `{
	  "kind": "CREDIT_POLICY", "objectId": "credit-1", "version": "v1", "scope": "SYN-SCOPE",
	  "effectiveStartsAt": "2026-10-01T00:00:00Z",
	  "creditPolicy": {
	    "legalEntity": "", "authorityLevel": "L1", "chargeType": "freight",
	    "limitRatioBasisPoints": 500, "ratioBase": "NOT_A_BASE",
	    "effectiveStartsAt": "2026-10-01T00:00:00Z"
	  }
	}`).Publication(pcNew(t, domain.NewTenantID, pcTenant))
	response := serve(commercialhttp.NewPreviewCommercialPublicationEndpoint(draftIntakeDouble{err: err}, application.NewPreviewCommercialPublicationHandler()), http.MethodPost)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.Bytes())
	}
	body := response.Body.String()
	for _, want := range []string{`"code":"REQUIRED"`, `"code":"NOT_IN_SET"`, `"params":{"value":"NOT_A_BASE","what":"比例基数"}`} {
		if !strings.Contains(body, want) {
			t.Errorf("答复里没有 %s：%s", want, body)
		}
	}
	if strings.Contains(body, `"code":"REQUIRED","params"`) {
		t.Errorf("没有参数的问题不该带 params 键：%s", body)
	}
}

// Covers: AT-PC-070——各册自己的问题码：适用对象两格都空答 EXACTLY_ONE，天数不是正整数答 DAYS_NOT_POSITIVE 带收到的数，期限种类集合外答
// NOT_IN_SET，判断顺序答 ORDER_NOT_POSITIVE，收费条件缺 charged 答 CHARGED_UNDECLARED，空清单答 LIST_EMPTY_OR_DUPLICATE。
func TestRegisterSpecificProblemCodes(t *testing.T) {
	service := problemsOfPayload(t, `{
	  "kind": "CUSTOMER_SERVICE_RULE", "objectId": "csr-1", "version": "v1", "scope": "SYN-SCOPE-PC02C",
	  "effectiveStartsAt": "2026-08-01T00:00:00Z",
	  "customerServiceRule": {
	    "responsible": "resp-1", "scope": "scope-1",
	    "claimDeadlines": [{"kind": "APPEAL", "startEvent": "event-delivered", "days": 0, "calendar": "calendar-cn"}],
	    "minimumMaterials": [{"claimKind": "claim-damage", "materials": ["material-photo"]}]
	  }
	}`)
	if got := service["customerServiceRule.applicability"]; got.Code != commercialhttp.ProblemCodeExactlyOne || got.Params["subject"] != "适用对象" {
		t.Errorf("适用对象：%+v", got)
	}
	if got := service["customerServiceRule.claimDeadlines[0].days"]; got.Code != commercialhttp.ProblemCodeDaysNotPositive || got.Params["value"] != "0" {
		t.Errorf("天数：%+v", got)
	}
	if got := service["customerServiceRule.claimDeadlines[0].kind"]; got.Code != commercialhttp.ProblemCodeNotInSet || got.Params["what"] != "期限种类" || got.Params["value"] != "APPEAL" {
		t.Errorf("期限种类：%+v", got)
	}

	control := problemsOfPayload(t, `{
	  "kind": "PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY", "objectId": "fcp-1", "version": "v1", "scope": "SYN-SCOPE-PC02C",
	  "effectiveStartsAt": "2026-08-01T00:00:00Z",
	  "preAcceptanceFinancialControlPolicy": {
	    "jointPassCondition": "ANY_CONTROL_PASSES",
	    "controls": [{"control": "PREPAID_FREEZE", "chargeScope": "charge-ok", "order": 0, "onFailure": "REJECT", "responsibility": "customer-1"}]
	  }
	}`)
	if got := control["preAcceptanceFinancialControlPolicy.controls[0].order"]; got.Code != commercialhttp.ProblemCodeOrderNotPositive || got.Params["value"] != "0" {
		t.Errorf("判断顺序：%+v", got)
	}

	charged := problemsOfPayload(t, strings.Replace(supplierAgreementPayload, `"effectiveEndsAt": "2026-12-31T16:00:00Z"`,
		`"effectiveEndsAt": "2026-12-31T16:00:00Z", "chargeConditions": [{"reason": "ACTUAL_FULFILLMENT", "quantityForm": "PER_SEGMENT"}]`, 1))
	if got := charged["supplierAgreement.chargeConditions[0].charged"]; got.Code != commercialhttp.ProblemCodeChargedUndeclared {
		t.Errorf("计不计费：%+v", got)
	}
	empty := problemsOfPayload(t, strings.Replace(supplierAgreementPayload, `"effectiveEndsAt": "2026-12-31T16:00:00Z"`,
		`"effectiveEndsAt": "2026-12-31T16:00:00Z", "chargeConditions": []`, 1))
	if got := empty["supplierAgreement.chargeConditions"]; got.Code != commercialhttp.ProblemCodeListEmptyOrDuplicate {
		t.Errorf("空清单：%+v", got)
	}
}

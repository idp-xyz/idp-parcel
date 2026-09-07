package commercialhttp_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// Covers: 票 admin-write-faces/06——`?kind=PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY` 是第九册，与既有
// `PRE_ACCEPTANCE_CONTROL`（合同级「要不要」声明，0007）是两本册：种类名照「kind 命名册子」的约定取
// 策略版本自己（拥有者与被列者同一，与 CREDIT_POLICY 同形），分派只走策略册、不碰声明册。行体照客户
// 服务规则册：壳先入册、正文随发布——只有壳的版本 contentRegistered 为假且 content 键不在场（那正是
// 「发布得出来、管理台看不见」要治的那一格，壳必须可见）；登了正文的行 content 节带共同通过条件、
// 登记时刻与按判断顺序的控制项，控制项各格的键与受控 CLI 批文同词（control / chargeScope / order /
// onFailure / responsibility），写面与读面不各说一套。
//
// 本用例是接手在途实现时先于读它写下的那片 red；对方那份同判据的用例已合并进本用例，不留两份。
func TestPoliciesEndpointListsPreAcceptanceFinancialControlPoliciesAsTheirOwnRegister(t *testing.T) {
	query := catalogueQuery(t)
	reader := &policyReaderDouble{
		tenant: query.Scope.Tenant(),
		controls: []ports.PreAcceptanceControlRow{
			{ContractObjectID: "contract-1", ContractVersion: "v1", Requirement: "REQUIRED", DeclaredAt: catBaseAt},
		},
		controlPols: []ports.PreAcceptanceFinancialControlPolicyRow{
			{ObjectID: "fcp-bare", VersionLabel: "v1", Scope: "scope-1", Status: "EFFECTIVE",
				EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt},
			{ObjectID: "fcp-full", VersionLabel: "v2", Scope: "scope-1", Status: "EFFECTIVE",
				EffectiveStartsAt: catBaseAt, EffectiveEndsAt: catBaseAt.Add(time.Hour), HasEffectiveEnd: true,
				PublishedAt: catBaseAt,
				HasContent:  true, JointPassCondition: "ALL_CONTROLS_PASS", RegisteredAt: catBaseAt.Add(time.Minute),
				Controls: []ports.PreAcceptanceControlItemRow{
					{Kind: "PREPAID_FREEZE", ChargeScope: "charge-scope-a", EvaluationOrder: 1,
						FailureDisposition: "REJECT", Responsibility: "customer-1"},
					{Kind: "CREDIT_CHECK", ChargeScope: "charge-scope-a", EvaluationOrder: 2,
						FailureDisposition: "AUTHORIZED_DISPOSITION", Responsibility: "operator-legal-1"},
				}},
		},
	}
	endpoint := commercialhttp.NewQueryCommercialPoliciesEndpoint(intakeDouble{query: query}, reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, "/commercial-policies?kind=PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if reader.calls["controlPolicies"] != 1 || reader.calls["controls"] != 0 {
		t.Fatalf("分派走错了册：%v", reader.calls)
	}
	body := decodeBody(t, recorder)
	if body["outcome"] != "COMMERCIAL_POLICIES_LISTED" || body["kind"] != "PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY" {
		t.Fatalf("outcome/kind = %v/%v", body["outcome"], body["kind"])
	}
	rows, ok := body["policies"].([]any)
	if !ok || len(rows) != 2 {
		t.Fatalf("policies = %v", body["policies"])
	}

	bare := rows[0].(map[string]any)
	if bare["objectId"] != "fcp-bare" || bare["version"] != "v1" || bare["status"] != "EFFECTIVE" || bare["scope"] != "scope-1" {
		t.Fatalf("壳自身的字段没有照列：%v", bare)
	}
	if bare["contentRegistered"] != false {
		t.Fatalf("只有壳的版本 contentRegistered = %v", bare["contentRegistered"])
	}
	if _, has := bare["content"]; has {
		t.Fatal("没登正文的版本长出了 content 节")
	}
	if _, has := bare["effectiveEndsAt"]; has {
		t.Fatal("开放结束的版本长出了 effectiveEndsAt 键")
	}

	full := rows[1].(map[string]any)
	if full["contentRegistered"] != true || full["effectiveEndsAt"] == nil {
		t.Fatalf("登了正文的版本 = %v", full)
	}
	content, ok := full["content"].(map[string]any)
	if !ok {
		t.Fatalf("content 节缺席：%v", full)
	}
	if content["jointPassCondition"] != "ALL_CONTROLS_PASS" || content["registeredAt"] == nil {
		t.Fatalf("正文字段变形：%v", content)
	}
	controls, ok := content["controls"].([]any)
	if !ok || len(controls) != 2 {
		t.Fatalf("controls = %v", content["controls"])
	}
	first := controls[0].(map[string]any)
	if first["control"] != "PREPAID_FREEZE" || first["chargeScope"] != "charge-scope-a" || first["order"] != float64(1) ||
		first["onFailure"] != "REJECT" || first["responsibility"] != "customer-1" {
		t.Fatalf("第一项转写变形：%v", first)
	}
	second := controls[1].(map[string]any)
	if second["control"] != "CREDIT_CHECK" || second["order"] != float64(2) || second["onFailure"] != "AUTHORIZED_DISPOSITION" {
		t.Fatalf("第二项转写变形：%v", second)
	}
}

// Covers: 既有的 `PRE_ACCEPTANCE_CONTROL` 仍只列合同级声明——第九册加进来不改第二册的分派，两本册在
// 名字上相邻却各自拥有对象不同（合同版本 / 策略版本），走错一格就是让合同拥有策略正文。
func TestPoliciesEndpointKeepsContractLevelControlDeclarationsOnTheirOwnKind(t *testing.T) {
	query := catalogueQuery(t)
	reader := &policyReaderDouble{
		tenant: query.Scope.Tenant(),
		controls: []ports.PreAcceptanceControlRow{
			{ContractObjectID: "contract-1", ContractVersion: "v1", Requirement: "REQUIRED", DeclaredAt: catBaseAt},
		},
		controlPols: []ports.PreAcceptanceFinancialControlPolicyRow{
			{ObjectID: "fcp-1", VersionLabel: "v1", Scope: "scope-1", Status: "EFFECTIVE",
				EffectiveStartsAt: catBaseAt, PublishedAt: catBaseAt},
		},
	}
	endpoint := commercialhttp.NewQueryCommercialPoliciesEndpoint(intakeDouble{query: query}, reader)

	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder,
		httptest.NewRequest(http.MethodGet, "/commercial-policies?kind=PRE_ACCEPTANCE_CONTROL", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d body=%s", recorder.Code, recorder.Body.String())
	}
	if reader.calls["controls"] != 1 || reader.calls["controlPolicies"] != 0 {
		t.Fatalf("分派走错了册：%v", reader.calls)
	}
	body := decodeBody(t, recorder)
	if body["kind"] != "PRE_ACCEPTANCE_CONTROL" {
		t.Fatalf("kind = %v", body["kind"])
	}
	if rows, ok := body["policies"].([]any); !ok || len(rows) != 1 {
		t.Fatalf("policies = %v", body["policies"])
	}
}

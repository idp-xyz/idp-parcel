package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// businessEndpointProbes 是本进程应当服务的全部业务端点及各自的有效探测请求。它与
// 装配点互为对照：表里多一项说明装配漏了一个端点（那一项会退回路由层的 404，正是
// ADR-0055 要治的折叠），装配点里多一项说明上线了一个没人钉过形状的面。
//
// 方法与必填分派参数必须逐项写对：拿错方法测出来的 405、漏掉 family/registry/kind
// 测出来的 400 都会盖过未配置 Intake 的 403，断言就什么也没守住。
type businessEndpointProbe struct {
	method string
	target string
}

var businessEndpointProbes = map[string]businessEndpointProbe{
	"/shipment-requests":                                {method: http.MethodPost, target: "/shipment-requests"},
	"/shipment-requests/withdrawals":                    {method: http.MethodPost, target: "/shipment-requests/withdrawals"},
	"/shipment-requests/parcel-cancellations":           {method: http.MethodPost, target: "/shipment-requests/parcel-cancellations"},
	"/shipment-requests/manual-review-completions":      {method: http.MethodPost, target: "/shipment-requests/manual-review-completions"},
	"/shipment-requests/rejections":                     {method: http.MethodPost, target: "/shipment-requests/rejections"},
	"/shipment-request-views":                           {method: http.MethodGet, target: "/shipment-request-views"},
	"/acceptance-review-queue":                          {method: http.MethodGet, target: "/acceptance-review-queue"},
	"/label-transactions":                               {method: http.MethodGet, target: "/label-transactions"},
	"/node-operations/receptions":                       {method: http.MethodPost, target: "/node-operations/receptions"},
	"/node-operations-records":                          {method: http.MethodGet, target: "/node-operations-records?registry=reception"},
	"/transport-fulfillment/deliveries":                 {method: http.MethodPost, target: "/transport-fulfillment/deliveries"},
	"/transport-fulfillment/delivery-proof-corrections": {method: http.MethodPost, target: "/transport-fulfillment/delivery-proof-corrections"},
	"/transport-fulfillment-records":                    {method: http.MethodGet, target: "/transport-fulfillment-records?registry=transport-schedule"},
	"/customer-tracking-view":                           {method: http.MethodGet, target: "/customer-tracking-view"},
	"/tracking-projections":                             {method: http.MethodGet, target: "/tracking-projections"},
	"/claims":                                           {method: http.MethodPost, target: "/claims"},
	"/customs/external-results":                         {method: http.MethodPost, target: "/customs/external-results"},
	"/pricing-price-cards":                              {method: http.MethodGet, target: "/pricing-price-cards"},
	"/pricing-reference-series":                         {method: http.MethodGet, target: "/pricing-reference-series"},
	"/pricing-evaluations":                              {method: http.MethodGet, target: "/pricing-evaluations"},
	"/pricing-price-card-registrations":                 {method: http.MethodPost, target: "/pricing-price-card-registrations"},
	"/pricing-reference-series-registrations":           {method: http.MethodPost, target: "/pricing-reference-series-registrations"},
	"/network-catalog":                                  {method: http.MethodGet, target: "/network-catalog?family=node"},
	"/route-plans":                                      {method: http.MethodGet, target: "/route-plans?register=initial-route"},
	"/governance-registers":                             {method: http.MethodGet, target: "/governance-registers?register=authority-interval"},
	"/customs-compliance-rules":                         {method: http.MethodGet, target: "/customs-compliance-rules?registry=case-requirement"},
	"/customs-case-registers":                           {method: http.MethodGet, target: "/customs-case-registers?registry=readiness"},
	"/customs-gate-conditions":                          {method: http.MethodGet, target: "/customs-gate-conditions"},
	"/customs-ports-paths":                              {method: http.MethodGet, target: "/customs-ports-paths?registry=candidate-port"},
	"/commercial-service-products":                      {method: http.MethodGet, target: "/commercial-service-products"},
	"/commercial-policies":                              {method: http.MethodGet, target: "/commercial-policies?kind=ACCEPTANCE_RULE_PACKAGE"},
	"/commercial-customer-contracts":                    {method: http.MethodGet, target: "/commercial-customer-contracts"},
	"/commercial-supplier-agreements":                   {method: http.MethodGet, target: "/commercial-supplier-agreements"},
	"/commercial-group-legal-entities":                  {method: http.MethodGet, target: "/commercial-group-legal-entities"},
	"/commercial-party-relationships":                   {method: http.MethodGet, target: "/commercial-party-relationships"},
	"/commercial-product-channel-mappings":              {method: http.MethodGet, target: "/commercial-product-channel-mappings"},
	"/visibility-catalogues":                            {method: http.MethodGet, target: "/visibility-catalogues?kind=MILESTONE_MAPPING"},
	"/exception-triage-records":                         {method: http.MethodGet, target: "/exception-triage-records?registry=signal-episode"},
	"/exception-case-records":                           {method: http.MethodGet, target: "/exception-case-records"},
	"/claims-recovery-records":                          {method: http.MethodGet, target: "/claims-recovery-records?registry=customer-notification"},
	"/collection-subledgers":                            {method: http.MethodGet, target: "/collection-subledgers"},
	"/settlement-charges":                               {method: http.MethodGet, target: "/settlement-charges?registry=customer-charge"},
	"/settlement-statements":                            {method: http.MethodGet, target: "/settlement-statements?registry=customer-statement"},
	"/settlement-funds-applications":                    {method: http.MethodGet, target: "/settlement-funds-applications"},
	"/settlement-operating-results":                     {method: http.MethodGet, target: "/settlement-operating-results?registry=operating-result"},
}

// Covers: ADR-0055 第一、二、三条 — 端点已装配、未配置自成一格、状态码取 403。
//
// 这是唯一证明「装配确实发生了」的地方：各上下文的传输层测试拿自己构造的处理器跑，
// 装不装配它们都绿。这里走的是 cmd/parcel-api 真正交给 http.Server 的那个路由。
func TestEveryAssembledEndpointAnswersUnconfigured(t *testing.T) {
	// 传 unwired* 占位而非真编排与真读口：本测试钉的是未配置面（403 在编排之前），
	// 真编排的装配与行为由各 assemble_*_test.go 对真库另证。
	endpoints := assembleUnconfiguredBusinessEndpoints()
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, endpoints)

	mounted := make(map[string]bool, len(endpoints))
	for _, endpoint := range endpoints {
		probe, listed := businessEndpointProbes[endpoint.Pattern]
		if !listed {
			t.Fatalf("装配了未登记的端点 %s：形状没有任何测试钉住", endpoint.Pattern)
		}
		if mounted[endpoint.Pattern] {
			t.Fatalf("端点 %s 装配了两次：后一个会静默盖掉前一个", endpoint.Pattern)
		}
		mounted[endpoint.Pattern] = true

		response := httptest.NewRecorder()
		router.ServeHTTP(response, httptest.NewRequest(probe.method, probe.target, nil))

		if response.Code != http.StatusForbidden {
			t.Fatalf("%s %s: status = %d, want %d", probe.method, probe.target, response.Code, http.StatusForbidden)
		}
		if got := problemCode(t, response); got != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("%s: code = %q, want ACCESS_CHANNEL_NOT_CONFIGURED", endpoint.Pattern, got)
		}
		assertNoOutcome(t, response, endpoint.Pattern)
	}

	for pattern := range businessEndpointProbes {
		if !mounted[pattern] {
			t.Fatalf("端点 %s 没进装配：它会答 404，与「产品没有这个能力」不可分辨", pattern)
		}
	}
}

// Covers: ADR-0055 「未配置格住在 Intake 缝里，不在路由层另设闸」 — 未配置不改变方法
// 约束：方法不对仍由处理器自己答 405，403 不越过它抢答。两处各有权威就会各改一次。
func TestUnconfiguredDoesNotSwallowTheMethodGate(t *testing.T) {
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnconfiguredBusinessEndpoints())

	response := httptest.NewRecorder()
	router.ServeHTTP(response, httptest.NewRequest(http.MethodDelete, "/shipment-requests", nil))

	if response.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status = %d, want %d", response.Code, http.StatusMethodNotAllowed)
	}
	if allow := response.Header().Get("Allow"); allow != http.MethodPost {
		t.Fatalf("Allow = %q, want %q", allow, http.MethodPost)
	}
}

// Covers: ADR-0055 「答复对一切请求内容与自报身份一致」 — 在装配后的路由上再钉一次：
// 各包的替身证的是自己那个处理器，这里证的是进程真正对外的那一个。
func TestAssembledEndpointsIgnoreSelfReportedIdentity(t *testing.T) {
	router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnconfiguredBusinessEndpoints())

	baseline := httptest.NewRecorder()
	router.ServeHTTP(baseline, httptest.NewRequest(http.MethodPost, "/shipment-requests", nil))

	reported := httptest.NewRequest(http.MethodPost, "/shipment-requests", nil)
	reported.Header.Set("X-Reported-Tenant", "TENANT-9")
	reported.Header.Set("Authorization", "Bearer whatever")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, reported)

	if response.Code != baseline.Code || response.Body.String() != baseline.Body.String() {
		t.Fatalf("answer differs from baseline: %d %s vs %d %s",
			response.Code, response.Body.String(), baseline.Code, baseline.Body.String())
	}
}

func assembleUnconfiguredBusinessEndpoints() []httpapi.BusinessEndpoint {
	return assembleUnwiredBusinessEndpoints(nil)
}

// assembleUnwiredBusinessEndpoints 以 unwired* 占位读口与编排装配全部端点，隔离读面
// 输入由调用方给：nil 钉「未配置面」，非 nil 钉「放行只及运营查阅行」（isolated_read_test.go）。
func assembleUnwiredBusinessEndpoints(isolatedRead *isolatedReadIntakes) []httpapi.BusinessEndpoint {
	return assembleBusinessEndpoints(
		unwiredSubmission{},
		unwiredWithdrawal{},
		unwiredRequestViews{},
		unwiredManualReview{},
		unwiredRejection{},
		unwiredReviewQueue{},
		unwiredReviewJudgments{},
		unwiredLabelTransactions{},
		unwiredCancellation{},
		unwiredReception{},
		unwiredNodeOperationsRecords{},
		unwiredDelivery{},
		unwiredTransportFulfillmentRecords{},
		unwiredTrackingViews{},
		unwiredProjectionViews{},
		unwiredClaims{},
		unwiredResults{},
		unwiredPricingCatalogue{},
		unwiredPricingCatalogue{},
		unwiredPricingEvaluations{},
		unwiredPriceCardRegistration{},
		unwiredReferenceSeriesRegistration{},
		unwiredNetworkCatalogue{},
		unwiredRoutePlans{},
		unwiredComplianceRules{},
		unwiredCaseRegisters{},
		unwiredGateConditions{},
		unwiredPortsPaths{},
		unwiredCommercialCatalogue{},
		unwiredCommercialCatalogue{},
		unwiredCommercialCatalogue{},
		unwiredCommercialCatalogue{},
		unwiredCommercialCatalogue{},
		unwiredVisibilityCatalogue{},
		unwiredCaseReview{},
		unwiredCaseReview{},
		unwiredCaseReview{},
		unwiredCodSubledgers{},
		unwiredSettlementCharges{},
		unwiredSettlementStatements{},
		unwiredSettlementFundsApplications{},
		unwiredSettlementOperatingResults{},
		unwiredGovernanceRegisters{},
		isolatedRead,
	)
}

func problemCode(t *testing.T, response *httptest.ResponseRecorder) string {
	t.Helper()
	var body struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode problem response %s: %v", response.Body.Bytes(), err)
	}
	return body.Error.Code
}

func assertNoOutcome(t *testing.T, response *httptest.ResponseRecorder, pattern string) {
	t.Helper()
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(response.Body.Bytes(), &fields); err != nil {
		t.Fatalf("decode response %s: %v", response.Body.Bytes(), err)
	}
	if _, present := fields["outcome"]; present {
		t.Fatalf("%s: 一个没形成答案的响应带了 outcome：%s", pattern, response.Body.Bytes())
	}
}

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
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
	"/shipment-requests":                                                       {method: http.MethodPost, target: "/shipment-requests"},
	"/shipment-requests/withdrawals":                                           {method: http.MethodPost, target: "/shipment-requests/withdrawals"},
	"/shipment-requests/parcel-cancellations":                                  {method: http.MethodPost, target: "/shipment-requests/parcel-cancellations"},
	"/shipment-requests/continued-attempt-closures":                            {method: http.MethodPost, target: "/shipment-requests/continued-attempt-closures"},
	"/shipment-requests/continued-attempt-reopenings":                          {method: http.MethodPost, target: "/shipment-requests/continued-attempt-reopenings"},
	"/shipment-requests/manual-review-completions":                             {method: http.MethodPost, target: "/shipment-requests/manual-review-completions"},
	"/shipment-requests/rejections":                                            {method: http.MethodPost, target: "/shipment-requests/rejections"},
	"/shipment-requests/authorized-dispositions":                               {method: http.MethodPost, target: "/shipment-requests/authorized-dispositions"},
	"/shipment-requests/supplements":                                           {method: http.MethodPost, target: "/shipment-requests/supplements"},
	"/shipment-requests/source-data-amendments":                                {method: http.MethodPost, target: "/shipment-requests/source-data-amendments"},
	"/shipment-request-views":                                                  {method: http.MethodGet, target: "/shipment-request-views"},
	"/acceptance-review-queue":                                                 {method: http.MethodGet, target: "/acceptance-review-queue"},
	"/authorized-disposition-queue":                                            {method: http.MethodGet, target: "/authorized-disposition-queue"},
	"/commercial-business-parties":                                             {method: http.MethodGet, target: "/commercial-business-parties"},
	"/commercial-business-parties/{partyId}/revisions":                         {method: http.MethodGet, target: "/commercial-business-parties/SYN-PARTY-01/revisions"},
	"/label-transactions":                                                      {method: http.MethodGet, target: "/label-transactions"},
	"/channel-selection-decisions":                                             {method: http.MethodGet, target: "/channel-selection-decisions?view=tied"},
	"/node-operations/receptions":                                              {method: http.MethodPost, target: "/node-operations/receptions"},
	"/node-operations-records":                                                 {method: http.MethodGet, target: "/node-operations-records?registry=reception"},
	"/transport-fulfillment/deliveries":                                        {method: http.MethodPost, target: "/transport-fulfillment/deliveries"},
	"/transport-fulfillment/delivery-proof-corrections":                        {method: http.MethodPost, target: "/transport-fulfillment/delivery-proof-corrections"},
	"/transport-fulfillment/handovers":                                         {method: http.MethodPost, target: "/transport-fulfillment/handovers"},
	"/transport-fulfillment/handover-corrections":                              {method: http.MethodPost, target: "/transport-fulfillment/handover-corrections"},
	"/transport-fulfillment/offsite-pickups":                                   {method: http.MethodPost, target: "/transport-fulfillment/offsite-pickups"},
	"/transport-fulfillment/offsite-pickup-corrections":                        {method: http.MethodPost, target: "/transport-fulfillment/offsite-pickup-corrections"},
	"/transport-fulfillment/offsite-pickup-attempts":                           {method: http.MethodPost, target: "/transport-fulfillment/offsite-pickup-attempts"},
	"/transport-fulfillment/movement-facts":                                    {method: http.MethodPost, target: "/transport-fulfillment/movement-facts"},
	"/transport-fulfillment-segment-closures":                                  {method: http.MethodPost, target: "/transport-fulfillment-segment-closures"},
	"/transport-fulfillment-dispatch-task-registrations":                       {method: http.MethodPost, target: "/transport-fulfillment-dispatch-task-registrations"},
	"/transport-fulfillment-delivery-dispatch-triggers":                        {method: http.MethodPost, target: "/transport-fulfillment-delivery-dispatch-triggers"},
	"/transport-fulfillment-load-assignment-registrations":                     {method: http.MethodPost, target: "/transport-fulfillment-load-assignment-registrations"},
	"/transport-fulfillment-participation-terminations":                        {method: http.MethodPost, target: "/transport-fulfillment-participation-terminations"},
	"/transport-fulfillment-external-carrier-credential-registrations":         {method: http.MethodPost, target: "/transport-fulfillment-external-carrier-credential-registrations"},
	"/transport-fulfillment-external-carrier-credential-applicability-changes": {method: http.MethodPost, target: "/transport-fulfillment-external-carrier-credential-applicability-changes"},
	"/transport-fulfillment-effective-time-rule-registrations":                 {method: http.MethodPost, target: "/transport-fulfillment-effective-time-rule-registrations"},
	"/transport-fulfillment-external-tracking-facts":                           {method: http.MethodGet, target: "/transport-fulfillment-external-tracking-facts?source=SYN-SOURCE-1&view=pending"},
	"/transport-fulfillment-effective-time-judgments":                          {method: http.MethodPost, target: "/transport-fulfillment-effective-time-judgments"},
	"/transport-fulfillment-carrier-first-effective-pickups":                   {method: http.MethodGet, target: "/transport-fulfillment-carrier-first-effective-pickups?object=SYN-PARCEL-1"},
	"/transport-fulfillment-carrier-first-effective-pickup-judgments":          {method: http.MethodPost, target: "/transport-fulfillment-carrier-first-effective-pickup-judgments"},
	"/transport-fulfillment-carrier-master-document-registrations":             {method: http.MethodPost, target: "/transport-fulfillment-carrier-master-document-registrations"},
	"/transport-fulfillment-carrier-master-document-revisions":                 {method: http.MethodPost, target: "/transport-fulfillment-carrier-master-document-revisions"},
	"/transport-fulfillment-records":                                           {method: http.MethodGet, target: "/transport-fulfillment-records?registry=transport-schedule"},
	"/transport-fulfillment-handover-scope-summary":                            {method: http.MethodGet, target: "/transport-fulfillment-handover-scope-summary?scope=scope-1"},
	"/customer-tracking-view":                                                  {method: http.MethodGet, target: "/customer-tracking-view"},
	"/tracking-projections":                                                    {method: http.MethodGet, target: "/tracking-projections"},
	"/claims":                                                                  {method: http.MethodPost, target: "/claims"},
	"/customs/external-results":                                                {method: http.MethodPost, target: "/customs/external-results"},
	"/pricing-price-cards":                                                     {method: http.MethodGet, target: "/pricing-price-cards"},
	"/pricing-reference-series":                                                {method: http.MethodGet, target: "/pricing-reference-series"},
	"/pricing-reference-series-coverage":                                       {method: http.MethodGet, target: "/pricing-reference-series-coverage"},
	"/pricing-pending-series-evaluations":                                      {method: http.MethodGet, target: "/pricing-pending-series-evaluations"},
	"/pricing-evaluations":                                                     {method: http.MethodGet, target: "/pricing-evaluations"},
	"/pricing-price-card-registrations":                                        {method: http.MethodPost, target: "/pricing-price-card-registrations"},
	"/pricing-reference-series-registrations":                                  {method: http.MethodPost, target: "/pricing-reference-series-registrations"},
	"/pricing-reference-series-reviews":                                        {method: http.MethodPost, target: "/pricing-reference-series-reviews"},
	"/pricing-reference-series-previews":                                       {method: http.MethodPost, target: "/pricing-reference-series-previews"},
	"/pricing-reference-catalogue-registrations":                               {method: http.MethodPost, target: "/pricing-reference-catalogue-registrations"},
	"/pricing-evaluation-replays":                                              {method: http.MethodPost, target: "/pricing-evaluation-replays"},
	"/network-catalog":                                                         {method: http.MethodGet, target: "/network-catalog?family=node"},
	"/route-plans":                                                             {method: http.MethodGet, target: "/route-plans?register=initial-route"},
	"/network-catalog-node-registrations":                                      {method: http.MethodPost, target: "/network-catalog-node-registrations"},
	"/network-catalog-connection-registrations":                                {method: http.MethodPost, target: "/network-catalog-connection-registrations"},
	"/network-catalog-line-registrations":                                      {method: http.MethodPost, target: "/network-catalog-line-registrations"},
	"/network-catalog-service-area-registrations":                              {method: http.MethodPost, target: "/network-catalog-service-area-registrations"},
	"/network-catalog-service-calendar-registrations":                          {method: http.MethodPost, target: "/network-catalog-service-calendar-registrations"},
	"/network-catalog-availability-adjustment-registrations":                   {method: http.MethodPost, target: "/network-catalog-availability-adjustment-registrations"},
	"/network-catalog-route-strategy-registrations":                            {method: http.MethodPost, target: "/network-catalog-route-strategy-registrations"},
	"/governance-registers":                                                    {method: http.MethodGet, target: "/governance-registers?register=authority-interval"},
	"/customs-compliance-rules":                                                {method: http.MethodGet, target: "/customs-compliance-rules?registry=case-requirement"},
	"/customs-case-registers":                                                  {method: http.MethodGet, target: "/customs-case-registers?registry=readiness"},
	"/customs-gate-conditions":                                                 {method: http.MethodGet, target: "/customs-gate-conditions"},
	"/customs-ports-paths":                                                     {method: http.MethodGet, target: "/customs-ports-paths?registry=candidate-port"},
	"/customs-credentials":                                                     {method: http.MethodGet, target: "/customs-credentials"},
	"/customs-duty-collaborations":                                             {method: http.MethodGet, target: "/customs-duty-collaborations"},
	"/customs-duty-verifications":                                              {method: http.MethodGet, target: "/customs-duty-verifications"},
	"/customs-interpretation-rule-registrations":                               {method: http.MethodPost, target: "/customs-interpretation-rule-registrations"},
	"/customs-gate-catalog-registrations":                                      {method: http.MethodPost, target: "/customs-gate-catalog-registrations"},
	"/customs-candidate-port-registrations":                                    {method: http.MethodPost, target: "/customs-candidate-port-registrations"},
	"/customs-declaration-path-registrations":                                  {method: http.MethodPost, target: "/customs-declaration-path-registrations"},
	"/customs-case-requirement-registrations":                                  {method: http.MethodPost, target: "/customs-case-requirement-registrations"},
	"/customs-regulatory-credential-registrations":                             {method: http.MethodPost, target: "/customs-regulatory-credential-registrations"},
	"/customs-duty-collaboration-registrations":                                {method: http.MethodPost, target: "/customs-duty-collaboration-registrations"},
	"/customs-duty-payment-verification-registrations":                         {method: http.MethodPost, target: "/customs-duty-payment-verification-registrations"},
	"/commercial-service-products":                                             {method: http.MethodGet, target: "/commercial-service-products"},
	"/commercial-policies":                                                     {method: http.MethodGet, target: "/commercial-policies?kind=ACCEPTANCE_RULE_PACKAGE"},
	"/commercial-customer-contracts":                                           {method: http.MethodGet, target: "/commercial-customer-contracts"},
	"/commercial-supplier-agreements":                                          {method: http.MethodGet, target: "/commercial-supplier-agreements"},
	"/commercial-group-legal-entities":                                         {method: http.MethodGet, target: "/commercial-group-legal-entities"},
	"/commercial-group-legal-entities/{legalEntityId}/revisions":               {method: http.MethodGet, target: "/commercial-group-legal-entities/SYN-LE-01/revisions"},
	"/commercial-group-legal-entities/{legalEntityId}/profile-revisions":       {method: http.MethodGet, target: "/commercial-group-legal-entities/SYN-LE-01/profile-revisions"},
	"/commercial-party-relationships":                                          {method: http.MethodGet, target: "/commercial-party-relationships"},
	"/commercial-customer-accounts":                                            {method: http.MethodGet, target: "/commercial-customer-accounts"},
	"/commercial-product-channel-mappings":                                     {method: http.MethodGet, target: "/commercial-product-channel-mappings"},
	"/commercial-registration-number-types":                                    {method: http.MethodGet, target: "/commercial-registration-number-types"},
	"/commercial-publications":                                                 {method: http.MethodPost, target: "/commercial-publications"},
	"/commercial-publication-previews":                                         {method: http.MethodPost, target: "/commercial-publication-previews"},
	"/commercial-publication-drafts":                                           {method: http.MethodPost, target: "/commercial-publication-drafts"},
	"/commercial-publication-draft-approvals":                                  {method: http.MethodPost, target: "/commercial-publication-draft-approvals"},
	"/commercial-publication-draft-publications":                               {method: http.MethodPost, target: "/commercial-publication-draft-publications"},
	"/commercial-publication-vocabularies":                                     {method: http.MethodGet, target: "/commercial-publication-vocabularies?kind=ACCEPTANCE_RULE_PACKAGE"},
	"/commercial-business-party-registrations":                                 {method: http.MethodPost, target: "/commercial-business-party-registrations"},
	"/commercial-legal-entity-registrations":                                   {method: http.MethodPost, target: "/commercial-legal-entity-registrations"},
	"/commercial-customer-account-registrations":                               {method: http.MethodPost, target: "/commercial-customer-account-registrations"},
	"/commercial-party-relationship-registrations":                             {method: http.MethodPost, target: "/commercial-party-relationship-registrations"},
	"/commercial-party-identity-deactivations":                                 {method: http.MethodPost, target: "/commercial-party-identity-deactivations"},
	"/commercial-service-product-form-registrations":                           {method: http.MethodPost, target: "/commercial-service-product-form-registrations"},
	"/commercial-product-channel-mapping-registrations":                        {method: http.MethodPost, target: "/commercial-product-channel-mapping-registrations"},
	"/commercial-registration-number-type-registrations":                       {method: http.MethodPost, target: "/commercial-registration-number-type-registrations"},
	"/commercial-registration-number-type-deactivations":                       {method: http.MethodPost, target: "/commercial-registration-number-type-deactivations"},
	"/commercial-legal-entity-profile-registrations":                           {method: http.MethodPost, target: "/commercial-legal-entity-profile-registrations"},
	"/commercial-channel-account-use-registrations":                            {method: http.MethodPost, target: "/commercial-channel-account-use-registrations"},
	"/commercial-channel-account-use-revocations":                              {method: http.MethodPost, target: "/commercial-channel-account-use-revocations"},
	"/visibility-catalogues":                                                   {method: http.MethodGet, target: "/visibility-catalogues?kind=MILESTONE_MAPPING"},
	"/visibility-catalogue-milestone-mapping-registrations":                    {method: http.MethodPost, target: "/visibility-catalogue-milestone-mapping-registrations"},
	"/visibility-catalogue-triage-rule-registrations":                          {method: http.MethodPost, target: "/visibility-catalogue-triage-rule-registrations"},
	"/visibility-catalogue-notification-policy-registrations":                  {method: http.MethodPost, target: "/visibility-catalogue-notification-policy-registrations"},
	"/visibility-catalogue-claim-eligibility-registrations":                    {method: http.MethodPost, target: "/visibility-catalogue-claim-eligibility-registrations"},
	"/visibility-catalogue-claim-authorization-registrations":                  {method: http.MethodPost, target: "/visibility-catalogue-claim-authorization-registrations"},
	"/visibility-catalogue-disclosure-policy-registrations":                    {method: http.MethodPost, target: "/visibility-catalogue-disclosure-policy-registrations"},
	"/visibility-catalogue-exception-disclosure-rule-registrations":            {method: http.MethodPost, target: "/visibility-catalogue-exception-disclosure-rule-registrations"},
	"/visibility-catalogue-conflict-signal-rule-registrations":                 {method: http.MethodPost, target: "/visibility-catalogue-conflict-signal-rule-registrations"},
	"/exception-triage-records":                                                {method: http.MethodGet, target: "/exception-triage-records?registry=signal-episode"},
	"/exception-case-records":                                                  {method: http.MethodGet, target: "/exception-case-records"},
	"/claims-recovery-records":                                                 {method: http.MethodGet, target: "/claims-recovery-records?registry=customer-notification"},
	"/collection-subledgers":                                                   {method: http.MethodGet, target: "/collection-subledgers"},
	"/settlement-charges":                                                      {method: http.MethodGet, target: "/settlement-charges?registry=customer-charge"},
	"/settlement-statements":                                                   {method: http.MethodGet, target: "/settlement-statements?registry=customer-statement"},
	"/settlement-funds-applications":                                           {method: http.MethodGet, target: "/settlement-funds-applications"},
	"/settlement-operating-results":                                            {method: http.MethodGet, target: "/settlement-operating-results?registry=operating-result"},
	"/settlement-external-funds-fact-registrations":                            {method: http.MethodPost, target: "/settlement-external-funds-fact-registrations"},
	"/settlement-external-funds-fact-correction-registrations":                 {method: http.MethodPost, target: "/settlement-external-funds-fact-correction-registrations"},
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
//
// 写行的两个隔离 Intake 恒为 nil：**读开关换不了写行**是 ADR-0091 决定四的形状，而这个
// 辅助函数正是隔离读那组用例的入口——它若把读写入参并成一个，那组用例就再也证不了这件事。
// 写行放行的三态另有 isolated_write_test.go。
func assembleUnwiredBusinessEndpoints(isolatedRead *isolatedReadIntakes) []httpapi.BusinessEndpoint {
	return assembleUnwiredBusinessEndpointsWith(isolatedRead, nil, nil)
}

func assembleUnwiredBusinessEndpointsWith(
	isolatedRead *isolatedReadIntakes,
	isolatedSubmission shipmenthttp.SubmissionIntake,
	isolatedPartyIdentity *commercialhttp.IsolatedPartyIdentityIntake,
) []httpapi.BusinessEndpoint {
	return assembleBusinessEndpoints(
		unwiredSubmission{},
		unwiredWithdrawal{},
		unwiredRequestViews{},
		unwiredManualReview{},
		unwiredRejection{},
		unwiredDisposition{},
		unwiredSupplement{},
		unwiredAmendment{},
		unwiredReviewQueue{},
		unwiredReviewJudgments{},
		unwiredDispositionQueue{},
		unwiredLabelTransactions{},
		unwiredChannelSelectionDecisions{},
		unwiredCancellation{},
		unwiredContinuedAttemptDecisions{},
		unwiredReception{},
		unwiredNodeOperationsRecords{},
		unwiredDelivery{},
		unwiredTransportFulfillmentRecords{},
		unwiredHandoverScopeSummary{},
		unwiredHandover{},
		unwiredPickupRegistration{},
		unwiredPickupCorrection{},
		unwiredPickupAttempt{},
		unwiredMovementFact{},
		unwiredSegmentCloser{},
		unwiredDispatchTaskOpener{},
		unwiredDeliveryDispatchTrigger{},
		unwiredLoadAssigner{},
		unwiredParticipationEnder{},
		unwiredCredentialRegistration{},
		unwiredEffectiveTimeRuleRegistration{},
		unwiredExternalTrackingFactReview{},
		unwiredEffectiveTimeJudgment{},
		unwiredCarrierPickupChain{},
		unwiredCarrierPickupJudgment{},
		unwiredMasterDocumentRegistration{},
		unwiredTrackingViews{},
		unwiredProjectionViews{},
		unwiredClaims{},
		unwiredResults{},
		unwiredPricingCatalogue{},
		unwiredPricingCatalogue{},
		unwiredReferenceSeriesCoverage{},
		unwiredPendingSeriesEvaluations{},
		unwiredPricingEvaluations{},
		unwiredPriceCardRegistration{},
		unwiredReferenceSeriesRegistration{},
		unwiredReferenceSeriesReview{},
		unwiredReferenceSeriesPreview{},
		unwiredReferenceCatalogueRegistration{},
		unwiredEvaluationReplay{},
		unwiredNetworkCatalogue{},
		unwiredRoutePlans{},
		unwiredNetworkCatalogRegistration{},
		unwiredComplianceRules{},
		unwiredCaseRegisters{},
		unwiredGateConditions{},
		unwiredPortsPaths{},
		unwiredCredentialCatalogue{},
		unwiredDutyReconciliationCatalogue{},
		unwiredDutyReconciliationCatalogue{},
		unwiredInterpretationRuleRegistration{},
		unwiredGateCatalogRegistration{},
		unwiredCandidatePortRegistration{},
		unwiredDeclarationPathRegistration{},
		unwiredCaseRequirementRegistration{},
		unwiredRegulatoryCredentialRegistration{},
		unwiredDutyCollaborationRegistration{},
		unwiredDutyPaymentVerificationRegistration{},
		unwiredCommercialCatalogue{},
		unwiredCommercialCatalogue{},
		unwiredCommercialCatalogue{},
		unwiredCommercialCatalogue{},
		unwiredCommercialCatalogue{},
		unwiredCommercialCatalogue{},
		unwiredCommercialCatalogue{},
		unwiredLegalEntityProfileRevisions{},
		unwiredCommercialPublication{},
		unwiredCommercialPublicationPreview{},
		unwiredPublicationDrafts{},
		unwiredPartyIdentityRegistration{},
		unwiredProductChannelRegistration{},
		unwiredRegistrationNumberTypeRegistration{},
		unwiredLegalEntityProfileRegistration{},
		unwiredChannelAccountUseRegistration{},
		unwiredVisibilityCatalogue{},
		unwiredMilestoneMappingRegistration{},
		unwiredTriageRulesRegistration{},
		unwiredNotificationPolicyRegistration{},
		unwiredClaimEligibilityRegistration{},
		unwiredClaimAuthorizationRegistration{},
		unwiredDisclosurePolicyRegistration{},
		unwiredExceptionDisclosureRulesRegistration{},
		unwiredConflictSignalRuleRegistration{},
		unwiredCaseReview{},
		unwiredCaseReview{},
		unwiredCaseReview{},
		unwiredCodSubledgers{},
		unwiredSettlementCharges{},
		unwiredSettlementStatements{},
		unwiredSettlementFundsApplications{},
		unwiredSettlementOperatingResults{},
		unwiredExternalFundsFactRegistration{},
		unwiredExternalFundsFactCorrectionRegistration{},
		unwiredGovernanceRegisters{},
		isolatedRead,
		isolatedSubmission,
		isolatedPartyIdentity,
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

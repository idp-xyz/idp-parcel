package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// swappedCatalogueReadFaces 是 ADR-0077 那一族目录查阅。隔离读放行已撤，答复走操作者查阅读。
var swappedCatalogueReadPatterns = []string{
	"/pricing-price-cards",
	"/pricing-reference-series",
	"/pricing-reference-series-coverage",
	"/pricing-pending-series-evaluations",
	"/pricing-evaluations",
	"/network-catalog",
	"/route-plans",
	"/customs-compliance-rules",
	"/customs-case-registers",
	"/customs-gate-conditions",
	"/customs-ports-paths",
	"/customs-credentials",
	"/customs-duty-collaborations",
	"/customs-duty-verifications",
	"/commercial-service-products",
	"/commercial-policies",
	"/commercial-customer-contracts",
	"/commercial-supplier-agreements",
	"/commercial-business-parties",
	"/commercial-business-parties/{partyId}/revisions",
	"/commercial-group-legal-entities",
	"/commercial-group-legal-entities/{legalEntityId}/revisions",
	"/commercial-group-legal-entities/{legalEntityId}/profile-revisions",
	"/commercial-group-legal-entities/{legalEntityId}/profile-resolution",
	"/commercial-party-relationships",
	"/commercial-customer-accounts",
	"/commercial-product-channel-mappings",
	"/commercial-registration-number-types",
}

// swappedOperationsReadPatterns 是运营查阅面余下各族（票 operator-channel/35）：面单交易、渠道择优、节点作业、运输履约、
// 追踪与异常、代收、结算、治理。委托查阅三口不在其中，仍挂隔离读（票 operator-channel/53）。
var swappedOperationsReadPatterns = []string{
	"/label-transactions",
	"/channel-selection-decisions",
	"/node-operations-records",
	"/transport-fulfillment-records",
	"/transport-fulfillment-handover-scope-summary",
	"/transport-fulfillment-external-tracking-facts",
	"/transport-fulfillment-carrier-first-effective-pickups",
	"/tracking-projections",
	"/visibility-catalogues",
	"/exception-triage-records",
	"/exception-case-records",
	"/claims-recovery-records",
	"/collection-subledgers",
	"/settlement-charges",
	"/settlement-statements",
	"/settlement-funds-applications",
	"/settlement-operating-results",
	"/governance-registers",
}

// operationsReaderRegister 持有主数据与运营查阅读，不持登记写。
type operationsReaderRegister struct{}

func (operationsReaderRegister) FindOperator(_ context.Context, subject accessidentity.OperatorSubject) (accessidentity.OperatorStanding, bool, error) {
	binding, err := accessidentity.NewOperatorBinding(subject, "SYN-TENANT-01", "SYN-BASIS-BINDING")
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	grant, err := accessidentity.NewOperatorGrant("SYN-TENANT-01", "SYN-GRANT-READ", subject, accessidentity.CapabilityMasterDataAndOperationsRead, interval, "SYN-BASIS-GRANT")
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

// Covers: 票 operator-channel/05、35 完成判据——已换各口三格各答其格；授予齐备的合成操作者越过 Intake，读口未接线所以答
// NO_ANSWER_FORMED，而不是未配置。隔离读打开也不再改这几口。
func TestSwappedCatalogueReadFacesAnswerFromTheOperatorChannel(t *testing.T) {
	granted, err := buildOperatorMinter(acceptingVerifier{}, operationsReaderRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	grantedRegistries, err := buildOperatorRegistryIntakes(granted)
	if err != nil {
		t.Fatal(err)
	}
	ungranted, err := buildOperatorMinter(acceptingVerifier{}, registryWriterRegister{}, admissionScopeUnset())
	if err != nil {
		t.Fatal(err)
	}
	ungrantedRegistries, err := buildOperatorRegistryIntakes(ungranted)
	if err != nil {
		t.Fatal(err)
	}
	isolated, err := buildIsolatedReadIntakes(fakeGetenv(map[string]string{isolatedReadTenantEnv: "SYN-TENANT-01"}))
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name       string
		registries operatorRegistryIntakes
		token      string
		status     int
		code       string
	}{
		{"channel not configured", unconfiguredOperatorRegistries(), "presented.operator.token", http.StatusForbidden, "ACCESS_CHANNEL_NOT_CONFIGURED"},
		{"no bearer token", grantedRegistries, "", http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		{"write grant is not a read grant", ungrantedRegistries, "presented.operator.token", http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		{"granted synthetic operator reaches the reader", grantedRegistries, "presented.operator.token", http.StatusInternalServerError, "NO_ANSWER_FORMED"},
	}
	for _, testCase := range cases {
		router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnwiredBusinessEndpointsWithOperatorIntakes(
			unconfiguredOperatorDecisions(), testCase.registries, unconfiguredIntegrationClientIntakes(),
			isolated, nil, nil, nil))
		for _, pattern := range append(append([]string{}, swappedCatalogueReadPatterns...), swappedOperationsReadPatterns...) {
			probe, ok := businessEndpointProbes[pattern]
			if !ok {
				t.Fatalf("探针表没有 %s", pattern)
			}
			request := httptest.NewRequest(probe.method, probe.target, nil)
			if testCase.token != "" {
				request.Header.Set("Authorization", "Bearer "+testCase.token)
			}
			response := httptest.NewRecorder()
			router.ServeHTTP(response, request)
			if response.Code != testCase.status || problemCode(t, response) != testCase.code {
				t.Fatalf("%s, %s: answer = %d %q, want %d %q", pattern, testCase.name, response.Code, problemCode(t, response), testCase.status, testCase.code)
			}
		}
	}
}

package main

import (
	"fmt"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	veaccess "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/accessidentity"
	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
)

type claimsWiring struct {
	claims visibilityhttp.ClaimReceiver
	// customerClaim 缺席即字面量未配置（customerClaimIntake 判 nil）。
	customerClaim *visibilityhttp.CustomerClaimIntake
}

func init() {
	registerEndpoints(endpointFeature[claimsWiring]{
		lines: map[string]endpointLine{
			"/v1/claims": {
				method: http.MethodPost, face: customerClientFace(accessidentity.CustomerCapabilityClaimSubmission), ownIntake: true,
			},
		},
		build: func(sources endpointSources) (claimsWiring, error) {
			claims, err := buildClaimsOrchestration(sources.db)
			if err != nil {
				return claimsWiring{claims: claims}, err
			}
			intake, err := buildCustomerClaimIntake(sources)
			return claimsWiring{claims: claims, customerClaim: intake}, err
		},
		mount: func(_ endpointChannels, wiring claimsWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				{Pattern: "/v1/claims", Handler: visibilityhttp.NewReceiveClaimEndpoint(customerClaimIntake(wiring.customerClaim), wiring.claims)},
			}
		},
	})
}

// buildCustomerClaimIntake 装配索赔口的首方 Intake（ADR-0139 决定四；票 first-party-api/09）：铸造器与提交口同一只，索赔提交在铸造侧判
// 准入范围；目标归属用追踪视图查阅口那一只反查口，合同归属读 VE 自己的声明册。
func buildCustomerClaimIntake(sources endpointSources) (*visibilityhttp.CustomerClaimIntake, error) {
	if sources.customerClientMinter == nil {
		return nil, fmt.Errorf("customer claim intake needs a minter")
	}
	authenticator, err := veaccess.NewCustomerClaimAuthenticator(sources.customerClientMinter)
	if err != nil {
		return nil, err
	}
	targets, err := buildCustomerTrackingViewAttributions(sources.db)
	if err != nil {
		return nil, err
	}
	contracts, err := vepostgres.NewMultiTenantClaimEligibilityRules(sources.db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: claim contract ownership: %w", err)
	}
	intake, err := visibilityhttp.NewCustomerClaimIntake(authenticator, targets, contracts, systemClock{})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: customer claim intake: %w", err)
	}
	return intake, nil
}

// customerClaimIntake 交回端点表那一行用的 Intake：首方 Intake 缺席时是字面量未配置即拒。在这里判 nil，是为了不把带类型的空指针塞进
// 接口——那样的 Intake 不等于 nil，第一次请求才会崩。两个隔离开关都换不了这一口（ADR-0150 决定三）。
func customerClaimIntake(intake *visibilityhttp.CustomerClaimIntake) visibilityhttp.ClaimIntake {
	if intake == nil {
		return visibilityhttp.UnconfiguredIntake{}
	}
	return intake
}

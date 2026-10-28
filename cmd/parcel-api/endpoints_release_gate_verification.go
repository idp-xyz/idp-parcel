package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type releaseGateVerificationWiring struct {
	verifier customshttp.ReleaseGateVerifier
}

func init() {
	registerEndpoints(endpointFeature[releaseGateVerificationWiring]{
		lines: map[string]endpointLine{
			"/customs-release-gate-verifications": {method: http.MethodPost, face: decisionFace(accessidentity.DecisionReleaseGateVerification)},
		},
		build: func(sources endpointSources) (releaseGateVerificationWiring, error) {
			verifier, err := buildReleaseGateVerification(sources.db)
			return releaseGateVerificationWiring{verifier: verifier}, err
		},
		mount: func(channels endpointChannels, wiring releaseGateVerificationWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 放行门禁核对口（UC-CC-009）：有权角色对（申报范围，拟执行动作，监管边界）请求核对，挂「运营决定」能力面的放行门禁核对一种，
				// 与建案、提交、关案分开授予。门禁满足不生成放行；折不出结论答未决、不落册；写开关换不了它（ADR-0150 决定三）。
				{Pattern: "/customs-release-gate-verifications", Handler: customshttp.NewVerifyReleaseGateEndpoint(channels.operatorDecisions.customs, wiring.verifier)},
			}
		},
	})
}

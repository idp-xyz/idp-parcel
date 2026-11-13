package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type manifestReferencesWiring struct {
	manifests customshttp.ManifestHandler
}

// 外部监管舱单引用的首登与来源版本推进（UC-CC-012）。两口同属一类事实：推进的提交方与首登是同一家承运商，认证与授予
// 也是同一份。
func init() {
	registerEndpoints(endpointFeature[manifestReferencesWiring]{
		lines: map[string]endpointLine{
			"/customs/external-manifest-references":          {method: http.MethodPost, face: integrationClientFace(accessidentity.FactExternalRegulatoryManifest)},
			"/customs/external-manifest-reference-revisions": {method: http.MethodPost, face: integrationClientFace(accessidentity.FactExternalRegulatoryManifest)},
		},
		build: func(sources endpointSources) (manifestReferencesWiring, error) {
			manifests, err := buildManifestReferencesOrchestration(sources.db)
			return manifestReferencesWiring{manifests: manifests}, err
		},
		mount: func(channels endpointChannels, wiring manifestReferencesWiring) []httpapi.BusinessEndpoint {
			intake := customshttp.ManifestReferenceIntake(channels.integrationClients.customs)
			return []httpapi.BusinessEndpoint{
				{Pattern: "/customs/external-manifest-references", Handler: customshttp.NewReceiveManifestReferenceEndpoint(intake, wiring.manifests)},
				{Pattern: "/customs/external-manifest-reference-revisions", Handler: customshttp.NewReviseManifestReferenceEndpoint(intake, wiring.manifests)},
			}
		},
	})
}

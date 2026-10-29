package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/platform/refdata/refdatahttp"
)

// 基础数据目录三口（国家 / 地区、币种、计量单位；ADR-0335 决定六）：产品内置、不分租户的只读目录，不属任何限界上下文，
// 所以不经 endpointChannels 里某个上下文的 Intake，认证件由本特性从生产共享的操作者铸造器自建（ownIntake），
// 装配测试里恒为未配置即拒；三口的授予、凭证与租户取自信封的用例在 assemble_reference_data_test.go 对真铸造器另证。

type referenceDataWiring struct {
	reference referenceDataHandlers
}

func init() {
	registerEndpoints(endpointFeature[referenceDataWiring]{
		lines: map[string]endpointLine{
			"/reference-data/countries":  {method: http.MethodGet, face: operationsReadFace(), ownIntake: true},
			"/reference-data/currencies": {method: http.MethodGet, face: operationsReadFace(), ownIntake: true},
			"/reference-data/units":      {method: http.MethodGet, face: operationsReadFace(), ownIntake: true},
		},
		build: func(sources endpointSources) (referenceDataWiring, error) {
			reference, err := buildReferenceData(sources.operatorMinter)
			return referenceDataWiring{reference: reference}, err
		},
		mount: func(_ endpointChannels, wiring referenceDataWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 路径写成字面量：管理台路径门禁只认端点表上的字符串，不认常量引用；与 refdatahttp 的路径常量同一批，
				// 由 assemble_reference_data_test.go 钉住两边一致。
				{Pattern: "/reference-data/countries", Handler: refdatahttp.HandlerOrUnconfigured(wiring.reference.countries)},
				{Pattern: "/reference-data/currencies", Handler: refdatahttp.HandlerOrUnconfigured(wiring.reference.currencies)},
				{Pattern: "/reference-data/units", Handler: refdatahttp.HandlerOrUnconfigured(wiring.reference.units)},
			}
		},
	})
}

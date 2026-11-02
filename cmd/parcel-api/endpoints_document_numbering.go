package main

import (
	"net/http"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber/docnumberhttp"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// 单据编号规则册四口（ADR-0355 决定六）：规则册是编号技术组件的配置，不属任何限界上下文，所以不经 endpointChannels 里
// 某个上下文的 Intake，认证件由本特性从生产共享的操作者铸造器自建（ownIntake），装配测试里恒为未配置即拒；授予、凭证与
// 租户取自信封的用例在 assemble_document_numbering_test.go 对真铸造器另证。预览口不写册、不占号，同试算挂查阅读。

type documentNumberingWiring struct {
	handlers documentNumberingHandlers
}

func init() {
	registerEndpoints(endpointFeature[documentNumberingWiring]{
		lines: map[string]endpointLine{
			"/document-numbering/rules":                   {method: http.MethodGet, face: operationsReadFace(), ownIntake: true},
			"/document-numbering/previews":                {method: http.MethodPost, face: operationsReadFace(), ownIntake: true},
			"/document-numbering/rule-registrations":      {method: http.MethodPost, face: registryWriteFace(), ownIntake: true},
			"/document-numbering/time-zone-registrations": {method: http.MethodPost, face: registryWriteFace(), ownIntake: true},
		},
		build: func(sources endpointSources) (documentNumberingWiring, error) {
			handlers, err := buildDocumentNumberingHandlers(sources.documentNumbers, sources.operatorMinter)
			return documentNumberingWiring{handlers: handlers}, err
		},
		mount: func(_ endpointChannels, wiring documentNumberingWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 路径写成字面量：管理台路径门禁只认端点表上的字符串，不认常量引用；与 docnumberhttp 的路径常量同一批，
				// 由 assemble_document_numbering_test.go 钉住两边一致。
				{Pattern: "/document-numbering/rules", Handler: docnumberhttp.HandlerOrUnconfigured(wiring.handlers.read, http.MethodGet)},
				{Pattern: "/document-numbering/previews", Handler: docnumberhttp.HandlerOrUnconfigured(wiring.handlers.preview, http.MethodPost)},
				{Pattern: "/document-numbering/rule-registrations", Handler: docnumberhttp.HandlerOrUnconfigured(wiring.handlers.ruleRegistration, http.MethodPost)},
				{Pattern: "/document-numbering/time-zone-registrations", Handler: docnumberhttp.HandlerOrUnconfigured(wiring.handlers.timeZoneRegistration, http.MethodPost)},
			}
		},
	})
}

package main

import (
	"net/http"

	identityhttp "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// operatorIdentityWiring 的几只处理器连认证件一起由本特性建：缺席即答未配置（identityhttp 的各 Handler 判 nil）。
type operatorIdentityWiring struct {
	workDevices     http.Handler
	operatorBinding http.Handler
	operatorGrants  http.Handler
}

func init() {
	registerEndpoints(endpointFeature[operatorIdentityWiring]{
		lines: map[string]endpointLine{
			"/work-devices":     {method: http.MethodGet, face: operationsReadFace(), ownIntake: true},
			"/operator-binding": {method: http.MethodGet, face: operationsReadFace(), ownIntake: true},
			"/operator-grants":  {method: http.MethodGet, face: operationsReadFace(), ownIntake: true},
		},
		build: func(sources endpointSources) (operatorIdentityWiring, error) {
			workDevices, err := buildWorkDeviceCatalogue(sources.operatorMinter, sources.deviceRegistry)
			if err != nil {
				return operatorIdentityWiring{}, err
			}
			operatorBinding, err := buildOperatorBinding(sources.operatorMinter)
			if err != nil {
				return operatorIdentityWiring{}, err
			}
			operatorGrants, err := buildOperatorGrants(sources.operatorMinter)
			return operatorIdentityWiring{workDevices: workDevices, operatorBinding: operatorBinding, operatorGrants: operatorGrants}, err
		},
		mount: func(_ endpointChannels, wiring operatorIdentityWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 作业设备册（ADR-0149 决定二、越权风险点 2；票 operator-channel/21）：只查阅已登记的行。
				// 能力面是主数据与运营查阅读，不形成作业事实，所以不挂作业事实那一组 Intake。
				// 路径写成字面量：管理台路径门禁只认端点表上的字符串，不认常量引用。与 WorkDeviceCataloguePath 同一条。
				{Pattern: "/work-devices", Handler: identityhttp.WorkDeviceCatalogueHandler(wiring.workDevices)},
				// 操作者绑定（票 group-legal-entities-uiux/11）：这个会话的操作者在操作者册上绑的那一个租户，管理台顶栏作用域与
				// 集团与法人页头靠它显示当前集团。主数据与运营查阅读守口，认证件同作业设备册。路径写成字面量，理由同上一行。
				{Pattern: "/operator-binding", Handler: identityhttp.OperatorBindingHandler(wiring.operatorBinding)},
				// 授予自查（ADR-0315）：这个会话的操作者此刻持有哪几格能力面授予与哪几种运营决定授予，管理台据此不摆做不了的写入口。
				// 守口与认证件同操作者绑定；路径写成字面量，理由同上。
				{Pattern: "/operator-grants", Handler: identityhttp.OperatorGrantsHandler(wiring.operatorGrants)},
			}
		},
	})
}

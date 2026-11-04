package main

import (
	"errors"
	"net/http"

	identityhttp "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/http"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

// operatorIdentityWiring 的几只处理器连认证件一起由本特性建：缺席即答未配置（identityhttp 的各 Handler 判 nil）。
type operatorIdentityWiring struct {
	workDevices     http.Handler
	operatorBinding http.Handler
	operatorGrants  http.Handler
	// 操作者接入管理面各口（ADR-0363 决定二）：本租户操作者册的查阅读与各写面。
	operators                  http.Handler
	operatorRegistrations      http.Handler
	operatorGrantRegistrations http.Handler
	operatorGrantRevocations   http.Handler
}

func init() {
	registerEndpoints(endpointFeature[operatorIdentityWiring]{
		lines: map[string]endpointLine{
			"/work-devices":                 {method: http.MethodGet, face: operationsReadFace(), ownIntake: true},
			"/operator-binding":             {method: http.MethodGet, face: operationsReadFace(), ownIntake: true},
			"/operator-grants":              {method: http.MethodGet, face: operationsReadFace(), ownIntake: true},
			"/operators":                    {method: http.MethodGet, face: operatorAccessAdministrationFace(), ownIntake: true},
			"/operator-registrations":       {method: http.MethodPost, face: operatorAccessAdministrationFace(), ownIntake: true},
			"/operator-grant-registrations": {method: http.MethodPost, face: operatorAccessAdministrationFace(), ownIntake: true},
			"/operator-grant-revocations":   {method: http.MethodPost, face: operatorAccessAdministrationFace(), ownIntake: true},
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
			if err != nil {
				return operatorIdentityWiring{}, err
			}
			// 操作者册的登记口与查阅口是同一只 Postgres 适配器（CLI 消费的也是它，ADR-0363 决定二）；在这里从库上建，
			// 不另从 sources 取：sources 上那只 operatorRegistry 只露铸造要的装载口。
			if sources.db == nil {
				return operatorIdentityWiring{}, errors.New("operator access administration needs a db")
			}
			operatorBooks, err := aipg.NewOperatorRegistry(sources.db)
			if err != nil {
				return operatorIdentityWiring{}, err
			}
			operators, err := buildOperatorCatalogue(sources.operatorMinter, operatorBooks, sources.operatorIssuer)
			if err != nil {
				return operatorIdentityWiring{}, err
			}
			operatorRegistrations, err := buildOperatorRegistration(sources.operatorMinter, sources.db.Transactor(), operatorBooks)
			if err != nil {
				return operatorIdentityWiring{}, err
			}
			operatorGrantRegistrations, err := buildOperatorGrantRegistration(sources.operatorMinter, sources.db.Transactor(), operatorBooks)
			if err != nil {
				return operatorIdentityWiring{}, err
			}
			operatorGrantRevocations, err := buildOperatorGrantRevocation(sources.operatorMinter, sources.db.Transactor(), operatorBooks, operatorBooks)
			if err != nil {
				return operatorIdentityWiring{}, err
			}
			return operatorIdentityWiring{
				workDevices: workDevices, operatorBinding: operatorBinding, operatorGrants: operatorGrants,
				operators: operators, operatorRegistrations: operatorRegistrations,
				operatorGrantRegistrations: operatorGrantRegistrations, operatorGrantRevocations: operatorGrantRevocations,
			}, nil
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
				// 操作者接入管理面（ADR-0363 决定二）：本租户操作者册的查阅读与三个写面，守口是操作者接入管理这一格，与登记册
				// 配置写分格——能登业务数据的人不因此能给人授权。路径写成字面量，理由同上；与 identityhttp 的各路径常量一一对应。
				{Pattern: "/operators", Handler: identityhttp.OperatorsHandler(wiring.operators)},
				{Pattern: "/operator-registrations", Handler: identityhttp.OperatorRegistrationsHandler(wiring.operatorRegistrations)},
				{Pattern: "/operator-grant-registrations", Handler: identityhttp.OperatorGrantRegistrationsHandler(wiring.operatorGrantRegistrations)},
				{Pattern: "/operator-grant-revocations", Handler: identityhttp.OperatorGrantRevocationsHandler(wiring.operatorGrantRevocations)},
			}
		},
	})
}

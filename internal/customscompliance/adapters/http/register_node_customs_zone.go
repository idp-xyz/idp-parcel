package customshttp

import (
	"context"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

// 节点关务区域登记的在线登记口（ADR-0085 决定一；ADR-0206 决定三）：在线口与登记 CLI 的 node-zone 命令消费同一
// 登记用例，答案代数同为 CaseConfigurationOutcome，端点体因此共用 newConfigurationRegistrationEndpoint。逐类分设
// 接口的判据见本包登记族头注（register_configuration.go）。

// NodeCustomsZoneRegistrationIntake 翻译节点关务区域版本登记。
type NodeCustomsZoneRegistrationIntake interface {
	IntakeNodeCustomsZoneRegistration(
		ctx context.Context,
		request *http.Request,
	) (application.RegisterNodeCustomsZoneCommand, error)
}

// NodeCustomsZoneRegistrar 转交节点关务区域登记编排，事务边界在编排之外给出。
type NodeCustomsZoneRegistrar interface {
	Handle(
		ctx context.Context,
		command application.RegisterNodeCustomsZoneCommand,
	) (application.CaseConfigurationOutcome, error)
}

// NewRegisterNodeCustomsZoneEndpoint 交回节点关务区域版本登记的 HTTP 入口。它登的是「这个租户自某起点把某节点
// 归入某关务区域」，区间约定同口岸目录；准入判断按它判一次移动跨不跨关务区域。
func NewRegisterNodeCustomsZoneEndpoint(
	intake NodeCustomsZoneRegistrationIntake,
	registrar NodeCustomsZoneRegistrar,
) http.Handler {
	return newConfigurationRegistrationEndpoint(
		intake.IntakeNodeCustomsZoneRegistration, registrar.Handle)
}

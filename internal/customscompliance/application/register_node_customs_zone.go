package application

import (
	"context"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 节点关务区域登记册的登记用例（ADR-0206 决定三）。不并入口岸路径那一个 handler：两本都是版本维目录，但口岸
// 与路径伺候路由候选的关务适用性，本册伺候动作准入判断，读的人不同，合进去就得让一边背上另一边的依赖。
// 冲突判定照口岸路径：写口撞键与撞重叠都只答`已登记`，按请求起点读回在册版本逐字段比。事务由进程级入口给出。

// RegisterNodeCustomsZoneCommand 携带一次节点关务区域版本登记。键是（租户，节点，生效起点），内容是区域；终点
// 不是输入——换区域走登记一个更晚起点的新版本。
type RegisterNodeCustomsZoneCommand struct {
	TenantID    domain.TenantID
	Node        domain.NetworkNodeReference
	Zone        domain.CustomsZoneReference
	AppliesFrom time.Time
}

type RegisterNodeCustomsZoneDeps struct {
	Registry ports.NodeCustomsZoneRegistry
	View     ports.NodeCustomsZoneView
}

type RegisterNodeCustomsZoneHandler struct {
	deps RegisterNodeCustomsZoneDeps
}

func NewRegisterNodeCustomsZoneHandler(deps RegisterNodeCustomsZoneDeps) *RegisterNodeCustomsZoneHandler {
	return &RegisterNodeCustomsZoneHandler{deps: deps}
}

// Handle 登记一版节点区域。已在册时按请求起点读回：起点与区域全同是重放；读不回、起点不同或区域不同都是冲突
// ——区域被顶替会让已作出的准入判断失去依据。
func (handler *RegisterNodeCustomsZoneHandler) Handle(
	ctx context.Context,
	command RegisterNodeCustomsZoneCommand,
) (CaseConfigurationOutcome, error) {
	if blankTenant(command.TenantID) ||
		strings.TrimSpace(command.Node.String()) == "" ||
		strings.TrimSpace(command.Zone.String()) == "" ||
		command.AppliesFrom.IsZero() {
		return ConfigurationNotAccepted, nil
	}

	saved, err := handler.deps.Registry.RegisterNodeCustomsZone(
		ctx, command.TenantID, command.Node, command.Zone, command.AppliesFrom)
	if err != nil {
		return ConfigurationUndecided, nil
	}
	if saved == ports.CaseConfigurationRegistered {
		return ConfigurationRegistered, nil
	}

	existing, found, err := handler.deps.View.LoadNodeCustomsZone(
		ctx, command.TenantID, command.Node, command.AppliesFrom)
	if err != nil {
		return ConfigurationUndecided, nil
	}
	if !found || !existing.AppliesFrom.Equal(command.AppliesFrom) || existing.Zone != command.Zone {
		return ConfigurationContentConflict, nil
	}
	return ConfigurationExisting, nil
}

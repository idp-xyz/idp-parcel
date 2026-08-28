package application

import (
	"context"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 口岸目录与申报路径目录两本登记册的登记用例（票 admin-remainder-mechanism-batch/03）。
// 不并入 RegisterCaseConfigurationHandler 那五本：那票按「五类案件配置」收窄执行完毕
// （第六本 case-requirement 另立的同一理由），且这两本是版本维目录，不是案件面配置。
//
// 冲突判定同其余登记册：写口一律 ON CONFLICT DO NOTHING，同键与撞重叠都只答`已登记`；
// 这里按**请求的生效起点**读回在册版本逐字段比。与解释规则用例不同的是读口交回整行
// （含起点），所以「区间也是登记内容的一部分」在这里比得出来：读回版本的起点不等于
// 请求起点，说明撞上的是起点不同的既有区间（错序或追改历史），一律冲突不是重放。
// 事务由进程级入口给出。

// RegisterCandidatePortCommand 携带一次口岸合规候选版本登记。键是（租户，口岸，
// 生效起点），无内容格——目录事实就是「该口岸自该起点是合规候选」；终点不是输入，
// 后继版本登记时落定（换版）。
type RegisterCandidatePortCommand struct {
	TenantID    domain.TenantID
	Port        domain.CustomsPortReference
	AppliesFrom time.Time
}

// RegisterDeclarationPathCommand 携带一次申报路径版本登记。键是（租户，路径，生效
// 起点），内容是路径三维（口岸、方向、申报模式）。
type RegisterDeclarationPathCommand struct {
	TenantID    domain.TenantID
	Path        domain.DeclarationPathReference
	Route       domain.DeclarationPathRoute
	AppliesFrom time.Time
}

// RegisterPortsPathsDeps 写口与读口两半，理由同案件配置五本：冲突判定靠读回。
type RegisterPortsPathsDeps struct {
	Registry ports.PortsPathsRegistry
	View     ports.PortsPathsView
}

type RegisterPortsPathsHandler struct {
	deps RegisterPortsPathsDeps
}

func NewRegisterPortsPathsHandler(deps RegisterPortsPathsDeps) *RegisterPortsPathsHandler {
	return &RegisterPortsPathsHandler{deps: deps}
}

// RegisterCandidatePort 登记一版口岸合规候选。已在册时按请求起点读回：起点相同即
// 重放（口岸册没有别的内容格）；读不回或起点不同即撞上了别的区间——冲突。
func (handler *RegisterPortsPathsHandler) RegisterCandidatePort(
	ctx context.Context,
	command RegisterCandidatePortCommand,
) (CaseConfigurationOutcome, error) {
	if blankTenant(command.TenantID) ||
		strings.TrimSpace(command.Port.String()) == "" ||
		command.AppliesFrom.IsZero() {
		return ConfigurationNotAccepted, nil
	}

	saved, err := handler.deps.Registry.RegisterCandidatePort(
		ctx, command.TenantID, command.Port, command.AppliesFrom)
	if err != nil {
		return ConfigurationUndecided, nil
	}
	if saved == ports.CaseConfigurationRegistered {
		return ConfigurationRegistered, nil
	}

	existing, found, err := handler.deps.View.LoadCandidatePort(
		ctx, command.TenantID, command.Port, command.AppliesFrom)
	if err != nil {
		return ConfigurationUndecided, nil
	}
	if !found || !existing.AppliesFrom.Equal(command.AppliesFrom) {
		return ConfigurationContentConflict, nil
	}
	return ConfigurationExisting, nil
}

// RegisterDeclarationPath 登记一版申报路径。已在册时按请求起点读回逐字段比：起点与
// 三维全同是重放；三维异或起点异都是冲突——路径三维是 NR 候选选择的输入，被顶替会让
// 已作出的选择失去依据，换内容走登记更晚起点的新版本，不走覆盖。
func (handler *RegisterPortsPathsHandler) RegisterDeclarationPath(
	ctx context.Context,
	command RegisterDeclarationPathCommand,
) (CaseConfigurationOutcome, error) {
	if blankTenant(command.TenantID) ||
		strings.TrimSpace(command.Path.String()) == "" ||
		command.AppliesFrom.IsZero() ||
		strings.TrimSpace(command.Route.Port().String()) == "" ||
		command.Route.Direction().String() == "" ||
		strings.TrimSpace(command.Route.Mode().String()) == "" {
		return ConfigurationNotAccepted, nil
	}

	saved, err := handler.deps.Registry.RegisterDeclarationPath(
		ctx, command.TenantID, command.Path, command.Route, command.AppliesFrom)
	if err != nil {
		return ConfigurationUndecided, nil
	}
	if saved == ports.CaseConfigurationRegistered {
		return ConfigurationRegistered, nil
	}

	existing, found, err := handler.deps.View.LoadDeclarationPath(
		ctx, command.TenantID, command.Path, command.AppliesFrom)
	if err != nil {
		return ConfigurationUndecided, nil
	}
	if !found || !existing.AppliesFrom.Equal(command.AppliesFrom) || existing.Route != command.Route {
		return ConfigurationContentConflict, nil
	}
	return ConfigurationExisting, nil
}

package application

import (
	"context"
	"strings"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// EstablishCaseForParcelsCommand 是运营命令面的建案请求（ADR-0227 决定二）：监管范围四维、指名的包裹与初始角色快照。
// 包裹只给身份——客户归属与来源资料引用由本编排按租户查 parcel-shipment 的当前已接受委托取，请求方说不了，也就冒充
// 不了另一个客户的包裹。
type EstablishCaseForParcelsCommand struct {
	TenantID     domain.TenantID
	Jurisdiction domain.RegulatoryJurisdictionReference
	Direction    domain.ManifestDirection
	Procedure    domain.CustomsProcedureReference
	Obligation   domain.ObligationScopeReference
	Parcels      []string
	Roles        []domain.CaseRoleSnapshot
}

type EstablishCaseForParcelsDeps struct {
	Parcels ports.AcceptedParcelSourceView
	Cases   *EstablishCaseHandler
}

// EstablishCaseForParcelsHandler 在建案编排之前补上「包裹须在当前已接受委托里」那一道（UC-CC-001 启动条件），再把补齐了
// 客户归属与来源引用的请求原样交给 EstablishCaseHandler——要不要建案、幂等与范围冲突仍由那一只答，不在这里另判一遍。
type EstablishCaseForParcelsHandler struct {
	deps EstablishCaseForParcelsDeps
}

func NewEstablishCaseForParcelsHandler(deps EstablishCaseForParcelsDeps) *EstablishCaseForParcelsHandler {
	return &EstablishCaseForParcelsHandler{deps: deps}
}

// Handle 逐件查完再作答：不在第一件未接受的包裹处停下，否则请求方改一件、撞下一件，要来回好几趟才知道全部缺口。
// 包裹重复与空清单交给领域构造门（经 EstablishCaseHandler 答未受理），这里只挡空白身份——空白身份拿去查源上下文没有意义。
func (handler *EstablishCaseForParcelsHandler) Handle(
	ctx context.Context,
	command EstablishCaseForParcelsCommand,
) (EstablishCaseResult, error) {
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return EstablishCaseResult{outcome: EstablishCaseNotAccepted}, nil
	}
	associations := make([]domain.CaseParcelAssociation, 0, len(command.Parcels))
	var unaccepted []string
	for _, parcel := range command.Parcels {
		if strings.TrimSpace(parcel) == "" {
			return EstablishCaseResult{outcome: EstablishCaseNotAccepted}, nil
		}
		source, found, err := handler.deps.Parcels.FindAcceptedParcelSource(ctx, command.TenantID, parcel)
		if err != nil {
			return establishUndecided(AcceptedParcelSourceUnavailable), nil
		}
		if !found {
			unaccepted = append(unaccepted, parcel)
			continue
		}
		associations = append(associations, domain.CaseParcelAssociation{
			Parcel:    parcel,
			Customer:  source.Customer,
			SourceRef: source.SourceRef,
		})
	}
	if len(unaccepted) > 0 {
		return EstablishCaseResult{outcome: CaseParcelNotAccepted, unaccepted: unaccepted}, nil
	}
	return handler.deps.Cases.Handle(ctx, EstablishCaseCommand{
		TenantID:     command.TenantID,
		Jurisdiction: command.Jurisdiction,
		Direction:    command.Direction,
		Procedure:    command.Procedure,
		Obligation:   command.Obligation,
		Parcels:      associations,
		Roles:        command.Roles,
	})
}

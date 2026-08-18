package partycommercial

import (
	"context"
	"errors"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

var (
	// ErrAdoptedResolutionTenantMismatch 表示按调用方租户取回的闭包，其解析键却指着另一个
	// 租户。解析标识不是能力凭证（ADR-0003 / ADR-0027）：混进 found=false 会让运维去等一份
	// 其实已经写坏的配置。
	ErrAdoptedResolutionTenantMismatch = errors.New(
		"parcel shipment partycommercial adapter: loaded commercial closure belongs to another tenant")
)

// acceptedShipmentFinder 按来源身份取回委托。真实装配交给仓储；本口只要 Find，不要
// Insert/Save——采用 owner 不写委托。
type acceptedShipmentFinder interface {
	FindBySourceIdentity(
		ctx context.Context,
		identity psdomain.SourceIdentity,
	) (psdomain.ShipmentRequest, bool, error)
}

// ResolvedAdoptedStageOwner 从已接受委托的解析标识回指提供方闭包，取出采用的规则版本
// （ADR-0062）。AdoptedStageOwner 的签名不改：调用方仍只给 SourceIdentity。
//
// 权威在闭包的 AdoptedFor，不在快照 RulePackage 字符串——后者可含 `/`，拆开是有损的。
type ResolvedAdoptedStageOwner struct {
	requests acceptedShipmentFinder
	closures pcports.CommercialResolutionView
}

func NewResolvedAdoptedStageOwner(
	requests acceptedShipmentFinder,
	closures pcports.CommercialResolutionView,
) (*ResolvedAdoptedStageOwner, error) {
	if requests == nil {
		return nil, fmt.Errorf("parcel shipment partycommercial adapter: shipment finder is nil")
	}
	if closures == nil {
		return nil, fmt.Errorf("parcel shipment partycommercial adapter: commercial resolution view is nil")
	}
	return &ResolvedAdoptedStageOwner{requests: requests, closures: closures}, nil
}

var _ AdoptedStageOwner = (*ResolvedAdoptedStageOwner)(nil)

func (owner *ResolvedAdoptedStageOwner) AcceptanceRulePackageFor(
	ctx context.Context,
	identity psdomain.SourceIdentity,
) (pcdomain.CommercialVersion, bool, error) {
	none := pcdomain.CommercialVersion{}
	closure, found, err := owner.loadAcceptedClosure(ctx, identity)
	if err != nil || !found {
		return none, found, err
	}
	adopted, ok := closure.AdoptedFor(pcdomain.AcceptanceRulePackageObject)
	if !ok {
		// 接受流的唯一闭包必须带接单规则包。缺席是装配缺陷，折成未配置会让运维去等一份
		// 不会到来的配置——与 adoptedResolution 同一条精神。
		return none, false, fmt.Errorf("%w: adopted closure carries no acceptance rule package",
			ErrUntranslatableAnswer)
	}
	return adopted.Version(), true, nil
}

func (owner *ResolvedAdoptedStageOwner) AuthorizationRuleFor(
	ctx context.Context,
	identity psdomain.SourceIdentity,
) (pcdomain.CommercialVersion, bool, error) {
	none := pcdomain.CommercialVersion{}
	closure, found, err := owner.loadAcceptedClosure(ctx, identity)
	if err != nil || !found {
		return none, found, err
	}
	adopted, ok := closure.AdoptedFor(pcdomain.AuthorizationRuleObject)
	if !ok {
		// 今天必需依据由消费方键源决定，闭包未列授权规则是真话，不发明一版。
		return none, false, nil
	}
	return adopted.Version(), true, nil
}

// loadAcceptedClosure 按身份取回已接受决定上的解析标识，再向提供方重取闭包。
// found=false 只用于「采用版本尚未固定或提供方还没有这份闭包」；读失败与租户不一致上抛。
func (owner *ResolvedAdoptedStageOwner) loadAcceptedClosure(
	ctx context.Context,
	identity psdomain.SourceIdentity,
) (pcdomain.CommercialClosure, bool, error) {
	none := pcdomain.CommercialClosure{}
	request, found, err := owner.requests.FindBySourceIdentity(ctx, identity)
	if err != nil {
		return none, false, fmt.Errorf("adopted stage owner: load shipment request: %w", err)
	}
	if !found || request.State() != psdomain.ShipmentRequestAccepted {
		return none, false, nil
	}
	decision, present := request.AcceptanceDecision()
	if !present {
		return none, false, nil
	}

	tenant, err := pcdomain.NewTenantID(identity.TenantID().String())
	if err != nil {
		return none, false, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	resolution, err := pcdomain.NewResolutionID(decision.Basis().ResolutionID().String())
	if err != nil {
		return none, false, fmt.Errorf("%w: resolution ID: %v", ErrUntranslatableAnswer, err)
	}

	closure, found, err := owner.closures.LoadResolution(ctx, tenant, resolution)
	if err != nil {
		return none, false, fmt.Errorf("adopted stage owner: load commercial resolution: %w", err)
	}
	if !found {
		return none, false, nil
	}
	if closure.ResolutionKey().TenantID != tenant {
		return none, false, fmt.Errorf("%w: identity %q closure %q",
			ErrAdoptedResolutionTenantMismatch, tenant, closure.ResolutionKey().TenantID)
	}
	return closure, true, nil
}

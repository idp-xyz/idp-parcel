package parcelshipment

import (
	"context"
	"errors"
	"fmt"
	"strings"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ParcelIdentityView 经 PS 的外部标识关联册核对现场观察到的号码（ADR-0184 决定五）。它只翻译：哪些包裹
// 此刻被这个号码标识，由 PS 的读口答；候选数怎么分走向（关联、待识别、身份冲突）由收寄编排判。
type ParcelIdentityView struct {
	parcels psports.ParcelExternalIdentifierView
}

var _ noports.ParcelIdentityView = (*ParcelIdentityView)(nil)

// NewParcelIdentityView 构造期拒缺件：漏装的读口到第一次明确接收才暴露，会被当成一次依赖故障。
func NewParcelIdentityView(parcels psports.ParcelExternalIdentifierView) (*ParcelIdentityView, error) {
	if parcels == nil {
		return nil, errors.New("node operations parcelshipment adapter: identity view needs a parcel external identifier view")
	}
	return &ParcelIdentityView{parcels: parcels}, nil
}

// ResolveParcelIdentity 交回此刻被这个号码标识的正式包裹。号码原样交给 PS 逐字比对，这里不折算；观察里一个号码
// 都没有（空或全空白）答零个候选——没有线索的实物就是待识别，不是依赖故障。PS 读不动原样交回，编排据此不形成答案、
// 不落册（ADR-0186）：拿空候选顶替会把可能已知的包裹落成待识别。
func (view *ParcelIdentityView) ResolveParcelIdentity(
	ctx context.Context,
	tenant nodomain.TenantID,
	observation noports.ExternalMarkObservation,
) ([]nodomain.ParcelAssociationReference, error) {
	if strings.TrimSpace(observation.Mark) == "" {
		return nil, nil
	}
	psTenant, err := psdomain.NewTenantID(tenant.String())
	if err != nil {
		return nil, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	number, err := psdomain.NewExternalIdentifierNumber(observation.Mark)
	if err != nil {
		return nil, fmt.Errorf("%w: mark: %v", ErrUntranslatableAnswer, err)
	}
	parcels, err := view.parcels.FindParcelsByExternalIdentifier(ctx, psTenant, number)
	if err != nil {
		return nil, fmt.Errorf("node operations parcelshipment adapter: find parcels by external identifier: %w", err)
	}
	candidates := make([]nodomain.ParcelAssociationReference, 0, len(parcels))
	for _, parcel := range parcels {
		candidate, err := nodomain.NewParcelAssociationReference(parcel.String())
		if err != nil {
			return nil, fmt.Errorf("%w: parcel: %v", ErrUntranslatableAnswer, err)
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

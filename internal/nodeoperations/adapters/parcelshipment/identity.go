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

// ParcelIdentityView 经 PS 的外部标识关联册核对现场观察到的号码（ADR-0184 决定五、ADR-0195）。它只翻译：哪些包裹
// 此刻被这个号码标识、号码是否另被一份`已提交`委托报过，由 PS 的读口答；答复怎么分走向（关联、待识别、身份冲突）由
// 收寄编排判。
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

// ResolveParcelIdentity 交回此刻被这个号码标识的正式包裹，与 PS 答出的歧义信号。号码原样交给 PS 逐字比对，这里不
// 折算；观察里一个号码都没有（空或全空白）答零个候选——没有线索的实物就是待识别，不是依赖故障；号码带 NUL，不可能在册，
// 同样答零个候选、不去问 PS。PS 读不动原样交回，
// 编排据此不形成答案、不落册（ADR-0186）：拿空候选顶替会把可能已知的包裹落成待识别。
func (view *ParcelIdentityView) ResolveParcelIdentity(
	ctx context.Context,
	tenant nodomain.TenantID,
	observation noports.ExternalMarkObservation,
) (noports.ParcelIdentityAnswer, error) {
	if strings.TrimSpace(observation.Mark) == "" {
		return noports.ParcelIdentityAnswer{}, nil
	}
	psTenant, err := psdomain.NewTenantID(tenant.String())
	if err != nil {
		return noports.ParcelIdentityAnswer{}, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	// 构造核对用的号，不走入册门：入册门只管写，读的一侧不拿它挡已在册的号。门立起来之前进册的超长号，现场扫到要核得到；
	// 册里没有的超长号，PS 照常答空（票 ps-external-mark-relations/06 裁定补）。
	number, err := psdomain.NewExternalIdentifierLookupNumber(observation.Mark)
	if errors.Is(err, psdomain.ErrExternalIdentifierNumberNotStorable) {
		// 只剩带 NUL 的号走到这里：Postgres 的 text 存不下它，它确定不在册，答零个候选是真话。拿去问反倒在库面报错，收寄会
		// 读成身份读不动，落未核对的待识别、响重核门铃，而重核拿着同一个号永远续不上。
		return noports.ParcelIdentityAnswer{}, nil
	}
	if err != nil {
		return noports.ParcelIdentityAnswer{}, fmt.Errorf("%w: mark: %v", ErrUntranslatableAnswer, err)
	}
	found, err := view.parcels.FindParcelsByExternalIdentifier(ctx, psTenant, number)
	if err != nil {
		return noports.ParcelIdentityAnswer{}, fmt.Errorf("node operations parcelshipment adapter: find parcels by external identifier: %w", err)
	}
	answer := noports.ParcelIdentityAnswer{
		Candidates: make([]nodomain.ParcelAssociationReference, 0, len(found.Parcels)),
		Ambiguous:  found.DeclaredBySubmittedRequest,
	}
	for _, parcel := range found.Parcels {
		candidate, err := nodomain.NewParcelAssociationReference(parcel.String())
		if err != nil {
			return noports.ParcelIdentityAnswer{}, fmt.Errorf("%w: parcel: %v", ErrUntranslatableAnswer, err)
		}
		answer.Candidates = append(answer.Candidates, candidate)
	}
	return answer, nil
}

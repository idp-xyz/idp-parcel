// Package parcelshipment 是 customs-compliance 读 parcel-shipment 的消费侧适配器（`internal/<consumer>/adapters/<provider>/`，
// ADR-0025）：跨上下文翻译只落在这里，`application` 不 import 提供方。本包只读提供方的只读口，不写它的库。
package parcelshipment

import (
	"context"
	"errors"
	"fmt"

	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// AcceptedParcelSources 实现 ccports.AcceptedParcelSourceView：按（租户，包裹）问提供方的当前已接受委托反查口，把那份委托的
// 客户账户译成案件关联的客户归属，把（委托，当前提交版本）译成来源资料引用——客户原始资料快照的家在那份提交版本上，引用指到
// 它即可，不复制内容。
type AcceptedParcelSources struct {
	view psports.CurrentAcceptedParcelTargetView
}

func NewAcceptedParcelSources(view psports.CurrentAcceptedParcelTargetView) (*AcceptedParcelSources, error) {
	if view == nil {
		return nil, errors.New("customs compliance parcelshipment adapter: accepted parcel target view is nil")
	}
	return &AcceptedParcelSources{view: view}, nil
}

var _ ccports.AcceptedParcelSourceView = (*AcceptedParcelSources)(nil)

// FindAcceptedParcelSource 只译不判。提供方答「两份已接受委托都声明了这件包裹」是它自己的坏状态（ErrAmbiguousParcelTarget），
// 原样作 error 交回，不在这里挑一份：挑哪份都是在替源上下文消歧。包裹身份在提供方词汇里构造不出来（空白），答「不在任何已接受
// 委托里」——那件包裹本就不可能被接受过。
func (sources *AcceptedParcelSources) FindAcceptedParcelSource(
	ctx context.Context,
	tenant ccdomain.TenantID,
	parcel string,
) (ccports.AcceptedParcelSource, bool, error) {
	providerTenant, err := psdomain.NewTenantID(tenant.String())
	if err != nil {
		return ccports.AcceptedParcelSource{}, false, fmt.Errorf("customs compliance parcelshipment adapter: tenant: %w", err)
	}
	providerParcel, err := psdomain.NewDeclaredParcelID(parcel)
	if err != nil {
		return ccports.AcceptedParcelSource{}, false, nil
	}
	target, found, err := sources.view.FindCurrentAcceptedByParcel(ctx, providerTenant, providerParcel)
	if err != nil {
		return ccports.AcceptedParcelSource{}, false, fmt.Errorf("customs compliance parcelshipment adapter: %w", err)
	}
	if !found {
		return ccports.AcceptedParcelSource{}, false, nil
	}
	return ccports.AcceptedParcelSource{
		Customer:  target.Identity().CustomerAccountID().String(),
		SourceRef: target.ShipmentRequestID().String() + "/" + target.SubmissionVersion().String(),
	}, true, nil
}

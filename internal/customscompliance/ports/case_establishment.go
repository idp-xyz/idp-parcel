package ports

import (
	"context"
	"errors"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// AcceptedParcelSource 是一件包裹在 parcel-shipment 当前已接受委托里的客户归属与来源资料引用：建案只引用这两样，不复制
// 也不覆盖源事实（UC-CC-001「包裹与来源资料」一组）。
type AcceptedParcelSource struct {
	Customer  string
	SourceRef string
}

// AcceptedParcelSourceView 按（租户，包裹）查当前已接受委托（ADR-0227 决定二）。found=false 是「这件包裹不在任何当前已接受
// 委托里」——只有提交版本、尚未接受的范围不得建正式案件（UC-CC-001 启动条件），所以它是建案的业务前提不成立，不是依赖故障。
// 读不动走 error；源上下文答不出唯一一份（同一包裹被多份已接受委托声明）交出 ErrAcceptedParcelAmbiguous。
type AcceptedParcelSourceView interface {
	FindAcceptedParcelSource(
		ctx context.Context,
		tenant domain.TenantID,
		parcel string,
	) (AcceptedParcelSource, bool, error)
}

// ErrAcceptedParcelAmbiguous 说源上下文里同一件包裹被不止一份当前已接受委托声明：那是源上下文自己的坏状态，重投多少次都是
// 同一份坏，本上下文也不能挑一份替它消歧。编排认出它就整笔报错，不答读不动的未决——未决的续办是等依赖恢复后重试，对它无效。
var ErrAcceptedParcelAmbiguous = errors.New("customs compliance: more than one current accepted request declares the parcel")

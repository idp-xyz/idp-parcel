package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// AcceptedParcelSource 是一件包裹在 parcel-shipment 当前已接受委托里的客户归属与来源资料引用：建案只引用这两样，不复制
// 也不覆盖源事实（UC-CC-001「包裹与来源资料」一组）。
type AcceptedParcelSource struct {
	Customer  string
	SourceRef string
}

// AcceptedParcelSourceView 按（租户，包裹）查当前已接受委托（ADR-0227 决定一）。found=false 是「这件包裹不在任何当前已接受
// 委托里」——只有提交版本、尚未接受的范围不得建正式案件（UC-CC-001 启动条件），所以它是建案的业务前提不成立，不是依赖故障。
// 读不动、或源上下文答不出唯一一份（同一包裹被多份已接受委托声明）都走 error：前者是故障，后者是源上下文的坏状态，二者都
// 不是本上下文能替它消歧的。
type AcceptedParcelSourceView interface {
	FindAcceptedParcelSource(
		ctx context.Context,
		tenant domain.TenantID,
		parcel string,
	) (AcceptedParcelSource, bool, error)
}

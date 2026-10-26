package transportfulfillment

import (
	"context"
	"errors"
	"fmt"
	"time"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

var (
	// ErrConsolidationUnitNotVisible 表示问节点作业「载运对象那一刻是不是集运单元」读不动。说不出是不是单元，就不能拿去反查
	// 委托（那是把一袋当包裹找），也不能当成「不是单元」放行；读得动之后重投，从头再问一遍。续办格。
	ErrConsolidationUnitNotVisible = errors.New(
		"parcel shipment transportfulfillment adapter: the consolidation unit book cannot be read")
	// ErrConsolidationUnitAnswerInconsistent 表示节点作业答了，答复却立不成本口的形状（租户译不过去、答复自相矛盾）。重问还是
	// 同一个结果，要查节点作业的册或两边的口径，不是等那边恢复（ADR-0029）。禁止进 WithUndecidedSentinels：登成未决，这一封
	// 会一路重投到失败预算耗尽，而现场要查的是册。
	ErrConsolidationUnitAnswerInconsistent = errors.New(
		"parcel shipment transportfulfillment adapter: the consolidation unit answer is inconsistent")
)

// carriedObjectIsConsolidationUnit 先于任何按包裹反查，问载运对象在来源事实发生那一刻是不是集运单元。
//
// 运输履约的载运对象引用不带种类。集运单元不是包裹，没有委托，也就不会有已接受委托：把单元串拿去反查包裹，反查必不中，而
// 「委托可能还没落到已接受」的未决格会让这样的信封一路重投。判据只是节点作业的单元册（同串的包裹号按单元认，ADR-0214 决定二、
// ADR-0225 决定四），不按串形或前缀猜；不是单元的照旧反查，反查不中仍是未决。
//
// 是单元的各路收工而不采用、不判终局：成员是否随单元被采用另由单元控制规则定，此刻不投到成员上。失败分两格：读不动是续办，答了却
// 译不回是确定性的（见两个哨兵）。
func carriedObjectIsConsolidationUnit(
	ctx context.Context,
	units psports.ConsolidationUnitView,
	tenant psdomain.TenantID,
	parcel psdomain.DeclaredParcelID,
	at time.Time,
) (bool, error) {
	unit, err := units.IsConsolidationUnitAt(ctx, tenant, parcel, at)
	if err != nil {
		if errors.Is(err, psports.ErrConsolidationUnitAnswerUntranslatable) {
			return false, fmt.Errorf("%w: object %q: %v", ErrConsolidationUnitAnswerInconsistent, parcel, err)
		}
		return false, fmt.Errorf("%w: object %q: %v", ErrConsolidationUnitNotVisible, parcel, err)
	}
	return unit, nil
}

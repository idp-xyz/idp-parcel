package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// OccupationKind 指名转出册上一行转的是哪本账里的占用：冻结账本还是暴露账本。两本账互不借用（ADR-0047），
// 同一控制请求在组合策略下两本都可能有占用，各转各的，所以种类是转出册键的一维。
type OccupationKind uint8

const (
	OccupationKindInvalid OccupationKind = iota
	OccupationFreeze
	OccupationExposure
)

func (kind OccupationKind) String() string {
	switch kind {
	case OccupationFreeze:
		return "FREEZE"
	case OccupationExposure:
		return "EXPOSURE"
	default:
		return ""
	}
}

// ParcelShare 是受理前估价留痕里一件包裹在某个控制请求下的份额：那件包裹的逐件试算合计。一次控制占用的金额是各件
// 试算合计之和（ADR-0179 决定二），所以各件份额之和等于该次占用的原金额。
type ParcelShare struct {
	RequestID  domain.ControlRequestID
	ShareMinor int64
}

// PreAcceptanceConversionBook 是费用确认把接受前占用转正式扣费时要读写的那几格：费用属于哪件包裹、那件包裹在受理前
// 估价留痕里挂在哪几个控制请求下、哪些份额已转出过。读写都在调用方已经打开的事务里（锁与账本回写同一笔）。
type PreAcceptanceConversionBook interface {
	// ParcelOfCharge 按形成费用的 SELL 评价请求回找包裹。找不到（费用不是包裹计费时点那一路形成的）不是错误。
	ParcelOfCharge(ctx context.Context, tenant domain.TenantID, charge domain.CustomerChargeID) (parcel string, found bool, err error)

	// ParcelShares 交回本作用域受理前估价留痕里这件包裹的份额，每个控制请求一行。留痕按（作用域，控制请求）存，
	// 包裹是留痕里逐件试算的一维；被取代版本的请求也在其中，是否仍占着由账本状态回答，不由这里筛。
	ParcelShares(ctx context.Context, tenant domain.TenantID, scope domain.SettlementScope, parcel string) ([]ParcelShare, error)

	// RecordConversion 记一份转出。同（占用，包裹，种类）已记过交回 false——重放认得出自己；金额与触发它的费用以先到者为准，
	// 不覆盖。
	RecordConversion(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.SettlementScope,
		requestID domain.ControlRequestID,
		parcel string,
		kind OccupationKind,
		minor int64,
		charge domain.CustomerChargeID,
		convertedAt time.Time,
	) (recorded bool, err error)
}

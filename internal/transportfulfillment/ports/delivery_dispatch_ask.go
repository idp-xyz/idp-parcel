package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// DeliveryDispatchAskKind 是一次按下的种类：补拍对单个（段，对象），按册重问对派送待形成册的一批。
type DeliveryDispatchAskKind uint8

const (
	DeliveryDispatchAskKindInvalid DeliveryDispatchAskKind = iota
	DeliveryDispatchAskTrigger
	DeliveryDispatchAskReask
)

func (kind DeliveryDispatchAskKind) String() string {
	switch kind {
	case DeliveryDispatchAskTrigger:
		return "TRIGGER"
	case DeliveryDispatchAskReask:
		return "REASK"
	default:
		return ""
	}
}

// DeliveryDispatchAskRecord 是经运营决定口按下一次补拍或按册重问留下的东西（ADR-0303 决定五与复核记录）：是哪一位、什么时候、
// 问的是哪一件或哪一批。它是审计，不是待办台账：不进派送待形成册的状态，也不进任务；执行器怎么答、任务形成没有，各在各的册上。
//
// 补拍带 Segment 与 Object、不带 Scope 与 Limit；按册重问反之。AskedAt 是这一次按下的业务时间（补拍取请求里报的时刻，
// 重问取重问那一刻）。
type DeliveryDispatchAskRecord struct {
	TenantID domain.TenantID
	Kind     DeliveryDispatchAskKind
	AskedBy  domain.RegistrantReference
	AskedAt  time.Time
	Segment  domain.FulfillmentSegmentReference
	Object   domain.CarriedObjectReference
	Scope    string
	Limit    int
}

// DeliveryDispatchAskLedger 是问询册：只增不改，一次按下一行，不去重——同一件被两位操作者各补拍一次是两次按下。
// RecordAsk 要环境事务：补拍与执行器那一拍同笔（那一拍回滚，这一次按下也跟着回滚，因为它没有留下任何效果）；按册重问另起一笔，
// 先于逐件去问，免得一件问不动让整次按下无处可查。
type DeliveryDispatchAskLedger interface {
	RecordAsk(ctx context.Context, record DeliveryDispatchAskRecord) error
	// ListAsks 由近及远列一个租户的按下记录，供审计查阅。
	ListAsks(ctx context.Context, tenant domain.TenantID, limit int) ([]DeliveryDispatchAskRecord, error)
}

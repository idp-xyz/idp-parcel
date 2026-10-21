package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 承运答复在核过答复方之前的两口（ADR-0239）。对外，「订舱不在册」与「答复方不是该订舱所属委托的服务提供方」答同一格；对内，
// 每一次拒收凭什么拒记进拒收册，审计与运营分得清两种情形。

// BookingWithProvider 是一次订舱连同它所属委托的服务提供方。CommissionRegistered 为假时 Provider 是零值：订舱只在委托在册时
// 成立、委托又不删，读不回委托是库里的数据违约，由编排答没形成答案。
type BookingWithProvider struct {
	Booking              BookingRecord
	Provider             domain.ServiceProviderReference
	CommissionRegistered bool
}

// BookingWithProviderReader 一次读回订舱与它的服务提供方。
//
// 另立一口、一次读回，而不让编排先读订舱再读委托：两条拒收路必须做一样多的事。分两次读时，订舱在册的那一条比不在册的多一次读，
// 响应时刻就把合并成一格的两种情形又分开了；读不动时答的未决原因也会一条是订舱册、一条是委托册。另立而不在 BookingStore 上加方法，
// 理由同 BookingCancellationStore。
type BookingWithProviderReader interface {
	FindBookingWithProvider(ctx context.Context, key BookingKey) (BookingWithProvider, bool, error)
}

// BookingAnswerRefusalBasis 是一份承运答复被拒收的依据。两种依据对外答同一格，只进拒收册。
type BookingAnswerRefusalBasis uint8

const (
	BookingAnswerRefusalBasisInvalid BookingAnswerRefusalBasis = iota
	// RefusedBookingNotRegistered：载荷指名的订舱在本租户下不在册（他租户名下的同名订舱同落这一格）。
	RefusedBookingNotRegistered
	// RefusedSourceNotServiceProvider：订舱在册，答复方的来源身份不是它所属委托的服务提供方。
	RefusedSourceNotServiceProvider
)

func (basis BookingAnswerRefusalBasis) String() string {
	switch basis {
	case RefusedBookingNotRegistered:
		return "BOOKING_NOT_REGISTERED"
	case RefusedSourceNotServiceProvider:
		return "SOURCE_NOT_SERVICE_PROVIDER"
	default:
		return ""
	}
}

// BookingAnswerRefusal 是一次拒收：哪个答复方、对哪个订舱的哪一份答复、凭什么拒、什么时候。Source 是集成客户端绑定的来源身份；
// RefusedAt 是本上下文记下它的时刻，不是答复方给的答复时刻——被拒的报文里那个时刻没有被收下。
type BookingAnswerRefusal struct {
	Key        BookingKey
	Source     string
	Acceptance domain.CarrierAcceptanceReference
	Basis      BookingAnswerRefusalBasis
	RefusedAt  time.Time
}

// BookingAnswerRefusalLedger 是承运答复拒收册的写口，与那一次答复同笔。
type BookingAnswerRefusalLedger interface {
	// RecordRefusal 记下一次拒收。同一（租户、答复方、订舱、答复标识、依据）再来一次不另起一行，只加次数、推后最近一次时刻。
	RecordRefusal(ctx context.Context, refusal BookingAnswerRefusal) error
}

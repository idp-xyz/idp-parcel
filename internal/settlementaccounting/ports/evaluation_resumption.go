package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// EvaluationRequestStopEffect 是在 BUY 请求册上记一个停因的落点。
type EvaluationRequestStopEffect uint8

const (
	EvaluationRequestStopEffectInvalid EvaluationRequestStopEffect = iota
	// EvaluationRequestStopped：请求原本没有停因，这一个写下了。
	EvaluationRequestStopped
	// EvaluationRequestStopReplayed：册上已是同一个停因，没有再写。
	EvaluationRequestStopReplayed
	// EvaluationRequestStoppedOtherwise：册上已是另一个停因，不覆盖——一份请求只停一次，换个停因就是两侧说法矛盾。
	EvaluationRequestStoppedOtherwise
)

func (effect EvaluationRequestStopEffect) String() string {
	switch effect {
	case EvaluationRequestStopped:
		return "STOPPED"
	case EvaluationRequestStopReplayed:
		return "REPLAYED"
	case EvaluationRequestStoppedOtherwise:
		return "STOPPED_OTHERWISE"
	default:
		return ""
	}
}

// EvaluationRequestStopRecorder 是 BUY 请求册的停因写口（ADR-0229 决定三）。BUY 请求册没有状态格：形成了的请求由派生的
// 预期成本版本标识可查，停下的请求在这里记停因。只从「没有停因」往外走，撞上已有停因按内容分重放与另有停因，都不
// 覆盖。请求不在册上是错误，不是一种落点——编排只在读到了这份请求之后才来记。stoppedAt 是记下停因的时刻，续办队列
// 按它定先后。要求环境事务。
type EvaluationRequestStopRecorder interface {
	RecordStop(
		ctx context.Context,
		key EvaluationRequestKey,
		cause domain.EvaluationStopCause,
		stoppedAt time.Time,
	) (EvaluationRequestStopEffect, error)
}

// EvaluationRequestSuccessorView 按前一代找 BUY 请求的下一代。另立接口而不扩 EvaluationRequestView：扩既有接口会拆
// 全部替身，而只有续办要问下一代。
type EvaluationRequestSuccessorView interface {
	FindSuccessor(ctx context.Context, key EvaluationRequestKey) (EvaluationRequestRecord, bool, error)
}

// SellEvaluationRequestSuccessorView 按前一代找 SELL 请求的下一代，理由同 EvaluationRequestSuccessorView。
type SellEvaluationRequestSuccessorView interface {
	FindSuccessor(ctx context.Context, key SellEvaluationRequestKey) (SellEvaluationRequestRecord, bool, error)
}

// SellEvaluationInitiationStallRegister 是 SELL 那一路的发起停办册（ADR-0229 决定四）。按（租户、包裹、计费时点）一行：
// Record 首次落行，同一自然键再停一次就换上这一次的停因、时刻与依据——它是一张等待表，不是历史：停过几次、凭什么
// 重跑过，看依据与时刻；这件包裹在这一时点有了请求就不再等着，那一行留作来路。
//
// Close 给一行落终止：续办重跑时这一户答`采用了别的时点`，这一拍本就不计费，不是停下，也不再等任何登记。终止的行不再
// 进队列，再停一次（Record）就重新等着。不在册上的键答 false。都要求环境事务。
type SellEvaluationInitiationStallRegister interface {
	RecordSellInitiationStall(ctx context.Context, tenant domain.TenantID, stall domain.SellEvaluationInitiationStall) error
	FindSellInitiationStall(
		ctx context.Context,
		tenant domain.TenantID,
		key domain.SellEvaluationRequestNaturalKey,
	) (domain.SellEvaluationInitiationStall, bool, error)
	CloseSellInitiationStall(
		ctx context.Context,
		tenant domain.TenantID,
		key domain.SellEvaluationRequestNaturalKey,
		closedAt time.Time,
	) (bool, error)
}

// BuyEvaluationInitiationStallRegister 是 BUY 那一路的发起停办册，按（租户、发生项、有效性版本）一行，写法同 SELL 那一本。
type BuyEvaluationInitiationStallRegister interface {
	RecordBuyInitiationStall(ctx context.Context, tenant domain.TenantID, stall domain.BuyEvaluationInitiationStall) error
	FindBuyInitiationStall(
		ctx context.Context,
		tenant domain.TenantID,
		occurrence domain.ChargeOccurrenceID,
		version domain.OccurrenceVersion,
	) (domain.BuyEvaluationInitiationStall, bool, error)
}

// StalledEvaluationKind 是一件等着续办的东西在哪一本册上。
type StalledEvaluationKind uint8

const (
	StalledEvaluationKindInvalid StalledEvaluationKind = iota
	StalledSellInitiation
	StalledBuyInitiation
	StalledSellRequest
	StalledBuyRequest
)

func (kind StalledEvaluationKind) String() string {
	switch kind {
	case StalledSellInitiation:
		return "SELL_INITIATION"
	case StalledBuyInitiation:
		return "BUY_INITIATION"
	case StalledSellRequest:
		return "SELL_REQUEST"
	case StalledBuyRequest:
		return "BUY_REQUEST"
	default:
		return ""
	}
}

// StalledEvaluation 是等着续办的一件：只带回读它的键与当时的停因，续办在自己的事务里按键再读一次再判。按 Kind 只有
// 一组键有值。Position 是它在队列里的位置，下一页从它之后取。
type StalledEvaluation struct {
	Kind              StalledEvaluationKind
	Cause             domain.EvaluationStopCause
	SellMoment        domain.SellEvaluationRequestNaturalKey
	Occurrence        domain.ChargeOccurrenceID
	OccurrenceVersion domain.OccurrenceVersion
	SellRequest       domain.SellEvaluationRequestID
	BuyRequest        domain.EvaluationRequestID
	Position          StalledEvaluationPosition
}

// StalledEvaluationPosition 是队列的定序键：停下时刻、种类与两段键。按它翻页而不是每页重读队首：办过却没有写回的件
// （这一户采用了别的时点、重跑被拒、撞上矛盾）仍满足「等着」的口径，每页重读队首会被它们占满、把后面等着的挡住。
type StalledEvaluationPosition struct {
	StoppedAt time.Time
	Kind      string
	FirstKey  string
	SecondKey string
}

// Present 答位置在不在场：零值表示从队首取。
func (position StalledEvaluationPosition) Present() bool { return position.Kind != "" }

// StalledEvaluationQuery 是取等待队列的条件。
//
//   - Causes 必填：只取停在这几种停因上的。一次参数登记交的是它解得开的那几种，运营续办交全集。
//   - ExcludeBasis 可缺：这条代次链上任一代凭这个依据续办过、或发起停办凭它重跑过的不再取——同一封信重投、同一条运营
//     命令重跑都不再推这条链。
//   - After 可缺：只取排在这个位置之后的，调用方按页往后翻。
//   - PricingScope 可缺：只取问的是这个计价范围的请求。计价的登记带它（卡带自己的范围，序列与目录带绑了它的卡的
//     范围），免得一次登记把别的范围上等着的请求全推一遍；发起停办没有计价范围，带了它就不取发起停办。
//   - Direction 可缺：只取这一路的。计价的登记带它，BUY 卡不推 SELL 请求。
//   - OccurrenceReason 可缺：只取 BUY 那一路里发生项原因是它的。BUY 触发册登记带它。
//   - Account 可缺：只取 SELL 那一路里落在这一户的；发起停办只有停在计费时点没采用的那几行记了户。SELL 计费时点采用
//     登记带它。
//   - Parcel 与 Occurrence（连同 OccurrenceVersion）可缺：运营续办按一件包裹或一版发生项点名时用，两样都带就什么都不取。
//
// 收窄的几格互相之间是「且」：带了只属一路的格（OccurrenceReason 只属 BUY、Account 只属 SELL），另一路整路不取。
type StalledEvaluationQuery struct {
	Causes            []domain.EvaluationStopCause
	ExcludeBasis      domain.ResumeBasis
	PricingScope      string
	Direction         domain.EvaluationDirection
	OccurrenceReason  domain.OccurrenceReasonReference
	Account           domain.SettlementAccountID
	Parcel            domain.ParcelReference
	Occurrence        domain.ChargeOccurrenceID
	OccurrenceVersion domain.OccurrenceVersion
	After             StalledEvaluationPosition
}

// ErrStalledEvaluationQueryInvalid 说明取队列的条件立不住（停因为空、点名的发生项缺版本）。是调用方的缺陷，不是依赖故障。
var ErrStalledEvaluationQueryInvalid = errors.New("settlement accounting: invalid stalled evaluation query")

// StalledEvaluationQueue 交回一个租户里等着续办的东西，四本册合起来按停下的时刻先后、同刻按种类与键定序，至多 limit 件。
// 「等着」的口径（ADR-0229 决定五）：发起停办——没有落终止，且这件包裹在这一时点（或这一版发生项）还没有任何一份请求；
// 请求——带停因、还没有下一代。调用方按 After 往后翻页，取到不足一页即到底。
type StalledEvaluationQueue interface {
	ListStalled(ctx context.Context, tenant domain.TenantID, query StalledEvaluationQuery, limit int) ([]StalledEvaluation, error)
}

// EvaluationParameterRegisteredIntent 是结算的一本评价参数册落了一行之后要通知续办的最小事实（ADR-0229 决定五）：租户、
// 种类、落的是哪一行（Subject：账户、计价费用代码或发生项原因，只进信封标识），以及登记时刻。
//
// 收窄格随种类必备、进载荷，续办门按它只取这一行解得开的件：BUY 触发册带 OccurrenceReason，SELL 计费时点采用带
// Account；别的种类两格都空。缺了必备的那一格，交接口拒收。
type EvaluationParameterRegisteredIntent struct {
	Tenant           domain.TenantID
	Kind             domain.EvaluationParameterKind
	Subject          string
	OccurrenceReason domain.OccurrenceReasonReference
	Account          domain.SettlementAccountID
	RegisteredAt     time.Time
}

// ErrEvaluationParameterHandoffRejected 表示「评价参数已登记」信封被框架的确定性校验拒收。同一份登记重交永远同一个结果，
// 登记口据此整笔回滚、交出硬失败，登记行不会脱离信封独自留下。
var ErrEvaluationParameterHandoffRejected = errors.New("settlement accounting: evaluation parameter registered envelope rejected by envelope validation")

// EvaluationParameterRegisteredHandoff 把「评价参数已登记」写入 Outbox，与登记行同一事务（ADR-0094 决定四：续办的触发与
// 登记同笔落地）。只收结算自己那几本册的种类；计价的登记由计价自己交。
type EvaluationParameterRegisteredHandoff interface {
	HandOffEvaluationParameterRegistered(ctx context.Context, intent EvaluationParameterRegisteredIntent) error
}

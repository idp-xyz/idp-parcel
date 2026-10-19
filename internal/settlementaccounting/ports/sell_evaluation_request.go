package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// SellEvaluationRequestKey 是 SELL 评价请求册的身份键：租户 + 铸造 ID（ADR-0190 决定一）。
type SellEvaluationRequestKey struct {
	TenantID domain.TenantID
	Request  domain.SellEvaluationRequestID
}

// SellEvaluationRequestRecord 是一次「发起 SELL 评价」越过提交边界留下的东西。RecordedAt 是编排时钟给的登记时刻，
// 随记录往返，也是信封的发生时刻。
//
// State 与 FormedCharge 是册上的状态格：登记只收`已提交`、不带费用标识；改记其余各格归形成那一半。读口原样交回，
// 按包裹找当前有效的预估客户费用看的就是这一对——FormedCharge 只在`已形成`时非空。
type SellEvaluationRequestRecord struct {
	Key          SellEvaluationRequestKey
	Request      domain.SellEvaluationRequest
	RecordedAt   time.Time
	State        domain.SellEvaluationRequestState
	FormedCharge domain.CustomerChargeID
}

// SellEvaluationRequestRegistry 是 SELL 评价请求的登记面。Save 的写入代数与 BUY 那本同一条（ADR-0031），所以共用
// EvaluationRequestSaveOutcome：撞身份键或撞自然键（租户、包裹、计费时点）唯一约束都答`已存在`、不覆盖先到者；
// 幂等由库上的唯一约束守。FindByNaturalKey 让编排在撞上时读回先到者、交回原 ID，而不是答失败。Save 收到状态不是
// `已提交`或带了费用标识的记录即拒——那是把形成那一半的结论冒充成登记。
type SellEvaluationRequestRegistry interface {
	Save(ctx context.Context, record SellEvaluationRequestRecord) (EvaluationRequestSaveOutcome, error)
	FindByNaturalKey(
		ctx context.Context,
		tenant domain.TenantID,
		key domain.SellEvaluationRequestNaturalKey,
	) (SellEvaluationRequestRecord, bool, error)
}

// SellEvaluationRequestView 是请求册的只读半边。计价收到信封后按 ID 回读请求，SELL 消费门按评价回指的 ID 读回包裹与
// 结算账户——走这里，拿不到 Save。FindCurrentByParcel 答一件包裹眼下的那份请求：自然键容得下同一件包裹在不同时点
// 各有一份，而一户只采用一种时点，所以通常至多一份；有多份时交回登记最晚的那份，同刻再按 ID 定序，让同一组行
// 永远答同一份。与登记面同一个适配器实现，行模型只有一份。
type SellEvaluationRequestView interface {
	FindByID(ctx context.Context, key SellEvaluationRequestKey) (SellEvaluationRequestRecord, bool, error)
	FindCurrentByParcel(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.ParcelReference,
	) (SellEvaluationRequestRecord, bool, error)
}

// SellEvaluationRequestIdentityFactory 为新请求签发铸造 ID。
type SellEvaluationRequestIdentityFactory interface {
	MintSellEvaluationRequestID(ctx context.Context) (domain.SellEvaluationRequestID, error)
}

// SellEvaluationRequestIntent 把一份已登记的 SELL 评价请求交给 parcel-pricing。载荷只带（租户、请求 ID），请求内容
// 只有登记册一处权威，消费方按 ID 读 SellEvaluationRequestView。重放重发同一份（ADR-0043）。
type SellEvaluationRequestIntent struct {
	Record SellEvaluationRequestRecord
}

// ErrSellEvaluationRequestHandoffRejected 表示交接口铸出的信封被框架的确定性校验拒收（租户或包裹标识超长时作用域或
// 分区键过不了上限）。同一份请求重交永远同一个结果，消费方据此入账不重投，而不是按依赖故障重试到预算耗尽。
var ErrSellEvaluationRequestHandoffRejected = errors.New("settlement accounting: sell evaluation request envelope rejected by envelope validation")

// SellEvaluationRequestHandoff 把 SELL 评价请求写入 Outbox，与请求行同一事务。信封被确定性拒收时交回的错误
// errors.Is ErrSellEvaluationRequestHandoffRejected；别的失败都是依赖不可用。
type SellEvaluationRequestHandoff interface {
	HandOffSellEvaluationRequest(ctx context.Context, intent SellEvaluationRequestIntent) error
}

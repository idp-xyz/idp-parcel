package psinbox

import (
	"encoding/json"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// pricingParameterResumeConsumerName 是计价「参数已登记」续办门在 inbox 键上的稳定名，与另几份登记续办门各占一个名（理由见
// OperatorRegistrationCompletedConsumer 的类型注释）。settlement-accounting 也收同一类信封、续办它停着的评价，那一路记在它
// 自己的名下：inbox 键是（消费者名、来源、事件标识），同一封信两路各记一行。
const pricingParameterResumeConsumerName = "parcel-shipment/advance-acceptance-chain-on-pricing-parameter-registration"

// PricingParameterRegisteredEventType 是本门认的事件类型：parcel-pricing 登记价卡、收尾价卡、序列与目录复核通过时与登记同一
// 事务交出（ADR-0229 决定五）。它要解开的是接受前财务控制停在价卡那一侧的等待（ADR-0380 决定一、二）。发布侧是
// parcel-pricing 的 Outbox 交接适配器，两边各写各的字面；两串相等由 cmd/parcel-dispatch 用发布侧真交接口铸封的用例钉住。
const PricingParameterRegisteredEventType eventing.EventType = "parcel-pricing.pricing-parameter.registered"

// NewPricingParameterRegisteredConsumer 装配收计价「参数已登记」的那一份登记续办门。重驱纪律与另几份完全相同：按租户取回
// `等待运营登记`的队列逐份再驱一拍，不认是哪一张卡、哪一个计价范围——链重跑到估价那一步自会去选卡，这一张对不上这份委托就
// 再停一次（ADR-0380 决定二）。按原因或计价范围收窄另立（ADR-0380 越权风险点 2）。
func NewPricingParameterRegisteredConsumer(
	transactor bentoapp.Transactor,
	store *inbox.Store,
	queue ports.OperatorRegistrationQueue,
	advancer AcceptanceChainAdvancer,
	pageSize int,
) (*OperatorRegistrationCompletedConsumer, error) {
	return newRegistrationResumeConsumer(
		pricingParameterResumeConsumerName,
		PricingParameterRegisteredEventType,
		"advance acceptance chain on pricing parameter registration",
		decodePricingParameterRegistered,
		transactor, store, queue, advancer, pageSize,
	)
}

// decodePricingParameterRegistered 译载荷：tenantId、registrationKind、pricingScope、direction 四键，本门只读租户与方向。
// 租户立不起来即毒丸。种类只核在场，理由同 decodeOperatorRegistrationCompleted：它是发布侧契约的一半，缺了说明载荷形状漂了，
// 却不据它分派。方向是本门唯一据以分派的格，只认 BUY 与 SELL：SELL 重驱，BUY 入账不重驱；别的取值或缺席说明契约漂了，
// 重投长不出对的方向，是毒丸——把它当成 BUY 放过，一次 SELL 登记就会静默地叫不醒任何委托。计价范围不读：多唤醒的再问
// 一次、照旧停，无害。
func decodePricingParameterRegistered(payload []byte) (operatorRegistrationCompleted, error) {
	var body struct {
		TenantID         string `json:"tenantId"`
		RegistrationKind string `json:"registrationKind"`
		Direction        string `json:"direction"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return operatorRegistrationCompleted{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	tenant, err := domain.NewTenantID(body.TenantID)
	if err != nil {
		return operatorRegistrationCompleted{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.RegistrationKind == "" {
		return operatorRegistrationCompleted{}, fmt.Errorf("%w: missing registration kind", ErrPoisonEnvelope)
	}
	// 方向词是计价载荷的原词。受理前估价只按 SELL·客户费用价卡试算（ADR-0179 决定一），BUY 那一侧的登记解不开任何一份在等
	// 估价的委托。
	switch body.Direction {
	case "SELL":
		return operatorRegistrationCompleted{tenant: tenant}, nil
	case "BUY":
		return operatorRegistrationCompleted{tenant: tenant, passOver: true}, nil
	default:
		return operatorRegistrationCompleted{}, fmt.Errorf("%w: pricing direction %q is neither BUY nor SELL", ErrPoisonEnvelope, body.Direction)
	}
}

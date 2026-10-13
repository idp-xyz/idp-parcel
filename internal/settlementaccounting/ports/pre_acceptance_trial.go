package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// PreAcceptanceTrialQuery 是结算向计价要一次受理前试算（ADR-0179 决定一、四）。委托与提交版本是 parcel-shipment 的标识原样，本上下文
// 不解释；计价范围取采用行；币种取本次控制作用域；At 是控制回显的判断时点。
type PreAcceptanceTrialQuery struct {
	Tenant            domain.TenantID
	PricingScope      domain.EstimatePricingScope
	ShipmentRequest   string
	SubmissionVersion string
	At                time.Time
	Currency          domain.CurrencyCode
}

// PreAcceptanceTrialOutcome 是试算那一侧的封闭答复，按计价侧的格原样分开（选卡三格、不可见、缺件），本上下文全都答未形成。
type PreAcceptanceTrialOutcome uint8

const (
	PreAcceptanceTrialOutcomeInvalid PreAcceptanceTrialOutcome = iota
	// PreAcceptanceTrialsFormed：每一件都有一份试算评价，Trials 与声明同序；评价有没有合计逐份看。
	PreAcceptanceTrialsFormed
	// PreAcceptanceTrialsIncomplete：至少一件缺测量或缺这张卡要的那一格。
	PreAcceptanceTrialsIncomplete
	PreAcceptanceTrialPriceCardNotConfigured
	PreAcceptanceTrialPriceCardConflict
	PreAcceptanceTrialSubmissionNotVisible
)

func (outcome PreAcceptanceTrialOutcome) String() string {
	switch outcome {
	case PreAcceptanceTrialsFormed:
		return "FORMED"
	case PreAcceptanceTrialsIncomplete:
		return "INCOMPLETE"
	case PreAcceptanceTrialPriceCardNotConfigured:
		return "PRICE_CARD_NOT_CONFIGURED"
	case PreAcceptanceTrialPriceCardConflict:
		return "PRICE_CARD_CONFLICT"
	case PreAcceptanceTrialSubmissionNotVisible:
		return "SUBMISSION_NOT_VISIBLE"
	default:
		return ""
	}
}

// PreAcceptanceTrialAnswer 是一次试算的答复。PriceCard 是所用价卡版本引用的字面，选到卡之后的各格都有值；Trials 只在已形成时有值。
type PreAcceptanceTrialAnswer struct {
	Outcome   PreAcceptanceTrialOutcome
	PriceCard string
	Trials    []domain.PreAcceptanceTrial
}

// ErrPreAcceptanceTrialsUnavailable 表示试算那一侧读不回、形成与否未知。施加编排据此停在待判断等重试，不读成未配置。
var ErrPreAcceptanceTrialsUnavailable = errors.New("settlement accounting: pre-acceptance trials unavailable")

// PreAcceptanceTrialSource 是结算消费计价受理前试算的口（ADR-0179 决定一）：试算只算不存、不交付，本上下文拿它合计、留痕，不据它
// 形成任何费用。实现方是 adapters/parcelpricing。读不回交回的错误 errors.Is ErrPreAcceptanceTrialsUnavailable。
type PreAcceptanceTrialSource interface {
	FormPreAcceptanceTrials(ctx context.Context, query PreAcceptanceTrialQuery) (PreAcceptanceTrialAnswer, error)
}

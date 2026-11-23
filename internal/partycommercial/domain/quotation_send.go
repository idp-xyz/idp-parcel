package domain

import (
	"strings"
	"time"
	"unicode"
)

// 发出记录就是报价。它归本上下文，只增不改：相对方、有效期与条款在发出时才写下，
// 没有发出之前就存在的报价单，也没有报价自己的状态列。待答复、已过期、已接受是读时
// 才能派生的结论，不在这条记录上。送达同样不在这里记——导出文件不是客户已经收到。

// ErrQuotationNotAccepted 是有效期或条款立不住。缺终点不当成永久有效。
var ErrQuotationNotAccepted = errQuotation("not accepted")

// ErrInvalidQuotationSend 是一条发出记录的其余事实立不住。
var ErrInvalidQuotationSend = errQuotation("invalid send")

type quotationError string

func errQuotation(text string) error { return quotationError(text) }

func (err quotationError) Error() string { return "party commercial: quotation " + string(err) }

// QuotationPriceLane 是所引价格内容的两条路。零值无效。
type QuotationPriceLane uint8

const (
	QuotationPriceLaneInvalid QuotationPriceLane = iota
	// QuotationPriceFromApprovedDraft 指名一份已批准的销售价卡草稿。
	QuotationPriceFromApprovedDraft
	// QuotationPriceFromRegisteredCard 指名一版已登记的销售价卡。
	QuotationPriceFromRegisteredCard
)

func (lane QuotationPriceLane) String() string {
	switch lane {
	case QuotationPriceFromApprovedDraft:
		return "APPROVED_DRAFT"
	case QuotationPriceFromRegisteredCard:
		return "REGISTERED_PRICE_CARD"
	default:
		return ""
	}
}

// QuotationPriceLaneNamed 按名字认一条路。
func QuotationPriceLaneNamed(name string) (QuotationPriceLane, bool) {
	switch name {
	case QuotationPriceFromApprovedDraft.String():
		return QuotationPriceFromApprovedDraft, true
	case QuotationPriceFromRegisteredCard.String():
		return QuotationPriceFromRegisteredCard, true
	default:
		return QuotationPriceLaneInvalid, false
	}
}

// QuotationSendMethod 是发出方式。本票只有导出工作簿。
type QuotationSendMethod uint8

const (
	QuotationSendMethodInvalid QuotationSendMethod = iota
	// QuotationSendByWorkbookExport 把对客价格表导出成交给客户的文件。导出不是送达。
	QuotationSendByWorkbookExport
)

func (method QuotationSendMethod) String() string {
	switch method {
	case QuotationSendByWorkbookExport:
		return "WORKBOOK_EXPORT"
	default:
		return ""
	}
}

// QuotationID 是一条发出记录的标识。
type QuotationID struct{ requiredValue }

func NewQuotationID(value string) (QuotationID, error) {
	required, err := newRequiredValue("quotation id", value)
	return QuotationID{required}, err
}

// QuotationValidity 是报价的有效期。起点与终点都必须在，终点晚于起点。
// 判法与 EffectiveInterval.Contains 相同：起点算在内、终点算在外。
// 不复用允许开放结束的 EffectiveInterval，缺终点在这里就是未受理。
type QuotationValidity struct {
	startsAt time.Time
	endsAt   time.Time
}

func NewQuotationValidity(startsAt, endsAt time.Time) (QuotationValidity, error) {
	if startsAt.IsZero() || endsAt.IsZero() || !endsAt.After(startsAt) {
		return QuotationValidity{}, ErrQuotationNotAccepted
	}
	return QuotationValidity{startsAt: startsAt.UTC(), endsAt: endsAt.UTC()}, nil
}

func (interval QuotationValidity) StartsAt() time.Time { return interval.startsAt }

func (interval QuotationValidity) EndsAt() time.Time { return interval.endsAt }

func (interval QuotationValidity) Contains(at time.Time) bool {
	if interval.startsAt.IsZero() || at.Before(interval.startsAt) {
		return false
	}
	return at.Before(interval.endsAt)
}

func (interval QuotationValidity) valid() bool {
	return !interval.startsAt.IsZero() && interval.endsAt.After(interval.startsAt)
}

// QuotationTerms 是一段自由文本。去首尾空白后可以为空，表示没有另加条款。
// 非空时不含控制字符。不做结构化条款。
type QuotationTerms struct {
	text string
}

func NewQuotationTerms(text string) (QuotationTerms, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed != "" && strings.IndexFunc(trimmed, unicode.IsControl) >= 0 {
		return QuotationTerms{}, ErrQuotationNotAccepted
	}
	return QuotationTerms{text: trimmed}, nil
}

func (terms QuotationTerms) Text() string { return terms.text }

// QuotationSendSpec 是立一条发出记录要的全部事实。发出人与时刻由调用方交服务端已经取定的值，
// 不在这里读时钟或信封。
type QuotationSendSpec struct {
	ID               QuotationID
	Tenant           TenantID
	Lane             QuotationPriceLane
	PlanID           string
	PlanVersion      string
	Canonicalization string
	ContentDigest    string
	Counterparty     PartyID
	CounterpartyName PartyName
	LegalEntity      LegalEntityReference
	LegalEntityName  PartyName
	StartsAt         time.Time
	EndsAt           time.Time
	Terms            string
	Sender           OperatorSubjectReference
	SentAt           time.Time
	Method           QuotationSendMethod
}

// QuotationSend 是一条已经发出的报价。值类型，没有改写方法。
type QuotationSend struct {
	id               QuotationID
	tenant           TenantID
	lane             QuotationPriceLane
	planID           string
	planVersion      string
	canonicalization string
	contentDigest    string
	counterparty     PartyID
	counterpartyName PartyName
	legalEntity      LegalEntityReference
	legalEntityName  PartyName
	validity         QuotationValidity
	terms            QuotationTerms
	sender           OperatorSubjectReference
	sentAt           time.Time
	method           QuotationSendMethod
}

func NewQuotationSend(spec QuotationSendSpec) (QuotationSend, error) {
	validity, err := NewQuotationValidity(spec.StartsAt, spec.EndsAt)
	if err != nil {
		return QuotationSend{}, err
	}
	terms, err := NewQuotationTerms(spec.Terms)
	if err != nil {
		return QuotationSend{}, err
	}
	send := QuotationSend{
		id:               spec.ID,
		tenant:           spec.Tenant,
		lane:             spec.Lane,
		planID:           strings.TrimSpace(spec.PlanID),
		planVersion:      strings.TrimSpace(spec.PlanVersion),
		canonicalization: strings.TrimSpace(spec.Canonicalization),
		contentDigest:    strings.TrimSpace(spec.ContentDigest),
		counterparty:     spec.Counterparty,
		counterpartyName: spec.CounterpartyName,
		legalEntity:      spec.LegalEntity,
		legalEntityName:  spec.LegalEntityName,
		validity:         validity,
		terms:            terms,
		sender:           spec.Sender,
		sentAt:           spec.SentAt.UTC(),
		method:           spec.Method,
	}
	if !send.valid() {
		return QuotationSend{}, ErrInvalidQuotationSend
	}
	return send, nil
}

func (send QuotationSend) valid() bool {
	return send.id.valid() && send.tenant.valid() && send.lane.String() != "" &&
		send.planID != "" && send.planID == strings.TrimSpace(send.planID) &&
		send.planVersion != "" && send.planVersion == strings.TrimSpace(send.planVersion) &&
		send.canonicalization != "" && send.contentDigest != "" &&
		send.counterparty.valid() && send.counterpartyName.valid() &&
		send.legalEntity.valid() && send.legalEntityName.valid() &&
		send.validity.valid() && send.sender.valid() && !send.sentAt.IsZero() &&
		send.method.String() != ""
}

func (send QuotationSend) ID() QuotationID                   { return send.id }
func (send QuotationSend) Tenant() TenantID                  { return send.tenant }
func (send QuotationSend) Lane() QuotationPriceLane          { return send.lane }
func (send QuotationSend) PlanID() string                    { return send.planID }
func (send QuotationSend) PlanVersion() string               { return send.planVersion }
func (send QuotationSend) Canonicalization() string          { return send.canonicalization }
func (send QuotationSend) ContentDigest() string             { return send.contentDigest }
func (send QuotationSend) Counterparty() PartyID             { return send.counterparty }
func (send QuotationSend) CounterpartyName() PartyName       { return send.counterpartyName }
func (send QuotationSend) LegalEntity() LegalEntityReference { return send.legalEntity }
func (send QuotationSend) LegalEntityName() PartyName        { return send.legalEntityName }
func (send QuotationSend) Validity() QuotationValidity       { return send.validity }
func (send QuotationSend) Terms() QuotationTerms             { return send.terms }
func (send QuotationSend) Sender() OperatorSubjectReference  { return send.sender }
func (send QuotationSend) SentAt() time.Time                 { return send.sentAt }
func (send QuotationSend) Method() QuotationSendMethod       { return send.method }

// DeliveryRecorded 始终为假。发出记录不记送达，导出也不把这一格改成真。
func (send QuotationSend) DeliveryRecorded() bool { return false }

// SameFact 用于同一标识再交来时判断是不是同一条发出，不比较之外的东西。
func (send QuotationSend) SameFact(other QuotationSend) bool {
	return send.id == other.id && send.tenant == other.tenant && send.lane == other.lane &&
		send.planID == other.planID && send.planVersion == other.planVersion &&
		send.canonicalization == other.canonicalization && send.contentDigest == other.contentDigest &&
		send.counterparty == other.counterparty && send.counterpartyName == other.counterpartyName &&
		send.legalEntity == other.legalEntity && send.legalEntityName == other.legalEntityName &&
		send.validity.startsAt.Equal(other.validity.startsAt) && send.validity.endsAt.Equal(other.validity.endsAt) &&
		send.terms == other.terms && send.sender == other.sender && send.sentAt.Equal(other.sentAt) &&
		send.method == other.method
}

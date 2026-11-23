package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 一条发出的读法。不另存状态，也不改发出册或答复册。
// 已接受之后草稿才撤下，仍读已接受，并告诉所引草稿此刻已撤下。
// 读不动发出册或价格草稿册是错误，不是业务拒绝。已登记的价卡没有撤下这一格，不为此去读价格册。
// 一条被接受不改其余发出的读法：每条各自取自己的答复和所引草稿。

// QuotationReadingDetail 是一条发出加上派生出来的读法。Found 为假时没有 Send。
type QuotationReadingDetail struct {
	Found                    bool
	Send                     domain.QuotationSend
	DeliveryNote             string
	Status                   domain.QuotationReadingStatus
	Conclusion               string
	ReferencedDraftWithdrawn bool
	Reply                    domain.QuotationReply
	HasReply                 bool
	Effectiveness            QuotationEffectiveness
}

// QuotationPublicationRule 是租户登记的那道门，与这一条报价自己是不是有效接受分开。
// 没登记不设门，读面答未配置，不把「还没带依据」写成规则本身。
type QuotationPublicationRule uint8

const (
	QuotationPublicationRuleInvalid QuotationPublicationRule = iota
	QuotationPublicationRuleUnconfigured
	QuotationPublicationRuleRequired
	QuotationPublicationRuleNotRequired
)

func (rule QuotationPublicationRule) String() string {
	switch rule {
	case QuotationPublicationRuleUnconfigured:
		return "UNCONFIGURED"
	case QuotationPublicationRuleRequired:
		return "REQUIRED"
	case QuotationPublicationRuleNotRequired:
		return "NOT_REQUIRED"
	default:
		return ""
	}
}

// QuotationEffectiveness 是这一条发出离「这位客户用上这套价」还差什么。不另存。
// 三格都成立才是已生效：所引价格内容已在册、这一版此刻仍可被选中、这位客户此刻适用的合同指名了这个范围。
// 合同还没指名时，下一步是发布指名这个范围的新一版合同。不在这里发布新合同，也不造合同正文。
type QuotationEffectiveness struct {
	PriceOnRegister bool
	SelectsNow      bool
	Scope           string
	ScopeNamed      bool
	Requirement     QuotationPublicationRule
	ValidAcceptance bool
	RecordedBasis   bool
	NextStep        string
}

// ReadQuotationHandler 读一条发出并派生状态。now 是服务端此刻。
type ReadQuotationHandler struct {
	sends       ports.QuotationSendStore
	replies     ports.QuotationReplyStore
	prices      ports.QuotedPriceReader
	publication ports.QuotationPublicationView
	registered  ports.RegisteredPriceReader
}

func NewReadQuotationHandler(
	sends ports.QuotationSendStore,
	replies ports.QuotationReplyStore,
	prices ports.QuotedPriceReader,
	publication ports.QuotationPublicationView,
	registered ports.RegisteredPriceReader,
) (*ReadQuotationHandler, error) {
	if sends == nil || replies == nil || prices == nil {
		return nil, fmt.Errorf("party commercial: quotation reading needs the send store, the reply store and the price reader")
	}
	return &ReadQuotationHandler{
		sends: sends, replies: replies, prices: prices, publication: publication, registered: registered,
	}, nil
}

func (handler *ReadQuotationHandler) Get(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.QuotationID,
	now time.Time,
) (QuotationReadingDetail, error) {
	if tenant.String() == "" || id.String() == "" || now.IsZero() {
		return QuotationReadingDetail{}, fmt.Errorf("party commercial: quotation reading needs a tenant, an id and the server time")
	}
	send, found, err := handler.sends.GetQuotationSend(ctx, tenant, id)
	if err != nil {
		return QuotationReadingDetail{}, err
	}
	if !found {
		return QuotationReadingDetail{DeliveryNote: quotationDeliveryNote}, nil
	}
	reply, hasReply, err := handler.replies.GetQuotationReply(ctx, tenant, id)
	if err != nil {
		return QuotationReadingDetail{}, err
	}
	draftWithdrawn := false
	if send.Lane() == domain.QuotationPriceFromApprovedDraft && (!hasReply || reply.Decision() == domain.QuotationReplyAccepted) {
		withdrawn, judgeErr := quotedDraftWithdrawn(ctx, handler.prices, send)
		if judgeErr != nil {
			return QuotationReadingDetail{}, judgeErr
		}
		draftWithdrawn = withdrawn
	}
	reading, err := domain.ReadQuotation(send, reply, hasReply, draftWithdrawn, now)
	if err != nil {
		return QuotationReadingDetail{}, err
	}
	effectiveness, err := handler.effectiveness(ctx, send, draftWithdrawn, now)
	if err != nil {
		return QuotationReadingDetail{}, err
	}
	return QuotationReadingDetail{
		Found: true, Send: send, DeliveryNote: quotationDeliveryNote,
		Status: reading.Status, Conclusion: quotationReadingConclusion(reading),
		ReferencedDraftWithdrawn: reading.ReferencedDraftWithdrawn,
		Reply:                    reply, HasReply: hasReply,
		Effectiveness: effectiveness,
	}, nil
}

func (handler *ReadQuotationHandler) effectiveness(ctx context.Context, send domain.QuotationSend, draftWithdrawn bool, now time.Time) (QuotationEffectiveness, error) {
	if handler.publication == nil {
		return QuotationEffectiveness{}, nil
	}
	fact, err := handler.prices.ReadQuotedPrice(ctx, ports.QuotedPriceQuery{
		Tenant: send.Tenant(), Lane: send.Lane(), PlanID: send.PlanID(), PlanVersion: send.PlanVersion(),
	})
	if err != nil {
		return QuotationEffectiveness{}, err
	}
	onRegister := fact.Standing == ports.QuotedPriceUseRegisteredCard ||
		(send.Lane() == domain.QuotationPriceFromRegisteredCard && fact.Standing == ports.QuotedPriceReady)
	selects := false
	recorded := false
	if handler.registered != nil {
		card, cardErr := handler.registered.ReadRegisteredPrice(ctx, send.Tenant(), send.PlanID(), send.PlanVersion(), now)
		if cardErr != nil {
			return QuotationEffectiveness{}, cardErr
		}
		onRegister = card.OnRegister
		selects = card.SelectsNow
		recorded = card.HasPublicationBasis && card.PublicationBasis == send.ID().String()
	}
	named, err := handler.publication.ContractNamesPricingScope(ctx, send.Tenant(), send.Counterparty().String(), fact.Scope, now)
	if err != nil {
		return QuotationEffectiveness{}, err
	}
	requires, configured, err := handler.publication.LoadQuotationPublicationRequirement(ctx, send.Tenant())
	if err != nil {
		return QuotationEffectiveness{}, err
	}
	requirement := QuotationPublicationRuleUnconfigured
	switch {
	case !configured:
		requirement = QuotationPublicationRuleUnconfigured
	case requires:
		requirement = QuotationPublicationRuleRequired
	default:
		requirement = QuotationPublicationRuleNotRequired
	}
	digest, accepted, found, err := handler.publication.AcceptedQuotationReply(ctx, send.Tenant(), send.ID().String())
	if err != nil {
		return QuotationEffectiveness{}, err
	}
	next := "PUBLISH_CONTRACT"
	switch {
	case draftWithdrawn && !onRegister:
		next = "CANNOT_PUBLISH"
	case !named:
		next = "PUBLISH_CONTRACT"
	case !onRegister:
		next = "PUBLISH_CARD"
	case !selects:
		next = "NOT_SELECTABLE"
	default:
		next = "ALREADY_EFFECTIVE"
	}
	return QuotationEffectiveness{
		PriceOnRegister: onRegister, SelectsNow: selects, Scope: fact.Scope, ScopeNamed: named,
		Requirement: requirement, ValidAcceptance: found && accepted && digest == send.ContentDigest(),
		RecordedBasis: recorded, NextStep: next,
	}, nil
}

func quotedDraftWithdrawn(ctx context.Context, prices ports.QuotedPriceReader, send domain.QuotationSend) (bool, error) {
	fact, err := prices.ReadQuotedPrice(ctx, ports.QuotedPriceQuery{
		Tenant: send.Tenant(), Lane: send.Lane(), PlanID: send.PlanID(), PlanVersion: send.PlanVersion(),
	})
	if err != nil {
		return false, err
	}
	if !fact.Found || fact.Standing.String() == "" {
		return false, fmt.Errorf("party commercial: quoted draft cannot be judged for withdrawal")
	}
	return fact.Standing == ports.QuotedPriceWithdrawn, nil
}

func quotationReadingConclusion(reading domain.QuotationReading) string {
	switch reading.Status {
	case domain.QuotationReadingPending:
		return "待答复"
	case domain.QuotationReadingAccepted:
		if reading.ReferencedDraftWithdrawn {
			return "已接受。所引草稿此刻已撤下。"
		}
		return "已接受"
	case domain.QuotationReadingRejected:
		return "已拒绝"
	case domain.QuotationReadingExpired:
		return "已过期"
	case domain.QuotationReadingPriceWithdrawn:
		return "所引价格已撤下"
	default:
		return ""
	}
}

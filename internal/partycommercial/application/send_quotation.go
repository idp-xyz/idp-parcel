package application

import (
	"context"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/identity"
)

// 发出一条报价。价格内容经消费侧读口来，相对方与责任法人经身份册来。
// 发出人与时刻由调用方交已经取定的信封和时钟，载荷里的自报值到不了这里。
// 读不动价格册或身份册是错误，不是业务拒绝。

const quotationSendPrefix = "QTS"

// SendQuotationOutcome 是发出的业务结论。SENT 才落一行。
type SendQuotationOutcome uint8

const (
	SendQuotationOutcomeInvalid SendQuotationOutcome = iota
	QuotationSent
	QuotationSendReplay
	QuotationSendConflict
	QuotationSendNotRegistered
	QuotationSendNotSell
	QuotationSendContentNotFixed
	QuotationSendWithdrawn
	QuotationSendAlreadyRegistered
	QuotationSendContentChanged
	QuotationSendCounterpartyNotRegistered
	QuotationSendCounterpartyNotYetEffective
	QuotationSendCounterpartyDeactivated
	QuotationSendLegalEntityNotRegistered
	QuotationSendLegalEntityNotYetEffective
	QuotationSendLegalEntityDeactivated
	QuotationSendNotAccepted
	QuotationSendNotRepresentable
)

func (outcome SendQuotationOutcome) String() string {
	switch outcome {
	case QuotationSent:
		return "SENT"
	case QuotationSendReplay:
		return "ALREADY_RECORDED"
	case QuotationSendConflict:
		return "CONTENT_CONFLICT"
	case QuotationSendNotRegistered:
		return "NOT_REGISTERED"
	case QuotationSendNotSell:
		return "NOT_SELL"
	case QuotationSendContentNotFixed:
		return "CONTENT_NOT_FIXED"
	case QuotationSendWithdrawn:
		return "WITHDRAWN"
	case QuotationSendAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case QuotationSendContentChanged:
		return "CONTENT_CHANGED"
	case QuotationSendCounterpartyNotRegistered:
		return "COUNTERPARTY_NOT_REGISTERED"
	case QuotationSendCounterpartyNotYetEffective:
		return "COUNTERPARTY_NOT_YET_EFFECTIVE"
	case QuotationSendCounterpartyDeactivated:
		return "COUNTERPARTY_DEACTIVATED"
	case QuotationSendLegalEntityNotRegistered:
		return "LEGAL_ENTITY_NOT_REGISTERED"
	case QuotationSendLegalEntityNotYetEffective:
		return "LEGAL_ENTITY_NOT_YET_EFFECTIVE"
	case QuotationSendLegalEntityDeactivated:
		return "LEGAL_ENTITY_DEACTIVATED"
	case QuotationSendNotAccepted:
		return "NOT_ACCEPTED"
	case QuotationSendNotRepresentable:
		return "NOT_REPRESENTABLE"
	default:
		return ""
	}
}

// SendQuotationCommand 是一次发出。ID 空着由服务端签发；种子可以带一个已经取定的标识。
type SendQuotationCommand struct {
	ID            domain.QuotationID
	Tenant        domain.TenantID
	Sender        domain.OperatorSubjectReference
	SentAt        time.Time
	Lane          domain.QuotationPriceLane
	PlanID        string
	PlanVersion   string
	ContentDigest string
	Counterparty  domain.PartyID
	LegalEntity   domain.LegalEntityReference
	StartsAt      time.Time
	EndsAt        time.Time
	Terms         string
}

// SendQuotationResult 是发出的答案。Workbook 只在 SENT 与重放时在场，供当次下载。
// 重放不再插一行。DeliveryRecorded 在有记录时为假：送达不在这里记。
type SendQuotationResult struct {
	Outcome          SendQuotationOutcome
	Conclusion       string
	Send             domain.QuotationSend
	Workbook         []byte
	MediaType        string
	FileName         string
	DeliveryRecorded bool
}

type quotationIdentityBook interface {
	LoadLatestBusinessParty(ctx context.Context, tenant domain.TenantID, party domain.PartyID) (domain.BusinessPartyRegistration, bool, error)
	LoadLatestLegalEntity(ctx context.Context, tenant domain.TenantID, entity domain.LegalEntityReference) (domain.LegalEntityRegistration, bool, error)
}

// SendQuotationHandler 编排价格内容、身份与发出记录。
type SendQuotationHandler struct {
	prices     ports.QuotedPriceReader
	identities quotationIdentityBook
	store      ports.QuotationSendStore
	ids        identity.Minter
}

func NewSendQuotationHandler(
	prices ports.QuotedPriceReader,
	identities quotationIdentityBook,
	store ports.QuotationSendStore,
) (*SendQuotationHandler, error) {
	if prices == nil || identities == nil || store == nil {
		return nil, fmt.Errorf("party commercial: quotation send needs the price reader, the identity book and the send store")
	}
	minter, err := identity.NewMinter(quotationSendPrefix)
	if err != nil {
		return nil, err
	}
	return &SendQuotationHandler{prices: prices, identities: identities, store: store, ids: minter}, nil
}

func (handler *SendQuotationHandler) Handle(ctx context.Context, command SendQuotationCommand) (SendQuotationResult, error) {
	var none SendQuotationResult
	if command.Tenant.String() == "" || command.Sender.String() == "" || command.SentAt.IsZero() ||
		command.Lane.String() == "" || command.PlanID == "" || command.PlanVersion == "" || command.ContentDigest == "" ||
		command.Counterparty.String() == "" || command.LegalEntity.String() == "" {
		return none, fmt.Errorf("party commercial: quotation send needs tenant, sender, time, lane, plan, digest, counterparty and legal entity")
	}
	if _, err := domain.NewQuotationValidity(command.StartsAt, command.EndsAt); err != nil {
		return refused(QuotationSendNotAccepted, "有效期未受理：起点和终点都要有，终点晚于起点"), nil
	}
	if _, err := domain.NewQuotationTerms(command.Terms); err != nil {
		return refused(QuotationSendNotAccepted, "条款未受理：非空条款不含控制字符"), nil
	}

	fact, err := handler.prices.ReadQuotedPrice(ctx, ports.QuotedPriceQuery{
		Tenant: command.Tenant, Lane: command.Lane, PlanID: command.PlanID, PlanVersion: command.PlanVersion,
	})
	if err != nil {
		return none, err
	}
	if !fact.Found {
		return refused(QuotationSendNotRegistered, "所引的价格内容不在册"), nil
	}
	switch fact.Standing {
	case ports.QuotedPriceNotFixed:
		return refused(QuotationSendContentNotFixed, "价格内容还没固定"), nil
	case ports.QuotedPriceWithdrawn:
		return refused(QuotationSendWithdrawn, "所引的价卡草稿已撤下"), nil
	case ports.QuotedPriceUseRegisteredCard:
		return refused(QuotationSendAlreadyRegistered, "这一版已经在册，改走价卡路"), nil
	case ports.QuotedPriceReady:
	default:
		return none, fmt.Errorf("party commercial: quotation price standing %s", fact.Standing)
	}
	if !fact.DirectionSell {
		return refused(QuotationSendNotSell, "所引的价格内容不是销售方向"), nil
	}
	if fact.ContentDigest != command.ContentDigest {
		return refused(QuotationSendContentChanged, "内容已变，与调用方看到的摘要不同"), nil
	}
	if fact.GapWhere != "" {
		conclusion := fact.GapConclusion
		if conclusion == "" {
			conclusion = "这份价格内容表达不成对客价格表"
		}
		return refused(QuotationSendNotRepresentable, conclusion), nil
	}
	if len(fact.Workbook) == 0 {
		return none, fmt.Errorf("party commercial: disclosed rate sheet is empty")
	}

	counterparty, found, err := handler.identities.LoadLatestBusinessParty(ctx, command.Tenant, command.Counterparty)
	if err != nil {
		return none, err
	}
	if !found {
		return refused(QuotationSendCounterpartyNotRegistered, "相对方不在册"), nil
	}
	switch counterparty.Lifecycle().StatusAt(command.SentAt) {
	case domain.IdentityEffective:
	case domain.IdentityRegistered:
		return refused(QuotationSendCounterpartyNotYetEffective, "相对方还没到生效时点"), nil
	case domain.IdentityDeactivated:
		return refused(QuotationSendCounterpartyDeactivated, "相对方已停用"), nil
	default:
		return none, fmt.Errorf("party commercial: counterparty lifecycle is unreadable")
	}

	entity, found, err := handler.identities.LoadLatestLegalEntity(ctx, command.Tenant, command.LegalEntity)
	if err != nil {
		return none, err
	}
	if !found {
		return refused(QuotationSendLegalEntityNotRegistered, "责任法人不在册"), nil
	}
	switch entity.Lifecycle().StatusAt(command.SentAt) {
	case domain.IdentityEffective:
	case domain.IdentityRegistered:
		return refused(QuotationSendLegalEntityNotYetEffective, "责任法人还没到生效时点"), nil
	case domain.IdentityDeactivated:
		return refused(QuotationSendLegalEntityDeactivated, "责任法人已停用"), nil
	default:
		return none, fmt.Errorf("party commercial: legal entity lifecycle is unreadable")
	}
	namedParty, found, err := handler.identities.LoadLatestBusinessParty(ctx, command.Tenant, entity.Entity().Party())
	if err != nil {
		return none, err
	}
	if !found {
		return none, fmt.Errorf("party commercial: legal entity %s pins a party that is not on the book", command.LegalEntity)
	}

	id := command.ID
	if id.String() == "" {
		minted, mintErr := handler.ids.Next()
		if mintErr != nil {
			return none, mintErr
		}
		id, err = domain.NewQuotationID(minted)
		if err != nil {
			return none, err
		}
	}
	send, err := domain.NewQuotationSend(domain.QuotationSendSpec{
		ID: id, Tenant: command.Tenant, Lane: command.Lane,
		PlanID: command.PlanID, PlanVersion: command.PlanVersion,
		Canonicalization: fact.Canonicalization, ContentDigest: fact.ContentDigest,
		Counterparty: command.Counterparty, CounterpartyName: counterparty.Party().Name(),
		LegalEntity: command.LegalEntity, LegalEntityName: namedParty.Party().Name(),
		StartsAt: command.StartsAt, EndsAt: command.EndsAt, Terms: command.Terms,
		Sender: command.Sender, SentAt: command.SentAt, Method: domain.QuotationSendByWorkbookExport,
	})
	if err != nil {
		return none, err
	}
	file, fileName, err := customerRateSheet(send, fact.Workbook)
	if err != nil {
		return none, err
	}
	outcome, err := handler.store.AppendQuotationSend(ctx, send)
	if err != nil {
		return none, err
	}
	switch outcome {
	case ports.QuotationAppended:
		return SendQuotationResult{
			Outcome: QuotationSent, Conclusion: "已发出。送达不在这里记。",
			Send: send, Workbook: file, MediaType: fact.MediaType, FileName: fileName,
		}, nil
	case ports.QuotationAppendReplay:
		return SendQuotationResult{
			Outcome: QuotationSendReplay, Conclusion: "这一条发出记录已经在册，没有再插一行。送达不在这里记。",
			Send: send, Workbook: file, MediaType: fact.MediaType, FileName: fileName,
		}, nil
	case ports.QuotationAppendConflict:
		return refused(QuotationSendConflict, "这个报价号已经发出过不同的内容"), nil
	default:
		return none, fmt.Errorf("party commercial: quotation append outcome %s", outcome)
	}
}

func refused(outcome SendQuotationOutcome, conclusion string) SendQuotationResult {
	return SendQuotationResult{Outcome: outcome, Conclusion: conclusion}
}

package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 报价发出的端口。价格内容由消费侧适配器读回来，本上下文不认识计价的类型。
// 发出记录只增不改。再导出是读，不经过写口。

// QuotedPriceStanding 是适配器译好的价格内容处境。应用层按它分格，不自己认计价的状态名。
type QuotedPriceStanding uint8

const (
	QuotedPriceStandingInvalid QuotedPriceStanding = iota
	// QuotedPriceNotFixed：草稿还在草稿或已校验，内容还可以被换掉。
	QuotedPriceNotFixed
	// QuotedPriceWithdrawn：这一版草稿已撤下。
	QuotedPriceWithdrawn
	// QuotedPriceUseRegisteredCard：草稿已经发布进登记册，草稿路不再发出。
	QuotedPriceUseRegisteredCard
	// QuotedPriceReady：已批准的草稿，或登记册上的那一版。截止与退役不改变这一格。
	QuotedPriceReady
)

func (standing QuotedPriceStanding) String() string {
	switch standing {
	case QuotedPriceNotFixed:
		return "NOT_FIXED"
	case QuotedPriceWithdrawn:
		return "WITHDRAWN"
	case QuotedPriceUseRegisteredCard:
		return "USE_REGISTERED_CARD"
	case QuotedPriceReady:
		return "READY"
	default:
		return ""
	}
}

// QuotedPriceQuery 指名一条路和一版方案。没有范围、时点或计价目的：报价不选卡。
type QuotedPriceQuery struct {
	Tenant      domain.TenantID
	Lane        domain.QuotationPriceLane
	PlanID      string
	PlanVersion string
}

// QuotedPriceFact 是读到的价格内容。Workbook 是对客价格表的电子表格，不含封面。
// 表达不了时 GapWhere 有值、Workbook 为空，摘要仍然在。读不动走 error，不折进这里。
type QuotedPriceFact struct {
	Found            bool
	Standing         QuotedPriceStanding
	DirectionSell    bool
	Canonicalization string
	ContentDigest    string
	Scope            string
	Workbook         []byte
	MediaType        string
	GapWhere         string
	GapConclusion    string
}

// QuotedPriceReader 读草稿册或价卡登记册，并投影对客价格表。
type QuotedPriceReader interface {
	ReadQuotedPrice(ctx context.Context, query QuotedPriceQuery) (QuotedPriceFact, error)
}

// RegisteredPriceFact 是所引方案版本在价卡册上的两件事实，加上发布时记下的不透明依据。
// 在册与此刻还能被选中是两件事：退役或被后继截止的版本仍在册，但一刻也选不中。
// PublicationBasis 原样交回，本上下文自己拿它和发出标识比，计价不解释它。
type RegisteredPriceFact struct {
	OnRegister          bool
	SelectsNow          bool
	PublicationBasis    string
	HasPublicationBasis bool
}

// RegisteredPriceReader 用计价已有的选用读法回答这一版此刻还能不能被选中。
type RegisteredPriceReader interface {
	ReadRegisteredPrice(ctx context.Context, tenant domain.TenantID, planID, planVersion string, at time.Time) (RegisteredPriceFact, error)
}

// QuotationAppendOutcome 是写入落点。重放不改行，冲突也不改行。
type QuotationAppendOutcome uint8

const (
	QuotationAppendOutcomeInvalid QuotationAppendOutcome = iota
	QuotationAppended
	QuotationAppendReplay
	QuotationAppendConflict
)

func (outcome QuotationAppendOutcome) String() string {
	switch outcome {
	case QuotationAppended:
		return "APPENDED"
	case QuotationAppendReplay:
		return "REPLAY"
	case QuotationAppendConflict:
		return "CONFLICT"
	default:
		return ""
	}
}

// QuotationSendCursor 是列表的下一页起点：按发出时刻倒序，同一时刻再按标识倒序。
type QuotationSendCursor struct {
	SentAt time.Time
	ID     domain.QuotationID
}

func (cursor QuotationSendCursor) IsZero() bool {
	return cursor.SentAt.IsZero() && cursor.ID.String() == ""
}

// QuotationSendPage 是一页发出记录。HasNext 为真时 Next 指向再下一页的起点。
type QuotationSendPage struct {
	Sends   []domain.QuotationSend
	Next    QuotationSendCursor
	HasNext bool
}

// QuotationSendStore 是发出记录册。Append 无事务即拒。读口不要求事务。
type QuotationSendStore interface {
	AppendQuotationSend(ctx context.Context, send domain.QuotationSend) (QuotationAppendOutcome, error)
	GetQuotationSend(ctx context.Context, tenant domain.TenantID, id domain.QuotationID) (domain.QuotationSend, bool, error)
	ListQuotationSends(ctx context.Context, tenant domain.TenantID, limit int, cursor QuotationSendCursor) (QuotationSendPage, error)
}

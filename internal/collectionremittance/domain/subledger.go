package domain

import (
	"fmt"
	"time"
)

// 代收分户账的词形：账的身份（四维键）、开立面、资金位置、追加式记账，以及由记账
// 派生的余额与分配守恒。
//
// **余额不是字段，是派生结果。** 本包没有任何设置余额的入口——要改账面只能追加一笔
// 记账。这不是风格选择：一旦余额可写，「余额对不对」就成了要靠对平去查的事，而对得上
// 与对不上在读的人眼里长着同一张脸；派生之后它在结构上不可能对不上。

// SubledgerKey 是一本代收分户账的身份：货主客户、责任法人、币种、代收渠道四维。
// 租户不在键上——它是入参，本上下文不做接入认证（同 CONTEXT 的四维句）。
//
// 四维齐备才成一本账。少任何一维，两笔本不该混的钱都会落进同一个余额；而事后按更
// 细的维再拆分要重演全部记账历史，所以这个粒度是难逆转的（ADR-0082）。
type SubledgerKey struct {
	customer CustomerReference
	entity   LegalEntityReference
	currency CurrencyCode
	channel  CollectionChannelReference
}

func NewSubledgerKey(
	customer CustomerReference,
	entity LegalEntityReference,
	currency CurrencyCode,
	channel CollectionChannelReference,
) (SubledgerKey, error) {
	if !customer.valid() {
		return SubledgerKey{}, fmt.Errorf("%w: customer reference", ErrBlankValue)
	}
	if !entity.valid() {
		return SubledgerKey{}, fmt.Errorf("%w: legal entity reference", ErrBlankValue)
	}
	if !currency.valid() {
		return SubledgerKey{}, fmt.Errorf("%w: currency code", ErrBlankValue)
	}
	if !channel.valid() {
		return SubledgerKey{}, fmt.Errorf("%w: collection channel reference", ErrBlankValue)
	}
	return SubledgerKey{customer: customer, entity: entity, currency: currency, channel: channel}, nil
}

func (key SubledgerKey) Customer() CustomerReference { return key.customer }

func (key SubledgerKey) LegalEntity() LegalEntityReference { return key.entity }

func (key SubledgerKey) Currency() CurrencyCode { return key.currency }

func (key SubledgerKey) Channel() CollectionChannelReference { return key.channel }

func (key SubledgerKey) valid() bool {
	return key.customer.valid() && key.entity.valid() && key.currency.valid() && key.channel.valid()
}

// Subledger 是一本已开立的分户账。开立面与记账面分开，是为了让「已开立但当期无记账」
// 说得出口——它与「未开立」是相反的答案，合成一个概念之后读面只能用「查不到」冒充
// 「无本金」。
type Subledger struct {
	key      SubledgerKey
	custody  CustodyBasisReference
	openedAt time.Time
}

// OpenSubledger 开立一本分户账。受托依据必填：没有依据的受托保管账在账上看不出这笔
// 钱凭什么由运营企业代管。
func OpenSubledger(
	key SubledgerKey,
	custody CustodyBasisReference,
	openedAt time.Time,
) (Subledger, error) {
	if !key.valid() {
		return Subledger{}, fmt.Errorf("%w: subledger key", ErrInvalidSubledger)
	}
	if !custody.valid() {
		return Subledger{}, fmt.Errorf("%w: custody basis reference", ErrInvalidSubledger)
	}
	if openedAt.IsZero() {
		return Subledger{}, fmt.Errorf("%w: opened at", ErrInvalidSubledger)
	}
	return Subledger{key: key, custody: custody, openedAt: openedAt.UTC()}, nil
}

func (ledger Subledger) Key() SubledgerKey { return ledger.key }

func (ledger Subledger) CustodyBasis() CustodyBasisReference { return ledger.custody }

func (ledger Subledger) OpenedAt() time.Time { return ledger.openedAt }

// FundPosition 是代收本金在分户账内所处的受托状态，封闭词表。
//
// `PositionExternalSource` 不是账内位置：它只作为入账记账的来源侧出现，代表本金自
// 一笔已接受的代收事实进入本账。去向侧没有账外位置——已汇付是账内终局，于是全账
// 总额恒等于入账之和，守恒在一条求和上就核得完。
type FundPosition uint8

const (
	FundPositionUnknown FundPosition = iota
	PositionExternalSource
	PositionInTransitAtChannel
	PositionAwaitingAllocation
	PositionPayableToCustomer
	PositionRemitted
	PositionShortfall
	PositionSurplus
)

// String 交回落库词。未知格交回空串而不是占位词：空串在 SQL 上撞 CHECK，占位词会
// 变成一个谁也没登记过的位置。
func (position FundPosition) String() string {
	switch position {
	case PositionExternalSource:
		return "EXTERNAL_SOURCE"
	case PositionInTransitAtChannel:
		return "IN_TRANSIT_AT_CHANNEL"
	case PositionAwaitingAllocation:
		return "AWAITING_ALLOCATION"
	case PositionPayableToCustomer:
		return "PAYABLE_TO_CUSTOMER"
	case PositionRemitted:
		return "REMITTED"
	case PositionShortfall:
		return "SHORTFALL"
	case PositionSurplus:
		return "SURPLUS"
	default:
		return ""
	}
}

func (position FundPosition) valid() bool { return position.String() != "" }

// InsideLedger 区分账内位置与外部来源。守恒与余额非负都只对账内位置成立。
func (position FundPosition) InsideLedger() bool {
	return position.valid() && position != PositionExternalSource
}

// ParseFundPosition 从落库词还原位置。第二个返回值为 false 即库里那格不是本词表的
// 成员——读回时宁可答不认识，也不折成某个默认位置。
func ParseFundPosition(text string) (FundPosition, bool) {
	for _, candidate := range []FundPosition{
		PositionExternalSource,
		PositionInTransitAtChannel,
		PositionAwaitingAllocation,
		PositionPayableToCustomer,
		PositionRemitted,
		PositionShortfall,
		PositionSurplus,
	} {
		if candidate.String() == text {
			return candidate, true
		}
	}
	return FundPositionUnknown, false
}

// PostingBasisKind 声明一笔记账凭什么落账，封闭四值。它与 BasisReference 成对：
// 种类说依据在哪张册子上，引用说是那张册子的哪一行。
type PostingBasisKind uint8

const (
	PostingBasisUnknown PostingBasisKind = iota
	// BasisCollectionFact 凭一条已接受的代收事实。入账只认这一种。
	BasisCollectionFact
	// BasisRemittanceBatch 凭一个已形成的回汇批次。进入`已汇付`只认这一种。
	BasisRemittanceBatch
	// BasisDiscrepancy 凭一项已登记的差异事项。进入`短款`/`溢款`只认这一种。
	BasisDiscrepancy
	// BasisCorrection 凭被冲正的原记账。冲正是追加一笔反向记账，原记账原样留在账上
	// ——本包没有修改或删除记账的入口。
	BasisCorrection
)

func (kind PostingBasisKind) String() string {
	switch kind {
	case BasisCollectionFact:
		return "COLLECTION_FACT"
	case BasisRemittanceBatch:
		return "REMITTANCE_BATCH"
	case BasisDiscrepancy:
		return "DISCREPANCY"
	case BasisCorrection:
		return "CORRECTION"
	default:
		return ""
	}
}

func (kind PostingBasisKind) valid() bool { return kind.String() != "" }

func ParsePostingBasisKind(text string) (PostingBasisKind, bool) {
	for _, candidate := range []PostingBasisKind{
		BasisCollectionFact, BasisRemittanceBatch, BasisDiscrepancy, BasisCorrection,
	} {
		if candidate.String() == text {
			return candidate, true
		}
	}
	return PostingBasisUnknown, false
}

// SubledgerPostingSpec 是形成一笔记账所需的全部输入。
type SubledgerPostingSpec struct {
	ID        PostingID
	Ledger    SubledgerKey
	From      FundPosition
	To        FundPosition
	Amount    Money
	BasisKind PostingBasisKind
	Basis     BasisReference
	PostedAt  time.Time
}

// SubledgerPosting 是分户账里一条不可覆盖的记账：一笔正金额自一个位置移到另一个
// 位置。本类型没有任何 setter，落库侧也只追加——写错走反向记账冲正。
type SubledgerPosting struct {
	id        PostingID
	ledger    SubledgerKey
	from      FundPosition
	to        FundPosition
	amount    Money
	basisKind PostingBasisKind
	basis     BasisReference
	postedAt  time.Time
}

// RecordSubledgerPosting 形成一笔记账。三条依据门在这里就把住，不留给调用方自觉：
//   - 入账（来源是外部来源）必须凭代收事实——账上不许出现无来源的本金；
//   - 进入`已汇付`必须凭回汇批次——没有批次依据的资金不算汇付出去；
//   - 进入`短款`或`溢款`必须凭差异事项——差额不自动落账，先有事项再有记账。
//
// 金额币种必须等于分户账币种：币种在键上，一本账内不发生换算，本包也没有折算入口。
func RecordSubledgerPosting(spec SubledgerPostingSpec) (SubledgerPosting, error) {
	if !spec.ID.valid() {
		return SubledgerPosting{}, fmt.Errorf("%w: posting ID", ErrInvalidPosting)
	}
	if !spec.Ledger.valid() {
		return SubledgerPosting{}, fmt.Errorf("%w: subledger key", ErrInvalidPosting)
	}
	if !spec.From.valid() || !spec.To.valid() {
		return SubledgerPosting{}, fmt.Errorf("%w: fund position", ErrInvalidPosting)
	}
	if !spec.To.InsideLedger() {
		return SubledgerPosting{}, fmt.Errorf(
			"%w: no posting moves principal out of the ledger", ErrInvalidPosting)
	}
	if spec.From == spec.To {
		return SubledgerPosting{}, fmt.Errorf("%w: positions must differ", ErrInvalidPosting)
	}
	if !spec.Amount.valid() {
		return SubledgerPosting{}, fmt.Errorf("%w: amount", ErrInvalidPosting)
	}
	if spec.Amount.Currency() != spec.Ledger.Currency() {
		return SubledgerPosting{}, fmt.Errorf(
			"%w: amount currency %q is not the subledger currency %q",
			ErrInvalidPosting, spec.Amount.Currency(), spec.Ledger.Currency())
	}
	if !spec.BasisKind.valid() || !spec.Basis.valid() {
		return SubledgerPosting{}, fmt.Errorf("%w: basis", ErrInvalidPosting)
	}
	if spec.From == PositionExternalSource && spec.BasisKind != BasisCollectionFact {
		return SubledgerPosting{}, fmt.Errorf(
			"%w: intake must cite a collection fact", ErrInvalidPosting)
	}
	if spec.To == PositionRemitted && spec.BasisKind != BasisRemittanceBatch {
		return SubledgerPosting{}, fmt.Errorf(
			"%w: remittance must cite a remittance batch", ErrInvalidPosting)
	}
	if (spec.To == PositionShortfall || spec.To == PositionSurplus) &&
		spec.BasisKind != BasisDiscrepancy {
		return SubledgerPosting{}, fmt.Errorf(
			"%w: shortfall or surplus must cite a discrepancy item", ErrInvalidPosting)
	}
	if spec.PostedAt.IsZero() {
		return SubledgerPosting{}, fmt.Errorf("%w: posted at", ErrInvalidPosting)
	}
	return SubledgerPosting{
		id:        spec.ID,
		ledger:    spec.Ledger,
		from:      spec.From,
		to:        spec.To,
		amount:    spec.Amount,
		basisKind: spec.BasisKind,
		basis:     spec.Basis,
		postedAt:  spec.PostedAt.UTC(),
	}, nil
}

func (posting SubledgerPosting) ID() PostingID { return posting.id }

func (posting SubledgerPosting) Ledger() SubledgerKey { return posting.ledger }

func (posting SubledgerPosting) From() FundPosition { return posting.from }

func (posting SubledgerPosting) To() FundPosition { return posting.to }

func (posting SubledgerPosting) Amount() Money { return posting.amount }

func (posting SubledgerPosting) BasisKind() PostingBasisKind { return posting.basisKind }

func (posting SubledgerPosting) Basis() BasisReference { return posting.basis }

func (posting SubledgerPosting) PostedAt() time.Time { return posting.postedAt }

// SubledgerBalance 是一本分户账各资金位置的派生余额，连同自外部来源入账的总额。
// 它是只读的快照：唯一的推进方式是 Apply 一笔新记账。
type SubledgerBalance struct {
	ledger   SubledgerKey
	held     map[FundPosition]int64
	intake   int64
	postings int
}

// DeriveSubledgerBalance 从一本账的全部记账派生余额，并当场核两件事：账内各位置余额
// 非负、非外部位置余额之和等于入账之和（分配守恒）。
//
// 核的是**终态**而不是逐笔中间态。逐笔核等于把切片顺序当成记账的真实先后，而同一
// posted_at 上的两笔谁先谁后并不由那一列决定；落账时的余额非负由 Admit 在事务内按
// 当刻账面把关，那一格才是有真实先后的地方。
func DeriveSubledgerBalance(
	ledger SubledgerKey,
	postings []SubledgerPosting,
) (SubledgerBalance, error) {
	if !ledger.valid() {
		return SubledgerBalance{}, fmt.Errorf("%w: subledger key", ErrInvalidSubledger)
	}
	balance := SubledgerBalance{ledger: ledger, held: make(map[FundPosition]int64)}
	for _, posting := range postings {
		if posting.ledger != ledger {
			return SubledgerBalance{}, fmt.Errorf(
				"%w: posting %s belongs to another subledger", ErrInvalidPosting, posting.id)
		}
		amount := posting.amount.AmountMinor()
		if posting.from == PositionExternalSource {
			balance.intake += amount
		} else {
			balance.held[posting.from] -= amount
		}
		balance.held[posting.to] += amount
		balance.postings++
	}

	var total int64
	for position, amount := range balance.held {
		if amount < 0 {
			return SubledgerBalance{}, fmt.Errorf(
				"%w: %s holds %d", ErrConservationBroken, position, amount)
		}
		total += amount
	}
	if total != balance.intake {
		return SubledgerBalance{}, fmt.Errorf(
			"%w: positions hold %d but %d was taken in", ErrConservationBroken, total, balance.intake)
	}
	return balance, nil
}

func (balance SubledgerBalance) Ledger() SubledgerKey { return balance.ledger }

// At 交回某个账内位置的余额。未出现过的位置是 0——本账在该位置上确实没有钱，这与
// 「账不存在」是两件事，后者由分户账开立面回答。
func (balance SubledgerBalance) At(position FundPosition) int64 {
	return balance.held[position]
}

// IntakeTotal 是自外部来源入账的总额，也就是本账受托保管过的全部本金。
func (balance SubledgerBalance) IntakeTotal() int64 { return balance.intake }

// PostingCount 是派生本余额所用的记账笔数。零笔即「已开立但当期无记账」那一格。
func (balance SubledgerBalance) PostingCount() int { return balance.postings }

// Admit 判定一笔待落账的记账能否加进当前账面。它只管余额那一格——形状与依据门在
// RecordSubledgerPosting 已经过了，这里不重判。
//
// 来源位置余额不足时交回 ErrPositionUnderfunded 而不是「输入不合法」：请求可能完全
// 正确，只是此刻账上还没有那么多钱。两者的处置相反——前者等实收或先清分，后者改请求。
func (balance SubledgerBalance) Admit(posting SubledgerPosting) error {
	if posting.ledger != balance.ledger {
		return fmt.Errorf("%w: posting %s belongs to another subledger", ErrInvalidPosting, posting.id)
	}
	if posting.from == PositionExternalSource {
		return nil
	}
	available := balance.held[posting.from]
	if available < posting.amount.AmountMinor() {
		return fmt.Errorf("%w: %s holds %d, posting %s needs %d",
			ErrPositionUnderfunded, posting.from, available, posting.id, posting.amount.AmountMinor())
	}
	return nil
}

// Apply 在当前账面上落一笔记账并交回新的余额快照。原快照不变——余额是派生值，没有
// 就地改写的入口。
func (balance SubledgerBalance) Apply(posting SubledgerPosting) (SubledgerBalance, error) {
	if err := balance.Admit(posting); err != nil {
		return SubledgerBalance{}, err
	}
	next := SubledgerBalance{
		ledger:   balance.ledger,
		held:     make(map[FundPosition]int64, len(balance.held)+1),
		intake:   balance.intake,
		postings: balance.postings + 1,
	}
	for position, amount := range balance.held {
		next.held[position] = amount
	}
	amount := posting.amount.AmountMinor()
	if posting.from == PositionExternalSource {
		next.intake += amount
	} else {
		next.held[posting.from] -= amount
	}
	next.held[posting.to] += amount
	return next, nil
}

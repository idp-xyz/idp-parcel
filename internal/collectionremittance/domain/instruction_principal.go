package domain

import "fmt"

// InstructionPrincipal 是一条代收指令名下本金的派生快照：凭它的报告入了多少、凭它的
// 到账、认可的协议抵扣与短款从`渠道在途`移出了多少、移进`待清分`的到账、落进`已抵扣`
// 的抵扣与落进`溢款`的差额各多少，以及它的每一条代收事实、每一条认可的协议抵扣已
// 支撑了哪一笔记账。
//
// 名下恒等式：入账报告 = `渠道在途`余额 + 移进`待清分`的到账 + 落进`已抵扣`的抵扣 +
// 落进`短款`的差额，各项都已减去冲正回去的。
//
// 按指令而不是按分户账判，是因为报告按指令出、差异按指令追责（ADR-0271 决定二）：
// 按账合用时，一单到账不足会被同账另一单的在途款填上，短款落在哪一单上就说不清了。
// 分户账余额（SubledgerBalance）照旧把住「位置不得为负」，这里在其上再加按指令的
// 那一层，两者都要过。
//
// 同 SubledgerBalance，它不落字段，只由记账派生；没有设置它的入口。
type InstructionPrincipal struct {
	instruction   CollectionInstruction
	inTransit     int64
	arrivalsMoved int64
	surplusLanded int64
	offsetApplied int64
	factPostings  map[CollectionFactID]PostingID
	offsetPosted  map[AgreedOffsetID]PostingID
	unmoved       bool
}

// DeriveInstructionPrincipal 从一条指令的全部代收事实与依据追溯到它的全部记账派生
// 快照。哪些记账「追溯到」这条指令由调用方按依据给出（凭它的事实、凭它的差异事项、
// 凭它清分、冲正这些记账的冲正）；本函数只核两件装错的情形：别的指令的事实、别本
// 账的记账。
//
// 名下`渠道在途`为负不在这里报错：它只可能出自按指令判上限之前落下的旧账（同账借用
// 别的指令的在途款）。对这类账判下一笔时上限为负，什么也移不出去，这就够了；让派生
// 失败会把这条指令此后的每一次记账都折成依赖故障。
func DeriveInstructionPrincipal(
	instruction CollectionInstruction,
	facts []CollectionFact,
	postings []SubledgerPosting,
) (InstructionPrincipal, error) {
	if !instruction.ID().valid() {
		return InstructionPrincipal{}, fmt.Errorf("%w: instruction ID", ErrInvalidInstruction)
	}
	principal := InstructionPrincipal{
		instruction:  instruction,
		factPostings: make(map[CollectionFactID]PostingID),
		offsetPosted: make(map[AgreedOffsetID]PostingID),
	}
	for _, posting := range postings {
		if posting.ledger != instruction.Ledger() {
			return InstructionPrincipal{}, fmt.Errorf(
				"%w: posting %s belongs to another subledger", ErrInvalidPosting, posting.id)
		}
		amount := posting.amount.AmountMinor()
		if posting.to == PositionInTransitAtChannel {
			principal.inTransit += amount
		}
		if posting.from == PositionInTransitAtChannel {
			principal.inTransit -= amount
		}
		switch {
		case posting.from == PositionInTransitAtChannel && posting.to == PositionAwaitingAllocation:
			principal.arrivalsMoved += amount
		case posting.from == PositionAwaitingAllocation && posting.to == PositionInTransitAtChannel:
			principal.arrivalsMoved -= amount
		case posting.from == PositionAwaitingAllocation && posting.to == PositionSurplus:
			principal.surplusLanded += amount
		case posting.from == PositionSurplus && posting.to == PositionAwaitingAllocation:
			principal.surplusLanded -= amount
		case posting.from == PositionInTransitAtChannel && posting.to == PositionOffsetApplied:
			principal.offsetApplied += amount
		case posting.from == PositionOffsetApplied && posting.to == PositionInTransitAtChannel:
			principal.offsetApplied -= amount
		}
		switch posting.basisKind {
		case BasisCollectionFact:
			principal.factPostings[CollectionFactID(posting.basis)] = posting.id
		case BasisAgreedOffset:
			principal.offsetPosted[AgreedOffsetID(posting.basis)] = posting.id
		}
	}
	for _, fact := range facts {
		if fact.instruction != instruction.ID() {
			return InstructionPrincipal{}, fmt.Errorf(
				"%w: fact %s belongs to instruction %s", ErrInvalidCollectionFact,
				fact.id, fact.instruction)
		}
		if _, posted := principal.factPostings[fact.id]; fact.layer.SettlesToOperator() && !posted {
			principal.unmoved = true
		}
	}
	return principal, nil
}

// TransferCeiling 是移位上限：该指令已入账报告 − 该指令自`渠道在途`移出的金额（到账
// 移位与短款落账，减去冲正回`渠道在途`的）。它就是该指令名下的`渠道在途`余额。
func (principal InstructionPrincipal) TransferCeiling() int64 { return principal.inTransit }

// ShortfallCeiling 是短款金额能到的上限，也就是此刻名下`渠道在途`的余额：认可的协议
// 抵扣一经落到`已抵扣`就已经从中扣掉（ADR-0298），余下的才是渠道报了、钱没回来、
// 又没有协议认可的那部分。抵扣还没落账的认可记录在这里读不出，所以短款先于抵扣落账
// 会吃掉抵扣的额度，此后抵扣答余额不足，要先冲正多落的短款。
func (principal InstructionPrincipal) ShortfallCeiling() int64 { return principal.TransferCeiling() }

// OffsetApplied 是该指令已落进`已抵扣`的抵扣额，减去冲正回`渠道在途`的。
func (principal InstructionPrincipal) OffsetApplied() int64 { return principal.offsetApplied }

// SurplusDue 是按指令算的溢款：该指令移入`待清分`的到账 − 指令金额 − 该指令已落入
// `溢款`的金额。减去已落入的那一项，同一笔多收就只能落一次。
func (principal InstructionPrincipal) SurplusDue() int64 {
	return principal.arrivalsMoved - principal.instruction.Amount().AmountMinor() - principal.surplusLanded
}

// PostingOf 交回这条代收事实已支撑的那一笔记账。一条事实至多支撑一笔（ADR-0271 决定三）。
func (principal InstructionPrincipal) PostingOf(fact CollectionFactID) (PostingID, bool) {
	id, found := principal.factPostings[fact]
	return id, found
}

// OffsetPostingOf 交回这条认可的协议抵扣已支撑的那一笔记账。一条认可抵扣至多支撑
// 一笔，冲正回去之后仍记着它——撤回的认可不能再落第二次，要落走新的认可。
func (principal InstructionPrincipal) OffsetPostingOf(offset AgreedOffsetID) (PostingID, bool) {
	id, found := principal.offsetPosted[offset]
	return id, found
}

// HasUnmovedArrival 答该指令是否有已登记、还没移位的真实到账。
func (principal InstructionPrincipal) HasUnmovedArrival() bool { return principal.unmoved }

// Admit 判一笔追溯到本指令的待落账记账过不过按指令的上限。形状与依据门已在
// RecordSubledgerPosting 与编排里过了，这里只看金额。
//
// 越过上限答 ErrPositionUnderfunded：请求可能完全对，等的是上游那一笔——该指令的
// 渠道报告入账、迟到的到账移位，或短款冲正回`渠道在途`。溢款金额小于按指令算的溢款
// 答 ErrInvalidPosting：多收的已经全在`待清分`里，事项报少了，等也等不对。
//
// 短款与抵扣另有一格：该指令有已登记而未移位的到账时不落。钱已经回来，只是报告没补上
// （或到账还没过账），此刻落短款会把补报告之后就能移位的本金落进`短款`，落抵扣会让
// 抵扣先吃掉在途款、真实到账的钱反而移不动；先落它们再移到账，与先移到账再落它们，
// 终局不同，而结果不该随到达次序变，实际到账优先于认可的抵扣。这一格也答余额不足：
// 处置同样是等上游那一笔、再重跑同一命令。
func (principal InstructionPrincipal) Admit(posting SubledgerPosting) error {
	if posting.ledger != principal.instruction.Ledger() {
		return fmt.Errorf("%w: posting %s belongs to another subledger", ErrInvalidPosting, posting.id)
	}
	amount := posting.amount.AmountMinor()
	if posting.to == PositionShortfall {
		if principal.unmoved {
			return fmt.Errorf("%w: instruction %s has an arrival not yet moved out of transit",
				ErrPositionUnderfunded, principal.instruction.ID())
		}
		if ceiling := principal.ShortfallCeiling(); amount > ceiling {
			return fmt.Errorf("%w: instruction %s can take at most %d as shortfall, posting %s needs %d",
				ErrPositionUnderfunded, principal.instruction.ID(), ceiling, posting.id, amount)
		}
		return nil
	}
	if posting.to == PositionOffsetApplied && principal.unmoved {
		return fmt.Errorf("%w: instruction %s has an arrival not yet moved out of transit",
			ErrPositionUnderfunded, principal.instruction.ID())
	}
	if posting.from == PositionInTransitAtChannel {
		if ceiling := principal.TransferCeiling(); amount > ceiling {
			return fmt.Errorf("%w: instruction %s holds %d in transit, posting %s needs %d",
				ErrPositionUnderfunded, principal.instruction.ID(), ceiling, posting.id, amount)
		}
		return nil
	}
	if posting.to == PositionSurplus {
		due := principal.SurplusDue()
		if amount > due {
			return fmt.Errorf("%w: instruction %s has %d received beyond it, posting %s needs %d",
				ErrPositionUnderfunded, principal.instruction.ID(), due, posting.id, amount)
		}
		if amount < due {
			return fmt.Errorf("%w: surplus %d is not the instruction's received overage %d",
				ErrInvalidPosting, amount, due)
		}
	}
	return nil
}

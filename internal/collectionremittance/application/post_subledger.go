package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/ports"
)

// 分户账的开立与记账用例。
//
// 本文件最要紧的一条：**分户账键不是记账的输入，它由依据推出来。** 依据是代收事实、
// 代收指令、回汇批次、差异事项或原记账，每一条都自带它所属的那本账；记账因此在结构上
// 不可能记进别人的账。让调用方传键的写法在库里看不出错——一笔记错账的行，与记对的行
// 逐列合法。

// OpenSubledgerCommand 携带一次分户账开立。
type OpenSubledgerCommand struct {
	Tenant domain.TenantID
	Ledger domain.Subledger
}

// OpenSubledger 开立一本分户账。它与记账分开，是为了让「已开立但当期无记账」说得
// 出口——那与「未开立」是相反的答案。
func (handler *Handler) OpenSubledger(ctx context.Context, command OpenSubledgerCommand) (Outcome, error) {
	if blank(command.Tenant) ||
		strings.TrimSpace(command.Ledger.Key().Customer().String()) == "" ||
		strings.TrimSpace(command.Ledger.CustodyBasis().String()) == "" {
		return OutcomeNotAccepted, nil
	}

	saved, err := handler.deps.Subledgers.OpenSubledger(ctx, command.Tenant, command.Ledger)
	if err != nil {
		return OutcomeUndecided, nil
	}
	if saved == ports.SaveRegistered {
		return OutcomeRegistered, nil
	}

	existing, found, err := handler.deps.Subledger.LoadSubledger(
		ctx, command.Tenant, command.Ledger.Key())
	if err != nil {
		return OutcomeUndecided, nil
	}
	if !found ||
		existing.CustodyBasis() != command.Ledger.CustodyBasis() ||
		!existing.OpenedAt().Equal(command.Ledger.OpenedAt()) {
		return OutcomeContentConflict, nil
	}
	return OutcomeExisting, nil
}

// PostCommand 携带一次分户账记账。分户账键不在里面——它由 BasisKind 与 Basis 指名的
// 那一行推出。金额只给最小单位，币种取自解出来的分户账：一本账内不发生换算，让调用方
// 传币种就等于给「记错币种」留了个说得通的入口。
type PostCommand struct {
	Tenant      domain.TenantID
	ID          domain.PostingID
	From        domain.FundPosition
	To          domain.FundPosition
	AmountMinor int64
	BasisKind   domain.PostingBasisKind
	Basis       domain.BasisReference
	PostedAt    time.Time
}

// PostSubledger 追加一笔记账：解依据 → 定分户账与形状 → 读回账面 → 余额守卫 → 落账。
//
// 余额守卫必须在同一笔事务内读回账面再判（写口 RequireExecutor 保证有环境事务）：
// 隔着事务判出来的「够扣」在落账时可能已经不够了，而透支后的账面看上去与正常账面
// 没有区别。
func (handler *Handler) PostSubledger(ctx context.Context, command PostCommand) (Outcome, error) {
	if blank(command.Tenant) ||
		strings.TrimSpace(command.ID.String()) == "" ||
		strings.TrimSpace(command.Basis.String()) == "" ||
		command.AmountMinor <= 0 ||
		command.PostedAt.IsZero() {
		return OutcomeNotAccepted, nil
	}

	ledger, outcome, err := handler.resolveLedgerForPosting(ctx, command)
	if outcome != OutcomeRegistered || err != nil {
		return outcome, err
	}

	amount, err := domain.NewMoney(ledger.Currency(), command.AmountMinor)
	if err != nil {
		return OutcomeNotAccepted, nil
	}
	posting, err := domain.RecordSubledgerPosting(domain.SubledgerPostingSpec{
		ID:        command.ID,
		Ledger:    ledger,
		From:      command.From,
		To:        command.To,
		Amount:    amount,
		BasisKind: command.BasisKind,
		Basis:     command.Basis,
		PostedAt:  command.PostedAt,
	})
	if err != nil {
		return OutcomeNotAccepted, nil
	}

	balance, opened, err := handler.deps.Subledger.LoadBalance(ctx, command.Tenant, ledger)
	if err != nil {
		return OutcomeUndecided, nil
	}
	if !opened {
		// 账没开立就没有受托依据。这不是输入不合法，是缺上游那一笔。
		return OutcomeBasisMissing, nil
	}
	if err := balance.Admit(posting); err != nil {
		if errors.Is(err, domain.ErrPositionUnderfunded) {
			return OutcomeUnderfunded, nil
		}
		return OutcomeNotAccepted, nil
	}

	saved, err := handler.deps.Subledgers.AppendPosting(ctx, command.Tenant, posting)
	if err != nil {
		return OutcomeUndecided, nil
	}
	if saved == ports.SaveRegistered {
		return OutcomeRegistered, nil
	}

	existing, found, err := handler.deps.Subledger.LoadPosting(ctx, command.Tenant, command.ID)
	if err != nil {
		return OutcomeUndecided, nil
	}
	if !found || !samePosting(existing, posting) {
		return OutcomeContentConflict, nil
	}
	return OutcomeExisting, nil
}

// resolveLedgerForPosting 按依据种类解出分户账，并核这笔记账的形状是不是该依据允许的
// 那一种。第二个返回值为 OutcomeRegistered 表示解析通过（此时还没落账），其余格是
// 已经定案的答案。
func (handler *Handler) resolveLedgerForPosting(
	ctx context.Context,
	command PostCommand,
) (domain.SubledgerKey, Outcome, error) {
	none := domain.SubledgerKey{}
	switch command.BasisKind {
	case domain.BasisCollectionFact:
		factID, err := domain.NewCollectionFactID(command.Basis.String())
		if err != nil {
			return none, OutcomeNotAccepted, nil
		}
		fact, found, err := handler.deps.Collection.LoadCollectionFact(ctx, command.Tenant, factID)
		if err != nil {
			return none, OutcomeUndecided, nil
		}
		if !found {
			return none, OutcomeBasisMissing, nil
		}
		ledger, outcome, err := handler.ledgerOfInstruction(ctx, command.Tenant, fact.Instruction())
		if outcome != OutcomeRegistered || err != nil {
			return none, outcome, err
		}
		if !factPostingShapeHolds(command, fact) {
			return none, OutcomeNotAccepted, nil
		}
		return ledger, OutcomeRegistered, nil

	case domain.BasisAllocation:
		instructionID, err := domain.NewCollectionInstructionID(command.Basis.String())
		if err != nil {
			return none, OutcomeNotAccepted, nil
		}
		ledger, outcome, err := handler.ledgerOfInstruction(ctx, command.Tenant, instructionID)
		if outcome != OutcomeRegistered || err != nil {
			return none, outcome, err
		}
		// 清分是唯一一跳：待清分 → 应付客户。别的方向要么该走别的依据，要么是冲正。
		if command.From != domain.PositionAwaitingAllocation ||
			command.To != domain.PositionPayableToCustomer {
			return none, OutcomeNotAccepted, nil
		}
		return ledger, OutcomeRegistered, nil

	case domain.BasisRemittanceBatch:
		batchID, err := domain.NewRemittanceBatchID(command.Basis.String())
		if err != nil {
			return none, OutcomeNotAccepted, nil
		}
		batch, found, err := handler.deps.Remittance.LoadBatch(ctx, command.Tenant, batchID)
		if err != nil {
			return none, OutcomeUndecided, nil
		}
		if !found {
			return none, OutcomeBasisMissing, nil
		}
		if !batch.AcceptsRemittance() {
			return none, OutcomeBatchClosed, nil
		}
		if command.From != domain.PositionPayableToCustomer ||
			command.To != domain.PositionRemitted {
			return none, OutcomeNotAccepted, nil
		}
		return batch.Ledger(), OutcomeRegistered, nil

	case domain.BasisDiscrepancy:
		itemID, err := domain.NewDiscrepancyItemID(command.Basis.String())
		if err != nil {
			return none, OutcomeNotAccepted, nil
		}
		item, found, err := handler.deps.Collection.LoadDiscrepancyItem(ctx, command.Tenant, itemID)
		if err != nil {
			return none, OutcomeUndecided, nil
		}
		if !found {
			return none, OutcomeBasisMissing, nil
		}
		ledger, outcome, err := handler.ledgerOfInstruction(ctx, command.Tenant, item.Instruction())
		if outcome != OutcomeRegistered || err != nil {
			return none, outcome, err
		}
		if !discrepancyPostingShapeHolds(command, item) {
			return none, OutcomeNotAccepted, nil
		}
		return ledger, OutcomeRegistered, nil

	case domain.BasisCorrection:
		originalID, err := domain.NewPostingID(command.Basis.String())
		if err != nil {
			return none, OutcomeNotAccepted, nil
		}
		original, found, err := handler.deps.Subledger.LoadPosting(ctx, command.Tenant, originalID)
		if err != nil {
			return none, OutcomeUndecided, nil
		}
		if !found {
			return none, OutcomeBasisMissing, nil
		}
		// 冲正就是原记账的反向同额，形状固定死。放开成自由移动之后，「冲正」会变成
		// 一个什么都能干、账上却看不出干了什么的通用口子。
		if command.From != original.To() ||
			command.To != original.From() ||
			command.AmountMinor != original.Amount().AmountMinor() {
			return none, OutcomeNotAccepted, nil
		}
		// 入账无法冲正：去向侧没有账外位置，反向那一笔落不下来。来源侧记错走追加
		// 更正事实，其账面处置尚无定论（本切片明确不猜，见 ADR-0082 的代价一节）。
		if !original.From().InsideLedger() {
			return none, OutcomeNotAccepted, nil
		}
		return original.Ledger(), OutcomeRegistered, nil

	default:
		return none, OutcomeNotAccepted, nil
	}
}

func (handler *Handler) ledgerOfInstruction(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.CollectionInstructionID,
) (domain.SubledgerKey, Outcome, error) {
	instruction, found, err := handler.deps.Collection.LoadInstruction(ctx, tenant, id)
	if err != nil {
		return domain.SubledgerKey{}, OutcomeUndecided, nil
	}
	if !found {
		return domain.SubledgerKey{}, OutcomeBasisMissing, nil
	}
	return instruction.Ledger(), OutcomeRegistered, nil
}

// factPostingShapeHolds 核凭代收事实的两种合法记账：
//   - 入账：外部来源 → 该事实的入账位置，全额一次。位置由事实的来源层级定，不由调用
//     方挑——挑错的那一次在库里看不出来（四层来源不得互相推导那条的落点）。
//   - 渠道回款到账：渠道在途 → 待清分，且只有真实到账那一层撑得起这一跳。
//
// 全额入账是有意的：一条事实入账一次，部分入账没有可表达的「余下部分」，而余下部分
// 无处可表达时，它会以「这条事实已处理」的样子消失。
func factPostingShapeHolds(command PostCommand, fact domain.CollectionFact) bool {
	if command.From == domain.PositionExternalSource {
		return command.To == fact.IntakePosition() &&
			command.AmountMinor == fact.Amount().AmountMinor()
	}
	return fact.Layer().SettlesToOperator() &&
		command.From == domain.PositionInTransitAtChannel &&
		command.To == domain.PositionAwaitingAllocation
}

// discrepancyPostingShapeHolds 核凭差异事项的两种合法记账：
//   - 差额落账：去向是该事项的落点（短款或溢款），金额与事项一致；
//   - 溢款归属确认：溢款 → 应付客户，金额可部分。
func discrepancyPostingShapeHolds(command PostCommand, item domain.DiscrepancyItem) bool {
	if command.To == item.SettlementPosition() {
		return command.AmountMinor == item.Amount().AmountMinor()
	}
	return item.Kind() == domain.DiscrepancySurplus &&
		command.From == domain.PositionSurplus &&
		command.To == domain.PositionPayableToCustomer
}

func samePosting(left, right domain.SubledgerPosting) bool {
	return left.ID() == right.ID() &&
		left.Ledger() == right.Ledger() &&
		left.From() == right.From() &&
		left.To() == right.To() &&
		left.Amount() == right.Amount() &&
		left.BasisKind() == right.BasisKind() &&
		left.Basis() == right.Basis() &&
		left.PostedAt().Equal(right.PostedAt())
}

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
// 代收指令、回汇批次、差异事项、认可的协议抵扣或原记账，每一条都自带它所属的那本账；
// 记账因此在结构上不可能记进别人的账。让调用方传键的写法在库里看不出错——一笔记错账的行，与记对的行
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

// PostSubledger 追加一笔记账：重放判定 → 解依据 → 定分户账与形状 → 锁这本账 →
// 锁内再判重放 → 按依据判重 → 读回账面 → 按指令的上限 → 余额守卫 → 落账。
//
// 重放判定必须先于一切按**当下状态**作答的门（余额守卫、批次收口）：那些门回答的是
// 「这笔新账现在能不能落」，而重放的真相是「这笔账已经落了」，答案只由在册记录定。
// 反过来排，重放一笔已把来源清空的记账会得到 UNDERFUNDED，其处置指引（等实收或先
// 清分，重跑就会成）对已落账的请求全是假话——已灌演示库上实测出过这一格。
//
// 按依据判重排在余额守卫之前，理由相同：换一个记账标识再凭同一条代收事实记账是同键
// 异内容（ADR-0271 决定三），它的真相由在册那一笔定，不由此刻账上够不够定。答已有
// 结果会把一次误操作伪装成成功，所以答冲突。库上同一口径的唯一约束兜住并发的那一格。
// 认可的协议抵扣一条至多落一笔，一笔记账至多被冲正一次（ADR-0298），都是同一口径：
// 重复到达的抵扣或撤回，哪怕账上余额恰好够扣，也答冲突而不是再落一笔。
//
// 取锁之后再判一次重放：同一条命令并发到达时，后到的那一路在锁外判重放时先到的还没
// 提交，读不到；等到锁、读到的账面已是落过的，这时再答余额不足就是在说一笔已经落账
// 的记账没落。
//
// 凭运营企业真实到账的移位，在重放判定与取锁之后、按指令的上限之前，再读一次它引用的已采用事实（采用事实门，
// admitArrivalMove）：登记之后结算一侧可能把事实更正小，容不下既有到账时移位答内容冲突，账面不动。
//
// 余额守卫必须在同一笔事务内读回账面再判（写口 RequireExecutor 保证有环境事务）：
// 隔着事务判出来的「够扣」在落账时可能已经不够了，而透支后的账面看上去与正常账面
// 没有区别。环境事务是 READ COMMITTED，同一份快照挡不住两笔并发的记账各自判够，所以
// 读回之前先取这本账的事务锁：后到的那一笔等先到的提交之后才读，读到的是落过的账面。
// 锁放在解依据之后，是因为账由依据推出；重放判定在锁外，它只看在册记录，并发重放
// 由写口的 DO NOTHING 与随后的读回兜住。按指令的上限先于按账的守卫：前者更紧，且它答的未受理（溢款报少）是请求
// 本身的错，不该被账面答案盖住。
func (handler *Handler) PostSubledger(ctx context.Context, command PostCommand) (Outcome, error) {
	if blank(command.Tenant) ||
		strings.TrimSpace(command.ID.String()) == "" ||
		strings.TrimSpace(command.Basis.String()) == "" ||
		command.AmountMinor <= 0 ||
		command.PostedAt.IsZero() {
		return OutcomeNotAccepted, nil
	}

	recorded, found, err := handler.deps.Subledger.LoadPosting(ctx, command.Tenant, command.ID)
	if err != nil {
		return OutcomeUndecided, nil
	}
	if found {
		if !postingMatchesCommand(recorded, command) {
			return OutcomeContentConflict, nil
		}
		return OutcomeExisting, nil
	}

	basis, outcome, err := handler.resolveBasisForPosting(ctx, command)
	if outcome != OutcomeRegistered || err != nil {
		return outcome, err
	}

	amount, err := domain.NewMoney(basis.ledger.Currency(), command.AmountMinor)
	if err != nil {
		return OutcomeNotAccepted, nil
	}
	posting, err := domain.RecordSubledgerPosting(domain.SubledgerPostingSpec{
		ID:        command.ID,
		Ledger:    basis.ledger,
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

	if err := handler.deps.Subledgers.LockLedger(ctx, command.Tenant, basis.ledger); err != nil {
		return OutcomeUndecided, nil
	}

	settled, found, err := handler.deps.Subledger.LoadPosting(ctx, command.Tenant, command.ID)
	if err != nil {
		return OutcomeUndecided, nil
	}
	if found {
		if !samePosting(settled, posting) {
			return OutcomeContentConflict, nil
		}
		return OutcomeExisting, nil
	}

	if basis.corrects != nil {
		reversed, found, err := handler.deps.Subledger.LoadCorrectionOf(ctx, command.Tenant, *basis.corrects)
		if err != nil {
			return OutcomeUndecided, nil
		}
		if found && reversed.ID() != command.ID {
			return OutcomeContentConflict, nil
		}
	}

	if basis.factKnown && basis.fact.Layer().SettlesToOperator() {
		// 真实到账的移位：登记时撑得住，此刻仍要被它引用的已采用事实撑住（admitArrivalMove）。在锁内、余额守卫之前：
		// 事实被更正小了是这条到账的依据变了，不是账上钱不够，答案不该被余额不足盖住。
		if outcome := handler.admitArrivalMove(ctx, command.Tenant, basis.fact); outcome != OutcomeRegistered {
			return outcome, nil
		}
	}

	var principal *domain.InstructionPrincipal
	if basis.instructionKnown && gatedByInstruction(posting) {
		derived, outcome := handler.loadInstructionPrincipal(ctx, command.Tenant, basis.instruction)
		if outcome != OutcomeRegistered {
			return outcome, nil
		}
		if basis.factKnown {
			if used, found := derived.PostingOf(basis.fact.ID()); found && used != command.ID {
				return OutcomeContentConflict, nil
			}
		}
		if basis.offsetKnown {
			if used, found := derived.OffsetPostingOf(basis.offset.ID()); found && used != command.ID {
				return OutcomeContentConflict, nil
			}
		}
		principal = &derived
	}

	balance, opened, err := handler.deps.Subledger.LoadBalance(ctx, command.Tenant, basis.ledger)
	if err != nil {
		return OutcomeUndecided, nil
	}
	if !opened {
		// 账没开立就没有受托依据。这不是输入不合法，是缺上游那一笔。
		return OutcomeBasisMissing, nil
	}
	if principal != nil {
		if outcome := admissionOutcome(principal.Admit(posting)); outcome != OutcomeRegistered {
			return outcome, nil
		}
	}
	if outcome := admissionOutcome(balance.Admit(posting)); outcome != OutcomeRegistered {
		return outcome, nil
	}

	saved, err := handler.deps.Subledgers.AppendPosting(ctx, command.Tenant, posting)
	if err != nil {
		return OutcomeUndecided, nil
	}
	if saved == ports.SaveRegistered {
		return OutcomeRegistered, nil
	}

	// 撞键而按标识读不回同一笔，就是撞上了按依据的唯一约束：并发的另一笔已凭同一条
	// 代收事实落账，同样是冲突。
	existing, found, err := handler.deps.Subledger.LoadPosting(ctx, command.Tenant, command.ID)
	if err != nil {
		return OutcomeUndecided, nil
	}
	if !found || !samePosting(existing, posting) {
		return OutcomeContentConflict, nil
	}
	return OutcomeExisting, nil
}

// admissionOutcome 把余额与上限两道门的判词译成答案；OutcomeRegistered 即放行。
func admissionOutcome(err error) Outcome {
	switch {
	case err == nil:
		return OutcomeRegistered
	case errors.Is(err, domain.ErrPositionUnderfunded):
		return OutcomeUnderfunded
	default:
		return OutcomeNotAccepted
	}
}

// gatedByInstruction 答这笔记账要不要过按指令的那一层：凭代收事实的（判重）、落入
// `短款`或`溢款`的（差额按指令算），以及一切自`渠道在途`移出的（移位上限按指令算，
// 认可抵扣落`已抵扣`也在其中）。清分、汇付与别的冲正不在此列——它们的来源位置不按
// 指令分账，按账的余额守卫就够。
func gatedByInstruction(posting domain.SubledgerPosting) bool {
	return posting.BasisKind() == domain.BasisCollectionFact ||
		posting.To() == domain.PositionShortfall ||
		posting.To() == domain.PositionSurplus ||
		posting.From() == domain.PositionInTransitAtChannel
}

func (handler *Handler) loadInstructionPrincipal(
	ctx context.Context,
	tenant domain.TenantID,
	instruction domain.CollectionInstruction,
) (domain.InstructionPrincipal, Outcome) {
	facts, err := handler.deps.Collection.LoadInstructionFacts(ctx, tenant, instruction.ID())
	if err != nil {
		return domain.InstructionPrincipal{}, OutcomeUndecided
	}
	postings, err := handler.deps.Subledger.LoadInstructionPostings(ctx, tenant, instruction)
	if err != nil {
		return domain.InstructionPrincipal{}, OutcomeUndecided
	}
	principal, err := domain.DeriveInstructionPrincipal(instruction, facts, postings)
	if err != nil {
		// 读口交回别的指令的事实或别本账的记账是读口坏了，不是业务答案。
		return domain.InstructionPrincipal{}, OutcomeUndecided
	}
	return principal, OutcomeRegistered
}

// postingBasis 是解出来的依据：记账所在的分户账，以及它追溯到的代收指令、代收事实或
// 认可的协议抵扣（有的话），冲正则带着被冲正的那一笔。批次依据的汇付记账不追溯到单条
// 指令。
type postingBasis struct {
	ledger           domain.SubledgerKey
	instruction      domain.CollectionInstruction
	instructionKnown bool
	fact             domain.CollectionFact
	factKnown        bool
	offset           domain.AgreedOffset
	offsetKnown      bool
	corrects         *domain.PostingID
}

// resolveBasisForPosting 按依据种类解出分户账，并核这笔记账的形状是不是该依据允许的
// 那一种。第二个返回值为 OutcomeRegistered 表示解析通过（此时还没落账），其余格是
// 已经定案的答案。
func (handler *Handler) resolveBasisForPosting(
	ctx context.Context,
	command PostCommand,
) (postingBasis, Outcome, error) {
	none := postingBasis{}
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
		instruction, outcome := handler.loadInstruction(ctx, command.Tenant, fact.Instruction())
		if outcome != OutcomeRegistered {
			return none, outcome, nil
		}
		if !factPostingShapeHolds(command, fact) {
			return none, OutcomeNotAccepted, nil
		}
		return postingBasis{
			ledger: instruction.Ledger(), instruction: instruction, instructionKnown: true,
			fact: fact, factKnown: true,
		}, OutcomeRegistered, nil

	case domain.BasisAllocation:
		instructionID, err := domain.NewCollectionInstructionID(command.Basis.String())
		if err != nil {
			return none, OutcomeNotAccepted, nil
		}
		instruction, outcome := handler.loadInstruction(ctx, command.Tenant, instructionID)
		if outcome != OutcomeRegistered {
			return none, outcome, nil
		}
		// 清分是唯一一跳：待清分 → 应付客户。别的方向要么该走别的依据，要么是冲正。
		if command.From != domain.PositionAwaitingAllocation ||
			command.To != domain.PositionPayableToCustomer {
			return none, OutcomeNotAccepted, nil
		}
		return postingBasis{
			ledger: instruction.Ledger(), instruction: instruction, instructionKnown: true,
		}, OutcomeRegistered, nil

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
		return postingBasis{ledger: batch.Ledger()}, OutcomeRegistered, nil

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
		instruction, outcome := handler.loadInstruction(ctx, command.Tenant, item.Instruction())
		if outcome != OutcomeRegistered {
			return none, outcome, nil
		}
		if !discrepancyPostingShapeHolds(command, item) {
			return none, OutcomeNotAccepted, nil
		}
		return postingBasis{
			ledger: instruction.Ledger(), instruction: instruction, instructionKnown: true,
		}, OutcomeRegistered, nil

	case domain.BasisAgreedOffset:
		offsetID, err := domain.NewAgreedOffsetID(command.Basis.String())
		if err != nil {
			return none, OutcomeNotAccepted, nil
		}
		offset, found, err := handler.deps.Collection.LoadAgreedOffset(ctx, command.Tenant, offsetID)
		if err != nil {
			return none, OutcomeUndecided, nil
		}
		if !found {
			return none, OutcomeBasisMissing, nil
		}
		instruction, outcome := handler.loadInstruction(ctx, command.Tenant, offset.Instruction())
		if outcome != OutcomeRegistered {
			return none, outcome, nil
		}
		if !agreedOffsetPostingShapeHolds(command, offset) {
			return none, OutcomeNotAccepted, nil
		}
		return postingBasis{
			ledger: instruction.Ledger(), instruction: instruction, instructionKnown: true,
			offset: offset, offsetKnown: true,
		}, OutcomeRegistered, nil

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
		resolved := postingBasis{ledger: original.Ledger(), corrects: &originalID}
		// 自`渠道在途`移出的冲正（冲正一笔冲正，把移回在途的到账再移出去）同样按指令
		// 判上限，否则它能把同账别的指令的在途款移走。
		if command.From == domain.PositionInTransitAtChannel {
			instruction, known, outcome := handler.instructionBehind(ctx, command.Tenant, original)
			if outcome != OutcomeRegistered {
				return none, outcome, nil
			}
			resolved.instruction, resolved.instructionKnown = instruction, known
		}
		return resolved, OutcomeRegistered, nil

	default:
		return none, OutcomeNotAccepted, nil
	}
}

func (handler *Handler) loadInstruction(
	ctx context.Context,
	tenant domain.TenantID,
	id domain.CollectionInstructionID,
) (domain.CollectionInstruction, Outcome) {
	instruction, found, err := handler.deps.Collection.LoadInstruction(ctx, tenant, id)
	if err != nil {
		return domain.CollectionInstruction{}, OutcomeUndecided
	}
	if !found {
		return domain.CollectionInstruction{}, OutcomeBasisMissing
	}
	return instruction, OutcomeRegistered
}

// instructionBehind 沿冲正链找到一笔记账最终追溯到的代收指令。冲正只能指向已在册的
// 更早一笔，链上不会成环；仍记下走过的标识，读口交回坏数据时停在未决而不是转圈。
func (handler *Handler) instructionBehind(
	ctx context.Context,
	tenant domain.TenantID,
	posting domain.SubledgerPosting,
) (domain.CollectionInstruction, bool, Outcome) {
	none := domain.CollectionInstruction{}
	seen := map[domain.PostingID]bool{}
	for posting.BasisKind() == domain.BasisCorrection {
		if seen[posting.ID()] {
			return none, false, OutcomeUndecided
		}
		seen[posting.ID()] = true
		originalID, err := domain.NewPostingID(posting.Basis().String())
		if err != nil {
			return none, false, OutcomeUndecided
		}
		original, found, err := handler.deps.Subledger.LoadPosting(ctx, tenant, originalID)
		if err != nil || !found {
			return none, false, OutcomeUndecided
		}
		posting = original
	}

	var instructionID domain.CollectionInstructionID
	switch posting.BasisKind() {
	case domain.BasisCollectionFact:
		factID, err := domain.NewCollectionFactID(posting.Basis().String())
		if err != nil {
			return none, false, OutcomeUndecided
		}
		fact, found, err := handler.deps.Collection.LoadCollectionFact(ctx, tenant, factID)
		if err != nil || !found {
			return none, false, OutcomeUndecided
		}
		instructionID = fact.Instruction()
	case domain.BasisDiscrepancy:
		itemID, err := domain.NewDiscrepancyItemID(posting.Basis().String())
		if err != nil {
			return none, false, OutcomeUndecided
		}
		item, found, err := handler.deps.Collection.LoadDiscrepancyItem(ctx, tenant, itemID)
		if err != nil || !found {
			return none, false, OutcomeUndecided
		}
		instructionID = item.Instruction()
	case domain.BasisAgreedOffset:
		offsetID, err := domain.NewAgreedOffsetID(posting.Basis().String())
		if err != nil {
			return none, false, OutcomeUndecided
		}
		offset, found, err := handler.deps.Collection.LoadAgreedOffset(ctx, tenant, offsetID)
		if err != nil || !found {
			return none, false, OutcomeUndecided
		}
		instructionID = offset.Instruction()
	case domain.BasisAllocation:
		id, err := domain.NewCollectionInstructionID(posting.Basis().String())
		if err != nil {
			return none, false, OutcomeUndecided
		}
		instructionID = id
	default:
		return none, false, OutcomeRegistered
	}
	instruction, outcome := handler.loadInstruction(ctx, tenant, instructionID)
	if outcome != OutcomeRegistered {
		// 在册记账的依据读不回来是账面坏了，不是缺上游那一笔。
		return none, false, OutcomeUndecided
	}
	return instruction, true, OutcomeRegistered
}

// factPostingShapeHolds 核凭代收事实的两种合法记账，金额都等于事实金额、全额一次：
//   - 入账：外部来源 → 该事实的入账位置。只有渠道代收报告有入账位置（`渠道在途`），
//     收件人付款、渠道回款通知与真实到账凭它入账一律不受理（ADR-0271 决定一）。
//   - 到账移位：渠道在途 → 待清分，只有真实到账那一层撑得起（决定二）。
//
// 全额是有意的：一条事实至多支撑一笔记账，部分记账没有可表达的「余下部分」，而余下
// 部分无处可表达时，它会以「这条事实已处理」的样子消失。到账多于该指令在途款时整笔
// 停在余额不足，不先移一部分，也是这个道理。
func factPostingShapeHolds(command PostCommand, fact domain.CollectionFact) bool {
	if command.AmountMinor != fact.Amount().AmountMinor() {
		return false
	}
	if command.From == domain.PositionExternalSource {
		intake := fact.IntakePosition()
		return intake.InsideLedger() && command.To == intake
	}
	return fact.Layer().SettlesToOperator() &&
		command.From == domain.PositionInTransitAtChannel &&
		command.To == domain.PositionAwaitingAllocation
}

// discrepancyPostingShapeHolds 核凭差异事项的两种合法记账：
//   - 差额落账：该事项的来源位置 → 落点（短款自`渠道在途`、溢款自`待清分`），金额与
//     事项一致（ADR-0271 决定四、五）；差额按指令够不够、溢款对不对由按指令的上限判；
//   - 溢款归属确认：溢款 → 应付客户，金额可部分。
func discrepancyPostingShapeHolds(command PostCommand, item domain.DiscrepancyItem) bool {
	if command.To == item.SettlementPosition() {
		return command.From == item.SettlementSource() &&
			command.AmountMinor == item.Amount().AmountMinor()
	}
	return item.Kind() == domain.DiscrepancySurplus &&
		command.From == domain.PositionSurplus &&
		command.To == domain.PositionPayableToCustomer
}

// agreedOffsetPostingShapeHolds 核凭认可的协议抵扣的唯一一种形：`渠道在途` → `已抵扣`，
// 金额等于登记的认可额、全额一次。部分落账没有可表达的「余下部分」，余下的抵扣额会以
// 「这条认可已处理」的样子消失；认可额变了走新的认可。抵扣够不够、到账是否已先移位由
// 按指令的上限判。
func agreedOffsetPostingShapeHolds(command PostCommand, offset domain.AgreedOffset) bool {
	return command.From == domain.PositionInTransitAtChannel &&
		command.To == domain.PositionOffsetApplied &&
		command.AmountMinor == offset.Amount().AmountMinor()
}

// postingMatchesCommand 按命令携带的格比对在册记账。分户账与币种不参与比对——它们
// 不是命令输入，由依据推出，而依据在册后不可变：命令格逐一相同时，派生格不可能不同。
func postingMatchesCommand(recorded domain.SubledgerPosting, command PostCommand) bool {
	return recorded.From() == command.From &&
		recorded.To() == command.To &&
		recorded.Amount().AmountMinor() == command.AmountMinor &&
		recorded.BasisKind() == command.BasisKind &&
		recorded.Basis() == command.Basis &&
		recorded.PostedAt().Equal(command.PostedAt)
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

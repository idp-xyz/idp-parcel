package application

import (
	"context"
	"errors"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/ports"
)

// 运营企业真实到账的登记与移位，都以 settlement-accounting 已采用的外部资金事实为证据（ADR-0274、ADR-0330）。
//
// 一笔银行回款只在结算里铸一次：到账不自铸资金事实，证据引用指向结算已采用、且已整条认作代收本金的那条事实，登记时按引用
// 读回它此刻的链头——版本钉在到账上留痕，金额才是判据。判据有三条，缺一不登：币种与事实、与指令所属的账相同；引用同一条
// 事实的各条到账金额之和不超过事实此刻的金额；一条事实对一条指令至多支撑一条到账。移位（`渠道在途` → `待清分`）之前
// 再读一次：登记时撑得住的到账，之后结算把事实更正小了，移位答内容冲突、账面不动。

// acceptArrival 登记一条运营企业真实到账：重放判定 → 币种门 → 取本上下文的事实锁 → 锁内再判一次重放 → 读结算
// （对结算一侧事实身份行取共享锁）→ 读回此事实名下已有的到账判合计与每指令至多一条 → 钉版本 → 落册。
//
// 重放判定必须先于一切按**此刻状态**作答的门（读结算、合计、币种之外的判据）：那些门答的是「这条新到账现在能不能登」，
// 而重放的真相是「这条到账已经登了」，答案只由在册记录定。反过来排，结算把事实更正小了之后，重放一条早已登记的到账会得到
// 内容冲突，其处置指引（去核对到账或更正）对已登记的请求全是假话。币种门排在读结算之前：币种对不上是请求本身的错，
// 不该去占结算的共享锁。
//
// 取锁之后再判一次重放、再读合计：两路并发引用同一条事实的到账各自读到同一份已有到账、各自判够，落下去合计就超过事实
// 金额；锁把它们排成先后，后到的一路读到的是先到那路已提交的合计。读结算在取本上下文的锁之后，次序与端口头注一致。
func (handler *Handler) acceptArrival(
	ctx context.Context,
	command AcceptFactCommand,
	instruction domain.CollectionInstruction,
) (Outcome, error) {
	if outcome, settled, err := handler.arrivalReplay(ctx, command); settled {
		return outcome, err
	}
	reference, referencing := command.Fact.AdoptedFundsReference()
	if !referencing {
		return OutcomeNotAccepted, nil
	}
	if command.Fact.Amount().Currency() != instruction.Ledger().Currency() {
		return OutcomeNotAccepted, nil
	}
	if handler.deps.AdoptedFunds == nil {
		return OutcomeUndecided, nil
	}

	if err := handler.deps.Collections.LockAdoptedFundsReferences(ctx, command.Tenant, reference); err != nil {
		return OutcomeUndecided, nil
	}
	if outcome, settled, err := handler.arrivalReplay(ctx, command); settled {
		return outcome, err
	}

	funds, found, err := handler.deps.AdoptedFunds.LockAdoptedFunds(ctx, command.Tenant, reference)
	if err != nil {
		return OutcomeUndecided, nil
	}
	if !found {
		return OutcomeBasisMissing, nil
	}
	referenced, err := handler.deps.Collection.LoadAdoptedFundsReferences(ctx, command.Tenant, reference)
	if err != nil {
		return OutcomeUndecided, nil
	}
	if err := domain.AdmitAdoptedFundsReference(funds, referenced, command.Fact); err != nil {
		return adoptedFundsOutcome(err), nil
	}

	pinned, err := command.Fact.PinnedToAdoptedFunds(funds.Version())
	if err != nil {
		return OutcomeNotAccepted, nil
	}
	saved, err := handler.deps.Collections.AcceptCollectionFact(ctx, command.Tenant, pinned)
	if err != nil {
		return OutcomeUndecided, nil
	}
	if saved == ports.SaveRegistered {
		return OutcomeRegistered, nil
	}
	// 撞键而按标识读不回同一条，就是撞上了（采用事实，指令）的唯一约束：并发的另一条已为这条指令登记了到账。
	return handler.arrivalAfterCollision(ctx, command)
}

// arrivalReplay 按事实标识读回既有登记：settled 为真即已定案（重放答已在册，同标识异内容答内容冲突，读不动答未决）。
// 比对只看到账自己的内容，不看钉的版本——版本是登记时读到的留痕，不是请求的一部分。
func (handler *Handler) arrivalReplay(ctx context.Context, command AcceptFactCommand) (Outcome, bool, error) {
	existing, found, err := handler.deps.Collection.LoadCollectionFact(ctx, command.Tenant, command.Fact.ID())
	if err != nil {
		return OutcomeUndecided, true, nil
	}
	if !found {
		return OutcomeInvalid, false, nil
	}
	if !sameFact(existing, command.Fact) {
		return OutcomeContentConflict, true, nil
	}
	return OutcomeExisting, true, nil
}

func (handler *Handler) arrivalAfterCollision(ctx context.Context, command AcceptFactCommand) (Outcome, error) {
	existing, found, err := handler.deps.Collection.LoadCollectionFact(ctx, command.Tenant, command.Fact.ID())
	if err != nil {
		return OutcomeUndecided, nil
	}
	if !found || !sameFact(existing, command.Fact) {
		return OutcomeContentConflict, nil
	}
	return OutcomeExisting, nil
}

// adoptedFundsOutcome 把采用事实门的判词译成答案：币种不同是请求本身的错（改请求），超额与同一指令已有到账都要人回头
// 核对既有登记（内容冲突）。
func adoptedFundsOutcome(err error) Outcome {
	switch {
	case errors.Is(err, domain.ErrAdoptedFundsCurrency):
		return OutcomeNotAccepted
	case errors.Is(err, domain.ErrAdoptedFundsExceeded), errors.Is(err, domain.ErrAdoptedFundsInstructionTaken):
		return OutcomeContentConflict
	default:
		return OutcomeNotAccepted
	}
}

// admitArrivalMove 是移位记账之前的采用事实门：这条到账登记时撑得住，此刻仍要被它引用的事实撑住——结算可能在登记
// 之后把事实更正小了（ADR-0285 照采，不拒）。读结算对事实身份行取共享锁，挡住读与落账之间插进来的更正。
// 返回 OutcomeRegistered 即放行，其余格是已定案的答案：没有钉版本的到账（按旧口径登记的存量）与结算一侧读不到这条事实
// 答依据不在册，事实容不下既有到账答内容冲突，读不动或没装配答未决。
func (handler *Handler) admitArrivalMove(
	ctx context.Context,
	tenant domain.TenantID,
	arrival domain.CollectionFact,
) Outcome {
	reference, referencing := arrival.AdoptedFundsReference()
	if _, pinned := arrival.AdoptedFundsVersion(); !referencing || !pinned {
		return OutcomeBasisMissing
	}
	if handler.deps.AdoptedFunds == nil {
		return OutcomeUndecided
	}
	funds, found, err := handler.deps.AdoptedFunds.LockAdoptedFunds(ctx, tenant, reference)
	if err != nil {
		return OutcomeUndecided
	}
	if !found {
		return OutcomeBasisMissing
	}
	referenced, err := handler.deps.Collection.LoadAdoptedFundsReferences(ctx, tenant, reference)
	if err != nil {
		return OutcomeUndecided
	}
	if err := domain.AdoptedFundsCover(funds, referenced); err != nil {
		return adoptedFundsOutcome(err)
	}
	return OutcomeRegistered
}

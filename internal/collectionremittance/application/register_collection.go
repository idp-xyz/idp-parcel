package application

import (
	"context"
	"strings"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/ports"
)

// 代收面三本册子的登记用例：代收指令、代收事实、差异事项。
//
// 冲突判定一律靠读回：写口 DO NOTHING 只答`已登记`，同键的内容是不是同一份，由这里
// 读回既有登记逐字段比。把比对放在编排而不是 SQL 里，是为了让「重放同一份」与「换了
// 内容」在用例答案上分得开（ADR-0031 的两格代数换来的就是这个）。

// Deps 是本上下文全部用例的依赖：三对写口读口。它们成对出现不是对称美——只有写口时
// 「已在册」永远说不出是重放还是改内容。
type Deps struct {
	Collections ports.CollectionRegistry
	Collection  ports.CollectionView
	Subledgers  ports.SubledgerRegistry
	Subledger   ports.SubledgerView
	Remittances ports.RemittanceRegistry
	Remittance  ports.RemittanceView
}

type Handler struct {
	deps Deps
}

func NewHandler(deps Deps) *Handler {
	return &Handler{deps: deps}
}

// RegisterInstructionCommand 携带一次代收指令登记。
type RegisterInstructionCommand struct {
	Tenant      domain.TenantID
	Instruction domain.CollectionInstruction
}

// RegisterInstruction 登记一条代收指令。已在册时读回逐字段比：全同是重放，任何一格
// 不同都是冲突——指令是清分归属的依据，被顶替会让已依它落账的记账指向一份不存在过的
// 义务，换内容走登记新指令，不走覆盖。
func (handler *Handler) RegisterInstruction(
	ctx context.Context,
	command RegisterInstructionCommand,
) (Outcome, error) {
	if blank(command.Tenant) || strings.TrimSpace(command.Instruction.ID().String()) == "" {
		return OutcomeNotAccepted, nil
	}

	saved, err := handler.deps.Collections.RegisterInstruction(ctx, command.Tenant, command.Instruction)
	if err != nil {
		return OutcomeUndecided, nil
	}
	if saved == ports.SaveRegistered {
		return OutcomeRegistered, nil
	}

	existing, found, err := handler.deps.Collection.LoadInstruction(
		ctx, command.Tenant, command.Instruction.ID())
	if err != nil {
		return OutcomeUndecided, nil
	}
	if !found || !sameInstruction(existing, command.Instruction) {
		return OutcomeContentConflict, nil
	}
	return OutcomeExisting, nil
}

// AcceptFactCommand 携带一次代收事实接受。
type AcceptFactCommand struct {
	Tenant domain.TenantID
	Fact   domain.CollectionFact
}

// AcceptFact 接受一条代收事实。指名的代收指令必须已在册——没有指令就没有代收义务，
// 无来源的本金不许进库。库上那道外键也拦得住它，但撞外键在写口是「依赖故障」那一格，
// 而这明明是一个说得清的业务答案：先把指令登进来。
func (handler *Handler) AcceptFact(ctx context.Context, command AcceptFactCommand) (Outcome, error) {
	if blank(command.Tenant) || strings.TrimSpace(command.Fact.ID().String()) == "" {
		return OutcomeNotAccepted, nil
	}

	_, found, err := handler.deps.Collection.LoadInstruction(
		ctx, command.Tenant, command.Fact.Instruction())
	if err != nil {
		return OutcomeUndecided, nil
	}
	if !found {
		return OutcomeBasisMissing, nil
	}

	saved, err := handler.deps.Collections.AcceptCollectionFact(ctx, command.Tenant, command.Fact)
	if err != nil {
		return OutcomeUndecided, nil
	}
	if saved == ports.SaveRegistered {
		return OutcomeRegistered, nil
	}

	existing, found, err := handler.deps.Collection.LoadCollectionFact(
		ctx, command.Tenant, command.Fact.ID())
	if err != nil {
		return OutcomeUndecided, nil
	}
	if !found || !sameFact(existing, command.Fact) {
		return OutcomeContentConflict, nil
	}
	return OutcomeExisting, nil
}

// RegisterDiscrepancyCommand 携带一次差异事项登记。
type RegisterDiscrepancyCommand struct {
	Tenant domain.TenantID
	Item   domain.DiscrepancyItem
}

// RegisterDiscrepancy 登记一项差异事项。它只登事项，**不落账**：差额进`短款`或`溢款`
// 要另有一笔以本事项为依据的记账（PostSubledger 的差异门）。分两步不是麻烦——一步做完
// 的那种写法在账上看不出差额是凭什么落的。
func (handler *Handler) RegisterDiscrepancy(
	ctx context.Context,
	command RegisterDiscrepancyCommand,
) (Outcome, error) {
	if blank(command.Tenant) || strings.TrimSpace(command.Item.ID().String()) == "" {
		return OutcomeNotAccepted, nil
	}

	_, found, err := handler.deps.Collection.LoadInstruction(
		ctx, command.Tenant, command.Item.Instruction())
	if err != nil {
		return OutcomeUndecided, nil
	}
	if !found {
		return OutcomeBasisMissing, nil
	}

	saved, err := handler.deps.Collections.RegisterDiscrepancyItem(ctx, command.Tenant, command.Item)
	if err != nil {
		return OutcomeUndecided, nil
	}
	if saved == ports.SaveRegistered {
		return OutcomeRegistered, nil
	}

	existing, found, err := handler.deps.Collection.LoadDiscrepancyItem(
		ctx, command.Tenant, command.Item.ID())
	if err != nil {
		return OutcomeUndecided, nil
	}
	if !found || !sameDiscrepancy(existing, command.Item) {
		return OutcomeContentConflict, nil
	}
	return OutcomeExisting, nil
}

func blank(tenant domain.TenantID) bool {
	return strings.TrimSpace(tenant.String()) == ""
}

// 逐字段比而不是用 ==：结构里带 time.Time，`==` 会把单调钟与时区读数算进去，于是
// 同一时刻的两份读数可能不相等，而那与「内容改了」在答案上分不开。
func sameInstruction(left, right domain.CollectionInstruction) bool {
	return left.ID() == right.ID() &&
		left.Parcel() == right.Parcel() &&
		left.Requirement() == right.Requirement() &&
		left.Ledger() == right.Ledger() &&
		left.Amount() == right.Amount() &&
		left.InstructedAt().Equal(right.InstructedAt())
}

func sameFact(left, right domain.CollectionFact) bool {
	return left.ID() == right.ID() &&
		left.Instruction() == right.Instruction() &&
		left.Layer() == right.Layer() &&
		left.Evidence() == right.Evidence() &&
		left.Amount() == right.Amount() &&
		left.OccurredAt().Equal(right.OccurredAt())
}

func sameDiscrepancy(left, right domain.DiscrepancyItem) bool {
	return left.ID() == right.ID() &&
		left.Instruction() == right.Instruction() &&
		left.Kind() == right.Kind() &&
		left.Amount() == right.Amount() &&
		left.Basis() == right.Basis() &&
		left.ObservedAt().Equal(right.ObservedAt())
}

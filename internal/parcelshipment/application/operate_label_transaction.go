package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// LabelTransactionOutcome 是面单交易写侧五步共用的应用结果代数。
//
// **按调用方的恢复动作分格，不按聚合拒绝的原因分格**（ADR-0029）。五步各写一套取值会让同一个
// 恢复动作在五处各有一个名字，而调用方要做的事其实只有这几种；反过来，恢复动作不同的两格
// 一律不合并——领域已经把这条守在错误上（`ErrInvalidLabelTransaction` 与
// `ErrLabelTransactionStateNotAdmitted` 分立，注释写明「压成一格，调用方会对着一份完好的请求
// 反复重发」），本层照搬那条分界，不重新发明。
type LabelTransactionOutcome uint8

const (
	LabelTransactionOutcomeInvalid LabelTransactionOutcome = iota
	// LabelTransactionApplied：这一步落下了，继续下一步。
	LabelTransactionApplied
	// LabelTransactionAlreadyApplied：重放。既有交易原样读回，**不再向渠道发第二次**。
	// 它与`已落下`分开，是因为调用方据以决定要不要接着发起渠道调用——按 ADR-0090，
	// 出向调用在`答案未确定`时不得重发，而重放正是那条路径回来时最常落的一格。
	LabelTransactionAlreadyApplied
	// LabelTransactionStepNotAdmitted：输入没问题，是这笔交易此刻不在允许这一步的状态上。
	// 恢复动作是去看它停在哪一格、等渠道结果，不是改参数重试。
	LabelTransactionStepNotAdmitted
	// LabelTransactionPriorNotFinalized：被指的原交易还没定案。它不并进上一格，理由与领域
	// 那条错误单列同一条——对结果不确定的交易发起重试或替代，就是把它按失败处理了。
	LabelTransactionPriorNotFinalized
	// LabelTransactionNotAccepted：输入本身立不起来（缺项、覆盖对不上、时间倒挂）。
	// 恢复动作是改输入重来。
	LabelTransactionNotAccepted
	// LabelTransactionWriteConflict：预期版本对不上，别人先写了一笔。重读再重放。
	LabelTransactionWriteConflict
)

func (outcome LabelTransactionOutcome) String() string {
	switch outcome {
	case LabelTransactionApplied:
		return "APPLIED"
	case LabelTransactionAlreadyApplied:
		return "ALREADY_APPLIED"
	case LabelTransactionStepNotAdmitted:
		return "STATE_NOT_ADMITTED"
	case LabelTransactionPriorNotFinalized:
		return "PRIOR_NOT_FINALIZED"
	case LabelTransactionNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case LabelTransactionWriteConflict:
		return "REVISION_CONFLICT"
	default:
		return ""
	}
}

// LabelTransactionResult 交回本步之后交易停在哪里。它带回聚合本体而不只是状态：调用方下一步
// 多半要接着操作同一笔交易，让它按标识再读一次会在两次读之间开一个窗口。
type LabelTransactionResult struct {
	outcome     LabelTransactionOutcome
	transaction domain.LabelTransaction
	hasValue    bool
}

func (result LabelTransactionResult) Outcome() LabelTransactionOutcome {
	return result.outcome
}

// Transaction 交回本步之后的交易；重放时交回既有那一笔。输入未受理与版本冲突时缺席——
// 前者没有形成任何交易，后者手上这份已经陈旧，交回去只会被当成最新的接着改。
func (result LabelTransactionResult) Transaction() (domain.LabelTransaction, bool) {
	return result.transaction, result.hasValue
}

func labelTransactionApplied(transaction domain.LabelTransaction) LabelTransactionResult {
	return LabelTransactionResult{outcome: LabelTransactionApplied, transaction: transaction, hasValue: true}
}

func labelTransactionAlreadyApplied(transaction domain.LabelTransaction) LabelTransactionResult {
	return LabelTransactionResult{outcome: LabelTransactionAlreadyApplied, transaction: transaction, hasValue: true}
}

// labelTransactionRefused 交回一次拒绝。transaction 为 nil 表示手上根本没有交易可交——
// 建立那一步被拒时就是这样，交回一个零值聚合会让调用方以为读到了一笔状态为空的交易。
func labelTransactionRefused(outcome LabelTransactionOutcome, transaction *domain.LabelTransaction) LabelTransactionResult {
	if transaction == nil {
		return LabelTransactionResult{outcome: outcome}
	}
	return LabelTransactionResult{outcome: outcome, transaction: *transaction, hasValue: true}
}

// EstablishLabelTransactionCommand 建立一笔面单交易。
//
// 覆盖范围与七类依据引用逐项由聚合构造门把守，本层不重复校验——重复一遍就有了第二处口径，
// 而它必然先于聚合那处过期。PriorTransactionID 留零值即首笔交易。
type EstablishLabelTransactionCommand struct {
	Tenant                 domain.TenantID
	TransactionID          domain.LabelTransactionID
	CoveredParcels         []domain.DeclaredParcelID
	ChannelAccount         domain.ChannelAccountReference
	AccountHolder          domain.ChannelAccountHolderReference
	ServiceProvider        domain.ChannelServiceProviderReference
	SettlementCounterparty domain.SettlementCounterpartyReference
	Contract               domain.ChannelContractReference
	Rate                   domain.ChannelRateReference
	ResponsibilityBasis    domain.ResponsibilityBasisSnapshotReference
	// PriorTransactionID 与 PriorLinkKind 成对给出：重试或替代关系指回的那一笔。
	// 关系由本层按原交易**本体**建立（聚合要求原交易已定案），调用方给标识即可。
	PriorTransactionID domain.LabelTransactionID
	PriorLinkKind      domain.LabelTransactionLinkKind
}

// SubmitLabelTransactionCommand 把已建立的交易推到`已提交渠道`。
type SubmitLabelTransactionCommand struct {
	Tenant        domain.TenantID
	TransactionID domain.LabelTransactionID
}

// MarkLabelResultUncertainCommand 记录「暂时无法确认渠道是否受理」。
//
// 它不带时间，与聚合同理：不确定是本上下文对渠道的一次观察结论，没有对应的渠道业务事实时间。
type MarkLabelResultUncertainCommand struct {
	Tenant        domain.TenantID
	TransactionID domain.LabelTransactionID
}

// RecordLabelChannelResultCommand 一次写下交易级与包裹级两层结果。
//
// ObservedAt 由调用方给而不是本层铸：它是**渠道**形成结果的业务时间，属外部事实
// （ADR-0023 同一条分界——本仓自己的动作自己铸时间，外部事实的时间不代铸）。
type RecordLabelChannelResultCommand struct {
	Tenant        domain.TenantID
	TransactionID domain.LabelTransactionID
	Outcome       domain.LabelTransactionState
	ParcelResults []domain.LabelTransactionParcelResultSpec
	ObservedAt    time.Time
}

// AppendLabelFollowUpActionCommand 追加一条渠道作废、渠道退款或替代记录。
//
// OccurredAt 同上由调用方给：作废与退款发生在渠道那边。Parcels 为空即整笔范围，
// 那是一种业务范围而不是漏填（聚合注释）。
type AppendLabelFollowUpActionCommand struct {
	Tenant        domain.TenantID
	TransactionID domain.LabelTransactionID
	Kind          domain.FollowUpActionKind
	Parcels       []domain.DeclaredParcelID
	Reason        domain.ChannelResultReasonReference
	OccurredAt    time.Time
}

// LabelTransactionDeps 只有仓储与时钟两项。
//
// 时钟只铸**本仓自己动作**的时间——建立与提交渠道是我方的动作。渠道结果时间与后续动作时间
// 一律随命令进来，不在这里铸：那两个是渠道那边的业务事实，代铸会让「渠道什么时候作废的」
// 变成「我们什么时候听说的」，而迟到与更正正是踩这一格。
type LabelTransactionDeps struct {
	Transactions ports.LabelTransactionRepository
	Clock        ports.Clock
}

// LabelTransactionHandler 是面单交易写侧的编排：建立 → 提交渠道 →（结果不确定）→ 记录结果 →
// 追加后续动作。五步同一个 handler，因为它们操作同一个聚合、依赖同一对端口；拆成五个 handler
// 只会让装配点多四次接线而缝一条都不少。
//
// **它不发起任何渠道调用。** 出向那一步的形状归 ADR-0090 与票 `07`；本编排在它上面只留缝——
// 调用方拿 `SubmitToChannel` 的结果去发请求，把回来的答案经 `MarkResultUncertain` 或
// `RecordChannelResult` 交回来。把出向塞进这里会让「答案未确定不得重发」这条纪律散落在编排里。
type LabelTransactionHandler struct {
	deps LabelTransactionDeps
}

func NewLabelTransactionHandler(deps LabelTransactionDeps) *LabelTransactionHandler {
	return &LabelTransactionHandler{deps: deps}
}

// Establish 建立一笔面单交易。
//
// 撞键即重放：同一标识再建一次读回既有那一笔，**不覆盖**。建立时固定的覆盖与依据此后改不了
// （ADR-0084），所以「同标识不同内容」不是一次更正而是一次说不清的写入，让它读回既有，由
// 调用方比对后决定是不是该另起一笔带替代关系的新交易。
func (handler *LabelTransactionHandler) Establish(
	ctx context.Context,
	command EstablishLabelTransactionCommand,
) (LabelTransactionResult, error) {
	link, refusal, err := handler.priorLink(ctx, command)
	if err != nil {
		return LabelTransactionResult{}, err
	}
	if refusal != LabelTransactionOutcomeInvalid {
		return LabelTransactionResult{outcome: refusal}, nil
	}

	transaction, err := domain.EstablishLabelTransaction(domain.EstablishLabelTransactionSpec{
		Tenant:                 command.Tenant,
		ID:                     command.TransactionID,
		CoveredParcels:         command.CoveredParcels,
		ChannelAccount:         command.ChannelAccount,
		AccountHolder:          command.AccountHolder,
		ServiceProvider:        command.ServiceProvider,
		SettlementCounterparty: command.SettlementCounterparty,
		Contract:               command.Contract,
		Rate:                   command.Rate,
		ResponsibilityBasis:    command.ResponsibilityBasis,
		EstablishedAt:          handler.deps.Clock.Now(),
		PriorLink:              link,
	})
	if err != nil {
		return handler.refuse("establish label transaction", err, nil)
	}

	inserted, err := handler.deps.Transactions.Insert(ctx, transaction)
	if err != nil {
		return LabelTransactionResult{}, fmt.Errorf("establish label transaction: %w", err)
	}
	if inserted == ports.LabelTransactionAlreadyExists {
		existing, found, err := handler.deps.Transactions.FindByID(ctx, command.Tenant, command.TransactionID)
		if err != nil {
			return LabelTransactionResult{}, fmt.Errorf("establish label transaction: %w", err)
		}
		if !found {
			// 撞键后又查不到：本聚合没有删除路径，这是库或适配器的 bug，不是业务答案。
			return LabelTransactionResult{}, fmt.Errorf(
				"establish label transaction: 撞键后读不回既有交易 %s", command.TransactionID)
		}
		return labelTransactionAlreadyApplied(existing), nil
	}
	return labelTransactionApplied(transaction), nil
}

// priorLink 按标识取回原交易并建立关系。首笔交易（未指名原交易）交回零值关系。
//
// 关系由**原交易本体**建立而不是一个裸标识：聚合的 EstablishPriorLabelTransactionLink 要求
// 原交易已定案，拿本体作参数正是为了让「对结果不确定的交易发起重试」在建立处就死。本层因此
// 必须先把它读回来，不能图省事直接拼一个关系值。
func (handler *LabelTransactionHandler) priorLink(
	ctx context.Context,
	command EstablishLabelTransactionCommand,
) (domain.PriorLabelTransactionLink, LabelTransactionOutcome, error) {
	if command.PriorLinkKind == domain.LabelTransactionLinkKindInvalid {
		return domain.PriorLabelTransactionLink{}, LabelTransactionOutcomeInvalid, nil
	}
	prior, found, err := handler.deps.Transactions.FindByID(ctx, command.Tenant, command.PriorTransactionID)
	if err != nil {
		return domain.PriorLabelTransactionLink{}, LabelTransactionOutcomeInvalid,
			fmt.Errorf("establish label transaction: 读原交易：%w", err)
	}
	if !found {
		// 指名一笔查不到的原交易：改输入重来，不是等它出现。
		return domain.PriorLabelTransactionLink{}, LabelTransactionNotAccepted, nil
	}
	link, err := domain.EstablishPriorLabelTransactionLink(prior, command.PriorLinkKind)
	if err != nil {
		if errors.Is(err, domain.ErrPriorLabelTransactionNotFinalized) {
			return domain.PriorLabelTransactionLink{}, LabelTransactionPriorNotFinalized, nil
		}
		return domain.PriorLabelTransactionLink{}, LabelTransactionNotAccepted, nil
	}
	return link, LabelTransactionOutcomeInvalid, nil
}

// SubmitToChannel 把交易推到`已提交渠道`并落库。提交时间由本层铸——发出请求是我方的动作。
func (handler *LabelTransactionHandler) SubmitToChannel(
	ctx context.Context,
	command SubmitLabelTransactionCommand,
) (LabelTransactionResult, error) {
	return handler.advance(ctx, "submit label transaction to channel", command.Tenant, command.TransactionID,
		func(transaction domain.LabelTransaction) (domain.LabelTransaction, error) {
			return transaction.SubmitToChannel(handler.deps.Clock.Now())
		})
}

// MarkResultUncertain 记录「暂时无法确认渠道是否受理」。
//
// 这一格进库是**要紧的**，不是一次日志：它落库之后，任何重发路径读到的都是`结果不确定`而不是
// `已提交渠道`，`SubmitToChannel` 的状态门因此自动挡住第二次提交。ADR-0090 那条「`答案未确定`
// 不得重发」在本上下文的落点就是这里。
func (handler *LabelTransactionHandler) MarkResultUncertain(
	ctx context.Context,
	command MarkLabelResultUncertainCommand,
) (LabelTransactionResult, error) {
	return handler.advance(ctx, "mark label result uncertain", command.Tenant, command.TransactionID,
		func(transaction domain.LabelTransaction) (domain.LabelTransaction, error) {
			return transaction.MarkResultUncertain()
		})
}

// RecordChannelResult 一次写下两层结果。跨层一致性由聚合刻意不校验，本层同样不补——
// 补上就是把 CONTEXT 禁止的「由一个层次覆盖另一个层次」搬到应用层再做一遍。
func (handler *LabelTransactionHandler) RecordChannelResult(
	ctx context.Context,
	command RecordLabelChannelResultCommand,
) (LabelTransactionResult, error) {
	return handler.advance(ctx, "record label channel result", command.Tenant, command.TransactionID,
		func(transaction domain.LabelTransaction) (domain.LabelTransaction, error) {
			return transaction.RecordChannelResult(domain.RecordChannelResultSpec{
				Outcome:       command.Outcome,
				ParcelResults: command.ParcelResults,
				ObservedAt:    command.ObservedAt,
			})
		})
}

// AppendFollowUpAction 追加一条渠道作废、渠道退款或替代记录。它不改交易级状态、不改任何包裹
// 结果、也不动定案谓词——那三条由聚合保证，本层只转交。
func (handler *LabelTransactionHandler) AppendFollowUpAction(
	ctx context.Context,
	command AppendLabelFollowUpActionCommand,
) (LabelTransactionResult, error) {
	return handler.advance(ctx, "append label follow-up action", command.Tenant, command.TransactionID,
		func(transaction domain.LabelTransaction) (domain.LabelTransaction, error) {
			return transaction.AppendFollowUpAction(domain.FollowUpActionSpec{
				Kind:       command.Kind,
				Parcels:    command.Parcels,
				Reason:     command.Reason,
				OccurredAt: command.OccurredAt,
			})
		})
}

// advance 是后四步共同的骨架：读回 → 转移 → 按预期版本写回。
//
// 抽出来而不是各写一遍，是因为这四步在**恢复动作**上完全同形——读不回怎么答、状态不允许怎么
// 答、版本冲突怎么答，四处一字不差。各写一遍就有了四份会各自漂移的口径，而漂移在编译期
// 不报。真正各不相同的那一格（哪个转移）是参数。
func (handler *LabelTransactionHandler) advance(
	ctx context.Context,
	step string,
	tenant domain.TenantID,
	transactionID domain.LabelTransactionID,
	transition func(domain.LabelTransaction) (domain.LabelTransaction, error),
) (LabelTransactionResult, error) {
	transaction, found, err := handler.deps.Transactions.FindByID(ctx, tenant, transactionID)
	if err != nil {
		return LabelTransactionResult{}, fmt.Errorf("%s: %w", step, err)
	}
	if !found {
		// 指名一笔查不到的交易是调用方的错，改输入重来。不细分「不存在」与「不属于你」——
		// 仓储以租户为键，否定结果本来就不携带这个差别（ADR-0022 统一不可见结果同理）。
		return LabelTransactionResult{outcome: LabelTransactionNotAccepted}, nil
	}

	advanced, err := transition(transaction)
	if err != nil {
		// 交回读回来的那一份而不是转移失败的零值：`状态不允许`的恢复动作是「去看它停在
		// 哪一格」，不把那一格交出去，调用方还得再读一次。
		return handler.refuse(step, err, &transaction)
	}

	saved, err := handler.deps.Transactions.Save(ctx, advanced)
	if err != nil {
		return LabelTransactionResult{}, fmt.Errorf("%s: %w", step, err)
	}
	if saved != ports.LabelTransactionSaved {
		// 版本冲突是业务答案（ADR-0031），不是错误：抢先那一方可能是另一次结果记录或一条
		// 后续动作。调用方重读再重放，本层不代猜库里此刻是什么，因此也不交回手上这份陈旧的。
		return LabelTransactionResult{outcome: LabelTransactionWriteConflict}, nil
	}
	return labelTransactionApplied(advanced), nil
}

// refuse 把领域错误译成本层的恢复动作分格。**只认领域显式分出来的那几格**，其余照原样上抛：
// 把一个没预料到的错误吞成某个业务答案，调用方会对着一个坏了的依赖反复重放。
func (handler *LabelTransactionHandler) refuse(
	step string,
	err error,
	transaction *domain.LabelTransaction,
) (LabelTransactionResult, error) {
	switch {
	case errors.Is(err, domain.ErrLabelTransactionStateNotAdmitted):
		return labelTransactionRefused(LabelTransactionStepNotAdmitted, transaction), nil
	case errors.Is(err, domain.ErrPriorLabelTransactionNotFinalized):
		return labelTransactionRefused(LabelTransactionPriorNotFinalized, transaction), nil
	case errors.Is(err, domain.ErrInvalidLabelTransaction):
		return labelTransactionRefused(LabelTransactionNotAccepted, transaction), nil
	default:
		return LabelTransactionResult{}, fmt.Errorf("%s: %w", step, err)
	}
}

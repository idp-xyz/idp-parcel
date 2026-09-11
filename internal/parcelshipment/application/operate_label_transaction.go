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
	// LabelTransactionParcelNotOpen：覆盖包裹里至少一件的`包裹级继续尝试判断`不是`开放`——仍有生效的受控关闭，
	// 或当前有效终局在场（CONTEXT「关闭生效后只拒绝把该包裹纳入边界后的新重试、替代或换单交易」）。恢复动作是
	// 去掉该包裹另建、为它申请重开、或把新服务需求进关联的新委托，与`输入未受理`的「改输入重来」不是同一个动作，
	// 也不并进`状态不允许`——那一格说的是**这笔交易**此刻的状态，这里交易还不存在。哪几件从 ClosedParcels 读。
	LabelTransactionParcelNotOpen
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
	case LabelTransactionParcelNotOpen:
		return "PARCEL_CONTINUED_ATTEMPT_CLOSED"
	default:
		return ""
	}
}

// LabelTransactionResult 交回本步之后交易停在哪里。它带回聚合本体而不只是状态：调用方下一步
// 多半要接着操作同一笔交易，让它按标识再读一次会在两次读之间开一个窗口。
type LabelTransactionResult struct {
	outcome       LabelTransactionOutcome
	transaction   domain.LabelTransaction
	hasValue      bool
	closedParcels []domain.DeclaredParcelID
}

func (result LabelTransactionResult) Outcome() LabelTransactionOutcome {
	return result.outcome
}

// Transaction 交回本步之后的交易；重放时交回既有那一笔。输入未受理与版本冲突时缺席——
// 前者没有形成任何交易，后者手上这份已经陈旧，交回去只会被当成最新的接着改。
func (result LabelTransactionResult) Transaction() (domain.LabelTransaction, bool) {
	return result.transaction, result.hasValue
}

// ClosedParcels 交回建立被`包裹未开放`拒掉时判为`受控关闭`的那几件覆盖包裹（按覆盖顺序），其余结果为空。
// 恢复动作要知道是哪几件——去掉哪件另建、为哪件申请重开——只给一格判断不够指；交易本体照旧缺席，理由同
// labelTransactionRefused 那一句：建立被拒时手上根本没有交易可交。
func (result LabelTransactionResult) ClosedParcels() []domain.DeclaredParcelID {
	parcels := make([]domain.DeclaredParcelID, len(result.closedParcels))
	copy(parcels, result.closedParcels)
	return parcels
}

func labelTransactionApplied(transaction domain.LabelTransaction) LabelTransactionResult {
	return LabelTransactionResult{outcome: LabelTransactionApplied, transaction: transaction, hasValue: true}
}

func labelTransactionAlreadyApplied(transaction domain.LabelTransaction) LabelTransactionResult {
	return LabelTransactionResult{outcome: LabelTransactionAlreadyApplied, transaction: transaction, hasValue: true}
}

func labelTransactionParcelNotOpen(closed []domain.DeclaredParcelID) LabelTransactionResult {
	return LabelTransactionResult{outcome: LabelTransactionParcelNotOpen, closedParcels: closed}
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
// 七类依据引用不逐格进命令，而是整份「择优结果」（domain.SelectedChannelBasis）进来（票 label-channel/28
// 判据 1 的形状约束）：**建立一步不问它从哪来**——系统择优作前置步产出它（EstablishSelectedLabelTransactionHandler），
// 日后运营端点人工择优也产同一个对象，06 的输入形状只改这一次。覆盖范围与七格逐项仍由聚合构造门把守，
// 本层不重复校验——重复一遍就有了第二处口径，而它必然先于聚合那处过期。PriorTransactionID 留零值即首笔交易。
type EstablishLabelTransactionCommand struct {
	Tenant         domain.TenantID
	TransactionID  domain.LabelTransactionID
	CoveredParcels []domain.DeclaredParcelID
	// Basis 是这笔交易按之建立的择优结果：选中的候选与七类依据引用（评价痕迹引用随它、可缺席）。
	// 候选本身不进聚合——哪些候选参过选、为什么是它，在择优步写下的渠道择优决定记录里。
	Basis domain.SelectedChannelBasis
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

// LabelTransactionDeps 收拢 06 编排的依赖：仓储、判断意图口、继续尝试登记册与当前有效终局的只读半边、时钟。
//
// 时钟只铸**本仓自己动作**的时间——建立与提交渠道是我方的动作。渠道结果时间与后续动作时间
// 一律随命令进来，不在这里铸：那两个是渠道那边的业务事实，代铸会让「渠道什么时候作废的」
// 变成「我们什么时候听说的」，而迟到与更正正是踩这一格。
//
// Judgments 不是渠道调用，是写后留的一道缝（ADR-0134 决定一 / 三）：记录结果与追加后续动作两拍
// `Save` 成功后，按覆盖包裹逐件交一份「值得判一次终局」的指针式意图，判断由下一拍的消费者形成，
// 本编排不知道终局判断存在。它与 `Save` 同一事务——两者都从 ctx 取同一个事务执行器，事务由
// 组合根的事务壳开；本编排刻意不持 Transactor，不在事务里调用会在 `Save` 处响亮失败。
//
// Registers 与 Finals **只在 Establish 用、只读**，不是第二处口径：建立前逐覆盖包裹取
// `register.Judge(currentFinalPresent)`，那是`包裹级继续尝试判断`的单一权威——本编排不看决定种类、不比时间。
// `Establish` 之后四步一律不核册：CONTEXT「关闭生效后只拒绝把该包裹纳入边界后的新重试、替代或换单交易，
// 不阻断既有交易的查询、确认、重打、渠道作废、渠道退款、对账和定案」。Finals 与终局采用路径接同款适配器
// （FinalOutcomeStore 的读半边，同一张表）：两种服务形态的终局只认那一处，另读一处就看不见面单服务自己判出的终局。
type LabelTransactionDeps struct {
	Transactions ports.LabelTransactionRepository
	Judgments    ports.LabelTransactionJudgmentHandoff
	Registers    ports.ContinuedAttemptRegisterView
	Finals       ports.CurrentFinalView
	Clock        ports.Clock
}

// LabelTransactionHandler 是面单交易写侧的编排：建立 → 提交渠道 →（结果不确定）→ 记录结果 →
// 追加后续动作。五步同一个 handler，因为它们操作同一个聚合、依赖同一组端口；拆成五个 handler
// 只会让装配点多四次接线而缝一条都不少。后两步在 `Save` 成功后多一段写后入队（见 Judgments），
// 前三步没有——建立、提交与「答案未确定」都不改变任何终局判断的输入。
//
// **它不发起任何渠道调用，也不择优。** 出向那一步的形状归 ADR-0090 与票 `07`；本编排在它上面只留缝——
// 调用方拿 `SubmitToChannel` 的结果去发请求，把回来的答案经 `MarkResultUncertain` 或
// `RecordChannelResult` 交回来。把出向塞进这里会让「答案未确定不得重发」这条纪律散落在编排里。
// 择优在它前面（票 label-channel/28 裁乙）：EstablishSelectedLabelTransactionHandler 先择优、译出「择优结果」，
// 再调本编排的 Establish——本编排只收那份对象，不认识择优编排，也不认识翻译适配器；两条缝分开留，
// 是为了让日后人工择优产出同一份对象时不必再改这里。
type LabelTransactionHandler struct {
	deps LabelTransactionDeps
}

// ErrNilDependency 是构造门对缺件的唯一答复；哪一口缺在包装信息里点名。它必须是构造期的错误而不是运行期
// 的 panic 或运行期的 error：装配疏漏要在进程启动那一刻炸出来（与组合根 fail-fast 同一纪律，票 label-channel/34），
// 而不是等第一笔建立或第一次 `Save` 之后才发现——那时一半写动作已经落库。形照 customscompliance/application
// 的同名哨兵（票 sa-cc/14）。
var ErrNilDependency = errors.New("parcel shipment: application dependency is nil")

// NewLabelTransactionHandler 构造期逐口拒 nil，五口全部必填。此前只拒两个只读口、让 `Judgments` 在 `Save` 之后运行期
// 响亮拒（lc/26 的取法），理由写的是「漏装要到第一次建立才 panic」——那条理由不成立：nil 接口在任何一步都会响亮 panic，
// 不会静默放行，所以两个只读口并不比其余三口更需要构造期的门；真正的分界在**何时**发现，装配错要在启动时露出来，
// 不是在第一笔业务上。运行期那道 `Judgments` 门随之撤掉：构造门保证了它不可能为 nil，留着就是第二处口径。
func NewLabelTransactionHandler(deps LabelTransactionDeps) (*LabelTransactionHandler, error) {
	for _, dependency := range []struct {
		name    string
		missing bool
	}{
		{"label transaction repository", deps.Transactions == nil},
		{"label transaction judgment handoff", deps.Judgments == nil},
		{"continued attempt register view", deps.Registers == nil},
		{"current final view", deps.Finals == nil},
		{"clock", deps.Clock == nil},
	} {
		if dependency.missing {
			return nil, fmt.Errorf("%w: %s", ErrNilDependency, dependency.name)
		}
	}
	return &LabelTransactionHandler{deps: deps}, nil
}

// Establish 建立一笔面单交易。
//
// 同标识即重放：再建一次读回既有那一笔，**不覆盖**。建立时固定的覆盖与依据此后改不了
// （ADR-0084），所以「同标识不同内容」不是一次更正而是一次说不清的写入，让它读回既有，由
// 调用方比对后决定是不是该另起一笔带替代关系的新交易。重放**先于核册**：先按标识读一次，命中就走重放分支——
// 一笔边界前建立的交易被重放时，册上可能已经有了关闭，硬句「权威业务截断边界前已经形成交易建立决定……仍属于
// 既有交易」要它照旧交回，而不是被报成「关闭中」；多读一次换这条硬句。`Insert` 撞键那一支仍留着，兜两次同标识
// 建立恰好在这一读与 `Insert` 之间并发的那一格。
//
// 关系立好之后、聚合构造之前核继续尝试登记册（lc/32）：对覆盖包裹逐件现算`包裹级继续尝试判断`，任一件为
// `受控关闭`就拒掉整笔建立、不 Insert——CONTEXT「关闭生效后只拒绝把该包裹纳入边界后的新重试、替代或换单交易」，
// 建立是本编排唯一一处「把包裹纳入新交易」，所以核册只在这一步；后四步作用于既有交易，硬句明写不阻断它们的
// 处理与定案。用 Judge 而不只看 StandingClosure，是因为「开放」的定义本就含「当前不存在有效终局」（CONTEXT
// 「当前有效终局服务结果存在时……后续新服务需求进入关联的新委托」），Judge 是那一格的单一权威，只看关闭那一支
// 就是在本编排里复述一半口径。
//
// 并发窄格如实记、不在这里关：本步读册与关闭决定的 Save 各在自己的事务里，读到「无关闭」之后关闭才提交的那一格
// 本门拦不住；CONTEXT 为它另备了一次事后判断（「关闭期间若仍发现已经实际提交的边界后交易，`parcel-shipment` 必须
// 形成违反截断边界的业务判断并保留该交易」），那需要建立决定与关闭决定之间的稳定领域顺序，归另一张票。本步不加行锁，也不给
// 交易加「观察到的册版本」出生属性——ADR-0084 决定二固定的清单不动。
func (handler *LabelTransactionHandler) Establish(
	ctx context.Context,
	command EstablishLabelTransactionCommand,
) (LabelTransactionResult, error) {
	existing, found, err := handler.deps.Transactions.FindByID(ctx, command.Tenant, command.TransactionID)
	if err != nil {
		return LabelTransactionResult{}, fmt.Errorf("establish label transaction: %w", err)
	}
	if found {
		return labelTransactionAlreadyApplied(existing), nil
	}

	link, refusal, err := handler.priorLink(ctx, command)
	if err != nil {
		return LabelTransactionResult{}, err
	}
	if refusal != LabelTransactionOutcomeInvalid {
		return LabelTransactionResult{outcome: refusal}, nil
	}

	closed, refusal, err := handler.closedParcels(ctx, command.Tenant, command.CoveredParcels)
	if err != nil {
		return LabelTransactionResult{}, err
	}
	if refusal != LabelTransactionOutcomeInvalid {
		return LabelTransactionResult{outcome: refusal}, nil
	}
	if len(closed) != 0 {
		return labelTransactionParcelNotOpen(closed), nil
	}

	transaction, err := domain.EstablishLabelTransaction(domain.EstablishLabelTransactionSpec{
		Tenant:                 command.Tenant,
		ID:                     command.TransactionID,
		CoveredParcels:         command.CoveredParcels,
		ChannelAccount:         command.Basis.ChannelAccount(),
		AccountHolder:          command.Basis.AccountHolder(),
		ServiceProvider:        command.Basis.ServiceProvider(),
		SettlementCounterparty: command.Basis.SettlementCounterparty(),
		Contract:               command.Basis.Contract(),
		Rate:                   command.Basis.Rate(),
		ResponsibilityBasis:    command.Basis.ResponsibilityBasis(),
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
			fmt.Errorf("establish label transaction: read prior transaction: %w", err)
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

// closedParcels 对覆盖包裹逐件现算`包裹级继续尝试判断`，交回判为`受控关闭`的那几件（按覆盖顺序）。
//
// 两个读口读不回都原样上抛，不译成「开放」放行、也不译成「关闭」拒绝：fail-closed 是拒绝建立这一步本身（返错），
// 不是猜一格——猜「开放」会让一道坏掉的读口放走边界后的交易，猜「关闭」会把库故障报成一条业务决定。未开册与
// 空册是同一格（照 JudgeLabelServiceFinalHandler.Handle：无生效关闭且无当前有效终局即开放）；开不出空册只因
// 租户或包裹标识立不住，那是输入的错，落`输入未受理`。`currentFinalPresent` 照 FormContinuedAttemptDecisionHandler
// 的取法：找到且已定案。
func (handler *LabelTransactionHandler) closedParcels(
	ctx context.Context,
	tenant domain.TenantID,
	parcels []domain.DeclaredParcelID,
) ([]domain.DeclaredParcelID, LabelTransactionOutcome, error) {
	var closed []domain.DeclaredParcelID
	for _, parcel := range parcels {
		current, found, err := handler.deps.Finals.FindCurrentFinal(ctx, tenant, parcel)
		if err != nil {
			return nil, LabelTransactionOutcomeInvalid,
				fmt.Errorf("establish label transaction: read current final for parcel %s: %w", parcel, err)
		}
		currentFinalPresent := found && current.Finalized

		register, found, err := handler.deps.Registers.FindByParcel(ctx, tenant, parcel)
		if err != nil {
			return nil, LabelTransactionOutcomeInvalid,
				fmt.Errorf("establish label transaction: read continued attempt register for parcel %s: %w", parcel, err)
		}
		if !found {
			if register, err = domain.OpenContinuedAttemptRegister(tenant, parcel); err != nil {
				return nil, LabelTransactionNotAccepted, nil
			}
		}
		if register.Judge(currentFinalPresent) == domain.ContinuedAttemptControlledClosed {
			closed = append(closed, parcel)
		}
	}
	return closed, LabelTransactionOutcomeInvalid, nil
}

// SubmitToChannel 把交易推到`已提交渠道`并落库。提交时间由本层铸——发出请求是我方的动作。
func (handler *LabelTransactionHandler) SubmitToChannel(
	ctx context.Context,
	command SubmitLabelTransactionCommand,
) (LabelTransactionResult, error) {
	return handler.advance(ctx, "submit label transaction to channel", command.Tenant, command.TransactionID,
		func(transaction domain.LabelTransaction) (domain.LabelTransaction, error) {
			return transaction.SubmitToChannel(handler.deps.Clock.Now())
		}, nil)
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
		}, nil)
}

// RecordChannelResult 一次写下两层结果。跨层一致性由聚合刻意不校验，本层同样不补——
// 补上就是把 CONTEXT 禁止的「由一个层次覆盖另一个层次」搬到应用层再做一遍。
//
// 这一拍让定案由假变真，`Save` 成功后逐覆盖包裹交判断意图（ADR-0134 决定四）。
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
		}, handler.judgmentBeat(ports.LabelTransactionResultRecorded, command.ObservedAt))
}

// AppendFollowUpAction 追加一条渠道作废、渠道退款或替代记录。它不改交易级状态、不改任何包裹
// 结果、也不动定案谓词——那三条由聚合保证，本层只转交。
//
// 它同样触发判断（ADR-0134 决定四）：作废改变关闭路径「已有成功结果均已成功作废」那一格的输入，
// 不触发则「成功结果全部作废后沿关闭路径形成终局」要等下一笔交易定案或一份关闭决定，而那两件
// 可能永不发生。不按动作种类或范围筛——筛就是在这里复述一遍关闭路径的口径。
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
		}, handler.judgmentBeat(ports.LabelTransactionFollowUpAppended, command.OccurredAt))
}

// judgmentBeat 交回「`Save` 成功之后按覆盖包裹逐件入队一份判断意图」的写后动作。
//
// 版本取 `Save` 成功那一代：仓储按预期版本加一写回，而聚合本体上仍是读出时那一代（转移不动它），
// 所以这里是 Revision()+1；两拍先后写回，版本相邻，各自入队。入队失败原样上抛——整步随事务回滚，
// 调用方重放这一步（渠道答案在它手上，不需要重发渠道调用）；不照终局采用那一路「失败不翻结果、
// 留续办引用」的形，那一形留的正是「结果已落、意图未交」的中间态。
func (handler *LabelTransactionHandler) judgmentBeat(
	beat ports.LabelTransactionBeat,
	occurredAt time.Time,
) func(context.Context, domain.LabelTransaction) error {
	return func(ctx context.Context, saved domain.LabelTransaction) error {
		for _, parcel := range saved.CoveredParcels() {
			if err := handler.deps.Judgments.HandOffLabelTransactionJudgment(ctx, ports.LabelTransactionJudgmentIntent{
				Tenant:        saved.Tenant(),
				TransactionID: saved.ID(),
				Parcel:        parcel,
				Revision:      saved.Revision() + 1,
				Beat:          beat,
				OccurredAt:    occurredAt,
			}); err != nil {
				return fmt.Errorf("hand off label transaction judgment for parcel %s: %w", parcel, err)
			}
		}
		return nil
	}
}

// advance 是后四步共同的骨架：读回 → 转移 → 按预期版本写回 →（若有）写后动作。
//
// 抽出来而不是各写一遍，是因为这四步在**恢复动作**上完全同形——读不回怎么答、状态不允许怎么
// 答、版本冲突怎么答，四处一字不差。各写一遍就有了四份会各自漂移的口径，而漂移在编译期
// 不报。真正各不相同的两格（哪个转移、写后做什么）是参数；afterSave 为 nil 即这一步写后无事。
func (handler *LabelTransactionHandler) advance(
	ctx context.Context,
	step string,
	tenant domain.TenantID,
	transactionID domain.LabelTransactionID,
	transition func(domain.LabelTransaction) (domain.LabelTransaction, error),
	afterSave func(context.Context, domain.LabelTransaction) error,
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
	if afterSave != nil {
		// 只在 `Save` 成功之后：状态门拒绝与版本冲突都没落库，一封指着未落库结果的信不该出去。
		if err := afterSave(ctx, advanced); err != nil {
			return LabelTransactionResult{}, fmt.Errorf("%s: %w", step, err)
		}
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

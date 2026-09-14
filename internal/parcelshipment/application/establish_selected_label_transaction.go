package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件是渠道择优作面单交易建立前置步的编排（票 label-channel/28 裁乙）：择优 → 译成「择优结果」→ 建立。
//
// 三段各归各家——择优归 SelectChannelCandidateHandler（候选收窄归 PC、评价归 PP、并列归本上下文比较器），翻译归
// adapters/partycommercial 的翻译适配器（票 29），建立归 LabelTransactionHandler（票 06）。本编排一条业务规则都不加，
// 它只负责三件：按序调、在段与段之间不掉东西、任一段停下就不往下走。
//
// **它不持 Transactor**（ADR-0134 决定三的同一条纪律）：择优要写决定记录、建立要写交易，两处都从 ctx 取事务执行器，
// 事务由组合根的事务壳开。两段要不要在同一个事务里，是组合根的决定——翻译适配器头注写着「择优留痕在择优那一步
// 已写完」，所以生产装配给两段各开一个壳，翻译停下时决定记录仍在；本编排对此不知情，也不该知情。

var (
	// ErrSelectedLabelTransactionFlowMisconfigured 说四口有一口没装。响亮失败而不是跳过那一段：跳过择优就是让调用方
	// 自己决定渠道，跳过翻译就是拿候选标识顶七类依据，跳过重放那一问就是让每次重试在决定册多留一条 SELECTED——
	// 三者都是本票族要拆掉的形状。
	ErrSelectedLabelTransactionFlowMisconfigured = errors.New("parcel shipment: selected label transaction flow is not fully wired")
	// ErrChannelSelectionStopped 标明停在**择优那一段**：取数口未配置、成本表不全、留痕半配、依赖故障……具体哪一格
	// 由择优编排与其适配器具名（成因原样包在里面），本层认不出适配器的错误值，只加段标不改写。并列冲突与无人参选
	// 不走这里——那两格是择优步已记决定的业务答案，择优编排以结果格交回，本层原样映射。
	ErrChannelSelectionStopped = errors.New("parcel shipment: channel selection stopped before a candidate was selected")
	// ErrChannelBasisTranslationStopped 标明停在**翻译那一段**：实例半边源（ChannelSelectionBasisTranslatorDeps 的
	// Accounts / Agreements / Resolutions）未配置、授权 / 协议对不上或不在有效期、费率缺席……同样由翻译适配器具名，
	// 本层只加段标。翻译停下不建立交易、也不再写任何决定记录。
	ErrChannelBasisTranslationStopped = errors.New("parcel shipment: channel selection result could not be translated into a label transaction basis")
)

// SelectedLabelTransactionOutcome 是前置步编排的结果代数，按调用方的恢复动作分格（ADR-0029）。
//
// 只列**没有 error 的**出口：择优或翻译停下时成因是适配器具名的错误值，本层不重新命名它们（见两个段标错误）。
type SelectedLabelTransactionOutcome uint8

const (
	SelectedLabelTransactionOutcomeInvalid SelectedLabelTransactionOutcome = iota
	// SelectedLabelTransactionEstablished：择优与翻译都走完，建立一步已经答过——答的是什么看 Establishment()，
	// 那一层的代数（已落下 / 重放 / 输入未受理 / 原交易未定案……）原样透传，本层不再分一遍。
	SelectedLabelTransactionEstablished
	// ChannelSelectionTied：候选并列，择优步已记下 TIED 决定，等人裁（票 14 / 23）；不建立交易。
	ChannelSelectionTied
	// NoQualifiedChannelCandidate：无人参选，择优步已记决定；不建立交易。续办是补价卡或放宽约束，不是重试。
	NoQualifiedChannelCandidate
)

func (outcome SelectedLabelTransactionOutcome) String() string {
	switch outcome {
	case SelectedLabelTransactionEstablished:
		return "ESTABLISHED"
	case ChannelSelectionTied:
		return "SELECTION_TIED"
	case NoQualifiedChannelCandidate:
		return "NO_QUALIFIED_CANDIDATE"
	default:
		return ""
	}
}

// SelectedLabelTransactionResult 交回前置步停在哪里，连同一路带出来的对象：选中候选（择优落定即有）、择优结果
// （翻译走完即有）、建立结果（建立答过即有）。调用方接着做的事——发起渠道调用——要交易本体，它在建立结果里。
// 重放那一格（交易已在、择优没发生）只有建立结果：选中候选与择优结果都缺席，它们是**这一次**择优的产物，
// 而这一次没有择优；交易本体在建立结果里，七类依据在交易上。
type SelectedLabelTransactionResult struct {
	outcome        SelectedLabelTransactionOutcome
	selected       domain.SelectedChannelCandidate
	hasSelected    bool
	basis          domain.SelectedChannelBasis
	hasBasis       bool
	establishment  LabelTransactionResult
	hasEstablished bool
}

func (result SelectedLabelTransactionResult) Outcome() SelectedLabelTransactionOutcome {
	return result.outcome
}

// Selected 交回择优选中的候选；并列冲突与无人参选时缺席。
func (result SelectedLabelTransactionResult) Selected() (domain.SelectedChannelCandidate, bool) {
	return result.selected, result.hasSelected
}

// Basis 交回译出的择优结果；只有走到建立那一段才有。
func (result SelectedLabelTransactionResult) Basis() (domain.SelectedChannelBasis, bool) {
	return result.basis, result.hasBasis
}

// Establishment 交回建立一步的答复；只有 SelectedLabelTransactionEstablished 那一格才有。
func (result SelectedLabelTransactionResult) Establishment() (LabelTransactionResult, bool) {
	return result.establishment, result.hasEstablished
}

// EstablishSelectedLabelTransactionCommand 是前置步的输入：这笔交易的身份与覆盖、择优用的查询、可选的原交易关系。
//
// 渠道不在命令里——那正是裁乙要拆掉的那一格：渠道由编排选出，调用方给的是「按哪笔映射、在哪个范围、对准哪个时点」。
type EstablishSelectedLabelTransactionCommand struct {
	Tenant         domain.TenantID
	TransactionID  domain.LabelTransactionID
	CoveredParcels []domain.DeclaredParcelID
	Selection      ports.ChannelSelectionQuery
	// PriorTransactionID 与 PriorLinkKind 成对给出，语义同 EstablishLabelTransactionCommand。
	PriorTransactionID domain.LabelTransactionID
	PriorLinkKind      domain.LabelTransactionLinkKind
}

// ChannelSelector 是择优那一段的窄面：SelectChannelCandidateHandler.Select 就是它。收接口而不是那个具体类型，
// 是为了让组合根在它外面套事务壳（决定记录要在事务里写）而编排不必知道壳的存在。壳怎么提交、怎么回滚写在组合根
// `cmd/parcel-api` 的 transactionalChannelSelection 头注，此处只承诺本层这一半：并列与无人参选是结果格不是 error，
// 壳因此不必认任何领域哨兵（票 35 做法二）。
type ChannelSelector interface {
	Select(ctx context.Context, query ports.ChannelSelectionQuery) (ChannelSelectionResult, error)
}

// LabelTransactionEstablisher 是建立那一段的窄面：LabelTransactionHandler.Establish 就是它。理由同上——
// 建立的 Insert 要在事务里，壳由组合根套在外面。
type LabelTransactionEstablisher interface {
	Establish(ctx context.Context, command EstablishLabelTransactionCommand) (LabelTransactionResult, error)
}

// LabelTransactionLookup 是建立前那一问的只读窄面：ports.LabelTransactionRepository 的 FindByID 就是它（票 35 裁决 1 取甲）。
// 单列一口而不扩 LabelTransactionEstablisher，是因为那一问不在建立的事务壳里、也不该在——它只是一次读，
// 读到「已在」就不择优、不记决定；读是不是要在事务里由组合根接哪个实现决定，本层不知情。
type LabelTransactionLookup interface {
	FindByID(ctx context.Context, tenant domain.TenantID, transactionID domain.LabelTransactionID) (domain.LabelTransaction, bool, error)
}

// EstablishSelectedLabelTransactionDeps 收拢三段加建立前那一问。四口都必填——缺一口不是「这一段不做」，是装配缺件。
type EstablishSelectedLabelTransactionDeps struct {
	Lookup       LabelTransactionLookup
	Selector     ChannelSelector
	Translator   ports.ChannelSelectionBasisTranslator
	Transactions LabelTransactionEstablisher
}

// EstablishSelectedLabelTransactionHandler 见文件头注。
type EstablishSelectedLabelTransactionHandler struct {
	deps EstablishSelectedLabelTransactionDeps
}

func NewEstablishSelectedLabelTransactionHandler(deps EstablishSelectedLabelTransactionDeps) *EstablishSelectedLabelTransactionHandler {
	return &EstablishSelectedLabelTransactionHandler{deps: deps}
}

// Establish 先问交易在不在，不在才择优、再翻译、再建立。
//
// 建立前那一问（票 35 裁决 1 取甲）：同 TransactionID 已在即直接答重放——建立那一步本就按标识重放（06 的
// `LabelTransactionAlreadyApplied`），但它在择优之后，而择优每次比较都在决定册追加一条 SELECTED；不先问一次，
// 任何一次重试都会让交易只有一笔、决定却多一条，且两者之间没有引用可对账。多一次读换「一笔交易一条 SELECTED」
// （裁决 3 认下这个代价）。这一问与建立步自己的重放分支并存：兜两次同标识建立恰好在这一读与 Insert 之间并发的那一格。
//
// 其余停法分三种形状：并列冲突与无人参选是结果格（择优步已答、已记）；择优或翻译没走完是带段标的错误（成因由
// 适配器具名，本层只说停在哪一段）；建立一步的答复不论落在它代数的哪一格都原样透传——它已经按恢复动作分过了。
func (handler *EstablishSelectedLabelTransactionHandler) Establish(
	ctx context.Context,
	command EstablishSelectedLabelTransactionCommand,
) (SelectedLabelTransactionResult, error) {
	deps := handler.deps
	if deps.Lookup == nil || deps.Selector == nil || deps.Translator == nil || deps.Transactions == nil {
		return SelectedLabelTransactionResult{}, ErrSelectedLabelTransactionFlowMisconfigured
	}

	existing, found, err := deps.Lookup.FindByID(ctx, command.Tenant, command.TransactionID)
	if err != nil {
		return SelectedLabelTransactionResult{}, fmt.Errorf("establish selected label transaction: look up transaction: %w", err)
	}
	if found {
		return SelectedLabelTransactionResult{
			outcome:        SelectedLabelTransactionEstablished,
			establishment:  labelTransactionAlreadyApplied(existing),
			hasEstablished: true,
		}, nil
	}

	selection, err := deps.Selector.Select(ctx, command.Selection)
	if err != nil {
		return SelectedLabelTransactionResult{}, fmt.Errorf("%w: %w", ErrChannelSelectionStopped, err)
	}
	var selected domain.SelectedChannelCandidate
	switch selection.Outcome() {
	case ChannelSelectionCostTied:
		return SelectedLabelTransactionResult{outcome: ChannelSelectionTied}, nil
	case ChannelSelectionNoQualifiedCandidate:
		return SelectedLabelTransactionResult{outcome: NoQualifiedChannelCandidate}, nil
	case ChannelSelectionSelected:
		var present bool
		if selected, present = selection.Selected(); !present {
			return SelectedLabelTransactionResult{}, fmt.Errorf("%w: selected outcome without a candidate", ErrChannelSelectionStopped)
		}
	default:
		// 择优编排交回它自己都不认识的格是实现坏了，不是业务答案。
		return SelectedLabelTransactionResult{}, fmt.Errorf("%w: unnamed selection outcome %d", ErrChannelSelectionStopped, selection.Outcome())
	}

	basis, err := deps.Translator.TranslateSelectedCandidate(ctx, command.Selection, selected)
	if err != nil {
		return SelectedLabelTransactionResult{}, fmt.Errorf("%w: %w", ErrChannelBasisTranslationStopped, err)
	}

	establishment, err := deps.Transactions.Establish(ctx, EstablishLabelTransactionCommand{
		Tenant:             command.Tenant,
		TransactionID:      command.TransactionID,
		CoveredParcels:     command.CoveredParcels,
		Basis:              basis,
		PriorTransactionID: command.PriorTransactionID,
		PriorLinkKind:      command.PriorLinkKind,
	})
	if err != nil {
		return SelectedLabelTransactionResult{}, fmt.Errorf("establish selected label transaction: %w", err)
	}
	return SelectedLabelTransactionResult{
		outcome:        SelectedLabelTransactionEstablished,
		selected:       selected,
		hasSelected:    true,
		basis:          basis,
		hasBasis:       true,
		establishment:  establishment,
		hasEstablished: true,
	}, nil
}

// 两个窄面由既有编排直接满足：装配时可以不套壳直接交（测试就是这么做的），生产装配在它们外面各套一个事务壳。
var (
	_ ChannelSelector             = (*SelectChannelCandidateHandler)(nil)
	_ LabelTransactionEstablisher = (*LabelTransactionHandler)(nil)
)

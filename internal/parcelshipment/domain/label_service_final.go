package domain

import (
	"errors"
	"time"
)

// 本文件是面单渠道服务的包裹终局判断（票 label-channel-service-first-release/11）。
//
// CONTEXT 把它判给本上下文：「任何交易级结果或单笔交易中的包裹结果都不直接推动包裹终局；
// `parcel-shipment` 必须跨该包裹全部相关交易及实际承运商收寄事实形成包裹级判断」。产物与网络
// 服务同名——「终局服务结果」——只是形成规则集不同（词条「终局服务结果」：「网络服务和面单
// 渠道服务可以具有不同终局结果」），所以这里只判，不另造第二种终局对象：判出来的格作为一份
// 责任结果进 FormParcelFinalOutcome 那条路。

// ErrInvalidLabelServiceFinalInput 说送进来的输入不是同一件包裹的东西：不覆盖本包裹的交易、
// 别的包裹的登记册。判断不替编排猜它想问哪件包裹。
var ErrInvalidLabelServiceFinalInput = errors.New("parcel shipment: invalid label service final input")

// CarrierFirstEffectivePickupFactReference 指名 transport-fulfillment 拥有的一条实际承运商首次有效收寄事实
// （ADR-0135）。只引用：事实的依据、承运主体、业务发生时间与版本规则都在那边，这里连一列都不复制。
//
// 名字里说的是「收寄事实」而不是「轨迹事实」：外部承运轨迹事实按 TF CONTEXT 本身不构成收寄，那是另一类
// 事实、本上下文不消费它；这里引用的是 TF 判过并登记的收寄，与 TF 侧的 CarrierFirstEffectivePickupReference
// 指同一条记录，多出的「Fact」只为与本文件的 CarrierFirstEffectivePickup（事实 + 版本 + 有效时间的只读引用）分开。
type CarrierFirstEffectivePickupFactReference struct{ requiredValue }

func NewCarrierFirstEffectivePickupFactReference(value string) (CarrierFirstEffectivePickupFactReference, error) {
	required, err := newRequiredValue("carrier first effective pickup fact reference", value)
	return CarrierFirstEffectivePickupFactReference{required}, err
}

// CarrierFirstEffectivePickupFactVersion 是那条事实的版本：源声明更正与有效时间判断在那边都换版本，终局
// 采用按它幂等（同一版本重放返原，新版本走重派生）。
type CarrierFirstEffectivePickupFactVersion struct{ requiredValue }

func NewCarrierFirstEffectivePickupFactVersion(value string) (CarrierFirstEffectivePickupFactVersion, error) {
	required, err := newRequiredValue("carrier first effective pickup fact version", value)
	return CarrierFirstEffectivePickupFactVersion{required}, err
}

// CarrierFirstEffectivePickup 是「实际承运商首次有效收寄」在本上下文里的只读引用：事实引用、
// 版本与那边判出的有效时间。有效时间是终局的生效时间——那是收寄发生的业务时间，不是本上下文
// 读到它的时间。
type CarrierFirstEffectivePickup struct {
	fact        CarrierFirstEffectivePickupFactReference
	version     CarrierFirstEffectivePickupFactVersion
	effectiveAt time.Time
}

// CarrierFirstEffectivePickupSpec 是建立收寄引用所需的三件。它以规格进命令（同
// ResponsibilityOutcomeSpec 的先例）：引用本体在编排里经 ReferenceCarrierFirstEffectivePickup
// 立起来，调用方递不进一个绕过校验的引用。
type CarrierFirstEffectivePickupSpec struct {
	Fact        CarrierFirstEffectivePickupFactReference
	Version     CarrierFirstEffectivePickupFactVersion
	EffectiveAt time.Time
}

// ReferenceCarrierFirstEffectivePickup 建立引用。三件缺一不立：没有版本就没法幂等，没有有效
// 时间就没有终局生效时间可取——有效时间待判断的事实那边本就不交出来。
func ReferenceCarrierFirstEffectivePickup(spec CarrierFirstEffectivePickupSpec) (CarrierFirstEffectivePickup, error) {
	if !spec.Fact.valid() || !spec.Version.valid() || spec.EffectiveAt.IsZero() {
		return CarrierFirstEffectivePickup{}, ErrInvalidLabelServiceFinalInput
	}
	return CarrierFirstEffectivePickup{fact: spec.Fact, version: spec.Version, effectiveAt: spec.EffectiveAt.UTC()}, nil
}

func (pickup CarrierFirstEffectivePickup) Fact() CarrierFirstEffectivePickupFactReference {
	return pickup.fact
}

func (pickup CarrierFirstEffectivePickup) Version() CarrierFirstEffectivePickupFactVersion {
	return pickup.version
}

func (pickup CarrierFirstEffectivePickup) EffectiveAt() time.Time {
	return pickup.effectiveAt
}

// present 区分「没有收寄事实」与「有」。零值即缺席。
func (pickup CarrierFirstEffectivePickup) present() bool {
	return pickup.fact.valid() && pickup.version.valid() && !pickup.effectiveAt.IsZero()
}

// LabelServiceFinalJudgment 是判断的封闭五格。三格形成终局，各对应 CONTEXT 生命周期节的一条
// 转换；`不形成`带原因；`取消在先`单列——它不是「不形成」的一种：终局已经有了，只是那份终局
// 是取消，非取消终局不得盖到它上面。
type LabelServiceFinalJudgment uint8

const (
	LabelServiceFinalJudgmentInvalid LabelServiceFinalJudgment = iota
	LabelServiceFinalByFirstPickup
	LabelServiceFinalOutcomeByClosure
	LabelServiceFinalFailureByClosure
	LabelServiceNotFinal
	LabelServiceCancellationStands
)

func (judgment LabelServiceFinalJudgment) String() string {
	switch judgment {
	case LabelServiceFinalByFirstPickup:
		return "FINAL_BY_FIRST_PICKUP"
	case LabelServiceFinalOutcomeByClosure:
		return "FINAL_OUTCOME_BY_CLOSURE"
	case LabelServiceFinalFailureByClosure:
		return "FINAL_FAILURE_BY_CLOSURE"
	case LabelServiceNotFinal:
		return "NOT_FINAL"
	case LabelServiceCancellationStands:
		return "CANCELLATION_STANDS"
	default:
		return ""
	}
}

// LabelServiceNotFinalReason 说`不形成`卡在关闭路径的哪一句上。三句逐字取 CONTEXT：没有生效的
// 受控关闭（继续尝试仍开放）、有交易未定案（结果待确认不得按失败处理）、仍有成功且有效的面单
// 结果（它「会继续阻止受控关闭路径形成终局」）。
type LabelServiceNotFinalReason uint8

const (
	LabelServiceNotFinalReasonNone LabelServiceNotFinalReason = iota
	ContinuedAttemptStillOpen
	LabelTransactionNotFinalized
	UsableLabelResultOutstanding
)

func (reason LabelServiceNotFinalReason) String() string {
	switch reason {
	case ContinuedAttemptStillOpen:
		return "CONTINUED_ATTEMPT_STILL_OPEN"
	case LabelTransactionNotFinalized:
		return "LABEL_TRANSACTION_NOT_FINALIZED"
	case UsableLabelResultOutstanding:
		return "USABLE_LABEL_RESULT_OUTSTANDING"
	default:
		return ""
	}
}

// LabelServiceFinalInput 是判断的全部输入。
//
//   - Transactions 是该包裹**全部**相关面单交易，含违反截断边界的边界后交易（CONTEXT：「任何
//     已经实际形成且归属该包裹的面单结果都必须参与终局判断」）；漏一笔的「全部定案」是假话，
//     编排要按包裹整册取，不筛。
//   - LapsedTransactions 指名其对本包裹的成功结果已「依据接受时固定的规则不可逆失效」的交易。
//     有效期规则是实例半边（渠道确认或接受时固定），判断不推算过期——没有规则就没有失效，
//     那笔成功照常阻止终局。
//   - Register 是该包裹的`面单继续尝试决定`登记册（可为空册）。
//   - FirstEffectivePickup 零值即缺席。
//   - CancellationStands 说当前存在有效的取消终局。
type LabelServiceFinalInput struct {
	Parcel               DeclaredParcelID
	Transactions         []LabelTransaction
	LapsedTransactions   []LabelTransactionID
	Register             ContinuedAttemptRegister
	FirstEffectivePickup CarrierFirstEffectivePickup
	CancellationStands   bool
}

// LabelServiceFinalVerdict 是判断的产物：格、原因、所引的证据（收寄事实或生效关闭）与终局
// 生效时间。它不是终局本体——终局由 FormParcelFinalOutcome 在采用路径上形成。
type LabelServiceFinalVerdict struct {
	judgment    LabelServiceFinalJudgment
	reason      LabelServiceNotFinalReason
	pickup      CarrierFirstEffectivePickup
	closure     ContinuedAttemptDecision
	hasClosure  bool
	effectiveAt time.Time
}

func (verdict LabelServiceFinalVerdict) Judgment() LabelServiceFinalJudgment {
	return verdict.judgment
}

// Reason 只在`不形成`时非零。
func (verdict LabelServiceFinalVerdict) Reason() LabelServiceNotFinalReason {
	return verdict.reason
}

// FirstEffectivePickup 在收寄路径上交回所引的事实。
func (verdict LabelServiceFinalVerdict) FirstEffectivePickup() (CarrierFirstEffectivePickup, bool) {
	return verdict.pickup, verdict.pickup.present()
}

// Closure 在关闭路径上交回作为证据的那份生效关闭决定。
func (verdict LabelServiceFinalVerdict) Closure() (ContinuedAttemptDecision, bool) {
	return verdict.closure, verdict.hasClosure
}

// EffectiveAt 是终局的生效时间：收寄路径取收寄的有效时间；关闭路径取关闭生效与最后一份参与
// 判断的面单结果/作废之中较晚者——终局在最后一个条件成立那一刻才成立。不形成时为零值。
func (verdict LabelServiceFinalVerdict) EffectiveAt() time.Time {
	return verdict.effectiveAt
}

// ResponsibilityOutcomeKind 把形成终局的三格译成责任结果种类：收寄与「成功均已作废/失效」都是
// 「面单渠道服务非取消终局结果」，「全部明确失败」是「终局失败结果」。不形成与取消在先答 false。
func (verdict LabelServiceFinalVerdict) ResponsibilityOutcomeKind() (ResponsibilityOutcomeKind, bool) {
	switch verdict.judgment {
	case LabelServiceFinalByFirstPickup, LabelServiceFinalOutcomeByClosure:
		return LabelServiceOutcome, true
	case LabelServiceFinalFailureByClosure:
		return LabelServiceFailure, true
	default:
		return ResponsibilityOutcomeKindInvalid, false
	}
}

// JudgeLabelServiceFinal 跨该包裹全部相关面单交易与收寄事实形成包裹级判断。规则逐句取 CONTEXT：
//
//  1. 已经形成的有效取消结果在先 → 取消在先，不形成非取消终局；
//  2. 实际承运商首次有效收寄 → 非取消终局（不看交易停在哪一格、不看有没有关闭）；
//  3. 否则要先有生效且未被重开的受控关闭——它只派生「不允许新增尝试」，本身不是终局；
//  4. 全部相关交易均已定案——任一结果待确认都不得按失败处理；
//  5. 不存在仍可使用的面单结果——本包裹被受理且未作废、未不可逆失效的成功继续阻止终局；
//  6. 到这里：从未有过成功 → 终局失败结果；有过成功但均已作废/失效 → 终局服务结果。
func JudgeLabelServiceFinal(input LabelServiceFinalInput) (LabelServiceFinalVerdict, error) {
	if !input.Parcel.valid() || input.Register.parcel != input.Parcel {
		return LabelServiceFinalVerdict{}, ErrInvalidLabelServiceFinalInput
	}
	for _, transaction := range input.Transactions {
		if !transaction.covers(input.Parcel) {
			return LabelServiceFinalVerdict{}, ErrInvalidLabelServiceFinalInput
		}
	}

	if input.CancellationStands {
		return LabelServiceFinalVerdict{judgment: LabelServiceCancellationStands}, nil
	}
	if input.FirstEffectivePickup.present() {
		return LabelServiceFinalVerdict{
			judgment:    LabelServiceFinalByFirstPickup,
			pickup:      input.FirstEffectivePickup,
			effectiveAt: input.FirstEffectivePickup.effectiveAt,
		}, nil
	}

	closure, closed := input.Register.standingClosure()
	if !closed {
		return LabelServiceFinalVerdict{judgment: LabelServiceNotFinal, reason: ContinuedAttemptStillOpen}, nil
	}

	lapsed := make(map[LabelTransactionID]bool, len(input.LapsedTransactions))
	for _, id := range input.LapsedTransactions {
		lapsed[id] = true
	}
	effectiveAt := closure.effectiveAt
	everAccepted := false
	for _, transaction := range input.Transactions {
		if !transaction.Finalized() {
			return LabelServiceFinalVerdict{judgment: LabelServiceNotFinal, reason: LabelTransactionNotFinalized}, nil
		}
		if transaction.resultObservedAt.After(effectiveAt) {
			effectiveAt = transaction.resultObservedAt
		}
		result, found := transaction.ParcelResult(input.Parcel)
		if !found || !result.accepted {
			// 未受理是本包裹在这笔交易上的明确失败——按包裹结果判，不按交易级结果推。
			continue
		}
		everAccepted = true
		voidedAt, voided := transaction.voidCovering(input.Parcel)
		if !voided && !lapsed[transaction.id] {
			return LabelServiceFinalVerdict{judgment: LabelServiceNotFinal, reason: UsableLabelResultOutstanding}, nil
		}
		if voided && voidedAt.After(effectiveAt) {
			effectiveAt = voidedAt
		}
	}

	verdict := LabelServiceFinalVerdict{
		judgment:    LabelServiceFinalFailureByClosure,
		closure:     closure,
		hasClosure:  true,
		effectiveAt: effectiveAt,
	}
	if everAccepted {
		verdict.judgment = LabelServiceFinalOutcomeByClosure
	}
	return verdict, nil
}

// voidCovering 说这笔交易上有没有一条渠道作废作用到该包裹（整笔范围或点名该包裹），并交回
// 最后一条的业务时间。作废是追加式后续动作，不改原结果——所以这里读动作清单，不读结果。
func (transaction LabelTransaction) voidCovering(parcel DeclaredParcelID) (time.Time, bool) {
	var latest time.Time
	voided := false
	for _, action := range transaction.followUpActions {
		if action.kind != ChannelVoidAction {
			continue
		}
		applies := action.CoversWholeTransaction()
		for _, scoped := range action.parcels {
			if scoped == parcel {
				applies = true
			}
		}
		if !applies {
			continue
		}
		voided = true
		if action.occurredAt.After(latest) {
			latest = action.occurredAt
		}
	}
	return latest, voided
}

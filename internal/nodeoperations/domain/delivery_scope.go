package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// 可验证交付范围（NO CONTEXT「可验证交付范围」；ADR-0212 决定一至三、五）：一次送站可验证地属于哪些客户账户。它只凭送站预报或
// 绑定已全部确认的送站交接单成立，设备上报的交付方与交接单上的声称都只是线索；它只消歧，不收窄候选，也不决定接收与控制。

// DeliveryScopeBasisKind 是交付范围依据引用的种类：送站预报或送站交接单（ADR-0212 决定七）。
type DeliveryScopeBasisKind uint8

const (
	DeliveryScopeBasisKindInvalid DeliveryScopeBasisKind = iota
	BasisDeliveryPreAdvice
	BasisDeliveryHandoverNote
)

func (kind DeliveryScopeBasisKind) String() string {
	switch kind {
	case BasisDeliveryPreAdvice:
		return "DELIVERY_PRE_ADVICE"
	case BasisDeliveryHandoverNote:
		return "DELIVERY_HANDOVER_NOTE"
	default:
		return ""
	}
}

// ErrInvalidDeliveryScopeBasis 说明一份依据引用立不起来：种类不在词表，或标识为空。
var ErrInvalidDeliveryScopeBasis = errors.New("node operations: invalid delivery scope basis reference")

// ParseDeliveryScopeBasisKind 按线上名认种类，大小写不折叠。
func ParseDeliveryScopeBasisKind(raw string) (DeliveryScopeBasisKind, error) {
	for _, kind := range []DeliveryScopeBasisKind{BasisDeliveryPreAdvice, BasisDeliveryHandoverNote} {
		if kind.String() == raw {
			return kind, nil
		}
	}
	return DeliveryScopeBasisKindInvalid, fmt.Errorf("%w: kind %q", ErrInvalidDeliveryScopeBasis, raw)
}

// DeliveryScopeBasisReference 是（种类，标识）的交付范围依据引用：收寄现场上报的那一格，或一件实物最后归入的那张交接单。零值即
// 「没有依据」——来源没带、既有行没回填，都读作这一格（ADR-0212 决定七）。标识逐字保存，不判它指的东西在不在册：那是推导按此刻的
// 证据状态答的。
type DeliveryScopeBasisReference struct {
	kind       DeliveryScopeBasisKind
	identifier string
}

func NewDeliveryScopeBasisReference(kind DeliveryScopeBasisKind, identifier string) (DeliveryScopeBasisReference, error) {
	if kind.String() == "" {
		return DeliveryScopeBasisReference{}, fmt.Errorf("%w: kind is not in the vocabulary", ErrInvalidDeliveryScopeBasis)
	}
	if strings.TrimSpace(identifier) == "" {
		return DeliveryScopeBasisReference{}, fmt.Errorf("%w: identifier is blank", ErrInvalidDeliveryScopeBasis)
	}
	return DeliveryScopeBasisReference{kind: kind, identifier: identifier}, nil
}

func (basis DeliveryScopeBasisReference) Kind() DeliveryScopeBasisKind { return basis.kind }
func (basis DeliveryScopeBasisReference) Identifier() string           { return basis.identifier }

// Present 报告引用在场；零值答否。
func (basis DeliveryScopeBasisReference) Present() bool {
	return basis.kind.String() != "" && strings.TrimSpace(basis.identifier) != ""
}

// UnitDeliveryScopeBasis 是一件作业实物可取的依据：收寄来源里的引用，与它最后归入的交接单（零值即没归入过）。
type UnitDeliveryScopeBasis struct {
	Source           DeliveryScopeBasisReference
	LastAssignedNote DeliveryHandoverNoteID
}

// Effective 交回这件实物此刻的依据：最后归入的交接单优先，没有归入过的取来源里的引用（ADR-0212 决定三）。归入是为补救而来——
// 来源没带依据、预报没覆盖这个号、原交接单被否认——让来源压过归入，补救就补不上。调用方按它读证据状态，再交给推导。
func (basis UnitDeliveryScopeBasis) Effective() DeliveryScopeBasisReference {
	if basis.LastAssignedNote.valid() {
		return DeliveryScopeBasisReference{kind: BasisDeliveryHandoverNote, identifier: basis.LastAssignedNote.value}
	}
	return basis.Source
}

// DeliveryScopeCheckState 是交付范围核对态的取值（ADR-0212 决定五）。它是信息格，不进 outcome。
type DeliveryScopeCheckState uint8

const (
	DeliveryScopeCheckStateInvalid DeliveryScopeCheckState = iota
	DeliveryScopeVerified
	DeliveryScopeUnverified
	// DeliveryScopeUnconfigured：凭以判断的租户取值（预报有效时窗）没有登记。与未证分开记，因为恢复动作不同：这一格要租户去登记
	// 时窗，未证要客户或节点补证据（ADR-0029 按恢复动作分格）。
	DeliveryScopeUnconfigured
)

func (state DeliveryScopeCheckState) String() string {
	switch state {
	case DeliveryScopeVerified:
		return "VERIFIED"
	case DeliveryScopeUnverified:
		return "UNVERIFIED"
	case DeliveryScopeUnconfigured:
		return "UNCONFIGURED"
	default:
		return ""
	}
}

// DeliveryScopeUnverifiedReason 是未证的原因，封闭词表（ADR-0212 决定五）。零值只在已证与未配置上出现。
type DeliveryScopeUnverifiedReason uint8

const (
	ScopeUnverifiedReasonAbsent DeliveryScopeUnverifiedReason = iota
	ScopeNoBasis
	ScopeBasisNotRegistered
	// ScopeBasisNotApplicable 另附一格细分（DeliveryScopeInapplicability）：节点不符、在时窗外、已撤回。
	ScopeBasisNotApplicable
	ScopeNumberNotPreAdvised
	ScopeBindingsNotAllConfirmed
)

func (reason DeliveryScopeUnverifiedReason) String() string {
	switch reason {
	case ScopeNoBasis:
		return "NO_BASIS"
	case ScopeBasisNotRegistered:
		return "BASIS_NOT_REGISTERED"
	case ScopeBasisNotApplicable:
		return "BASIS_NOT_APPLICABLE"
	case ScopeNumberNotPreAdvised:
		return "NUMBER_NOT_PRE_ADVISED"
	case ScopeBindingsNotAllConfirmed:
		return "BINDINGS_NOT_ALL_CONFIRMED"
	default:
		return ""
	}
}

// DeliveryScopeInapplicability 细分「依据不适用」，取 ADR-0212 决定五括号里列的那几格，另加「不是交接单成员」一格（ADR-0212 文末
// 复核记录）。未证的原因按恢复动作分；这几格的恢复动作相同（补证据），仍单列，是为了让复核的人看得出补哪一样：换节点的依据、在时窗
// 里重交、交一份没撤回的、把它归入一张开放的交接单。
type DeliveryScopeInapplicability uint8

const (
	ScopeInapplicabilityAbsent DeliveryScopeInapplicability = iota
	ScopeNodeMismatch
	ScopeOutsideWindow
	ScopeBasisWithdrawn
	// ScopeNotAHandoverMember：依据是一张送站交接单，这件实物却不在它名下（收寄带来的与归入的成员都算）。交接单成立的范围只落到
	// 它名下的作业实物（NO CONTEXT「可验证交付范围」）；关闭之后才带着单号送来的件不是客户确认过的那份冻结的成员清单里的。
	ScopeNotAHandoverMember
)

func (detail DeliveryScopeInapplicability) String() string {
	switch detail {
	case ScopeNodeMismatch:
		return "NODE_MISMATCH"
	case ScopeOutsideWindow:
		return "OUTSIDE_WINDOW"
	case ScopeBasisWithdrawn:
		return "WITHDRAWN"
	case ScopeNotAHandoverMember:
		return "NOT_A_HANDOVER_MEMBER"
	default:
		return ""
	}
}

// ErrInvalidDeliveryScopeCheck 说明核对态的取值、原因与细分搭配不对，或某一格的线上名不在词表。
var ErrInvalidDeliveryScopeCheck = errors.New("node operations: invalid delivery scope check")

// ParseDeliveryScopeCheckState 按线上名认核对态，大小写不折叠。
func ParseDeliveryScopeCheckState(raw string) (DeliveryScopeCheckState, error) {
	for _, state := range []DeliveryScopeCheckState{DeliveryScopeVerified, DeliveryScopeUnverified, DeliveryScopeUnconfigured} {
		if state.String() == raw {
			return state, nil
		}
	}
	return DeliveryScopeCheckStateInvalid, fmt.Errorf("%w: state %q", ErrInvalidDeliveryScopeCheck, raw)
}

// ParseDeliveryScopeUnverifiedReason 按线上名认未证原因，大小写不折叠。
func ParseDeliveryScopeUnverifiedReason(raw string) (DeliveryScopeUnverifiedReason, error) {
	for _, reason := range []DeliveryScopeUnverifiedReason{ScopeNoBasis, ScopeBasisNotRegistered, ScopeBasisNotApplicable,
		ScopeNumberNotPreAdvised, ScopeBindingsNotAllConfirmed} {
		if reason.String() == raw {
			return reason, nil
		}
	}
	return ScopeUnverifiedReasonAbsent, fmt.Errorf("%w: reason %q", ErrInvalidDeliveryScopeCheck, raw)
}

// ParseDeliveryScopeInapplicability 按线上名认依据不适用的细分，大小写不折叠。
func ParseDeliveryScopeInapplicability(raw string) (DeliveryScopeInapplicability, error) {
	for _, detail := range []DeliveryScopeInapplicability{ScopeNodeMismatch, ScopeOutsideWindow, ScopeBasisWithdrawn, ScopeNotAHandoverMember} {
		if detail.String() == raw {
			return detail, nil
		}
	}
	return ScopeInapplicabilityAbsent, fmt.Errorf("%w: inapplicability %q", ErrInvalidDeliveryScopeCheck, raw)
}

// DeliveryScopeCheck 是一次身份核对记下的交付范围核对态：取值，未证时的原因，依据不适用时的细分。
type DeliveryScopeCheck struct {
	state           DeliveryScopeCheckState
	reason          DeliveryScopeUnverifiedReason
	inapplicability DeliveryScopeInapplicability
}

// NewDeliveryScopeCheck 立一个核对态并验取值、原因与细分的搭配：只有未证带原因，只有依据不适用带细分。它给落册的那一版判断还原用，也给推导
// 自己用，两处同一道门。
func NewDeliveryScopeCheck(
	state DeliveryScopeCheckState,
	reason DeliveryScopeUnverifiedReason,
	inapplicability DeliveryScopeInapplicability,
) (DeliveryScopeCheck, error) {
	switch state {
	case DeliveryScopeVerified, DeliveryScopeUnconfigured:
		if reason != ScopeUnverifiedReasonAbsent || inapplicability != ScopeInapplicabilityAbsent {
			return DeliveryScopeCheck{}, fmt.Errorf("%w: %s carries no reason", ErrInvalidDeliveryScopeCheck, state)
		}
	case DeliveryScopeUnverified:
		if reason.String() == "" {
			return DeliveryScopeCheck{}, fmt.Errorf("%w: unverified needs a reason", ErrInvalidDeliveryScopeCheck)
		}
		if (reason == ScopeBasisNotApplicable) != (inapplicability.String() != "") {
			return DeliveryScopeCheck{}, fmt.Errorf("%w: only basis-not-applicable carries an inapplicability", ErrInvalidDeliveryScopeCheck)
		}
	default:
		return DeliveryScopeCheck{}, fmt.Errorf("%w: state is not in the vocabulary", ErrInvalidDeliveryScopeCheck)
	}
	return DeliveryScopeCheck{state: state, reason: reason, inapplicability: inapplicability}, nil
}

func (check DeliveryScopeCheck) State() DeliveryScopeCheckState        { return check.state }
func (check DeliveryScopeCheck) Reason() DeliveryScopeUnverifiedReason { return check.reason }
func (check DeliveryScopeCheck) Inapplicability() DeliveryScopeInapplicability {
	return check.inapplicability
}

// VerifiableDeliveryScope 是推导的答复：客户账户集合与核对态。集合只在已证时非空。
type VerifiableDeliveryScope struct {
	accounts []CustomerAccountReference
	check    DeliveryScopeCheck
}

// Accounts 交回范围给出的客户账户的副本；未证与未配置时为空。
func (scope VerifiableDeliveryScope) Accounts() []CustomerAccountReference {
	return append([]CustomerAccountReference(nil), scope.accounts...)
}

func (scope VerifiableDeliveryScope) Check() DeliveryScopeCheck { return scope.check }

// DeliveryScopeDerivation 是推导一件作业实物的可验证交付范围所需的全部输入（ADR-0212 决定四至六）。证据状态由调用方按
// Basis.Effective() 读好交进来：依据是预报就填 PreAdvice，是交接单就填 HandoverNote，零值即不在册。HandoverMember 只在依据是交接单
// 时有意义：这件实物此刻是不是那张交接单名下的成员（收寄带来的或归入的），零值即不是。Window 是时窗册在原收寄业务时间的生效行，
// 零值即没有生效行——重核时也按原收寄的业务时间取，不按重核时刻（决定六）。Mark 是外部标识观察，逐字。
type DeliveryScopeDerivation struct {
	TenantID       TenantID
	Basis          UnitDeliveryScopeBasis
	PreAdvice      DeliveryPreAdviceState
	HandoverNote   DeliveryHandoverNoteState
	HandoverMember bool
	Window         PreAdviceValidityWindow
	Node           NodeReference
	ReceivedAt     time.Time
	Mark           string
}

// DeriveVerifiableDeliveryScope 推导一件作业实物的可验证交付范围。纯函数：同样的依据与证据状态答同样的范围，收寄与重核走同一份。
func DeriveVerifiableDeliveryScope(input DeliveryScopeDerivation) VerifiableDeliveryScope {
	basis := input.Basis.Effective()
	if !basis.Present() {
		return unverifiedScope(ScopeNoBasis, ScopeInapplicabilityAbsent)
	}
	switch basis.kind {
	case BasisDeliveryPreAdvice:
		return derivePreAdviceScope(input, basis.identifier)
	case BasisDeliveryHandoverNote:
		return deriveHandoverNoteScope(input, basis.identifier)
	default:
		return unverifiedScope(ScopeNoBasis, ScopeInapplicabilityAbsent)
	}
}

// derivePreAdviceScope 按 ADR-0212 决定二那组条件逐条判，范围落到｛预报的客户账户｝。
//
// 次序有意：只读预报自己就判得了的条件（在册、节点、撤回、号码）都排在时窗前面，时窗是唯一要读租户取值的那一格。这样答未配置只在
// 时窗是唯一挡路的那一格时出现（AT-NO-068「其余条件都满足」）：号码本来就不在列，登了时窗也不会变成已证，答未配置就把人指去做一件
// 不解决问题的事。
func derivePreAdviceScope(input DeliveryScopeDerivation, identifier string) VerifiableDeliveryScope {
	preAdvice := input.PreAdvice.PreAdvice
	// 证据与依据对不上（调用方读错了一份），或预报属别的租户，都与不在册同答：跨租户的预报对这件实物不存在。
	if !input.PreAdvice.Registered() || preAdvice.id.value != identifier || preAdvice.tenantID != input.TenantID {
		return unverifiedScope(ScopeBasisNotRegistered, ScopeInapplicabilityAbsent)
	}
	if preAdvice.node != input.Node {
		return unverifiedScope(ScopeBasisNotApplicable, ScopeNodeMismatch)
	}
	if input.PreAdvice.Withdrawn() {
		return unverifiedScope(ScopeBasisNotApplicable, ScopeBasisWithdrawn)
	}
	if !preAdvice.Lists(input.Mark) {
		return unverifiedScope(ScopeNumberNotPreAdvised, ScopeInapplicabilityAbsent)
	}
	if !input.Window.EffectiveAt(input.ReceivedAt) || input.Window.tenantID != input.TenantID {
		return VerifiableDeliveryScope{check: DeliveryScopeCheck{state: DeliveryScopeUnconfigured}}
	}
	if !input.Window.Admits(preAdvice.expectedArrivalAt, input.ReceivedAt) {
		return unverifiedScope(ScopeBasisNotApplicable, ScopeOutsideWindow)
	}
	return VerifiableDeliveryScope{
		accounts: []CustomerAccountReference{preAdvice.customerAccount},
		check:    DeliveryScopeCheck{state: DeliveryScopeVerified},
	}
}

// deriveHandoverNoteScope 按 ADR-0212 决定三判：全有或全无，范围是声称的整组客户账户，不看号码（交接单是账户级）。交接单不需要
// 时窗。在别的节点开立的交接单对这一次收寄不适用：成员与归入都只在开立的那个节点上发生，收寄上报了别处的交接单是扫错了依据。
//
// 这件实物要是交接单名下的成员（ADR-0212 文末复核记录）：客户渠道只对已关闭的交接单下结论，确认的是关闭那一刻冻结的成员清单，
// 关闭之后才带着单号送来的件借这张单拿到已证，就绕过了客户的确认。开放期间带着单号收寄的件由编排先落收寄成员、再走到这里。成员判
// 在绑定之前：不是成员，补齐绑定也补不出范围，恢复动作是把它归入一张开放的交接单。
func deriveHandoverNoteScope(input DeliveryScopeDerivation, identifier string) VerifiableDeliveryScope {
	note := input.HandoverNote.Note
	if !input.HandoverNote.Registered() || note.id.value != identifier || note.tenantID != input.TenantID {
		return unverifiedScope(ScopeBasisNotRegistered, ScopeInapplicabilityAbsent)
	}
	if note.node != input.Node {
		return unverifiedScope(ScopeBasisNotApplicable, ScopeNodeMismatch)
	}
	if !input.HandoverMember {
		return unverifiedScope(ScopeBasisNotApplicable, ScopeNotAHandoverMember)
	}
	if !input.HandoverNote.ScopeEstablished() {
		return unverifiedScope(ScopeBindingsNotAllConfirmed, ScopeInapplicabilityAbsent)
	}
	return VerifiableDeliveryScope{
		accounts: note.ClaimedAccounts(),
		check:    DeliveryScopeCheck{state: DeliveryScopeVerified},
	}
}

func unverifiedScope(reason DeliveryScopeUnverifiedReason, detail DeliveryScopeInapplicability) VerifiableDeliveryScope {
	return VerifiableDeliveryScope{check: DeliveryScopeCheck{state: DeliveryScopeUnverified, reason: reason, inapplicability: detail}}
}

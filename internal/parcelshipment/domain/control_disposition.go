package domain

import "errors"

// ErrControlDispositionNotAdopted 说的是「有受限项在正文里对不上一行处置」。它与 ErrInvalidControlItemResult
// 分格：后者是交入的处置自己立不住（集外、缺责任引用、往成立项上放），前者是本上下文与
// settlement-accounting 读到了不同版本的正文（换版竞争）或坏数据——恢复动作是内部续办重读，不是改请求，
// 也不折成任一去向（ADR-0132 决定三）。
var ErrControlDispositionNotAdopted = errors.New("parcel shipment: a restricted control item has no failure disposition to adopt")

// ControlFailureDisposition 镜像策略正文里一项控制的失败处置：本上下文自有封闭集，与 party-commercial 的
// 同名词汇不 import（ADR-0025 两侧各有封闭集、适配器全函数翻译）。它只答委托的去向——按策略拒绝，或进入
// 授权处置——不拥有拒绝决定（ADR-0115）。集外取值在构造期报错不吸收：一个本上下文还不认识的处置若静默
// 按 `REJECT` 折，就是替租户决定了委托的去向。
type ControlFailureDisposition uint8

const (
	ControlFailureDispositionInvalid ControlFailureDisposition = iota
	// RejectOnControlFailure 让这一项不成立即确定不通过、照今天形成拒绝。
	RejectOnControlFailure
	// AuthorizedDispositionOnControlFailure 让这一项不成立时把去向交给授权角色——全部受限项都登记它时
	// 任务停`等待授权处置`（ADR-0132 决定一）。
	AuthorizedDispositionOnControlFailure
)

func (disposition ControlFailureDisposition) valid() bool {
	return disposition == RejectOnControlFailure || disposition == AuthorizedDispositionOnControlFailure
}

func (disposition ControlFailureDisposition) String() string {
	switch disposition {
	case RejectOnControlFailure:
		return "REJECT"
	case AuthorizedDispositionOnControlFailure:
		return "AUTHORIZED_DISPOSITION"
	default:
		return ""
	}
}

// ControlResponsibilityReference 是策略正文为一项控制指名的失败或补偿责任方（ADR-0115），开放引用。它答的
// 是「谁承担失败或补偿责任」，不是「谁有权处置」——处置权来自授权规则。本上下文只保存并透出它，不据它裁
// 费用、追偿或通知（ADR-0132 决定四）。
type ControlResponsibilityReference struct{ requiredValue }

func NewControlResponsibilityReference(value string) (ControlResponsibilityReference, error) {
	required, err := newRequiredValue("control responsibility reference", value)
	return ControlResponsibilityReference{required}, err
}

// AdoptedControlDisposition 是本上下文在形成控制判断那一步从策略正文读回、记在受限项上的采用引用：失败处置
// × 责任引用成对在场。记成采用引用而不在 Decide 时重读正文，是 CONTEXT「保存实际采用的规则」——策略换版不
// 改已形成的判断，读面与处置角色看到的也是当时那一版。
type AdoptedControlDisposition struct {
	failureDisposition ControlFailureDisposition
	responsibility     ControlResponsibilityReference
}

// NewAdoptedControlDisposition 只校形状：处置在集内、责任引用在场。两格缺一不成立——正文那一侧（PC
// NewPreAcceptanceControlItem）本就要求两格同在，这里读到半截只可能是翻译错了。
func NewAdoptedControlDisposition(
	disposition ControlFailureDisposition,
	responsibility ControlResponsibilityReference,
) (AdoptedControlDisposition, error) {
	if !disposition.valid() || !responsibility.valid() {
		return AdoptedControlDisposition{}, ErrInvalidControlItemResult
	}
	return AdoptedControlDisposition{failureDisposition: disposition, responsibility: responsibility}, nil
}

func (adopted AdoptedControlDisposition) FailureDisposition() ControlFailureDisposition {
	return adopted.failureDisposition
}

func (adopted AdoptedControlDisposition) Responsibility() ControlResponsibilityReference {
	return adopted.responsibility
}

func (adopted AdoptedControlDisposition) present() bool {
	return adopted.failureDisposition.valid()
}

func (adopted AdoptedControlDisposition) valid() bool {
	return adopted.failureDisposition.valid() && adopted.responsibility.valid()
}

// WithAdoptedDisposition 把从正文读回的处置记到这一项上。只有受限项收：成立项的去向没有意义，带一份等于
// 给读面一个「这项通过了但有处置」的矛盾。一次采用不覆盖：处置是这份判断当时按哪一版正文形成的一部分，
// 第二次采用要么是重放要么是换版后重读，两者都不该改写第一次。结论与原因原样保留——采用只补去向，
// 不改写 settlement-accounting 交回的`业务限制`。
func (item ControlItemResult) WithAdoptedDisposition(adopted AdoptedControlDisposition) (ControlItemResult, error) {
	if !adopted.valid() || item.conclusion != ControlItemRestricted || item.adopted.present() {
		return ControlItemResult{}, ErrInvalidControlItemResult
	}
	item.adopted = adopted
	return item, nil
}

// AdoptedDisposition 交回这一项上的采用引用。成立项、刚由 SA 交回还没读正文的受限项、以及登记册里采用
// 引用落库之前的存量行都报告缺席——缺席是真话，不折成任一处置。
func (item ControlItemResult) AdoptedDisposition() (AdoptedControlDisposition, bool) {
	return item.adopted, item.adopted.present()
}

// AdoptControlDispositions 把按控制种类读回的处置采用到本结果的受限项上（ADR-0132 决定三）。
//
// 只落在受限项上：成立项那一种类的行读到了也不记（没有可采用的去向）。每一个受限项都必须对上一行，对不上
// 交回 ErrControlDispositionNotAdopted、整份不成立——半份采用会让`全部受限项都是 AUTHORIZED_DISPOSITION`
// 这条判据对着一份缺格的结果作答。结论、依据、标识、逐项顺序与占用事实都不因采用而变：采用是给判断
// 补出处，不是重判。`明确无控制`没有受限项可采用，交回 ErrInvalidFinancialControlResult。
func (result FinancialControlResult) AdoptControlDispositions(
	dispositions map[ControlItemKind]AdoptedControlDisposition,
) (FinancialControlResult, error) {
	if len(result.items) == 0 {
		return FinancialControlResult{}, ErrInvalidFinancialControlResult
	}
	items := make([]ControlItemResult, 0, len(result.items))
	for _, item := range result.items {
		if item.conclusion != ControlItemRestricted {
			items = append(items, item)
			continue
		}
		adopted, present := dispositions[item.kind]
		if !present {
			return FinancialControlResult{}, ErrControlDispositionNotAdopted
		}
		withDisposition, err := item.WithAdoptedDisposition(adopted)
		if err != nil {
			return FinancialControlResult{}, err
		}
		items = append(items, withDisposition)
	}
	result.items = items
	return result, nil
}

// AwaitsAuthorizedDisposition 报告这份结果是否该把去向交给授权角色：有受限项，且**全部**受限项采用的失败
// 处置都是 `AUTHORIZED_DISPOSITION`。任一受限项登记 `REJECT`、或还没采用处置（刚交回、或采用引用落库之前
// 的存量行），都答否——一行 `REJECT` 已经替合同定了去向；没读到处置时按 ADR-0125 的过渡口径拒绝，不得凭空
// 进入授权处置。
func (result FinancialControlResult) AwaitsAuthorizedDisposition() bool {
	restricted := false
	for _, item := range result.items {
		if item.conclusion != ControlItemRestricted {
			continue
		}
		restricted = true
		if item.adopted.failureDisposition != AuthorizedDispositionOnControlFailure {
			return false
		}
	}
	return restricted
}

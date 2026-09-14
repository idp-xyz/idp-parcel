package domain

import (
	"errors"
	"fmt"
)

// 外部资金事实的付款人维度，与「真实程序要不要付款人」的登记规则（票 sa-cc/12）。
//
// CONTEXT「税费付款核对」把付款人定为「来源提供或真实程序要求的」维度——两种来处都成立时才必备；Rules 一句
// 「未提供或不适用必须明确记录，规则要求但缺失时保持未决」给出三格的答案：来源未提供是一格要**明确记录**的
// 值，不是空串默认；程序要不要是**登记的规则**，不是编排常量也不是参数登记册里的取值（票 sa-cc/12 裁决 1，形照
// ADR-0137 决定三的规则型目录行）；规则要求而来源没给，核对保持未决而不是拒收、也不是「不适用」。哪个程序要、
// 哪个不要属实例半边，这里一格都不拟。

var (
	// ErrInvalidFundsPayer 说送进来的付款人维度两格都不是（零值）。它是调用方编程错误，不是「未提供」——
	// 「未提供」要显式说出来，零值悄悄当成它，入向登记就分不出「来源明说没有」与「有人忘了填」。
	ErrInvalidFundsPayer = errors.New("customs compliance: invalid funds payer")
	// ErrInvalidPayerRequirement 说规则不是封闭二值里的任何一格（零值）。规则未登记由读口的 found=false 说，
	// 不用零值顶。
	ErrInvalidPayerRequirement = errors.New("customs compliance: invalid payer requirement")
	// ErrFundsPayerRequired 是三停格里「程序要求付款人而来源未提供」那一格的哨兵：核对停在未决并点名缺付款人，
	// 恢复动作是来源补事实——与「规则未配置」（恢复动作是登记方补规则）不是同一格（ADR-0029 按恢复动作分格）。
	ErrFundsPayerRequired = errors.New("customs compliance: the customs procedure requires a payer the source did not provide")
)

type fundsPayerKind uint8

const (
	fundsPayerKindInvalid fundsPayerKind = iota
	fundsPayerProvided
	fundsPayerNotProvided
)

// FundsPayer 是外部资金事实上的付款人维度，两格：来源提供了一条付款人引用；或来源显式未提供。它要进入向登记册、
// 读回、进核对判断三处，所以是一格显式值而不是 `(string, bool)` 一对（裁决 3）——SA 侧 `Payer()` 的那一对由
// 消费侧适配器译成这一格。零值两格都不是。
type FundsPayer struct {
	kind      fundsPayerKind
	reference string
}

// ProvidedFundsPayer 形成「来源提供」那一格；空白引用不是提供。
func ProvidedFundsPayer(reference string) (FundsPayer, error) {
	required, err := newRequiredValue("funds payer reference", reference)
	if err != nil {
		return FundsPayer{}, err
	}
	return FundsPayer{kind: fundsPayerProvided, reference: required.String()}, nil
}

// FundsPayerNotProvided 形成「来源未提供」那一格——CONTEXT 要「明确记录」的就是它。
func FundsPayerNotProvided() FundsPayer {
	return FundsPayer{kind: fundsPayerNotProvided}
}

// Provided 报告来源有没有提供付款人。
func (payer FundsPayer) Provided() bool {
	return payer.kind == fundsPayerProvided
}

// Reference 交回付款人引用；「未提供」那一格为空串。要分辨「未提供」与零值看 Provided 与 valid，不看这里。
func (payer FundsPayer) Reference() string {
	return payer.reference
}

func (payer FundsPayer) valid() bool {
	switch payer.kind {
	case fundsPayerProvided:
		return payer.reference != ""
	case fundsPayerNotProvided:
		return payer.reference == ""
	default:
		return false
	}
}

// Valid 是 valid 的导出面：入向登记与读回都要在越过边界前问一次它是不是两格之一。
func (payer FundsPayer) Valid() bool {
	return payer.valid()
}

// PayerRequirement 是「真实程序要不要求付款人」的登记规则，封闭二值。没有默认：未登记不是任何一格，由读口的
// found=false 表达（编排答「规则未配置」，ADR-0137 决定三同一停点的形）。
type PayerRequirement uint8

const (
	PayerRequirementInvalid PayerRequirement = iota
	PayerRequired
	PayerNotRequired
)

func (requirement PayerRequirement) valid() bool {
	return requirement == PayerRequired || requirement == PayerNotRequired
}

func (requirement PayerRequirement) String() string {
	switch requirement {
	case PayerRequired:
		return "REQUIRED"
	case PayerNotRequired:
		return "NOT_REQUIRED"
	default:
		return ""
	}
}

// Admit 拿规则对着事实的付款人维度折出核对能不能进行：要求而未提供 → ErrFundsPayerRequired（未决，点名缺
// 付款人）；不要求 → 提供与否都放行，「未提供」原样带着进核对；要求且提供 → 放行。两边任一是零值都是调用方
// 编程错误，响亮拒。
func (requirement PayerRequirement) Admit(payer FundsPayer) error {
	if !requirement.valid() {
		return fmt.Errorf("%w: %d", ErrInvalidPayerRequirement, requirement)
	}
	if !payer.valid() {
		return ErrInvalidFundsPayer
	}
	if requirement == PayerRequired && !payer.Provided() {
		return ErrFundsPayerRequired
	}
	return nil
}

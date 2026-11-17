package domain

import (
	"errors"
	"fmt"
)

// 本文件是结算从已固定商业解析读出的销售价格约定（ADR-0380 决定二）。三种答复分得开：有序来源、显式不计销售价、
// 这一版合同没有销售价格约定。空列表造不出第一种，也不拿它顶第三种。

var (
	// ErrInvalidAdoptedSellPriceArrangement 是三种答复之外的形状。恢复动作是把适配器译对，不是本上下文代选一种。
	ErrInvalidAdoptedSellPriceArrangement = errors.New("settlement accounting: invalid adopted sell price arrangement")
)

// NoSellPriceBasis 指名合同凭什么说本合同不计销售价。镜像 party-commercial 同名值对象而不 import 它
// （纪律同 SettlementMethod / CreditBasis）。原串照收、不去空白：这个依据是合同写下的原文。
type NoSellPriceBasis struct{ requiredValue }

func NewNoSellPriceBasis(value string) (NoSellPriceBasis, error) {
	required, err := newRequiredValue("no sell price basis", value)
	return NoSellPriceBasis{required}, err
}

// adoptedSellPriceKind 是三种答复的判别式，不导出。零值不是其中任何一种：调用方先看 Sources、
// NoSellPriceBasis、NotDeclared，三口都不是才是没构造过的零值。
type adoptedSellPriceKind uint8

const (
	adoptedSellPriceSources adoptedSellPriceKind = iota + 1
	adoptedSellPriceNotPriced
	adoptedSellPriceNotDeclared
)

// AdoptedSellPriceArrangement 是结算顺着找结算账户的同一条链多取的那一格（ADR-0380 决定二）：一份已固定
// 的解析采用了哪一版合同，读出的就是那一版的销售价格约定。合同之后发新版，已固定的解析不改口。
type AdoptedSellPriceArrangement struct {
	kind    adoptedSellPriceKind
	sources []EstimatePricingScope
	basis   NoSellPriceBasis
}

// NewOrderedSellPriceSources 建立「一份有序的销售价格来源」答复。顺序就是取价的先后，原样保留、不排序。
// 空列表不是一种合法答复——那是「没有约定」，走 NewNotDeclaredSellPriceArrangement。
func NewOrderedSellPriceSources(sources []EstimatePricingScope) (AdoptedSellPriceArrangement, error) {
	if len(sources) == 0 {
		return AdoptedSellPriceArrangement{}, ErrInvalidAdoptedSellPriceArrangement
	}
	ordered := make([]EstimatePricingScope, 0, len(sources))
	seen := make(map[EstimatePricingScope]struct{}, len(sources))
	for _, source := range sources {
		if !source.valid() {
			return AdoptedSellPriceArrangement{}, ErrInvalidAdoptedSellPriceArrangement
		}
		if _, repeated := seen[source]; repeated {
			return AdoptedSellPriceArrangement{}, fmt.Errorf(
				"%w: one pricing scope appears twice", ErrInvalidAdoptedSellPriceArrangement)
		}
		seen[source] = struct{}{}
		ordered = append(ordered, source)
	}
	return AdoptedSellPriceArrangement{kind: adoptedSellPriceSources, sources: ordered}, nil
}

// NewExplicitlyNotPricedSellPrice 建立「合同显式声明不计销售价」答复。
func NewExplicitlyNotPricedSellPrice(basis NoSellPriceBasis) (AdoptedSellPriceArrangement, error) {
	if !basis.valid() {
		return AdoptedSellPriceArrangement{}, ErrInvalidAdoptedSellPriceArrangement
	}
	return AdoptedSellPriceArrangement{kind: adoptedSellPriceNotPriced, basis: basis}, nil
}

// NewNotDeclaredSellPriceArrangement 建立「这一版合同没有销售价格约定」答复——旧规范化版本，或从未登记。
func NewNotDeclaredSellPriceArrangement() AdoptedSellPriceArrangement {
	return AdoptedSellPriceArrangement{kind: adoptedSellPriceNotDeclared}
}

// Sources 按声明顺序交回销售价格来源（副本）。第二个返回值为 false 时不是「列表丢了」：先问
// NoSellPriceBasis 与 NotDeclared，才分得开是显式不计销售价、没有约定，还是零值。
func (arrangement AdoptedSellPriceArrangement) Sources() ([]EstimatePricingScope, bool) {
	if arrangement.kind != adoptedSellPriceSources {
		return nil, false
	}
	return append([]EstimatePricingScope(nil), arrangement.sources...), true
}

// NoSellPriceBasis 只在合同显式声明不计销售价时给出依据。
func (arrangement AdoptedSellPriceArrangement) NoSellPriceBasis() (NoSellPriceBasis, bool) {
	return arrangement.basis, arrangement.kind == adoptedSellPriceNotPriced
}

// NotDeclared 只在「这一版合同没有销售价格约定」时为真。零值三口都不是，免得没构造过的答复被当成旧合同。
func (arrangement AdoptedSellPriceArrangement) NotDeclared() bool {
	return arrangement.kind == adoptedSellPriceNotDeclared
}

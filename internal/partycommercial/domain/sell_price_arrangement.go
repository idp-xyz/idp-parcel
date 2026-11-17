package domain

import (
	"errors"
	"strings"
)

// 本文件是客户合同版本的销售价格约定（CONTEXT「客户合同版本」，ADR-0380 决定一）：包裹的 SELL 客户费用按哪几个
// 计价范围、依什么顺序取价，由合同在这里声明。它与合同级接受前控制声明、合同层交付条件一样，是合同版本正文的一层，
// 各自可缺——缺席就是这一版合同没有销售价格约定，不补默认。

var (
	// ErrInvalidSellPriceArrangement 是约定立不住：来源列表与不计销售价的依据不是恰一在场，或列表里有一项指不出
	// 计价范围。恢复动作是把合同的这一格写全，不是本上下文代选一种。
	ErrInvalidSellPriceArrangement = errors.New("party commercial: invalid sell price arrangement")
	// ErrDuplicateSellPriceSource 单列一格：同一计价范围在列表里出现两次，后一次永远问不到，写的人多半是想写另一个
	// 范围——这与「没写全」要人做的事不同。
	ErrDuplicateSellPriceSource = errors.New("party commercial: one pricing scope appears twice in the sell price sources")
	// ErrPricingScopeReferenceNotExact 是计价范围引用带着首尾空白。它与「没填」不同：填了，但填的这个名字指不到
	// 任何计价范围，恢复动作是把空白去掉重填。
	ErrPricingScopeReferenceNotExact = errors.New("party commercial: pricing scope reference carries surrounding whitespace")
)

// PricingScopeReference 指名计价一侧价卡登记册选卡键上的计价范围。
//
// 开放引用：本上下文只登记这个名字，不核它在计价一侧有没有卡（ADR-0380 决定一）——那要本上下文读计价的册。它与
// CommercialScopeReference 不是一回事：后者是商业版本自己的适用范围，不是选卡键。
type PricingScopeReference struct{ requiredValue }

// NewPricingScopeReference 不收带首尾空白的名字，也不替写的人去掉：这个名字要逐字对上计价一侧的计价范围标识，而
// 那边的构造门（parcelpricing 领域的 NewPricingScopeID）不收首尾空白，带着空白的名字指不到任何范围；收下它，「同一
// 范围不出现两次」就能被一个空格绕过。判据同 NewExternalReference：要与对方逐字比对的码不带首尾空白。
func NewPricingScopeReference(value string) (PricingScopeReference, error) {
	required, err := newRequiredValue("pricing scope reference", value)
	if err != nil {
		return PricingScopeReference{}, err
	}
	if strings.TrimSpace(value) != value {
		return PricingScopeReference{}, ErrPricingScopeReferenceNotExact
	}
	return PricingScopeReference{required}, nil
}

// NoSellPriceBasis 指名合同凭什么说本合同不计销售价。
type NoSellPriceBasis struct{ requiredValue }

func NewNoSellPriceBasis(value string) (NoSellPriceBasis, error) {
	required, err := newRequiredValue("no sell price basis", value)
	return NoSellPriceBasis{required}, err
}

// ContractSellPriceArrangement 是一份客户合同版本的销售价格约定：一份有序的销售价格来源，或显式不计销售价并带
// 依据。形与同一份正文里的财务控制约定相同（FinancialControlBinding）——两格恰一，零值两者都不是。
//
// 拥有对象是客户合同版本：哪位客户用哪套销售价只在合同上声明这一处（ADR-0380 决定一），不挂在价格政策或结算
// 账户上。
type ContractSellPriceArrangement struct {
	contract CommercialVersion
	sources  []PricingScopeReference
	basis    NoSellPriceBasis
}

// DeclareContractSellPriceArrangement 把销售价格约定绑定到客户合同版本上。sources 的顺序就是取价的顺序，原样保留、不排序：
// 回退只在这份顺序里发生，重排它等于改了合同。
func DeclareContractSellPriceArrangement(
	contract CommercialVersion,
	sources []PricingScopeReference,
	basis NoSellPriceBasis,
) (ContractSellPriceArrangement, error) {
	if contract.kind != CustomerContractObject ||
		contract.status != CommercialVersionEffective {
		return ContractSellPriceArrangement{}, ErrUnusableContract
	}
	if err := sellPriceArrangementDeclared(sources, basis); err != nil {
		return ContractSellPriceArrangement{}, err
	}
	return ContractSellPriceArrangement{
		contract: contract,
		sources:  append([]PricingScopeReference(nil), sources...),
		basis:    basis,
	}, nil
}

// sellPriceArrangementDeclared 是两格恰一的那道判，抽成一处让发布前的正文输入面（SellPriceArrangementBody）与绑定
// 到版本上的约定过同一道门。空列表与缺列表是同一句话（没有来源），不收两种读法。
func sellPriceArrangementDeclared(sources []PricingScopeReference, basis NoSellPriceBasis) error {
	if (len(sources) > 0) == basis.valid() {
		return ErrInvalidSellPriceArrangement
	}
	named := make(map[PricingScopeReference]struct{}, len(sources))
	for _, source := range sources {
		if !source.valid() {
			return ErrInvalidSellPriceArrangement
		}
		if _, repeated := named[source]; repeated {
			return ErrDuplicateSellPriceSource
		}
		named[source] = struct{}{}
	}
	return nil
}

func (arrangement ContractSellPriceArrangement) Contract() CommercialVersion {
	return arrangement.contract
}

// Sources 按声明的顺序交回销售价格来源（副本）。第二个返回值为 false 即这份合同显式声明不计销售价——不是
// 「列表丢了」。
func (arrangement ContractSellPriceArrangement) Sources() ([]PricingScopeReference, bool) {
	return append([]PricingScopeReference(nil), arrangement.sources...), len(arrangement.sources) > 0
}

// NoSellPriceBasis 只在合同显式声明不计销售价时给出依据。
func (arrangement ContractSellPriceArrangement) NoSellPriceBasis() (NoSellPriceBasis, bool) {
	return arrangement.basis, arrangement.basis.valid()
}

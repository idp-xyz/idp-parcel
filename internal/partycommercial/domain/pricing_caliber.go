package domain

import "errors"

// ErrInvalidTaxCaliber 拒绝立不住的税务口径。
var ErrInvalidTaxCaliber = errors.New("party commercial: invalid tax caliber")

// TaxClassificationReference 指向一份适用税务分类。本上下文只持引用不持分类正文——分类由税务
// 主数据拥有，抄一份进来会多出一处会过期的副本。
type TaxClassificationReference struct{ requiredValue }

func NewTaxClassificationReference(value string) (TaxClassificationReference, error) {
	required, err := newRequiredValue("tax classification reference", value)
	return TaxClassificationReference{required}, err
}

// TaxDisposition 是商业价格规则对税的三值声明（CONTEXT：「必须声明含税、未税或税务不适用」）。
//
// 零值哨兵刻意不落进「不适用」：缺席与「已判定为不适用」要人做的事不同——前者去补声明，后者
// 不必。让零值落进不适用，一次漏填就会静默变成一句商业声明。
type TaxDisposition uint8

const (
	TaxDispositionInvalid TaxDisposition = iota
	TaxInclusive
	TaxExclusive
	TaxNotApplicable
)

func (disposition TaxDisposition) String() string {
	switch disposition {
	case TaxInclusive:
		return "TAX_INCLUSIVE"
	case TaxExclusive:
		return "TAX_EXCLUSIVE"
	case TaxNotApplicable:
		return "TAX_NOT_APPLICABLE"
	default:
		return ""
	}
}

// TaxCaliber 是税务口径：三值声明加它要求的适用分类。
//
// 两者**充要耦合**，不是两个各自校验的字段（票 party-commercial-context-gaps/02 的裁决之一）。
// 含税与未税各自要求一份分类——不同分类下同一个数是不同的钱；「不适用」恰恰要求没有分类，给它
// 挂一个就是在说它适用。拆成两条独立校验会放过两种行，而两种都不报错、都改变报出去的价：声明
// 含税却没有分类（下游不知道按哪套算），以及声明不适用却挂着分类（读起来像适用）。
type TaxCaliber struct {
	disposition    TaxDisposition
	classification TaxClassificationReference
}

func NewTaxCaliber(
	disposition TaxDisposition,
	classification TaxClassificationReference,
) (TaxCaliber, error) {
	switch disposition {
	case TaxInclusive, TaxExclusive:
		if !classification.valid() {
			return TaxCaliber{}, ErrInvalidTaxCaliber
		}
	case TaxNotApplicable:
		if classification.valid() {
			return TaxCaliber{}, ErrInvalidTaxCaliber
		}
	default:
		return TaxCaliber{}, ErrInvalidTaxCaliber
	}
	return TaxCaliber{disposition: disposition, classification: classification}, nil
}

func (caliber TaxCaliber) Disposition() TaxDisposition {
	return caliber.disposition
}

// Classification 的第二个返回值把「不适用因而没有分类」与「本该有却缺了」分开。后者根本
// 造不出来（构造门已拒），所以这里为假只意味着前者——而不带这个布尔的话，调用方拿到一个零值
// 引用时分不出自己遇到的是哪一种。
func (caliber TaxCaliber) Classification() (TaxClassificationReference, bool) {
	return caliber.classification, caliber.classification.valid()
}

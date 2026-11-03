package domain

import (
	"errors"
	"fmt"
)

// ErrInconsistentEffectiveParcels 说一份已接受委托的当前有效包裹立不起来：基线为空、成员重复、基线与首个提交版本的成员集合
// 不等，或件号配到了基线之外。它出自读回的数据而不是调用方的请求，读口据它答读不动，不答查无——说成「没有这单」就把一行坏
// 数据藏起来了。
var ErrInconsistentEffectiveParcels = errors.New("parcel shipment: inconsistent current effective parcels")

// EffectiveParcel 是委托的一件当前有效包裹（GLOSSARY「当前有效包裹」）：内部标识、它在本委托当前有效包裹次序上的位次、客户为
// 它声明的包裹件号，以及它替代的来源包裹。
//
// 位次一经给出不变，也不因排在前面的件被拆分或合并替代而前移：接受基线成员占首个提交版本里的位次，谱系形成的后继按形成先后
// 接在全部基线成员之后。按页取的游标记的是末件位次，位次前移就会让下一页跳件或重出（ADR-0356 越权风险点 4）。
//
// replaces 只在拆分合并形成的后继上非空，列出它替代的来源包裹。包裹身份谱系今天在领域里还没有模型，立得出的件全是基线成员，
// 它恒空；谱系落地那一票在这里接上后继，调用方读的形状不变。
type EffectiveParcel struct {
	parcel         DeclaredParcelID
	position       int
	customerNumber ExternalIdentifierNumber
	hasNumber      bool
	replaces       []DeclaredParcelID
}

func (parcel EffectiveParcel) Parcel() DeclaredParcelID {
	return parcel.parcel
}

// Position 是这一件在委托当前有效包裹次序上的位次，从 1 起。
func (parcel EffectiveParcel) Position() int {
	return parcel.position
}

// CustomerParcelNumber 交回客户为这一件声明的包裹件号；没声明（含后继包裹）答 false。
func (parcel EffectiveParcel) CustomerParcelNumber() (ExternalIdentifierNumber, bool) {
	return parcel.customerNumber, parcel.hasNumber
}

// Replaces 交回这一件替代的来源包裹（拷贝）；不是后继即为空。
func (parcel EffectiveParcel) Replaces() []DeclaredParcelID {
	return append([]DeclaredParcelID(nil), parcel.replaces...)
}

// CurrentEffectiveParcelsOfBaseline 立出谱系落地之前一份已接受委托的当前有效包裹：接受基线的声明成员，次序与位次取首个提交
// 版本，每件配上委托上为它声明的客户包裹件号。
//
// 取首个提交版本而不是基线所在的版本：受控补充只要求成员集合等值、不管次序，按后来的版本排，客户手里那份提交答复的 parcels
// 次序就对不上了（application 的 submittedParcelsOf 按同一个版本排答复）。两者成员集合不等说明数据坏了——补充改不了成员，改
// 成员走关联新委托——这里不择一、不补齐，照实报错。
func CurrentEffectiveParcelsOfBaseline(
	firstVersion []DeclaredParcelID,
	baseline []DeclaredParcelID,
	numbers []CustomerParcelNumberDeclaration,
) ([]EffectiveParcel, error) {
	if len(baseline) == 0 {
		return nil, fmt.Errorf("%w: the acceptance baseline has no member", ErrInconsistentEffectiveParcels)
	}
	members := make(map[DeclaredParcelID]bool, len(baseline))
	for _, member := range baseline {
		if !member.valid() || members[member] {
			return nil, fmt.Errorf("%w: baseline member %q is blank or repeated", ErrInconsistentEffectiveParcels, member.String())
		}
		members[member] = true
	}
	if len(firstVersion) != len(baseline) {
		return nil, fmt.Errorf("%w: the first submission version has %d members, the baseline %d",
			ErrInconsistentEffectiveParcels, len(firstVersion), len(baseline))
	}
	declared := make(map[DeclaredParcelID]ExternalIdentifierNumber, len(numbers))
	for _, declaration := range numbers {
		if !members[declaration.parcel] {
			return nil, fmt.Errorf("%w: a customer parcel number names %q outside the baseline",
				ErrInconsistentEffectiveParcels, declaration.parcel.String())
		}
		if _, twice := declared[declaration.parcel]; twice {
			return nil, fmt.Errorf("%w: %q carries two customer parcel numbers", ErrInconsistentEffectiveParcels, declaration.parcel.String())
		}
		declared[declaration.parcel] = declaration.number
	}

	parcels := make([]EffectiveParcel, 0, len(firstVersion))
	placed := make(map[DeclaredParcelID]bool, len(firstVersion))
	for index, member := range firstVersion {
		if !members[member] || placed[member] {
			return nil, fmt.Errorf("%w: first submission version member %q is outside the baseline or repeated",
				ErrInconsistentEffectiveParcels, member.String())
		}
		placed[member] = true
		number, hasNumber := declared[member]
		parcels = append(parcels, EffectiveParcel{
			parcel:         member,
			position:       index + 1,
			customerNumber: number,
			hasNumber:      hasNumber,
		})
	}
	return parcels, nil
}

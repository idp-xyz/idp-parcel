package domain

import (
	"cmp"
	"slices"
	"time"
)

// shortageKind 是完结时记下的少收。它不是某一次收货事实比出来的那几种。
const shortageKind = "SHORTAGE"

// ClosureNote 是完结说明，逐字保存。零值就是没带。
type ClosureNote struct{ value string }

func NewClosureNote(value string) (ClosureNote, error) {
	required, err := newRequiredValue("closure note", value)
	if err != nil {
		return ClosureNote{}, err
	}
	return ClosureNote{value: required.String()}, nil
}

func (note ClosureNote) String() string { return note.value }

func (note ClosureNote) Given() bool { return note.value != "" }

// PreAdviceShortage 是完结时还没到的一格。商品少收的数量已经折成基本单位。
// 整箱少收是预报上的一个箱号，数量是一箱，不折成件。一行只带其中一格。
type PreAdviceShortage struct {
	goods    GoodsCode
	carton   CartonNumber
	quantity int64
}

func NewPreAdviceShortage(goods GoodsCode, quantity int64) (PreAdviceShortage, error) {
	if goods.String() == "" || quantity <= 0 {
		return PreAdviceShortage{}, ErrInvalidPreAdvice
	}
	return PreAdviceShortage{goods: goods, quantity: quantity}, nil
}

func NewCartonPreAdviceShortage(carton CartonNumber) (PreAdviceShortage, error) {
	if carton.String() == "" {
		return PreAdviceShortage{}, ErrInvalidPreAdvice
	}
	return PreAdviceShortage{carton: carton, quantity: 1}, nil
}

func (line PreAdviceShortage) Goods() GoodsCode { return line.goods }

func (line PreAdviceShortage) Carton() CartonNumber { return line.carton }

func (line PreAdviceShortage) Quantity() int64 { return line.quantity }

func (line PreAdviceShortage) Kind() string { return shortageKind }

func (line PreAdviceShortage) orderKey() string {
	if line.goods.String() != "" {
		return line.goods.String()
	}
	return line.carton.String()
}

// PreAdviceShortageKind 是少收行上的种类字面量。库里的封闭检查与这一词是同一份。
func PreAdviceShortageKind() string { return shortageKind }

// PreAdviceClosure 是一份预报的完结。身份是（租户，预报），至多一行。少收行只在数量大于 0 时出现。
// 有少收时整份少收共用一个差异标识，和收货形成的那一份不是同一个。没有少收就不签发。
type PreAdviceClosure struct {
	tenant      TenantID
	preAdvice   PreAdviceID
	operator    Registrant
	note        ClosureNote
	closedAt    time.Time
	shortages   []PreAdviceShortage
	discrepancy ReceiptDiscrepancyID
}

func NewPreAdviceClosure(
	tenant TenantID,
	preAdvice PreAdviceID,
	operator Registrant,
	note ClosureNote,
	closedAt time.Time,
	shortages []PreAdviceShortage,
	discrepancy ReceiptDiscrepancyID,
) (PreAdviceClosure, error) {
	if tenant.String() == "" || preAdvice.String() == "" || operator.String() == "" || closedAt.IsZero() {
		return PreAdviceClosure{}, ErrInvalidPreAdvice
	}
	if (len(shortages) > 0) != discrepancy.valid() {
		return PreAdviceClosure{}, ErrInvalidPreAdvice
	}
	lines := slices.Clone(shortages)
	slices.SortFunc(lines, func(left, right PreAdviceShortage) int {
		return cmp.Compare(left.orderKey(), right.orderKey())
	})
	seen := map[string]bool{}
	for _, line := range lines {
		goodsGiven := line.goods.String() != ""
		cartonGiven := line.carton.String() != ""
		if goodsGiven == cartonGiven {
			return PreAdviceClosure{}, ErrInvalidPreAdvice
		}
		if cartonGiven && line.quantity != 1 {
			return PreAdviceClosure{}, ErrInvalidPreAdvice
		}
		if goodsGiven && line.quantity <= 0 {
			return PreAdviceClosure{}, ErrInvalidPreAdvice
		}
		if seen[line.orderKey()] {
			return PreAdviceClosure{}, ErrInvalidPreAdvice
		}
		seen[line.orderKey()] = true
	}
	return PreAdviceClosure{
		tenant: tenant, preAdvice: preAdvice, operator: operator, note: note, closedAt: closedAt,
		shortages: lines, discrepancy: discrepancy,
	}, nil
}

func (closure PreAdviceClosure) Tenant() TenantID { return closure.tenant }

func (closure PreAdviceClosure) PreAdvice() PreAdviceID { return closure.preAdvice }

func (closure PreAdviceClosure) Operator() Registrant { return closure.operator }

func (closure PreAdviceClosure) Note() ClosureNote { return closure.note }

func (closure PreAdviceClosure) ClosedAt() time.Time { return closure.closedAt }

func (closure PreAdviceClosure) Shortages() []PreAdviceShortage {
	return slices.Clone(closure.shortages)
}

func (closure PreAdviceClosure) Discrepancy() (ReceiptDiscrepancyID, bool) {
	return closure.discrepancy, closure.discrepancy.valid()
}

// ShortageQuantity 是预报数量减去已入账数量后还没到的那一段，两边都已经是基本单位。
// 已经到齐或超出时是 0：超出的部分是收货当时的多收，完结不再记少收。
func ShortageQuantity(advised, posted int64) (int64, bool) {
	if advised < 0 || posted < 0 {
		return 0, false
	}
	if posted >= advised {
		return 0, true
	}
	return AddQuantity(advised, -posted)
}

package domain

import "errors"

// 发生项有效性修订驱动的两种预期成本纠错（ADR-0232 决定七、八；票 demo-chain-gaps/15）。CONTEXT「运输收费发生项更正、失效或替代时，原
// 预期成本和计算展开继续保留，本上下文追加新的预期成本版本表达计价纠错」：失效那一种长出失效纠错版本，替代那一种由新评价按
// AppendCorrection 重述全额。两种都回指被纠正的那一版，原版一字不动。

// ErrSupplierCostAlreadyWithdrawn：这一版已是失效纠错版本。失效说的是「这份成本不再成立」，再失效一次说不出新东西。
var ErrSupplierCostAlreadyWithdrawn = errors.New("settlement accounting: the supplier expected cost is already withdrawn")

// supplierCostWithdrawalVersionPrefix 是失效纠错版本标识的类别段。
const supplierCostWithdrawalVersionPrefix = "BUYSCW"

// SupplierCostWithdrawalVersionID 是纠正 prior 的那一版失效纠错版本的标识：类别段、一道横线、被纠正版本的标识。由被纠正版本派生而
// 不铸造：一版至多被纠正一次（迁移 SA 0044 的唯一索引），同一封失效信封重投或并发两笔都算出同一个标识，库主键挡住第二份，重放
// 读回比内容即可作答。
func SupplierCostWithdrawalVersionID(prior SupplierCostVersionID) (SupplierCostVersionID, error) {
	if !prior.valid() {
		return SupplierCostVersionID{}, ErrInvalidSupplierCost
	}
	return NewSupplierCostVersionID(supplierCostWithdrawalVersionPrefix + "-" + prior.String())
}

// 两种纠错原因的字面：失效那一种与替代那一种。原因是册上的列，审计按它分辨这一版为什么换了。
const (
	occurrenceInvalidatedCorrection = "OCCURRENCE_INVALIDATED"
	occurrenceSupersededCorrection  = "OCCURRENCE_SUPERSEDED"
)

// OccurrenceInvalidatedCorrection 是发生项失效驱动的那一种纠错原因。
func OccurrenceInvalidatedCorrection() CostCorrectionReason {
	return CostCorrectionReason{requiredValue{value: occurrenceInvalidatedCorrection}}
}

// OccurrenceSupersededCorrection 是发生项替代版本的新评价驱动的那一种纠错原因。
func OccurrenceSupersededCorrection() CostCorrectionReason {
	return CostCorrectionReason{requiredValue{value: occurrenceSupersededCorrection}}
}

// Withdrawn 说这一版是不是失效纠错版本：链头是它，这一项发生项在这个费用项目上此刻没有当前有效的预期成本。
func (cost SupplierExpectedCost) Withdrawn() bool {
	return cost.withdrawn
}

// Withdraw 依发生项的失效版本长出失效纠错版本：版本标识由这一版派生，回指它，原因是发生项失效，发生项引用换成失效的那一版（它就是
// 这次纠错的依据）。计价结果原样带过去而不是记成零：失效说的是「这份成本不再成立」，不是「它重算之后为零」——零是一个价格，
// 失效不是；经营口径按 Withdrawn 不采用这一版，原样的数让读的人看得出撤掉的是哪一笔。
//
// 发生项 ID 必须同一：换 ID 就是另一份成本（AppendCorrection 同一条，ADR-0067 决定三）。
func (cost SupplierExpectedCost) Withdraw(occurrence TransportChargeOccurrence) (SupplierExpectedCost, error) {
	if cost.withdrawn {
		return SupplierExpectedCost{}, ErrSupplierCostAlreadyWithdrawn
	}
	if !occurrence.id.valid() || occurrence.id != cost.occurrence.id || occurrence.version == cost.occurrence.version {
		return SupplierExpectedCost{}, ErrInvalidSupplierCost
	}
	version, err := SupplierCostWithdrawalVersionID(cost.version)
	if err != nil {
		return SupplierExpectedCost{}, err
	}
	withdrawn := cost
	withdrawn.version = version
	withdrawn.occurrence = occurrence
	withdrawn.priorVersion = cost.version
	withdrawn.correctionReason = OccurrenceInvalidatedCorrection()
	withdrawn.withdrawn = true
	return withdrawn, nil
}

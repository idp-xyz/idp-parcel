package accessidentity

import "slices"

// Grants 交出信封里铸造那一刻生效的能力面授予，给「本会话的操作者持有哪几格」的自查读口用。运营决定与作业事实登记
// 两族不在里面：前者按决定种类另问（Decisions），后者按作业范围给、要带设备才铸，不是管理台信封的整格授予。
// 交出的是拷贝，调用方改它不影响信封。
func (envelope OperatorEnvelope) Grants() []CapabilityFace {
	return slices.Clone(envelope.grants)
}

// Decisions 交出信封里铸造那一刻生效的运营决定种类，同 Grants 的拷贝纪律。
func (envelope OperatorEnvelope) Decisions() []DecisionKind {
	return slices.Clone(envelope.decisions)
}

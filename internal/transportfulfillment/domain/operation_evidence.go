package domain

import "strings"

// 运营决定的操作证据（ADR-0303）：关段、明确终止、建派送任务、装载分配、有效时间判断、承运商首次有效收寄判断与运输
// 委托提交，把信封里的提交操作者记在决定上。类型与写法同费用发生登记的登记者（RegistrantReference，「发行方#sub」），
// 不另立一套；不同在于这几处可以缺——规则生效之前落下的存量、执行器或规则形成的那一版、以交付或交接结束的参与，都没有
// 一位提交操作者。它只进审计：重放与内容冲突的比对、准入与状态转换的门都不读它，同一个决定换一位操作者重放仍是这个决定。

// OptionalRegistrantReference 立一个可缺的操作者：空白读作缺席，交回零值；带 NUL 的存不进登记册，照 newRequiredValue
// 的纪律在构造期拒。
func OptionalRegistrantReference(value string) (RegistrantReference, error) {
	if strings.TrimSpace(value) == "" {
		return RegistrantReference{}, nil
	}
	return NewRegistrantReference(value)
}

// Present 报告这一格记没记下操作者。
func (reference RegistrantReference) Present() bool {
	return reference.valid()
}

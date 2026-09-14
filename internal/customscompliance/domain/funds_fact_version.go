package domain

// FundsFactVersion 是外部资金事实的版本字面——与 settlement-accounting 的 `FundsFactVersion` 同字面、与采用
// 信封 ID 里 `<版本>` 那一段同源（票 sa-cc/13 裁决 1）。本上下文只登它作引用：版本链的权威在提供方，这里
// 不推导、不比大小、不复制链（票面红线「不把 SA 的版本链复制成 CC 的第二份」）；哪一版更正了哪一版由
// 登记上的回指说，不由字面猜。更正 / 撤销在提供方是同一事实的新版本（CC CONTEXT「资金退回、付款撤销或
// 外部资金事实更正只作为重新核对的来源事实」），所以版本是登记的键维，不是登记的内容。
type FundsFactVersion struct{ requiredValue }

func NewFundsFactVersion(value string) (FundsFactVersion, error) {
	required, err := newRequiredValue("funds fact version", value)
	return FundsFactVersion{required}, err
}

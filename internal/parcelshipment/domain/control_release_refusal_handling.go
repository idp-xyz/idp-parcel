package domain

// 释放未受理册上一笔被人处理的两项引用：处理人是认证出的运营操作者，依据是处理人核对后留下的说明引用。本上下文只记引用，
// 不拥有操作者目录，也不判依据够不够——有没有权处理由「运营决定」的授予把守，核对得对不对是处理人的责任。

// ControlReleaseRefusalHandlerReference 是处理人的引用，取（发行方，sub）这一对：sub 只在它的发行方之内唯一。
type ControlReleaseRefusalHandlerReference struct{ requiredValue }

func NewControlReleaseRefusalHandlerReference(value string) (ControlReleaseRefusalHandlerReference, error) {
	required, err := newRequiredValue("control release refusal handler reference", value)
	return ControlReleaseRefusalHandlerReference{required}, err
}

// ControlReleaseRefusalHandlingBasis 是处理依据：处理人核对了什么、凭什么认定这一笔已收口。两种处理都必填——在结算侧释放
// 之前也要说明核对了什么，事后读册的人才分得出这一笔是例行放掉还是出过差错。
type ControlReleaseRefusalHandlingBasis struct{ requiredValue }

func NewControlReleaseRefusalHandlingBasis(value string) (ControlReleaseRefusalHandlingBasis, error) {
	required, err := newRequiredValue("control release refusal handling basis", value)
	return ControlReleaseRefusalHandlingBasis{required}, err
}

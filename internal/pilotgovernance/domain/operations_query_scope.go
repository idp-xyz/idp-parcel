package domain

import (
	"errors"
	"fmt"
)

var ErrInvalidOperationsQueryScope = errors.New("pilot governance: invalid operations query scope")

// OperationsScopeReference 指名共享身份能力交来的那份运营授权作用域判断，供审计与
// 续办回溯；它不是权限内容本身，权限内容由认证与授权结果拥有。
type OperationsScopeReference struct{ requiredValue }

func NewOperationsScopeReference(value string) (OperationsScopeReference, error) {
	required, err := newRequiredValue("operations scope reference", value)
	return OperationsScopeReference{required}, err
}

// OperationsQueryScope 是治理登记册运营查阅的授权作用域：只有作用域引用一维
// （ADR-0083 Decision 三）。没有租户维——不是「租户维可选」，是这个维不存在：治理
// 是产品级机制，登记册没有租户列（ADR-0083 Decision 一），册子的最高（也是唯一）
// 隔离边界就是产品实例本身。给查阅作用域挂租户等于在签名上断言一种按租户分片的
// 治理，领域里没有这种东西。
//
// 它是本上下文语言的一部分，不从别的上下文导入共享作用域类型：共享类型让边界在
// 类型上互相依赖——「每上下文自立，各自成形互不参数化」（ADR-0077 Decision 二）。
type OperationsQueryScope struct {
	reference OperationsScopeReference
}

// NewOperationsQueryScope 拒绝空引用：授权能力答不出作用域判断时，该在接入处拒绝
// 这次查询，而不是造一个空作用域让读口自行决定它意味着什么。
func NewOperationsQueryScope(reference OperationsScopeReference) (OperationsQueryScope, error) {
	if !reference.valid() {
		return OperationsQueryScope{}, fmt.Errorf("%w: scope reference is required", ErrInvalidOperationsQueryScope)
	}
	return OperationsQueryScope{reference: reference}, nil
}

func (scope OperationsQueryScope) Reference() OperationsScopeReference {
	return scope.reference
}

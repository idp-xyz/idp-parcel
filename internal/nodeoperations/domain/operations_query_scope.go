package domain

import (
	"errors"
	"fmt"
)

var ErrInvalidOperationsQueryScope = errors.New("node operations: invalid operations query scope")

// OperationsScopeReference 指名共享身份能力交来的那份运营授权作用域判断，供审计与
// 续办回溯；它不是权限内容本身，权限内容由认证与授权结果拥有。
type OperationsScopeReference struct{ requiredValue }

func NewOperationsScopeReference(value string) (OperationsScopeReference, error) {
	required, err := newRequiredValue("operations scope reference", value)
	return OperationsScopeReference{required}, err
}

// OperationsQueryScope 是节点作业运营查阅的授权作用域（ADR-0077 Decision 二）：
// （作用域引用，租户）两维，两样必须非零。
//
// 没有物流节点维、也没有作业位置维——不是「那两维可选」，是它们在这里不是查阅方
// 身份：CONTEXT 把物流节点、作业位置定为作业事实的归属与放置关系，那是登记内容；
// 运营查阅的授权边界只有租户（租户是最高数据隔离边界，ADR-0003）。把归属维当授权
// 维会得到一个看起来更严实、实际把「查得到哪些事实」与「事实发生在哪」混成一格的
// 作用域。
//
// 它是本上下文语言的一部分，不从别的上下文导入共享作用域类型：共享类型让边界在
// 类型上互相依赖——「每上下文自立，各自成形互不参数化」（ADR-0077 Decision 二）。
type OperationsQueryScope struct {
	reference OperationsScopeReference
	tenant    TenantID
}

// NewOperationsQueryScope 拒绝空租户与空引用：授权能力答不出租户时，该在接入处拒绝
// 这次查询，而不是造一个空作用域让每个读口各自决定它是「全租户可见」还是「什么都
// 不可见」。
func NewOperationsQueryScope(
	reference OperationsScopeReference,
	tenant TenantID,
) (OperationsQueryScope, error) {
	if !reference.valid() {
		return OperationsQueryScope{}, fmt.Errorf("%w: scope reference is required", ErrInvalidOperationsQueryScope)
	}
	if !tenant.valid() {
		return OperationsQueryScope{}, fmt.Errorf("%w: tenant is required", ErrInvalidOperationsQueryScope)
	}
	return OperationsQueryScope{reference: reference, tenant: tenant}, nil
}

func (scope OperationsQueryScope) Reference() OperationsScopeReference {
	return scope.reference
}

func (scope OperationsQueryScope) Tenant() TenantID {
	return scope.tenant
}

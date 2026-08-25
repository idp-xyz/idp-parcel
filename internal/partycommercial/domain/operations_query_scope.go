package domain

import (
	"errors"
	"fmt"
)

var ErrInvalidOperationsQueryScope = errors.New("party commercial: invalid operations query scope")

// OperationsScopeReference 指名共享身份能力交来的那份运营授权作用域判断,供审计与
// 续办回溯;它不是权限内容本身,权限内容由认证与授权结果拥有。
type OperationsScopeReference struct{ requiredValue }

func NewOperationsScopeReference(value string) (OperationsScopeReference, error) {
	required, err := newRequiredValue("operations scope reference", value)
	return OperationsScopeReference{required}, err
}

// OperationsQueryScope 是主数据登记目录查阅的授权作用域(ADR-0077 Decision 二,形状
// 同 ADR-0076):(作用域引用,租户)两维,两样必须非零。没有货主客户账户维——不是
// 「客户维可选」,是这个维不存在:目录是租户内部对象,运营查阅的授权边界只有租户
// (租户是最高数据隔离边界,ADR-0003)。按对象或种类过滤可以是查询条件,但那是过滤器
// 不是授权边界。
//
// 它是本上下文语言的一部分,不从 visibilityexception 导入共享类型:共享作用域会让
// 两个上下文的边界在类型上互相依赖(ADR-0077 Decision 二,两个作用域各自成形,谁也
// 不参数化谁)。
type OperationsQueryScope struct {
	reference OperationsScopeReference
	tenant    TenantID
}

// NewOperationsQueryScope 拒绝空租户与空引用:授权能力答不出租户时,该在接入处拒绝
// 这次查询,而不是造一个空作用域让每个读口各自决定它是「全租户可见」还是「什么都
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

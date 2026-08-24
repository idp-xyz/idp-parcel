package domain

import (
	"errors"
	"fmt"
)

var ErrInvalidOperationsQueryScope = errors.New("visibility exception: invalid operations query scope")

// OperationsScopeReference 指名共享身份能力交来的那份运营授权作用域判断,供审计与
// 续办回溯;它不是权限内容本身,权限内容由认证与授权结果拥有。
type OperationsScopeReference struct{ requiredValue }

func NewOperationsScopeReference(value string) (OperationsScopeReference, error) {
	required, err := newRequiredValue("operations scope reference", value)
	return OperationsScopeReference{required}, err
}

// OperationsQueryScope 是运营追踪查阅的授权作用域(ADR-0076、CONTEXT「运营追踪查阅」):
// (作用域引用,租户)两维,两样必须非零。没有货主客户账户维——不是「客户维可选」,
// 是这个维不存在:留一个可选客户维就是把两套披露语义装回同一个口。按客户过滤可以是
// 查询条件,但那是过滤器不是授权边界——运营查阅的授权边界只有租户(租户是最高数据
// 隔离边界,ADR-0003)。
//
// 它与 parcel-shipment 的 AuthorizedQueryScope 互不参数化:后者以「可见账户至少一个」
// 为不变量,空账户集被拒是它的正确性来源;两个作用域各自成形,谁也不是谁的特例。
type OperationsQueryScope struct {
	reference OperationsScopeReference
	tenant    TenantID
}

// NewOperationsQueryScope 拒绝空租户与空引用:授权能力答不出租户时,该在接入处拒绝
// 这次查询,而不是造一个空作用域让每个读口各自决定它是「全租户可见」还是「什么都
// 不可见」(判据与 AuthorizedQueryScope 同款)。
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

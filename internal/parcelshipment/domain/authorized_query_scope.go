package domain

import (
	"errors"
	"fmt"
)

// ErrInvalidQueryScope 表示一份授权查询作用域立不起来——缺段或段坏。它是编程错误一侧：
// 作用域由共享身份/授权能力形成并传入，立不起来说明接入翻译坏了，不是一种业务答案。
var ErrInvalidQueryScope = errors.New("parcel shipment: invalid authorized query scope")

// QueryScopeReference 是共享授权能力为本次可见范围签发的引用。合成日志与证据索引只记
// 它，不记客户原文或其他租户内容（CONTEXT 查询规则）。
type QueryScopeReference struct{ requiredValue }

func NewQueryScopeReference(value string) (QueryScopeReference, error) {
	required, err := newRequiredValue("query scope reference", value)
	return QueryScopeReference{required}, err
}

// AuthorizedQueryScope 是共享身份认证/授权技术能力已经判断并传入的可见范围引用
// （CONTEXT「授权查询作用域」）。本上下文只消费：按租户与可见客户账户过滤自身对象，
// 不拥有企业身份主数据、凭据验证或通用授权策略。
//
// 可见账户至少一个：授权能力答「什么都看不见」时该在接入处拒绝这次查询，而不是造出
// 一个空作用域让每个读口各自决定空集合是「全都可见」还是「全都不可见」。
type AuthorizedQueryScope struct {
	reference QueryScopeReference
	tenant    TenantID
	accounts  []CustomerAccountID
}

func NewAuthorizedQueryScope(
	reference QueryScopeReference,
	tenant TenantID,
	accounts []CustomerAccountID,
) (AuthorizedQueryScope, error) {
	if !reference.valid() {
		return AuthorizedQueryScope{}, fmt.Errorf("%w: scope reference is required", ErrInvalidQueryScope)
	}
	if !tenant.valid() {
		return AuthorizedQueryScope{}, fmt.Errorf("%w: tenant is required", ErrInvalidQueryScope)
	}
	if len(accounts) == 0 {
		return AuthorizedQueryScope{}, fmt.Errorf("%w: at least one visible customer account is required", ErrInvalidQueryScope)
	}
	seen := make(map[string]struct{}, len(accounts))
	owned := make([]CustomerAccountID, len(accounts))
	for index, account := range accounts {
		if !account.valid() {
			return AuthorizedQueryScope{}, fmt.Errorf("%w: customer account is required", ErrInvalidQueryScope)
		}
		// 重复不去重而是拒绝：作用域是授权能力交来的既成判断，重复说明那份判断坏了，
		// 静默修好它会把上游的错误一直藏到对账那天。
		if _, duplicated := seen[account.String()]; duplicated {
			return AuthorizedQueryScope{}, fmt.Errorf("%w: duplicated customer account %s", ErrInvalidQueryScope, account)
		}
		seen[account.String()] = struct{}{}
		owned[index] = account
	}
	return AuthorizedQueryScope{reference: reference, tenant: tenant, accounts: owned}, nil
}

func (scope AuthorizedQueryScope) Reference() QueryScopeReference {
	return scope.reference
}

func (scope AuthorizedQueryScope) TenantID() TenantID {
	return scope.tenant
}

// CustomerAccountIDs 交回可见客户账户的副本：读方改动自己的那份，改不动作用域本体。
func (scope AuthorizedQueryScope) CustomerAccountIDs() []CustomerAccountID {
	copied := make([]CustomerAccountID, len(scope.accounts))
	copy(copied, scope.accounts)
	return copied
}

// ShipmentReadScope 是委托查阅读口吃的可见范围。两种形状各自立得起来：
// AuthorizedQueryScope 点名可见的客户账户；OperatorTenantScope 是持有主数据与运营查阅读的操作者，看见本租户全部账户。
// 不把「空账户集」塞进 AuthorizedQueryScope 来表示全部——那个构造器拒绝空集，就是不让每个读口自己解释空的意思。
type ShipmentReadScope interface {
	TenantID() TenantID
	// RestrictedAccounts 的 restricted 为真时，读口只看见这些账户；为假时看见本租户的全部账户。
	RestrictedAccounts() (accounts []CustomerAccountID, restricted bool)
}

// RestrictedAccounts 把客户侧作用域报成「只看见这些账户」。
func (scope AuthorizedQueryScope) RestrictedAccounts() ([]CustomerAccountID, bool) {
	return scope.CustomerAccountIDs(), true
}

// OperatorTenantScope 是运营侧的委托查阅作用域：整个租户，不点名账户。
type OperatorTenantScope struct {
	reference QueryScopeReference
	tenant    TenantID
}

// NewOperatorTenantScope 要引用与租户都在。引用只进证据索引，不参与过滤。
func NewOperatorTenantScope(reference QueryScopeReference, tenant TenantID) (OperatorTenantScope, error) {
	if !reference.valid() {
		return OperatorTenantScope{}, fmt.Errorf("%w: scope reference is required", ErrInvalidQueryScope)
	}
	if !tenant.valid() {
		return OperatorTenantScope{}, fmt.Errorf("%w: tenant is required", ErrInvalidQueryScope)
	}
	return OperatorTenantScope{reference: reference, tenant: tenant}, nil
}

func (scope OperatorTenantScope) Reference() QueryScopeReference { return scope.reference }

func (scope OperatorTenantScope) TenantID() TenantID { return scope.tenant }

func (scope OperatorTenantScope) RestrictedAccounts() ([]CustomerAccountID, bool) {
	return nil, false
}

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

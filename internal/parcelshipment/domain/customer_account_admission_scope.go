package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// customerAccountScopeShape 是按客户账户立的拟受理范围快照的形状版本，进摘要：形状一改，新旧快照的摘要就不会相等，
// 决定记录里的范围摘要也就不会把两种形状认成同一个范围。
const customerAccountScopeShape = "PS-CUSTOMER-ACCOUNT-ADMISSION-SCOPE/1"

// customerAccountScopeScheme 是这一形状的范围引用的开头。引用的其余部分是转义后的（租户、客户账户），由 / 分隔。
const customerAccountScopeScheme = "customer-account:"

// 租户与客户账户的值只要求非空，可以含 /；不转义的话，「A/B」+「C」与「A」+「B/C」拼出同一个引用，从引用认回租户时
// 就会认错人。% 一并转义，转义本身才可逆。
var (
	customerAccountScopeEscaper   = strings.NewReplacer("%", "%25", "/", "%2F")
	customerAccountScopeUnescaper = strings.NewReplacer("%2F", "/", "%25", "%")
)

// NewCustomerAccountAdmissionScope 立一次提交按（租户、客户账户）的拟受理范围快照。
//
// 本用例的客户来源批次只覆盖一个明确货主客户账户（`UC-PS-001`），首方客户族一个客户端又只绑一个客户账户，所以这一个
// 账户就是这次提交能确定的完整范围。形状的构造与认回（AdmissionScope.CustomerAccount）定在同一处：接入那一侧立快照，
// 归属目录那一侧从快照认租户，两侧各写一份拼法，改了一侧另一侧就静默地认不出来。
func NewCustomerAccountAdmissionScope(tenant TenantID, account CustomerAccountID) (AdmissionScope, error) {
	if !tenant.valid() || !account.valid() {
		return AdmissionScope{}, ErrInvalidAdmissionScope
	}
	referenceValue := customerAccountScopeScheme +
		customerAccountScopeEscaper.Replace(tenant.String()) + "/" +
		customerAccountScopeEscaper.Replace(account.String())
	reference, err := NewAdmissionScopeReference(referenceValue)
	if err != nil {
		return AdmissionScope{}, err
	}
	sum := sha256.Sum256([]byte(customerAccountScopeShape + "\n" + referenceValue))
	digest, err := NewAdmissionScopeDigest("sha256:" + hex.EncodeToString(sum[:]))
	if err != nil {
		return AdmissionScope{}, err
	}
	return NewAdmissionScope(reference, digest)
}

// CustomerAccount 认回一份按客户账户立的拟受理范围覆盖的（租户、客户账户）。不是这一形状的范围答 false。
//
// 认法是拆开引用、再按 NewCustomerAccountAdmissionScope 重立一份、引用与摘要都对上才算：只看引用开头的话，一份手拼的
// 引用配一个对不上的摘要也会被认成某个租户的范围，而读它的人正是要凭租户决定答不答。
func (scope AdmissionScope) CustomerAccount() (TenantID, CustomerAccountID, bool) {
	rest, found := strings.CutPrefix(scope.reference.String(), customerAccountScopeScheme)
	if !found {
		return TenantID{}, CustomerAccountID{}, false
	}
	escapedTenant, escapedAccount, found := strings.Cut(rest, "/")
	if !found {
		return TenantID{}, CustomerAccountID{}, false
	}
	tenant, err := NewTenantID(customerAccountScopeUnescaper.Replace(escapedTenant))
	if err != nil {
		return TenantID{}, CustomerAccountID{}, false
	}
	account, err := NewCustomerAccountID(customerAccountScopeUnescaper.Replace(escapedAccount))
	if err != nil {
		return TenantID{}, CustomerAccountID{}, false
	}
	rebuilt, err := NewCustomerAccountAdmissionScope(tenant, account)
	if err != nil || rebuilt != scope {
		return TenantID{}, CustomerAccountID{}, false
	}
	return tenant, account, true
}

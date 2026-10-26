package partycommercial

import (
	"context"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// CustomerSelfCancellationParty 实现 CancellationRequesterClassSource：首方直客（ADR-0310 取舍 B，请求方即客户本人，不涉委派）
// 的请求方格映射（ADR-0313）。
//
// 请求方引用必须正是委托所属客户账户本人（psdomain.NewCustomerAccountCancellationRequester 造的那一种）才折成「客户」格；别的
// 写法（含别的账户、运营角色）一概不折：交回 formed=false，授权停在`授权不可用`，不在这里猜它是不是运营。运营代办若将来要接，
// 另起一份映射，不扩本映射。
//
// 与资料修订的坐标映射不同，取消授权没有法人、权限等级与范围这几格坐标：取消授权目录按接受时采用的授权规则版本说话，
// 目录只按请求方格取（ServiceStageRulesAdapter.JudgeCancellationAuthority），所以这里只出请求方格。
type CustomerSelfCancellationParty struct{}

var _ CancellationRequesterClassSource = CustomerSelfCancellationParty{}

func (CustomerSelfCancellationParty) FormCancellationParty(
	_ context.Context,
	identity psdomain.SourceIdentity,
	requester psdomain.CancellationRequesterReference,
) (pcdomain.DeclaredCancellationParty, bool, error) {
	if !requester.IsCustomerAccount(identity.CustomerAccountID()) {
		return pcdomain.DeclaredCancellationPartyInvalid, false, nil
	}
	return pcdomain.DeclaredCustomerCancellation, true, nil
}

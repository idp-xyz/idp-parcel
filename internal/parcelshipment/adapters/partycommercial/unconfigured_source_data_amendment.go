package partycommercial

import (
	"context"

	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// UnconfiguredSourceDataAmendmentAuthorizer 是资料修订授权口的未配置替身：对每个询问不读内容、不采信
// 请求方，一律答`授权规则未配置`。
//
// 它不是生产装配的选择——那里接的是 SourceDataAmendmentAuthorizationAdapter（PC 裁定编排 + 真授权册 +
// 真委派册，票 ps-port-remainder/03 落地）。它留给两类地方：装配用例要一个不碰 PC 库、答复不随询问
// 内容变的授权口；以及没有 PC 库可接的路径（那里没有登记册可问，如实答`未配置`比造一份替身规则诚实）。
// 取`未配置`这一格而不是交回 error：error 那格的恢复动作是重试，重试改不了一个还没人登记的规则；
// `未配置`至少说对了要办的事是让规则存在（ADR-0063 对「显式未配置」的分界）。编排据以停在未决
// （`SourceDataAmendmentAuthorityRulesNotConfigured`），不判客户越权，也不放行。
type UnconfiguredSourceDataAmendmentAuthorizer struct{}

var _ psports.SourceDataAmendmentAuthorizer = UnconfiguredSourceDataAmendmentAuthorizer{}

// AuthorizeSourceDataAmendment 不读询问。参数刻意匿名：连签名都不给「读一眼请求方再决定」留位置。
func (UnconfiguredSourceDataAmendmentAuthorizer) AuthorizeSourceDataAmendment(
	context.Context,
	psports.SourceDataAmendmentAuthorizationQuery,
) (psports.SourceDataAmendmentAuthorization, error) {
	return psports.SourceDataAmendmentAuthorization{Outcome: psports.AuthorizationRulesNotConfigured}, nil
}

// 矩阵那一口原先与本文件同居的 UnconfiguredSourceDataRuleDeclaration（一律答 NotDeclared）已随提供方
// 声明读口立起退场，换成 DeclaredSourceDataAmendmentAllowance（票 ps-port-remainder/02 余段，ADR-0120）。

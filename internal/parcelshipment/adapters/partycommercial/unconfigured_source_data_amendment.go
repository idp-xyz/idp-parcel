package partycommercial

import (
	"context"

	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// UnconfiguredSourceDataAmendmentAuthorizer 是资料修订授权口在提供方那半还没立起来之前的如实答复：
// 对每个询问不读内容、不采信请求方，一律答`授权规则未配置`。
//
// 提供方缺的不是一条登记（party-commercial 的授权动作封闭集今天没有「资料修订」这一格，裁定结果
// 也不带实际决定方——票 ps-port-remainder/03），所以严格说恢复动作是「PC 侧建模」而不是「去登记」。
// 仍取`未配置`这一格而不是交回 error：error 那格的恢复动作是重试，重试改不了一个还没人建的规则；
// `未配置`至少说对了要办的事是让规则存在（ADR-0063 对「显式未配置」的分界）。编排据以停在未决
// （`SourceDataAmendmentAuthorityRulesNotConfigured`），不判客户越权，也不放行。提供方那半落地后
// 在装配点换成照 WithdrawalAuthorizationAdapter 形状的适配器，本类型随之退场。
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

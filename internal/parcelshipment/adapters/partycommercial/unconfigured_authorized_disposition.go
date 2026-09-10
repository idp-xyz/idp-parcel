package partycommercial

import (
	"context"

	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// UnconfiguredAuthorizedDispositionAuthorizer 是处置授权口在提供方那半还没立起来之前的如实答复：对每个
// 询问不读内容、不采信请求方，一律答`授权规则未配置`。
//
// 提供方缺的不是一条登记：party-commercial 的授权动作封闭集今天没有「授权处置」这一格（ADR-0132 越权风险点 2，
// 归 PC 另票），一份处置授权规则今天根本登记不出来，所以形照 ManualReviewAuthorizationAdapter 的翻译适配器
// 也立不起来——它全部的意义在动作守卫上（折出的请求动作必须是处置动作，否则一条只授复核权的规则会被读成
// 处置权），而守卫要比对的那个动作还不存在。仍取`未配置`这一格而不是交回 error：error 那格的恢复动作是重试，
// 重试改不了一个还没人建的规则；`未配置`至少说对了要办的事是让规则存在（ADR-0063 对「显式未配置」的分界）。
// 编排据以停在未决，不判处置人越权，也不放行。提供方那半落地后在装配点换成翻译适配器，本类型随之退场。
type UnconfiguredAuthorizedDispositionAuthorizer struct{}

var _ psports.AuthorizedDispositionAuthorizer = UnconfiguredAuthorizedDispositionAuthorizer{}

// AuthorizeDisposition 不读询问。参数刻意匿名：连签名都不给「读一眼请求方再决定」留位置。
func (UnconfiguredAuthorizedDispositionAuthorizer) AuthorizeDisposition(
	context.Context,
	psports.AuthorizedDispositionAuthorizationQuery,
) (psports.AuthorizedDispositionAuthorization, error) {
	return psports.AuthorizedDispositionAuthorization{Outcome: psports.AuthorizationRulesNotConfigured}, nil
}

package ports

import (
	"context"
	"errors"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// ErrSupplierAgreementScopeUntranslatable 表示协议范围这一问答出了确定性的坏答案：租户、协议指称或 PC 交回的范围
// 译不进结算的词汇表，或那一份指称在 PC 一侧比中不止一版协议。重投同一份永远同一个结果，调用方原样上抛、落发布失败，
// 不答未决（ADR-0208 决定五「词汇表外……上抛为发布失败」）——当成读不动重投，只会以未决之名耗尽失败预算。
var ErrSupplierAgreementScopeUntranslatable = errors.New(
	"settlement accounting: supplier agreement scope answer cannot be translated")

// SupplierAgreementScopeView 按发生项上固定的那一版供应商协议，答它的商业范围，供 BUY 请求作主要范围（ADR-0208）。
//
// 协议版本归 party-commercial，所以这一问只能经消费侧适配器问那一侧（ADR-0025）。按恢复动作分（ADR-0029）：
//   - found=true：交回范围。
//   - found=false：PC 不认这一版协议。发生项在运输履约一侧登记时已核过协议，两侧对不上是矛盾，调用方上抛等人看，
//     不重投、也不替它挑一个范围。
//   - error 包 ErrSupplierAgreementScopeUntranslatable：确定性的坏答案，调用方上抛，同上不重投。
//   - 其余 error：PC 读不动，调用方答未决、整笔回滚等重投。
//
// 不核协议此刻是否生效：发生项形成时那一版协议就固定下来了，协议后来终止不改写在它之下已发生的事实。
type SupplierAgreementScopeView interface {
	LoadSupplierAgreementScope(
		ctx context.Context,
		tenant domain.TenantID,
		agreement domain.SupplierAgreementReference,
	) (domain.PrimaryScopeReference, bool, error)
}

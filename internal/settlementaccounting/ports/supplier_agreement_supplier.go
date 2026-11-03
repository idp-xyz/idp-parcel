package ports

import (
	"context"
	"errors"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// ErrSupplierAgreementSupplierUntranslatable 表示协议供应商这一问答出了确定性的坏答案：租户、协议指称或 PC 交回的供应商译不进
// 结算的词汇表，那一份指称在 PC 一侧比中不止一版协议，或 PC 库里那一版正文存的供应商立不成标识。判据同
// ErrSupplierAgreementScopeUntranslatable：重投同一份永远同一个结果，形成上抛、不答未决，补记把那一版留空列出。
var ErrSupplierAgreementSupplierUntranslatable = errors.New(
	"settlement accounting: supplier agreement supplier answer cannot be translated")

// SupplierAgreementSupplierView 按预期成本所依的那一版供应商协议，答它的供应商（ADR-0358 决定一）。形成编排凭它把供应商记在
// 成本上，补记编排凭它补旧行；匹配门此后只在结算之内比对，不再问。
//
// 协议版本归 party-commercial，所以这一问只能经消费侧适配器问那一侧（ADR-0025）。三格同 SupplierAgreementScopeView：
//   - found=true：交回供应商。
//   - found=false：PC 不认这一版协议。成本所依的协议来自发生项，运输履约登记发生项时已核过，两侧对不上是矛盾，调用方
//     上抛或留空等人看，不重投、也不替它挑一家。
//   - error 包 ErrSupplierAgreementSupplierUntranslatable：确定性的坏答案，同上。
//   - 其余 error：PC 读不动，调用方答未决、整笔回滚等重投。
//
// 不核协议此刻是否生效：已发布版本的正文不可覆盖，同一版协议问几次都是同一家。
type SupplierAgreementSupplierView interface {
	LoadSupplierAgreementSupplier(
		ctx context.Context,
		tenant domain.TenantID,
		agreement domain.SupplierAgreementReference,
	) (domain.SupplierPartyReference, bool, error)
}

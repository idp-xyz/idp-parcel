package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// SupplierAgreementSupplierView 按「对象/版本」两段式指称答那一版供应商协议正文上的供应商（ADR-0358 决定一）。
//
// 与 SupplierAgreementScopeView 分开立、只交供应商，理由同那一口：问话的一方（结算形成供应商预期成本、补记旧成本）手里只有
// 成本所依那一版协议的指称，要的只是「这一版协议是哪家供应商的」，好让账单只对得上同一家的成本。整份正文交出去，就把生效判断、
// 法人与范围也交给一个用不着它们的消费方。
//
// 三格同 SupplierAgreementScopeView：found=true 交回供应商；found=false 是这个租户没有这一版协议的正文；读取失败与坏数据走
// error，指称比中不止一版的那一种包 ErrSupplierAgreementLabelAmbiguous。不核协议此刻是否生效：已发布版本的正文不可覆盖，
// 同一版协议问几次都是同一家，协议后来终止也不改写在它之下形成过的成本。
type SupplierAgreementSupplierView interface {
	LoadSupplierAgreementSupplier(
		ctx context.Context,
		tenant domain.TenantID,
		agreement domain.CommercialVersionLabel,
	) (domain.PartyID, bool, error)
}

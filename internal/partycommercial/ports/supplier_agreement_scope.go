package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// SupplierAgreementScopeView 按「对象/版本」两段式指称答那一版供应商协议正文上的适用范围（ADR-0208）。
//
// 另开一份文件、只交范围：问话的一方（结算按运输收费发生项发起 BUY 评价）手里只有发生项上固定的那一版协议的指称，
// 要的只是「这份协议在哪个商业范围里约定」。整份正文与版本壳交出去，就等于把生效判断、供应商与法人也一并交给一个
// 用不着它们的消费方。
//
// 指称串的形状只认 domain.NewQualifiedVersionLabel 那一处拼出的样子；实现方按那一处去比，不另写分隔符。
//
// 三格：found=true 交回范围；found=false 是这个租户没有这一版协议的正文；读取失败与坏数据走 error，不折成
// found=false。不核协议此刻是否生效：协议终止不改写在它之下已经发生的事实。
type SupplierAgreementScopeView interface {
	LoadSupplierAgreementScope(
		ctx context.Context,
		tenant domain.TenantID,
		agreement domain.CommercialVersionLabel,
	) (domain.CommercialScopeReference, bool, error)
}

// Package docnumbering 是编号组件在组合根一侧的共享装配（ADR-0355 决定四、六）：汇总各上下文的单据编号类型声明、
// 读加扰密钥那一项实例配置，并按同一份汇总、同一本册装出登记、读与取号编排。
//
// 它只能落在组合根一侧：编号组件在 internal/platform 下，不得反过来导入任何上下文。放在 cmd/internal 而不是写进
// 某一个进程：开立单据、登记规则的各个进程都要同一份汇总与同一项密钥配置，各写一遍就是几张会各自漏登的清单。
package docnumbering

import (
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	shipmentdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// Catalogue 交回汇总后的类型声明表。声明不合规（类型代码重复、默认模板过不了模板检查）是编程错误，调用方带着原因退出。
func Catalogue() (*docnumber.Catalogue, error) {
	return docnumber.NewCatalogue(
		shipmentdomain.DocumentNumberTypes(),
		sadomain.DocumentNumberTypes(),
		vedomain.DocumentNumberTypes(),
		ccdomain.DocumentNumberTypes(),
		nodomain.DocumentNumberTypes(),
		tfdomain.DocumentNumberTypes(),
	)
}

// Package docnumbertypes 汇总各上下文的单据编号类型声明（ADR-0355 决定六：类型与默认模板由所属上下文各声明一处，
// 装配时汇总）。
//
// 汇总只能落在组合根一侧：编号组件在 internal/platform 下，不得反过来导入任何上下文。放在 cmd/internal 而不是写进
// parcel-api：在线口与受控登记口两个 main 包都要同一份汇总，各写一遍就是两张会各自漏登的清单。
package docnumbertypes

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

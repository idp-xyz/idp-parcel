package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// ChannelSelectionBasisTranslator 把择优步选中的候选译成建立面单交易所需的「择优结果」（票
// `label-channel/29`）：候选标识本身是 party-commercial 的渠道产品引用，七类依据引用要从那一侧的账号使用
// 授权与供应商协议、以及委托接受时的商业解析上取——编排住应用层，架构门禁不许它导入 party-commercial，
// 所以它看得见的只有这个口，翻译只在 `adapters/partycommercial` 发生。
//
// 费率不在这里取：它随择优步带出（SelectedChannelCandidate.Rate），实现不再问 `parcel-pricing`。
//
// 停下的格由实现具名（授权 / 协议 / 接受时解析未配置、授权渠道不等于候选、授权不在有效期或已撤销、协议
// 不在有效期或已终止……），调用方一律不建立交易、不写决定记录——择优留痕在择优那一步已写完。
type ChannelSelectionBasisTranslator interface {
	TranslateSelectedCandidate(
		ctx context.Context,
		query ChannelSelectionQuery,
		selected domain.SelectedChannelCandidate,
	) (domain.SelectedChannelBasis, error)
}

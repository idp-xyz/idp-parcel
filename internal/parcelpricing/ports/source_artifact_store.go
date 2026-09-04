package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/platform/outbound"
)

// 本文件是抓取原文本体的存放出向端口，形照 ADR-0092 决定二：本体不进领域快照也不进业务库，
// 走一条出向缝；摘要必备而定位符可缺，未配置即如实作答，凭证引用里定位符一格留空、摘要照旧在。
// 不引对象存储依赖（票 06 裁决二）：等对象存储存在时接上的是本端口的一个实现，迁移与契约不动。

// ArtifactPlacement 是存放端口的答复：落在哪一格（ADR-0090 的出向代数，不另立形），以及存成了
// 时本体的定位符。只有 Accepted 那一格的 Locator 有意义；NotConfigured 表示调用没有发生，本体
// 没有存放处——记录照登，凭证不带定位符。
type ArtifactPlacement struct {
	Outcome outbound.Outcome
	Locator string
}

// StoredLocator 只在本体确实存成了时给出定位符；其余各格答空——凭证等级不依赖本体在不在，
// 但「此刻在哪」只能在真存成时回答。
func (placement ArtifactPlacement) StoredLocator() (string, bool) {
	if placement.Outcome.Disposition() != outbound.Accepted || placement.Locator == "" {
		return "", false
	}
	return placement.Locator, true
}

// SourceArtifactStore 存放一份公布记录的原文本体。
//
// FileConnector 场景下本体本来就躺在受控目录里，接的是「原地引用」实现（定位符即来源地址）；
// 出网连接器就位前若无存放处，装配交入一个答 NotConfigured 的实现，不静默丢弃也不假装存过。
type SourceArtifactStore interface {
	Store(ctx context.Context, record domain.PublishedRecord) (ArtifactPlacement, error)
}

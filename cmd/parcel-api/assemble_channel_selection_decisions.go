package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	shipmenthttp "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/http"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
)

// buildChannelSelectionDecisionRead 装配 `/channel-selection-decisions` 的读口（票 label-channel/23）。
//
// 读口就是「渠道择优决定」登记册本尊：同一只 postgres 适配器同时是择优编排写入决定的
// ports.ChannelSelectionDecisionRegistry 与查阅端点消费的 ports.ChannelSelectionDecisionRead，两个契约一只
// 实现（判据同外部承运轨迹事实那格）。读面不是编排——查阅不触发判断、决定或披露，这里没有事务边界要包，
// 也没有「显式未配置」缝：读的都是本上下文自己形成的记录，没有等租户参数的实例半边。
//
// 写那一半的组合根在 assemble_label_channel.go（票 label-channel/28）：择优编排带留痕三件全装，决定记录在择优那一段
// 的事务里写进同一只登记册。但今天没有触发面调那条链（乙路无运营端点，触发面归后继票），且三取数口显式未配置——
// 链在择优装配那一格就停在「未配置」、不写决定，所以生产上这一口读到的仍是空册，而空册是如实答案（ADR-0077
// Decision 四），与「接入渠道未配置」分得开。
func buildChannelSelectionDecisionRead(db *bentopg.DB) (shipmenthttp.ChannelSelectionDecisionsReader, error) {
	decisions, err := pspostgres.NewChannelSelectionDecisions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: channel selection decisions: %w", err)
	}
	return decisions, nil
}

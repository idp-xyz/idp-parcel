package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// LegalEntityRevisionRow 是一个责任法人修订链上的一笔登记修订，照册上那一行转写。
//
// 它与 GroupLegalEntityRow 同源不同物：目录行是「最新修订 + 装载时点导出的状态 + 左连接
// 的参与方名称」，这里是修订本身——没有 Status，也不连名称。状态是对某一时点的推导，一段
// 修订史里每一笔各有自己的生效时点与（可能的）停用时点，给每一笔算一个「此刻的状态」会让
// 早已被后继修订顶替的那几笔也各自显出一格状态，读的人分不清那是历史事实还是此刻判断；
// 名称在参与方册上、不随法人修订走，抄进每一笔等于给同一个名称造 N 份副本。
//
// PartyID 在场：法人钉着哪个业务参与方身份是这一笔修订的内容（内容更正翻旧插新，CONTEXT
// Lifecycles 下「参与方身份（业务参与方、责任法人、货主客户账户）」），两笔之间改了什么由前端
// 并排显，读口只交事实、不做 diff。
// 停用两件只在 HasDeactivation 为真时有意义，判据同 GroupLegalEntityRow。
type LegalEntityRevisionRow struct {
	TenantID          string
	LegalEntityID     string
	PartyID           string
	Revision          int
	Basis             string
	EffectiveFrom     time.Time
	DeactivatedAt     time.Time
	DeactivationBasis string
	HasDeactivation   bool
	RegisteredAt      time.Time
}

// LegalEntityRevisionHistoryRead 是责任法人修订历史的读端口（票 admin-web-group-legal-
// entities/03）：按单个法人展开其登记册上的全部修订，按修订号升序，是集团与法人页详情抽屉
// 「修订历史」区的供数面。
//
// 它单立而不并进 PartyIdentityCatalogueRead 的目录方法：那些上列的是目录行（每身份一行、
// 最新修订），这里展开的是登记册的证据面（一身份多行、全部修订），行形状不同、也没有页大小
// ——一个法人的修订链就是要全部交出，截断的历史不是历史。但它经 PartyIdentityCatalogueRead
// 嵌入随同一个读口参数装配：读的是同一张表、同一租户作用域，供数的是同一页，读口参数跟着
// 页走（判据同 CustomerAccountCatalogueRead 独立成参那条的反面）。
//
// 输入只有租户与法人标识：租户来自授权查询作用域（ADR-0003 隔离边界在 SQL 条件上），法人
// 标识由端点从路径取。不在册与跨租户在这里同形——都是零行、无错——传输层据此答 200 + 空数组
// 而不必、也不能分出「没有」与「别家的」（票 03 按 ADR-0022 的裁决）。
type LegalEntityRevisionHistoryRead interface {
	ListLegalEntityRevisions(
		ctx context.Context,
		tenant domain.TenantID,
		entity domain.LegalEntityReference,
	) ([]LegalEntityRevisionRow, error)
}

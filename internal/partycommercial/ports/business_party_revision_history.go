package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// BusinessPartyRevisionRow 是一个业务参与方修订链上的一笔登记修订，照册上那一行转写。
//
// 它与 BusinessPartyRow 同源不同物，分法同 LegalEntityRevisionRow 对 GroupLegalEntityRow：目录行是
// 「最新修订 + 装载时点导出的状态」，这里是修订本身——没有 Status。一段修订史里每一笔各有自己的
// 生效时点与（可能的）停用时点，给每一笔算一个「此刻的状态」会让早已被后继修订顶替的那几笔也各自
// 显出一格状态，读的人分不清那是历史事实还是此刻判断。
//
// PartyName 在场，这是本册比法人修订行多出的一格：名称就登在 business_party_registration 自己的行上、
// 随修订走（内容更正翻旧插新，CONTEXT Lifecycles 下「参与方身份（业务参与方、责任法人、货主客户账户）」），
// 「名称从哪份换到哪份」正是两笔之间改了什么的一部分——不抄进来，修订链就只剩依据与时点两条可比。
// 法人修订行不带名称是因为名称不在法人册上（理由在 LegalEntityRevisionRow），两册的差别是表形的差别，
// 不是口径分歧。两笔之间改了什么由前端并排显，读口只交事实、不做 diff。
// 停用两件只在 HasDeactivation 为真时有意义，判据同 BusinessPartyRow。
type BusinessPartyRevisionRow struct {
	TenantID          string
	PartyID           string
	PartyName         string
	Revision          int
	Basis             string
	EffectiveFrom     time.Time
	DeactivatedAt     time.Time
	DeactivationBasis string
	HasDeactivation   bool
	RegisteredAt      time.Time
}

// BusinessPartyRevisionHistoryRead 是业务参与方修订历史的读端口（票 admin-web-group-legal-entities/12）：
// 按单个参与方展开其登记册上的全部修订，按修订号升序，是业务参与方页详情抽屉「修订历史」区的供数面。
//
// 它把 LegalEntityRevisionHistoryRead 的形状搬到参与方册，分立与嵌入的裁决一字不改：单立而不并进
// PartyIdentityCatalogueRead 的目录方法（目录上列每身份一行的最新修订、有页大小；这里展开一身份多行的
// 全部修订、无页大小——一个参与方的修订链就是要全部交出，截断的历史不是历史），但经
// PartyIdentityCatalogueRead 嵌入随同一个读口参数装配（读同一张表、同一租户作用域、供同一页）。
//
// 输入只有租户与参与方标识：租户来自授权查询作用域（ADR-0003 隔离边界在 SQL 条件上），参与方标识由
// 端点从路径取。不在册与跨租户在这里同形——都是零行、无错——传输层据此答 200 + 空数组而不必、也不能
// 分出「没有」与「别家的」（票 12 沿票 03 按 ADR-0022 的裁决）。
type BusinessPartyRevisionHistoryRead interface {
	ListBusinessPartyRevisions(
		ctx context.Context,
		tenant domain.TenantID,
		party domain.PartyID,
	) ([]BusinessPartyRevisionRow, error)
}

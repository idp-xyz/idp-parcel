package ports

import (
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件是本上下文其余各列表读口的查询声明（ADR-0144 决定三、四、七；票 catalogue-read-pagination-rollout/11）：可排维与
// 筛选维按答复体的 JSON 字段名点名，查询对象由端点按它解出，各维落到哪一列、q 覆盖哪几列在 postgres 读面里。缺省序保迁移
// 前的首列与方向（该票 spec 取舍 1）。决胜键是行标识、与排序同向（决定三）：迁移前各册同刻时按标识正序收尾，迁移后随
// 排序倒序，只在首列完全相同的行之间次序不同。客户账户那一册早于本文件，声明在 CustomerAccountCatalogue。

// 参与方身份三册（业务参与方、集团法人、参与方关系）与客户账户读同一份登记册（0015），按修订折叠到最新一笔再排：缺省序
// -registeredAt 即迁移前的首列与方向，另放生效时点与行标识两维可排，与 CustomerAccountCatalogue 同形。业务参与方与法人
// 的状态随时钟变，不作可排维（理由同 CustomerAccountCatalogue）。

// identityStatuses 是业务参与方与法人两册 status 的词表：两册与客户账户的 status 出自同一个 domain.IdentityLifecycle 的
// 时点导出，词与 CustomerAccountCatalogue 的那一维逐字相同。
var identityStatuses = cataloguepage.Words[domain.PartyIdentityStatus]()

// BusinessPartyPages 是 /commercial-business-parties 的查询声明。
var BusinessPartyPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "commercial-business-parties",
	Sorts: []cataloguepage.SortDimension{
		{Name: "registeredAt", Kind: cataloguepage.Instant},
		{Name: "effectiveFrom", Kind: cataloguepage.Instant},
		{Name: "partyId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "registeredAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters:     []cataloguepage.FilterDimension{{Name: "status", Vocabulary: identityStatuses}},
})

// GroupLegalEntityPages 是 /commercial-group-legal-entities 的查询声明。registrationCountry 是开放码（ISO 3166 两位字母形状，
// 码表不内置，判据同 0033），不设词表；最新修订未登身份层的法人不匹配任何国家。
var GroupLegalEntityPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "commercial-group-legal-entities",
	Sorts: []cataloguepage.SortDimension{
		{Name: "registeredAt", Kind: cataloguepage.Instant},
		{Name: "effectiveFrom", Kind: cataloguepage.Instant},
		{Name: "legalEntityId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "registeredAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "status", Vocabulary: identityStatuses},
		{Name: "registrationCountry"},
	},
})

// PartyRelationshipPages 是 /commercial-party-relationships 的查询声明。status 是登记的关系状态事实，不随时钟走（与身份
// 两册的时点导出不同，理由在 postgres 读面 ListPartyRelationships 的头注），词表取自 domain.RelationshipStatus；role 取自
// domain.PartyRole。
var PartyRelationshipPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "commercial-party-relationships",
	Sorts: []cataloguepage.SortDimension{
		{Name: "registeredAt", Kind: cataloguepage.Instant},
		{Name: "effectiveStartsAt", Kind: cataloguepage.Instant},
		{Name: "relationshipId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "registeredAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "status", Vocabulary: cataloguepage.Words[domain.RelationshipStatus]()},
		{Name: "role", Vocabulary: cataloguepage.Words[domain.PartyRole]()},
	},
})

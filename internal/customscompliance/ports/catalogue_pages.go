package ports

import (
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件是本上下文各列表读口的查询声明（ADR-0144 决定三、四、七；票 catalogue-read-pagination-rollout/09）：可排维与筛选维
// 按答复体的 JSON 字段名点名，查询对象由端点按它解出，各维落到哪一列、q 覆盖哪几列在 postgres 读面里。
//
// 缺省序保迁移前的次序（该票 spec「缺省序保今天的次序」），所以各册都不是决定三通例的登记时刻倒序：这些登记册按键序上列、读者按键找行，
// 运营记录各按它自己的时刻读先后。有登记时刻一类格子的册把它放出作可排维，要新的在前就 sort=-<那一格>。
//
// 按 registry 分派的路由每种各是一册、各有册名，registry 登为选择器：同一取值是同一份列表，换了取值就是另一册，
// 拿旧游标过去答 400。

// CustomsCasePages 是关务案件查阅（GET /customs-case-records）的声明。
//
// 缺省序是案件标识升序。行标识取案件标识而不是主键：主键（辖区、方向、程序、义务）是「同一法律行为一案」的幂等键，
// 案件标识在租户内同样唯一（customs_case_id_unique），又是答复与各册引用案件时用的那一格。case 是按案件收窄的筛选维——
// 单份读回用的就是它，名字即答复体的字段名；direction 是封闭词表一维。
var CustomsCasePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "customs-case-records",
	Sorts: []cataloguepage.SortDimension{
		{Name: "case", Kind: cataloguepage.Text},
		{Name: "establishedAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "case"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "case"},
		{Name: "direction", Vocabulary: cataloguepage.Words[domain.ManifestDirection]()},
	},
})

// SubmissionVersionPages 是提交版本查阅（GET /customs-submission-records）的声明。
//
// 缺省序是申报单元升序。行标识是（单元、程序、固定时刻、版本号），比主键多插一段固定时刻：版本号随机签发、不带先后，
// 同一申报目标下的更正链只有按固定时刻排才读得出先后。version 是按版本收窄的筛选维——单份读回用的就是它；isCurrent 是版本
// 现行与否，词即答复体里那个布尔的两种写法。
var SubmissionVersionPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "customs-submission-records",
	Sorts: []cataloguepage.SortDimension{
		{Name: "unit", Kind: cataloguepage.Text},
		{Name: "fixedAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "unit"},
	Identity: []cataloguepage.ValueKind{
		cataloguepage.Text, cataloguepage.Text, cataloguepage.Instant, cataloguepage.Text,
	},
	Filters: []cataloguepage.FilterDimension{
		{Name: "version"},
		{Name: "isCurrent", Vocabulary: []string{"true", "false"}},
	},
})

// ExternalResultPages 是外部监管结果查阅（GET /customs-external-result-records）的声明。
//
// 缺省序是记下时刻升序：结果按到达的先后读。行标识是来源标识（主键）。version 是选择器而不是筛选维：它按结果归属的提交版本
// 收窄、空即不收窄，原义不动；答复体里那一格叫 submissionVersion，筛选维的名字取不到 version（决定四）。layer 是结果层的
// 封闭词表——归属不上的响应没有层，按层筛时不在其中。
var ExternalResultPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "customs-external-result-records",
	Sorts: []cataloguepage.SortDimension{
		{Name: "recordedAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "recordedAt"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "layer", Vocabulary: cataloguepage.Words[domain.ResultLayer]()},
	},
	Selectors: []string{"version"},
})

// AdmissionJudgementPages 是动作准入判断册查阅（GET /customs-action-admission-judgements）的声明。
//
// 缺省序是判断时刻倒序，最近的在前；同一刻按判断标识倒序，与排序同向（决定三）。object 是按对象收窄的筛选维——按包裹号
// 精确查用的就是它；answer 是判断的结论，封闭词表一维。
var AdmissionJudgementPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "customs-action-admission-judgements",
	Sorts: []cataloguepage.SortDimension{
		{Name: "judgedAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "judgedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "object"},
		{Name: "answer", Vocabulary: cataloguepage.Words[domain.AdmissionAnswer]()},
	},
})

// UnadmittedObjectPages 是卡关对象查阅（GET /customs-unadmitted-objects）的声明。
//
// 一个对象一行（UnadmittedObjects 头注），行标识是对象。缺省序是动作时点升序，卡得最久的在前。answer 的词表只收不过门的
// 那几格：过门的答复进不了这一册，收进词表只会让一个恒为空的筛选看起来合法。
var UnadmittedObjectPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "customs-unadmitted-objects",
	Sorts: []cataloguepage.SortDimension{
		{Name: "actionAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "actionAt"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "answer", Vocabulary: unadmittedAnswerWords()},
	},
})

// unadmittedAnswerWords 从领域给的全集里按 domain.AdmissionAnswer.Admits 挑出不过门的那几格，不另抄一份词。
func unadmittedAnswerWords() []string {
	var words []string
	for _, answer := range domain.AdmissionAnswers() {
		if !answer.Admits() {
			words = append(words, answer.String())
		}
	}
	return words
}

// CaseRequirementRulePages 是建案要求规则（GET /customs-compliance-rules?registry=case-requirement）的声明。
//
// 缺省序是辖区升序，行标识是登记册键（辖区、方向、程序），两者合起来就是登记册的键序。direction 是封闭词表一维。
var CaseRequirementRulePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "customs-compliance-rules/case-requirement",
	Sorts: []cataloguepage.SortDimension{
		{Name: "jurisdiction", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "jurisdiction"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text, cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "direction", Vocabulary: cataloguepage.Words[domain.ManifestDirection]()},
	},
	Selectors: []string{"registry"},
})

// InterpretationRulePages 是解释规则（GET /customs-compliance-rules?registry=interpretation）的声明。
//
// 缺省序是结果层升序，行标识是登记册键（层、辖区、生效起点）。决胜键与排序同向（决定三）：同一选择键下的各版按生效起点
// 升序；要生效起点晚的在前就 sort=-appliesFrom。layer 是结果层的封闭词表。
var InterpretationRulePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "customs-compliance-rules/interpretation",
	Sorts: []cataloguepage.SortDimension{
		{Name: "layer", Kind: cataloguepage.Text},
		{Name: "appliesFrom", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "layer"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text, cataloguepage.Instant},
	Filters: []cataloguepage.FilterDimension{
		{Name: "layer", Vocabulary: cataloguepage.Words[domain.ResultLayer]()},
	},
	Selectors: []string{"registry"},
})

// ReadinessJudgmentPages 是就绪判断（GET /customs-case-registers?registry=readiness）的声明。
//
// 缺省序是申报单元升序，行标识是单元（主键）。不放筛选维：失效与否由 revokedBy、revokedAt 的在场表达，答复体刻意不设状态字段
// （customshttp 的 readinessJudgmentBody 头注），筛选维名又只取答复体的字段名（决定四），不为筛选另造一格。
var ReadinessJudgmentPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "customs-case-registers/readiness",
	Sorts: []cataloguepage.SortDimension{
		{Name: "unit", Kind: cataloguepage.Text},
		{Name: "judgedAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "unit"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Selectors:   []string{"registry"},
})

// SubmissionAuthorityPages 是提交授权（GET /customs-case-registers?registry=submission-authority）的声明，与就绪判断
// 同形：缺省序是申报单元升序，行标识是单元（主键），不放筛选维的理由也同 ReadinessJudgmentPages。
var SubmissionAuthorityPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "customs-case-registers/submission-authority",
	Sorts: []cataloguepage.SortDimension{
		{Name: "unit", Kind: cataloguepage.Text},
		{Name: "grantedAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "unit"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Selectors:   []string{"registry"},
})

// ClosureObligationPages 是关闭义务目录（GET /customs-case-registers?registry=closure-obligation）的声明。
//
// 一份目录一行，义务项嵌在行里；缺省序是案件升序，行标识是案件（主键）。目录行没有状态，义务项才有：按项状态筛目录分不清
// 筛掉的是项还是目录，不放。
var ClosureObligationPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "customs-case-registers/closure-obligation",
	Sorts: []cataloguepage.SortDimension{
		{Name: "case", Kind: cataloguepage.Text},
		{Name: "registeredAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "case"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Selectors:   []string{"registry"},
})

// GateConditionPages 是门禁条件册（GET /customs-gate-conditions）的声明。
//
// 缺省序是决定范围升序，行标识是登记册三维键（范围、动作、边界），两者合起来就是登记册的键序。action 是受守护动作的封闭词表。
var GateConditionPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "customs-gate-conditions",
	Sorts: []cataloguepage.SortDimension{
		{Name: "scope", Kind: cataloguepage.Text},
		{Name: "registeredAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "scope"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text, cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "action", Vocabulary: cataloguepage.Words[domain.GuardedAction]()},
	},
})

// CandidatePortPages 是口岸目录（GET /customs-ports-paths?registry=candidate-port）的声明。
//
// 缺省序是口岸升序，行标识是（口岸、生效起点）。决胜键与排序同向（决定三）：同一口岸的各版按生效起点升序。目录没有状态一格，
// 在不在期是读者拿区间判读的事（PortsPathsCatalogueRead 头注），不放筛选维。
var CandidatePortPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "customs-ports-paths/candidate-port",
	Sorts: []cataloguepage.SortDimension{
		{Name: "port", Kind: cataloguepage.Text},
		{Name: "appliesFrom", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "port"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Instant},
	Selectors:   []string{"registry"},
})

// DeclarationPathPages 是申报路径目录（GET /customs-ports-paths?registry=declaration-path）的声明。
//
// 缺省序是路径升序，行标识是（路径、生效起点），决胜键与排序同向，同 CandidatePortPages。direction 是封闭词表一维。
var DeclarationPathPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "customs-ports-paths/declaration-path",
	Sorts: []cataloguepage.SortDimension{
		{Name: "path", Kind: cataloguepage.Text},
		{Name: "appliesFrom", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "path"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Instant},
	Filters: []cataloguepage.FilterDimension{
		{Name: "direction", Vocabulary: cataloguepage.Words[domain.ManifestDirection]()},
	},
	Selectors: []string{"registry"},
})

// CredentialPages 是监管凭证册（GET /customs-credentials）的声明。
//
// 缺省序是凭证标识升序，行标识是凭证（主键）。凭证是不可变版本、没有状态一格，有效期是读者判读的事，不放筛选维。
var CredentialPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "customs-credentials",
	Sorts: []cataloguepage.SortDimension{
		{Name: "credential", Kind: cataloguepage.Text},
		{Name: "registeredAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "credential"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
})

// DutyCollaborationPages 是税费付款协作事项册（GET /customs-duty-collaborations）的声明。
//
// 缺省序是决定范围升序，行标识是登记册键（范围、税费引用）；明确无需付款的那一格税费引用是空串，照样是键的一段。kind 是
// 义务依据的封闭词表。
var DutyCollaborationPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "customs-duty-collaborations",
	Sorts: []cataloguepage.SortDimension{
		{Name: "scope", Kind: cataloguepage.Text},
		{Name: "formedAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "scope"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "kind", Vocabulary: cataloguepage.Words[domain.DutyObligationKind]()},
	},
})

// DutyVerificationPages 是税费付款核对册（GET /customs-duty-verifications）的声明。
//
// 缺省序是税费引用升序。行标识是（税费、资金事实、范围、核对时刻、版本指纹），比主键多插一段核对时刻：指纹是内容摘要、
// 不带先后，同三维键的多版本只有按核对时刻排才读得出先后（哪版是当前由读者按它判读，DutyVerificationCatalogueRead 头注）。
// 三轴各是封闭词表、各放一维，不折总状态（ADR-0137 决定三）。
var DutyVerificationPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "customs-duty-verifications",
	Sorts: []cataloguepage.SortDimension{
		{Name: "duty", Kind: cataloguepage.Text},
		{Name: "verifiedAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "duty"},
	Identity: []cataloguepage.ValueKind{
		cataloguepage.Text, cataloguepage.Text, cataloguepage.Text, cataloguepage.Instant, cataloguepage.Text,
	},
	Filters: []cataloguepage.FilterDimension{
		{Name: "coverage", Vocabulary: cataloguepage.Words[domain.DutyCoverage]()},
		{Name: "delta", Vocabulary: cataloguepage.Words[domain.DutyDelta]()},
		{Name: "validity", Vocabulary: cataloguepage.Words[domain.DutyFactValidity]()},
	},
})

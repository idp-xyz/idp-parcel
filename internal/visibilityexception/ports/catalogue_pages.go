package ports

import (
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件是本上下文各列表读口的查询声明（ADR-0144 决定三、四、七；票 catalogue-read-pagination-rollout/03）：可排维与筛选维按
// 答复体的 JSON 字段名点名，查询对象由端点按它解出，各维落到哪一列在 postgres 读面里。
//
// 缺省序保迁移前的次序（该票 spec「缺省序保今天的次序」）；同刻的行按标识与排序维同向收尾（spec「行标识取登记表主键去掉租户
// 那几段」），所以迁移前按标识正序收尾的各册如今同刻倒序收尾。可排维在缺省维之外只放出管理台表格今天已能排的列；筛选维只放出
// 各册的状态或封闭词表，q 覆盖册上列出的标识与名称（spec「可排维、筛选维从小放」）。按 registry 或 kind 分派的端点，选择器的
// 每个取值各是一册、各有册名，选择器键登为 Selectors，不兼作筛选维（决定四）。

// TrackingProjectionPages 是运营追踪投影查阅列表那一支（GET /tracking-projections，parcel 与 version 都不带）的声明；按包裹或
// 按版本取单件的两支不经它。
//
// 本册一个包裹一行（它的当前版），按包裹计量。缺省序是推导时刻倒序、同刻按包裹倒序，与迁移前同一口径。推导时刻随重推导前移：
// 倒序翻页时，途中被重推导的包裹挪到已翻过的那头，本轮可能不再见到它；正序翻页时它挪到游标之后，可能在后面的页再出现一次。
// 投影没有状态一格，不放筛选维；q 覆盖包裹与版本。
var TrackingProjectionPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "tracking-projections",
	Sorts: []cataloguepage.SortDimension{
		{Name: "derivedAt", Kind: cataloguepage.Instant},
		{Name: "parcel", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "derivedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
})

// ExceptionCasePages 是异常案件查阅（GET /exception-case-records）的声明。
//
// 缺省序是建立时刻倒序，与迁移前同一口径。管理台案件表还能按单据号排，但单据号可空（未取得号的案件该格为空），当不了键集游标的
// 排序列（spec「可排维与行标识的列必须非空」），不放出。phase 放出作筛选维（决定四：有状态的册至少放出状态一维），词表取自领域 CasePhase。q 覆盖案件标识、
// 单据号、根包裹与责任团队。
var ExceptionCasePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "exception-case-records",
	Sorts: []cataloguepage.SortDimension{
		{Name: "establishedAt", Kind: cataloguepage.Instant},
		{Name: "rootParcel", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "establishedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "phase", Vocabulary: cataloguepage.Words[domain.CasePhase]()},
	},
})

// SignalEpisodePages 是分诊查阅信号发作期一册（GET /exception-triage-records?registry=signal-episode）的声明。
//
// 缺省序是开始时刻倒序，与迁移前同一口径；同刻收尾从库内序号换成发作期标识，标识取登记表主键（只有发作期标识这一段，不含租户）。lastHitAt 随每次命中
// 前移：按它倒序翻页时，途中再被命中的发作期挪到已翻过的那头，本轮可能不再见到它；正序翻页时它挪到游标之后，可能在后面的页
// 再出现一次。outcome 是行上已记下的分诊结论
// （triage.outcome），词表取自领域 TriageOutcome，尚未分诊的发作期不落在任何一个词上；它只按已有结论收窄，不是分诊决定参数。
// q 覆盖发作期标识、包裹与信号种类。
var SignalEpisodePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "exception-triage-records/signal-episode",
	Sorts: []cataloguepage.SortDimension{
		{Name: "startedAt", Kind: cataloguepage.Instant},
		{Name: "lastHitAt", Kind: cataloguepage.Instant},
		{Name: "parcel", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "startedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "outcome", Vocabulary: cataloguepage.Words[domain.TriageOutcome]()},
	},
	Selectors: []string{"registry"},
})

// DispositionRequestPages 是分诊查阅处置请求一册（GET /exception-triage-records?registry=disposition-request）的声明。
//
// 缺省序是发出时刻倒序，与迁移前同一口径。judgment 是源上下文答复的走向，词表取自领域 SourceJudgment，尚无答复的请求不落在
// 任何一个词上；被替代的请求照列，也按它自己的答复参与筛选。q 覆盖请求标识、案件标识、案件单据号与请求的动作。
var DispositionRequestPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "exception-triage-records/disposition-request",
	Sorts: []cataloguepage.SortDimension{
		{Name: "sentAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "sentAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "judgment", Vocabulary: cataloguepage.Words[domain.SourceJudgment]()},
	},
	Selectors: []string{"registry"},
})

// CustomerNotificationPages 是理赔追偿查阅客户通知一册（GET /claims-recovery-records?registry=customer-notification）的声明。
//
// 缺省序是决定时刻倒序，与迁移前同一口径。通知的进度是行上逐节点登记的序列，答复不把它折成「最新状态」单值，本册也就没有可筛的
// 状态一维。q 覆盖通知标识、客户、发作期与渠道。
var CustomerNotificationPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "claims-recovery-records/customer-notification",
	Sorts: []cataloguepage.SortDimension{
		{Name: "decidedAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "decidedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Selectors:   []string{"registry"},
})

// ClaimItemPages 是理赔追偿查阅索赔项一册（GET /claims-recovery-records?registry=claim-item）的声明。
//
// 缺省序是提交时刻倒序，与迁移前同一口径；标识是批次与索赔项两段。索赔项的状态分在两判上：资格审核 screen（词表取自领域
// EligibilityScreen）与责任结论 conclusion（词表取自领域 LiabilityConclusion），各放出一维，两维之间为与；尚未审核或尚无结论的
// 项不落在那一维的任何词上。q 覆盖批次、索赔项、单据号、客户、目标与索赔类型。
var ClaimItemPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "claims-recovery-records/claim-item",
	Sorts: []cataloguepage.SortDimension{
		{Name: "submittedAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "submittedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "screen", Vocabulary: cataloguepage.Words[domain.EligibilityScreen]()},
		{Name: "conclusion", Vocabulary: cataloguepage.Words[domain.LiabilityConclusion]()},
	},
	Selectors: []string{"registry"},
})

// RecoveryMatterPages 是理赔追偿查阅追偿事项一册（GET /claims-recovery-records?registry=recovery-matter）的声明。
//
// 缺省序是立项时刻倒序，与迁移前同一口径。事项的进展落在分立的预通知与正式追偿两个动作上，行上没有状态一格，不放筛选维。q 覆盖
// 事项标识、案件标识、案件单据号与相对方。
var RecoveryMatterPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "claims-recovery-records/recovery-matter",
	Sorts: []cataloguepage.SortDimension{
		{Name: "openedAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "openedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Selectors:   []string{"registry"},
})

// 目录查阅（GET /visibility-catalogues?kind=）各册。版本册的缺省序是生效起点倒序，与迁移前同一口径，标识是版本；版本行的当前与
// 已闭只由生效区间分辨，没有状态一格，不放筛选维，q 覆盖版本与批准人。声明册各按自己的键正序，键即标识，q 覆盖键与版本或渠道。

// MilestoneMappingCataloguePages 是里程碑映射版本册（kind=MILESTONE_MAPPING）的声明。
var MilestoneMappingCataloguePages = versionCataloguePages("visibility-catalogues/MILESTONE_MAPPING")

// TriageRuleCataloguePages 是分诊规则版本册（kind=TRIAGE_RULE）的声明。
var TriageRuleCataloguePages = versionCataloguePages("visibility-catalogues/TRIAGE_RULE")

// DisclosurePolicyCataloguePages 是披露策略版本册（kind=DISCLOSURE_POLICY）的声明。
var DisclosurePolicyCataloguePages = versionCataloguePages("visibility-catalogues/DISCLOSURE_POLICY")

// ExceptionDisclosureRuleCataloguePages 是异常披露规则版本册（kind=EXCEPTION_DISCLOSURE_RULE）的声明。
var ExceptionDisclosureRuleCataloguePages = versionCataloguePages("visibility-catalogues/EXCEPTION_DISCLOSURE_RULE")

// NotificationPolicyCataloguePages 是通知策略册（kind=NOTIFICATION_POLICY）的声明：一条披露策略引用一行，按策略引用正序。
var NotificationPolicyCataloguePages = keyedCataloguePages("visibility-catalogues/NOTIFICATION_POLICY", "policy")

// ClaimEligibilityCataloguePages 是索赔资格目录（kind=CLAIM_ELIGIBILITY）的声明：一个合同责任范围一行，按合同正序。
var ClaimEligibilityCataloguePages = keyedCataloguePages("visibility-catalogues/CLAIM_ELIGIBILITY", "contract")

// ClaimAuthorizationCataloguePages 是索赔代提授权目录（kind=CLAIM_AUTHORIZATION）的声明：一个货主客户账户一行，按客户正序。
var ClaimAuthorizationCataloguePages = keyedCataloguePages("visibility-catalogues/CLAIM_AUTHORIZATION", "customer")

// ConflictSignalRuleCataloguePages 是冲突信号规则册（kind=CONFLICT_SIGNAL_RULE）的声明。
//
// 缺省序按信号种类正序，不取决定三建议的登记时刻倒序：迁移前就按它排（spec「缺省序保今天的次序」），且这本册一个租户至多一行，
// 主键去掉租户段后不剩一段可作标识，就拿行上的信号种类当标识——一租户一行之下它在租户内唯一，任何次序排出来都是同一页。
var ConflictSignalRuleCataloguePages = keyedCataloguePages("visibility-catalogues/CONFLICT_SIGNAL_RULE", "signalKind")

func versionCataloguePages(name string) *cataloguepage.Catalogue {
	return cataloguepage.MustCatalogue(cataloguepage.Spec{
		Name: name,
		Sorts: []cataloguepage.SortDimension{
			{Name: "effectiveFrom", Kind: cataloguepage.Instant},
		},
		DefaultSort: cataloguepage.Sort{Field: "effectiveFrom", Descending: true},
		Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
		Selectors:   []string{"kind"},
	})
}

func keyedCataloguePages(name, key string) *cataloguepage.Catalogue {
	return cataloguepage.MustCatalogue(cataloguepage.Spec{
		Name:        name,
		Sorts:       []cataloguepage.SortDimension{{Name: key, Kind: cataloguepage.Text}},
		DefaultSort: cataloguepage.Sort{Field: key},
		Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
		Selectors:   []string{"kind"},
	})
}

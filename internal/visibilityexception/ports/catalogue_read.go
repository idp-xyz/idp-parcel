package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件是 VE 规则与策略目录的伴生列表读端口（ADR-0077，票
// admin-web-page-wiring-frontier/02；异常披露规则与冲突信号规则两册随票
// ve-disclosure-policy-view/03 加入）：管理台三张目录查阅页的供数面。它与写入口成
// 镜像——CatalogRegistry 的六个 Register 方法加两个单立登记口，这边逐一各有一个 List
// 方法，封闭集同一份；
// 与既有的装载口（MilestoneMappingView 等）不是一回事：装载口按查询键取一条、
// 交回领域判断的输入，本口上列整册、不重建领域对象、不形成判断。**不拓宽任何一边**：
// 扩装载口会拆全部编排侧测试替身（理由与来历见 parcelpricing/ports/catalogue_read.go
// 的文件注释），扩写口同理。
//
// 上列的是**检索列面**的照实转写：列面与登记字段对齐，不发明列。三份区间型目录
// （映射/分诊/披露）连同整版条目一并上列——「版本 + 条目」是登记的原子（一版条目
// 随抬头一次写全，CatalogRegistry 注释），拆开上列等于把登记原子按表结构切碎。
//
// 租户在方法签名上（ADR-0077 Decision 五）；Limit 必须为正，每页多大由接入面按渠道
// 契约裁决，读口只拒绝无意义的取值；空登记册如实交回空列表（Decision 四：空册本身
// 就是内容，续办是登记责任方去 parcel-ve-register 登记，不折成未配置——注意这与五个
// 装载口的「未配置」格不冲突：装载口答的是「这个判断作不了」，本口答的是「册上有
// 什么」，同一份空册对两个问题的如实答案本就不同）。

// MilestoneMappingEntryRow 是一条映射条目的检索列面：某源上下文的某类事实归到哪个
// 标准里程碑。三列都是登记原词的转写（源上下文封闭五值取 String() 词形）。
type MilestoneMappingEntryRow struct {
	Source    string
	FactKind  string
	Milestone string
}

// MilestoneMappingCatalogueRow 是一版里程碑映射连同整版条目（`PAR-VIS-01`）。
//
// HasEffectiveTo 为假即未闭区间（当前版本）。用显式布尔而不是零值判断：零时刻是
// 一个合法的绝对时刻，拿它兼作「没有终点」会让补历史的区间读不出来。
type MilestoneMappingCatalogueRow struct {
	Version        string
	EffectiveFrom  time.Time
	EffectiveTo    time.Time
	HasEffectiveTo bool
	ApprovedBy     string
	Entries        []MilestoneMappingEntryRow
}

// TriageRuleEntryRow 是一条分诊条目的检索列面：某信号类型在某可信度依据下走哪一格
// （走向封闭四值取 String() 词形）。
type TriageRuleEntryRow struct {
	SignalKind string
	Confidence string
	Outcome    string
}

// TriageRuleCatalogueRow 是一版分诊规则连同整版条目（`PAR-VIS-05`）。
type TriageRuleCatalogueRow struct {
	Version        string
	EffectiveFrom  time.Time
	EffectiveTo    time.Time
	HasEffectiveTo bool
	ApprovedBy     string
	Entries        []TriageRuleEntryRow
}

// NotificationPolicyCatalogueRow 是一条通知策略的检索列面（`PAR-VIS-07`）。这份目录
// 没有版本与区间列：键里的披露策略引用本身承担版本化（0010），换版即换引用、新旧两
// 行并存，所以行上也没有 HasEffectiveTo 可言。
//
// DeadlineAfter 是库存 interval 的文本转写（如 "72:00:00"）——它是**相对量**，绝对
// 截止点在目录上根本不存在（由披露决定时间加出来，加法归 Postgres）。转写成文本而
// 不折回 Go Duration：interval 可携月与日，Duration 装不下那套进位语义，折算就是改写。
type NotificationPolicyCatalogueRow struct {
	Policy        string
	Channel       string
	DeadlineAfter string
	Obligation    string
	ApprovedBy    string
}

// ClaimEligibilityCatalogueRow 是一份索赔资格声明连同承担的索赔类型（`PAR-VIS-08`
// 的合同覆盖角）。声明以（租户+合同范围）为键、版本存在列上（0011），CoveredKinds
// 是覆盖表的整组转写——只有声明在场，「不在集合内」才说得通，所以覆盖集不单独上列。
type ClaimEligibilityCatalogueRow struct {
	Contract     string
	Version      string
	ApprovedBy   string
	CoveredKinds []string
}

// ClaimAuthorizationCatalogueRow 是一个货主客户账户的申请人授权名单（`PAR-VIS-08`
// 的申请人授权角）。Applicants 允许为空：目录在场而名单为空是「这个账户目前不授权
// 任何人代提」的显式声明（0018），与「还没登记」不是一回事——后者由整行不在列表里
// 表达。页面把空名单说成未登记，就是把一次已作出的授权决定读丢了。
type ClaimAuthorizationCatalogueRow struct {
	Customer   string
	Version    string
	ApprovedBy string
	Applicants []string
}

// DisclosureDimensionCell 是披露条目一维的检索列面：封闭三态原词加内容来处引用。
// Content 只在 SHOWN 时非空——展示必带内容来处、待确认与不展示必不带（0012 的四条
// shape 约束），所以空串在这里不歧义，不需要第二个布尔。
type DisclosureDimensionCell struct {
	State   string
	Content string
}

// DisclosurePolicyEntryRow 是一条披露条目：对某货主客户账户，客户视图四维各自获准
// 展示什么。
type DisclosurePolicyEntryRow struct {
	Customer   string
	Milestones DisclosureDimensionCell
	ETA        DisclosureDimensionCell
	Final      DisclosureDimensionCell
	Note       DisclosureDimensionCell
}

// DisclosurePolicyCatalogueRow 是一版披露策略连同整版条目（`PAR-VIS-09`）。
type DisclosurePolicyCatalogueRow struct {
	Version        string
	EffectiveFrom  time.Time
	EffectiveTo    time.Time
	HasEffectiveTo bool
	ApprovedBy     string
	Entries        []DisclosurePolicyEntryRow
}

// ExceptionDisclosureRuleEntryRow 是一条异常披露规则条目的检索列面（0023）：对某货主客户
// 账户的某类信号在某可信度依据下，披露条件成不成立、批准范围允不允许自动发布、内容从哪来。
// Content 只在 Disclosable 时非空——披露必带内容来处、不披露必不带（0023 的成对约束），所以
// 空串在这里不歧义；AutoRelease 不会在 Disclosable 为假时为真（同表第二条约束）。三个布尔
// / 内容列照登转写，读口不重演 ExceptionDisclosureRuleView 那一道成对复核。
type ExceptionDisclosureRuleEntryRow struct {
	Customer    string
	SignalKind  string
	Confidence  string
	Disclosable bool
	AutoRelease bool
	Content     string
}

// ExceptionDisclosureRuleCatalogueRow 是一版异常披露规则连同整版条目（`PAR-VIS-07` 的
// 「披露和自动发布范围」半边；渠道半边是 NotificationPolicyCatalogueRow）。它与披露策略
// （DisclosurePolicyCatalogueRow，0012）是相邻的两本册：那边按客户答客户视图四维各展示
// 什么，这边按客户 × 信号 × 可信度答异常要不要对外说——版本引用同为决定带着走的披露策略
// 引用，但册不同、行形状不同，读面不把两者并成一格。
type ExceptionDisclosureRuleCatalogueRow struct {
	Version        string
	EffectiveFrom  time.Time
	EffectiveTo    time.Time
	HasEffectiveTo bool
	ApprovedBy     string
	Entries        []ExceptionDisclosureRuleEntryRow
}

// ConflictSignalRuleCatalogueRow 是冲突信号规则的检索列面（0025）：无法按业务时间裁决的
// 替代链分叉形成异常信号时，用哪个信号类型、哪一版识别规则、记什么可信度依据。这份目录
// 一租户至多一行（键只有租户）且没有生效区间——换版是一次治理动作而不是接续闭合（0025
// 头注），所以行上没有 HasEffectiveTo；RegisteredAt 是库落下的登记时刻，照实转写。仍以
// 列表交回而不是单值加布尔：与同族各法同形，空册同样如实答空列表。
type ConflictSignalRuleCatalogueRow struct {
	SignalKind   string
	Version      string
	Confidence   string
	ApprovedBy   string
	RegisteredAt time.Time
}

// CatalogueListRead 是 VE 目录的伴生列表读端口。八个方法一口装下而不按页面分组拆
// 三个接口：分组（判断规则/披露口径/索赔前置）是页面层的呈现裁决，端口的封闭集要
// 与写入口逐一对上——目录册各法对 CatalogRegistry 的各方法，其余各法对单立的
// ExceptionDisclosureRuleRegistry / ConflictSignalRuleRegistry（票
// ve-disclosure-policy-view/03）——「本上下文支持哪几类目录查阅」从这一个接口就
// 读得出来，页面重新分组不改端口。
//
// 拼写从读面家族（Catalogue，同 parcelpricing / partycommercial 的目录读口），不随
// 本包写侧的 Catalog：读口的消费方是跨上下文的装配层与管理台，两边词形一致比包内
// 一致更值钱。
type CatalogueListRead interface {
	ListMilestoneMappings(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]MilestoneMappingCatalogueRow, error)
	ListTriageRules(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]TriageRuleCatalogueRow, error)
	ListNotificationPolicies(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]NotificationPolicyCatalogueRow, error)
	ListClaimEligibilities(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ClaimEligibilityCatalogueRow, error)
	ListClaimAuthorizations(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ClaimAuthorizationCatalogueRow, error)
	ListDisclosurePolicies(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]DisclosurePolicyCatalogueRow, error)
	ListExceptionDisclosureRules(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ExceptionDisclosureRuleCatalogueRow, error)
	ListConflictSignalRules(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ConflictSignalRuleCatalogueRow, error)
}

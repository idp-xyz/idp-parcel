package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件是主数据登记目录查阅的伴生列表读端口(ADR-0077,票 master-data-wiring/02):
// 管理台 price-card-catalog 与 reference-series 两页的供数面。两口都不拓宽既有登记
// 写口(PriceCardCatalog / ReferenceSeriesRegister)——扩写侧接口会拆全部写侧测试
// 替身,伴生读端口另立(267cb44 提交信记过这条风险)。
//
// 上列的是**检索列面**的照实转写:两张登记册的权威内容都在领域折装的快照里,读回
// 要经领域整图重验——那是评价装载的纪律;目录上列不重建领域对象、不形成判断,列面
// 与登记字段对齐,不发明列,快照不透出。
//
// 租户在方法签名上(ADR-0077 Decision 五);Limit 必须为正,每页多大由接入面按渠道
// 契约裁决,读口只拒绝无意义的取值;空登记册如实交回空列表(Decision 四:空册本身
// 就是内容,续办是登记责任方去登记口登记,不折成未配置)。

// PriceCardCatalogueRow 是价卡目录上列的一行:一份已登记价卡版本的检索列面。
// 方向与计算目的成对照列(CONTEXT:首发一一对应);源文件名与 SHA-256 是证据索引
// (真实价卡文件外置于受限证据库,ADR-0008),照登转写。
//
// HasEffectiveTo 为假即无上界适用期。用显式布尔而不是零值判断:零时刻是一个合法的
// 绝对时刻,拿它兼作「没有终点」会让补历史的区间读不出来。
type PriceCardCatalogueRow struct {
	PlanID               string
	PlanVersion          string
	Direction            string
	Purpose              string
	Scope                string
	RateTableID          string
	RateTableVersion     string
	EffectiveFrom        time.Time
	EffectiveTo          time.Time
	HasEffectiveTo       bool
	Canonicalization     string
	ContentDigest        string
	SourceFileName       string
	SourceFileSHA256     string
	AuthorizationID      string
	AuthorizationVersion string
	PublicationApprover  string
	RegisteredAt         time.Time
}

// PriceCardCatalogueRead 是价卡目录的伴生列表读端口。
type PriceCardCatalogueRead interface {
	ListPriceCards(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]PriceCardCatalogueRow, error)
}

// ReferenceSeriesCatalogueRow 是计价参考序列登记册上列的一行:一版序列的检索列面。
// 逐期取值在快照内,不上列——登记一期取值是一次来源事实断言,期次的消费口是按计价
// 基准时点的 ResolveAt,目录只答「登了哪些版本、什么来源、什么证据等级」。
//
// EvidenceGrade 照列转写(VERIFIABLE/ASSERTED):任何一期缺可复核凭证整版只有断言
// 强度,那是登记册汇总好的事实,目录不重算。HasQuoteBasis 为假即无口径声明(燃油
// 无需口径;汇率必有,库上 CHECK 钉住)。IsCorrection 为真时 PriorVersion 与
// CorrectionBasis 成对在场(更正两件成对,库上 CHECK 钉住),如实转写不补。
type ReferenceSeriesCatalogueRow struct {
	SeriesID          string
	SeriesVersion     string
	Kind              string
	SourceIdentifier  string
	Registrant        string
	QuoteBasisID      string
	QuoteBasisVersion string
	HasQuoteBasis     bool
	EffectiveFrom     time.Time
	EffectiveTo       time.Time
	HasEffectiveTo    bool
	EvidenceGrade     string
	PriorVersion      string
	CorrectionBasis   string
	IsCorrection      bool
	Canonicalization  string
	ContentDigest     string
	RegisteredAt      time.Time
}

// ReferenceSeriesCatalogueRead 是计价参考序列登记册的伴生列表读端口。
type ReferenceSeriesCatalogueRead interface {
	ListReferenceSeries(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ReferenceSeriesCatalogueRow, error)
}

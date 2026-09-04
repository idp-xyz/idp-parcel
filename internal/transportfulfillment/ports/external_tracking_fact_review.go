package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 外部承运轨迹事实的查阅读口（label-channel/21）：管理台「有效时间判断」页的供数面。它是端口而不是
// HTTP 私有形状，因为票 22（按新登记的规则重判该源全部待判断事实）要读的是同一批行——两处各写一遍
// 「待判断的当前版」就是两个口径。读口不与 ExternalTrackingFactRegistry 共接口，理由同 ReviewCatalogueRead：
// 扩写侧接口会拆全部写侧测试替身。
//
// 上列的是**当前版**——一条事实此刻未被任何版本回指的那一版。待判断的当前版是判断人要看的「该判哪几条」；
// 全部当前版让再判之前看得见「这一条已经判过、按哪个依据判的」（票 21 红线）。被回指的旧版不上列：
// 它们仍在库里，按键与版本走登记册读回，不在列面展开。

// ExternalTrackingFactReviewRow 是一条当前版的照实转写。三个时间各归各位（ADR-0102）：OccurredAt 源给、
// ReceivedAt 本上下文铸、EffectiveAt 只在判断过时在场——待判断是 nil，不拿另两个时间顶上。
// EffectiveRule 与 EffectiveRuleVersion 只在按规则判断时在场；Supersedes 首版为空；Origin 说这一版是素材
// 到达形成的（MATERIAL）还是一次判断形成的（JUDGMENT）。状态词原样透出，不解释、不映射（ADR-0102 决定五）。
type ExternalTrackingFactReviewRow struct {
	Fact                 string
	Version              string
	Source               string
	Credential           string
	Object               string
	SourceEvent          string
	Status               string
	OccurredAt           time.Time
	ReceivedAt           time.Time
	EffectiveBasis       string
	EffectiveAt          *time.Time
	EffectiveRule        string
	EffectiveRuleVersion string
	Supersedes           string
	Origin               string
	RecordedAt           time.Time
}

// EffectiveTimeReviewFilter 说上列哪些当前版。封闭两格而不是一个布尔：读口不接受「没传就当某一格」，
// 词不在集合内即拒——与端点上 `view` 参数缺席答 400 是同一条纪律的两端。
type EffectiveTimeReviewFilter uint8

const (
	EffectiveTimeReviewFilterInvalid EffectiveTimeReviewFilter = iota
	PendingEffectiveTimeOnly
	EveryCurrentVersion
)

func (filter EffectiveTimeReviewFilter) String() string {
	switch filter {
	case PendingEffectiveTimeOnly:
		return "PENDING_ONLY"
	case EveryCurrentVersion:
		return "EVERY_CURRENT_VERSION"
	default:
		return ""
	}
}

// ExternalTrackingFactReviewRead 按（租户，轨迹源）上列当前版。租户与源都在签名上（ADR-0077 决定五）：
// 源是这本册子的分组维——规则按源登记、回填按源重判，判断人也是一家源一家源地看。Limit 必须为正；
// 没有事实的源如实交回空列表。
type ExternalTrackingFactReviewRead interface {
	ListCurrentExternalTrackingFacts(
		ctx context.Context,
		tenant domain.TenantID,
		source domain.TrackingSourceReference,
		filter EffectiveTimeReviewFilter,
		limit int,
	) ([]ExternalTrackingFactReviewRow, error)
}

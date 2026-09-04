package ports

import "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"

// 本文件是来源喂价失败的可观察记录口（ADR-0095 决定二的形状）。抓取失败不补数、不沿用旧值、
// 不登空版本：库里什么都不留，缺口让评价挂起（ADR-0013 要的行为）——于是这一格与 ADR-0095
// 的「自愈那格」同属：要留的是「此刻发生了什么」这个观察，不是一条业务事实。告警通道属实例
// 半边，机制只把观察交给装配方，怎么出声、出声到哪里归装配方。

// SourceFeedStage 是喂价链上失败发生的站点。按站点而不按现象分格：编排要答的是「停在哪」，
// 现象（文件不在、解不开、库不通）留在原始错误里原样交出。
type SourceFeedStage uint8

const (
	SourceFeedStageInvalid SourceFeedStage = iota
	// SourceFeedStageFetch：抓取失败——来源不可达或原文解不开。
	SourceFeedStageFetch
	// SourceFeedStageStore：本体存放答了未配置之外的失败格；登记照常进行，凭证不带定位符。
	SourceFeedStageStore
	// SourceFeedStageTranscribe：转录被拒——公布顺序倒置、序列不符、原文装不成观测。
	SourceFeedStageTranscribe
	// SourceFeedStageRegister：登记册给了非入册答案（同版本内容冲突、形状不同）。
	SourceFeedStageRegister
	// SourceFeedStageReview：免复核声明下系统代写复核未被记录（四眼门拒、同键冲突）。
	SourceFeedStageReview
)

func (stage SourceFeedStage) String() string {
	switch stage {
	case SourceFeedStageFetch:
		return "FETCH"
	case SourceFeedStageStore:
		return "STORE"
	case SourceFeedStageTranscribe:
		return "TRANSCRIBE"
	case SourceFeedStageRegister:
		return "REGISTER"
	case SourceFeedStageReview:
		return "REVIEW"
	default:
		return ""
	}
}

// SourceFeedObservation 是一次喂价失败的观察：哪个租户哪条序列、按哪一版绑定、哪种连接器、
// 从哪个定位符、停在哪一站、原始错误或登记册答案的字面。
type SourceFeedObservation struct {
	Tenant         domain.TenantID
	SeriesID       string
	BindingVersion string
	ConnectorKind  string
	Locator        string
	Stage          SourceFeedStage
	// Err 是原始错误，原样交出（ADR-0095：丢掉它，那些字就没有任何人读得到）。登记册与复核册
	// 的治理答案没有 error，那时 Err 为 nil、答案写在 Answer 里。
	Err error
	// Answer 是治理答案的字面（如 CONTENT_CONFLICT、NEEDS_ANOTHER_REVIEWER），Err 为 nil 时有。
	Answer string
}

// SourceFeedObserver 由装配方注入。它只观察，不改变编排行为：返回值不看，出声失败不影响
// 这一次喂价的答案。
type SourceFeedObserver func(observation SourceFeedObservation)

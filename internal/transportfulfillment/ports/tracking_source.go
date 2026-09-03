package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/outbound"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件是朝外部轨迹源取轨迹的**出向**端口（label-channel/15）。本包其余端口都朝内（登记册、
// 读面），出向是本上下文头一条，形状因此按 ADR-0090 定的出向契约走，不照本包内向端口的样子写。
//
// 拉取是首发形态；回调接收是第二形态，与拉取共用同一份素材类型 TrackingMaterial，端点随第一家
// 推送源另立。两种形态的差别只在素材**怎么到达**，到达之后交给收编方的是同一样东西。
//
// 端口交出的是**待收编的原始素材**，不是事实：本上下文对外部状态的认领是一次显式判断
// （CONTEXT「不无条件接受外部 `DELIVERED` 状态码」），归收编执行器；`visibility-exception`
// 的 `SourceContext` 封闭五值本文件一字不动。任何一家源的账号、地址、频率、状态码表都不在
// 这里（实例半边，`PAR-INT-02`）。

// TrackingSourceReference 是一个已登记轨迹源的引用。
//
// 聚合平台是一个源，承运商直连也是一个源——端口按源实现，一口多家。收编方从不问「素材从聚合
// 还是直连来」，它问的三样（哪个承运方、哪份凭证、什么时候发生了什么）都在素材上。
type TrackingSourceReference string

// TrackingSubject 是一次要问的对象。
//
// CredentialReference 是 CONTEXT 里的**外部承运凭证**引用，不解释为包裹的当前运单号——凭证
// 分配给的可能是运输委托、订舱、班次或载运对象，不一定是包裹。
//
// CarrierReference 由源侧或登记侧给出，本仓不推断。聚合平台常要求调用方指名承运商，直连
// 不需要，因此允许为空——它不是每个源都要填的字段。
type TrackingSubject struct {
	Tenant              domain.TenantID
	CredentialReference string
	CarrierReference    string
}

// SourceTime 是源给的一个时间。
//
// Given 为假即「源未给」，此时 At 无意义。做成两字段而不是让零值 time.Time 承担「没给」，
// 是因为零值会被任何一次 `.IsZero()` 忘记检查的读法当成一个真时间——而 ADR-0023 禁的正是
// 替外部事实补一个发生时间。
type SourceTime struct {
	Given bool
	At    time.Time
}

// TrackingMaterial 是一条待收编的原始素材，两种到达形态共用。
//
// 三个时间里这里只出现两个，且归属不同（票 `03` 裁定）：
//
//   - OccurredAt 由源给，**源不给就 Given=false**，不许拿 ReceivedAt 或任何时间顶替——顶替之后
//     迟到轨迹与实时轨迹在类型上就分不开了。
//   - ReceivedAt 由本仓铸：它答的是「本仓什么时候拿到的」，本来就是本仓自己的事实，铸它不构成
//     代铸。这是端口这一层**唯一**铸的时间。
//   - EffectiveAt 不在素材上。它是收编方的一次显式判断，不是外部事实的时间，归票 `16`。
//
// SourceEventID 由源给，源不给就空着。它与 Source 合起来是接收形态的幂等锚（ADR-0090 决定四：
// 锚在已有身份上，不新造传输层的键）；没有它的素材**不判重**——按内容摘要判重等于替源发明一个
// 身份，那也是代铸。
//
// StatusReference 是源的原始状态词或码，原样引用、不解释、不映射；哪家源的哪个码表示什么属
// 实例半边。CorrectionOf 是源**显式声明**的「本条更正了哪条」（按源事件标识），源不声明就空；
// 取代关系由收编方判断，端口不从到达先后推。
//
// PayloadDigest 必须在字节还在手上时算出，理由同取面单件的 Digest：它是「我们确实收到过这条」
// 的全部证据，本体只过路。
type TrackingMaterial struct {
	Source          TrackingSourceReference
	Subject         TrackingSubject
	SourceEventID   string
	OccurredAt      SourceTime
	ReceivedAt      time.Time
	StatusReference string
	CorrectionOf    string
	PayloadDigest   string
	Payload         []byte
}

// PullCursor 是「上次拉到哪」的不透明游标，由该源的适配器解释，空即从头。
//
// 不透明是刻意的：聚合平台用时间水位、直连用页码或事件序号，机制若给它一个结构就得替每家源
// 选一种。游标的形状属机制半边，取值属实例半边。
type PullCursor string

// TrackingPullSupport 说这一家源提不提供拉取。
//
// ADR-0090 决定六要求每个出向端口欠一个查询能力，拉取本身就是那个能力；只推送不提供拉取的源
// 因此如实记为一格，而不是让拉取交回一个失败处置——两者的续办不同：拉取失败可以再拉，无拉取
// 口只能等推送或交人对账。
type TrackingPullSupport uint8

const (
	TrackingPullSupportInvalid TrackingPullSupport = iota
	TrackingPullOffered
	TrackingPullNotOfferedBySource
)

func (support TrackingPullSupport) String() string {
	switch support {
	case TrackingPullOffered:
		return "OFFERED"
	case TrackingPullNotOfferedBySource:
		return "NOT_OFFERED_BY_SOURCE"
	default:
		return ""
	}
}

// TrackingPullRequest 是一次拉取的请求。
//
// Subjects 允许多个：聚合平台按批答，直连一次答一个也装得下。Configuration 零值即未配置，
// 适配器据此交回 outbound.NotConfigured 并不得发起调用（ADR-0090 决定五）。
type TrackingPullRequest struct {
	Tenant        domain.TenantID
	Source        TrackingSourceReference
	Subjects      []TrackingSubject
	Cursor        PullCursor
	Configuration outbound.ChannelConfiguration
}

// TrackingPullOutcome 是一次拉取的结果。
type TrackingPullOutcome struct {
	// Support 要先看。它答 TrackingPullNotOfferedBySource 时 Outcome 无意义，这一家只能等推送。
	Support TrackingPullSupport
	// Outcome 是这一次调用落在哪一格，由适配器判定，**不得从 HTTP 状态码推出**（ADR-0090
	// 决定三）。拉取是幂等读，重拉一次不产生供应商成本，`答案未确定`对它的实际约束因此只是
	// 「别在同一拍里重拉」——但代数不为此改形：两条链共用一套代数正是 ADR-0090 立票的理由，
	// 拉取侧不给自己开一个「可安全重拉」的例外格；多久再拉是 `PAR-INT-02` 的实例参数。
	Outcome   outbound.Outcome
	Materials []TrackingMaterial
	// NextCursor 空即「没有更多」。
	NextCursor PullCursor
}

// TrackingSource 是朝外部轨迹源拉取轨迹的出向端口。
//
// 交回已经分好格的封闭代数，不是 `*http.Response`，也不是裸 error（ADR-0090 决定三）。
// **error 只留给调用方的错**——请求不合法、上下文取消；线路上的一切都必须落进 Disposition。
//
// **本口今天没有生产实现，这是设计而不是欠账。** 各家源的适配器按渠道适配缝备忘「一类数据
// 一张票」逐家另立；合成替身只在 `adapters/trackingsource` 的测试里，生产代码导入不了它。
type TrackingSource interface {
	Pull(ctx context.Context, request TrackingPullRequest) (TrackingPullOutcome, error)
}

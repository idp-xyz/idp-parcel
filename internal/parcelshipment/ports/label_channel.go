package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/outbound"
)

// 本文件是朝末端渠道取面单的**出向**端口。本包其余端口都朝内（存取聚合、读面），出向是
// 头一条，形状因此按 ADR-0090 定的出向契约走，不照本包内向端口的样子写。
//
// 形状要容下六家末端渠道公开文档上的四处差异而不被撑破——它们指向类型而不是取值，逐条的
// 落点记在下面各自的注释里。任何一家的账号、字段名、格式取值与 DPI 都不在这里（实例半边，
// `PAR-INT-02`／`PAR-SET-03`）。

// LabelDocumentGranularity 说一份面单件覆盖的是一件包裹还是一批。
//
// **这是四处差异里的「面单粒度是否恒为包裹」。** 有渠道按批签发面单，一份件覆盖多件包裹；
// 若把粒度设成恒为包裹，那种件只能被拆成假的逐件记录或被丢掉一部分覆盖范围，两种都是在
// 类型层面说谎。
type LabelDocumentGranularity uint8

const (
	LabelDocumentGranularityInvalid LabelDocumentGranularity = iota
	LabelDocumentPerParcel
	LabelDocumentPerBatch
)

func (granularity LabelDocumentGranularity) String() string {
	switch granularity {
	case LabelDocumentPerParcel:
		return "PER_PARCEL"
	case LabelDocumentPerBatch:
		return "PER_BATCH"
	default:
		return ""
	}
}

// LabelDocumentAvailability 说面单件此刻在不在手上。
//
// **这是四处差异里的「取面单与下单是否同一次调用」。** 有渠道一次下单调用同时回单号与面单，
// 有渠道要先建单、再购买、再单独取件，面单是第三次调用才拿到的。用一个「有没有面单」的
// 布尔表达不了两者的区别——`false` 在前一种是失败，在后一种是正常的中间态。
//
// LabelDocumentsNotProducedByChannel 是第三格，不是凑数：公开文档里存在「受理了但按约定
// 不回图件」与「回的是取件码而非面单」两种应答。**「取了面单」与「回了图件」不是同一件事**，
// 合成两格会让一次正常受理被读成取件失败，从而触发一次本不该有的重取。
type LabelDocumentAvailability uint8

const (
	LabelDocumentAvailabilityInvalid LabelDocumentAvailability = iota
	LabelDocumentsReturnedWithSubmission
	LabelDocumentsAwaitSeparateFetch
	LabelDocumentsNotProducedByChannel
)

func (availability LabelDocumentAvailability) String() string {
	switch availability {
	case LabelDocumentsReturnedWithSubmission:
		return "RETURNED_WITH_SUBMISSION"
	case LabelDocumentsAwaitSeparateFetch:
		return "AWAIT_SEPARATE_FETCH"
	case LabelDocumentsNotProducedByChannel:
		return "NOT_PRODUCED_BY_CHANNEL"
	default:
		return ""
	}
}

// LabelChannelQuerySupport 说这一家提不提供查询口。
//
// ADR-0090 决定六要求每个出向端口都配一个查询能力，并明写查不了的要**如实记为「该源无查询
// 口」，不得以重发顶替**。它单独成格而不是让查询交回一个失败处置，是因为两者的续办完全
// 不同：查询失败可以再查一次，无查询口则要交人对账，再查一万次也不会有答案。
type LabelChannelQuerySupport uint8

const (
	LabelChannelQuerySupportInvalid LabelChannelQuerySupport = iota
	LabelChannelQueryOffered
	LabelChannelQueryNotOfferedByChannel
)

func (support LabelChannelQuerySupport) String() string {
	switch support {
	case LabelChannelQueryOffered:
		return "OFFERED"
	case LabelChannelQueryNotOfferedByChannel:
		return "NOT_OFFERED_BY_CHANNEL"
	default:
		return ""
	}
}

// LabelDocument 是渠道交回的一份件。
//
// **这是四处差异里的「一次结果是否只有一份图件」。** 有渠道一次结果回多份件——面单之外另有
// 随附图件、签名图件与条码图件；它们不可互换，因此各自成行并自带角色，而不是塞进一个
// 「载荷」字段由读的人猜哪份是面单。
//
// Role 与 Format 是渠道声明的取值，机制不列举也不校验其集合：哪家回什么格式、什么角色属
// 实例半边。把它们做成封闭枚举会在接第一家真渠道时就被撑破，而撑破的表现是一个认不出的
// 取值被译成零值。
//
// CoveredParcels 在 LabelDocumentPerBatch 时列出该件覆盖的全部包裹，在 LabelDocumentPerParcel
// 时恰一件。**这条不变量在本层没有守卫**：本包按仓内约定只放记录结构，构造门在领域侧；
// 件往领域与库里落的形状归票 `09`，那一票要把它守住。
//
// Content 是件的字节。它在这里只是过路——落到领域、库与读面的哪里同样归票 `09`。
type LabelDocument struct {
	Role           string
	Format         string
	Granularity    LabelDocumentGranularity
	CoveredParcels []domain.DeclaredParcelID
	Content        []byte
}

// LabelChannelParcelOutcome 是渠道对某一件包裹给出的结果，与领域侧的逐包裹结果同形。
//
// HasResult 与 Accepted 分开的理由同读面那一行：结果未回时 Accepted 的零值读作「未受理」，
// 就是把「还没有结果」当成失败处理。部分成功因此表达得出——一笔交易里有的包裹有结果、
// 有的没有，两层结果不互推。
type LabelChannelParcelOutcome struct {
	Parcel          domain.DeclaredParcelID
	HasResult       bool
	Accepted        bool
	Identifier      string
	ReasonReference string
}

// LabelChannelRequest 是三个出向操作共用的请求头。
//
// **幂等锚取领域侧已有的身份，不新造传输层的键**（ADR-0090 决定四）。因此这里有
// TransactionID 与 CoveredParcels，没有 `IdempotencyKey` 一类字段——多一套键就会有两套键不
// 一致的时候，而那时没有规则说谁算数。
//
// Configuration 零值即未配置，适配器据此交回 outbound.NotConfigured 并不得发起调用。
type LabelChannelRequest struct {
	Tenant         domain.TenantID
	TransactionID  domain.LabelTransactionID
	CoveredParcels []domain.DeclaredParcelID
	Configuration  outbound.ChannelConfiguration
}

// LabelSubmissionOutcome 是提交给渠道那一次的结果。
type LabelSubmissionOutcome struct {
	// Outcome 是这一次调用落在哪一格，连同该格的依据引用。它由适配器判定，**不得从 HTTP
	// 状态码推出**（ADR-0090 决定三）：状态码在出向侧什么都不保证，公开文档里就有恒回 200、
	// 成败在响应体里的渠道，照状态码读会把每一次失败读成成功。
	//
	// 收 outbound.Outcome 而不是裸的 Disposition，是为了让`确证未受理`那一格过得了举证门——
	// 它只能由 outbound.NotAccepted 交出，而那个构造器举不出实据时会降级。
	Outcome              outbound.Outcome
	ChannelReference     string
	ParcelOutcomes       []LabelChannelParcelOutcome
	DocumentAvailability LabelDocumentAvailability
	Documents            []LabelDocument
}

// LabelDocumentFetchOutcome 是单独一次取件调用的结果。
//
// 它与提交分开成一个操作，正是「取面单与下单不一定同一次调用」那一处差异的落点：对同次
// 回件的渠道，编排根本不会走到这里；对分次回件的渠道，这一次调用有它自己的处置格——提交
// 已经受理、取件却超时，是一个必须表达得出的组合。
type LabelDocumentFetchOutcome struct {
	Outcome      outbound.Outcome
	Availability LabelDocumentAvailability
	Documents    []LabelDocument
}

// LabelSubmissionQueryOutcome 是查询原提交下落的结果。
type LabelSubmissionQueryOutcome struct {
	// Support 要先看。它答 LabelChannelQueryNotOfferedByChannel 时 Outcome 无意义，
	// 这一笔只能交人对账。
	Support LabelChannelQuerySupport
	// Outcome 说的是**原提交**落在哪一格，不是这次查询调用本身的成败。两者很容易被读混，
	// 而读混的代价是实在的：查询调用自己超时应当再查一次，原提交查出`确证未受理`才是准许
	// 重发的那一格。查询调用本身的技术失败走 error。
	Outcome              outbound.Outcome
	ParcelOutcomes       []LabelChannelParcelOutcome
	DocumentAvailability LabelDocumentAvailability
}

// LabelChannelGateway 是朝末端渠道取面单的出向端口。
//
// 三个方法交回的都是已经分好格的封闭代数，不是 `*http.Response`，也不是裸 error
// （ADR-0090 决定三，形状同 ADR-0031 对自有仓储端口的要求）。
//
// **error 只留给调用方的错**——请求不合法、上下文取消。线路上的一切都必须落进 Disposition：
// 把超时或连接失败交回 error，就等于把「对端受理了没有」这一问漏掉了，而那正是整条出向链
// 上唯一难逆转的一问。
//
// **本口今天没有生产实现，这是设计而不是欠账。** 各家渠道的适配器按渠道适配缝备忘「一类
// 数据一张票」逐家另立，合成替身归票 `08`。先立端口是因为形状一旦发布，此后每一家适配器
// 都按它写；等第一家能接入时再定，改的就是既有契约而不是空白。
type LabelChannelGateway interface {
	SubmitLabelRequest(ctx context.Context, request LabelChannelRequest) (LabelSubmissionOutcome, error)
	FetchLabelDocuments(ctx context.Context, request LabelChannelRequest) (LabelDocumentFetchOutcome, error)
	QuerySubmission(ctx context.Context, request LabelChannelRequest) (LabelSubmissionQueryOutcome, error)
}

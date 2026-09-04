package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件是来源连接器的契约（ADR-0099 决定六；票 pricing-reference-series-operations/06）。连接器
// 是登记责任方的转录代理，不是第二种登记：它抓回公布记录原文、转录成整版重述的登记 spec，spec
// 走同一登记用例（RegisterReferenceSeriesHandler）。连接器不算数、不改数、不选口径——口径、
// 来源标识、登记责任方全部来自绑定（domain.SourceConnectorBinding）。
//
// 契约立在 ports 而不是 adapters/sourcefeed：编排在 application，应用包不得依赖适配器（边界门禁），
// 而各家连接器在 adapters/sourcefeed 里实现本接口。首个实现是以受控目录文件为源的 FileConnector；
// 出网连接器（CFETS 人民币中间价一段）留 draft，开工前置是部署侧登记出网能力，届时加的是一个
// 实现与一行装配，不是契约。

// FetchSpec 是一次抓取的输入：抓哪个租户哪条序列、从哪个定位符抓。定位符来自绑定，连接器里
// 不写死任何地址。
type FetchSpec struct {
	Tenant   domain.TenantID
	SeriesID string
	// Locator 是绑定声明的来源定位符：FileConnector 解作受控目录内的相对路径。
	Locator string
}

// TranscriptionInput 是一次转录的输入。Placement 是本体存放端口的答复（定位符可缺）；Prior 是
// 该序列最近登记的一版，首版时为 nil。
type TranscriptionInput struct {
	Record    domain.PublishedRecord
	Placement ArtifactPlacement
	Binding   domain.SourceConnectorBinding
	Prior     *domain.ReferenceSeriesRegistration
}

// SourceConnector 是一种来源的抓取与转录实现。
//
// Fetch 交回的记录已带内容摘要（构造器在字节还在手上那一刻算）、抓取时刻、来源地址与来源声明的
// 公布日期；抓不到、解不开一律 error——编排据以留缺口、出一条可观察记录，不补数不沿用旧值。
//
// Transcribe 只该做一件连接器特有的事：从原文里读出观测（生效起点与取值）。整版重述、幂等重放
// 识别、版本号与凭证的写法都在 domain.TranscribeSourceFeed 一处，连接器把观测交给它，不重写。
type SourceConnector interface {
	// Kind 是本连接器的种类字面量，绑定按它选连接器。
	Kind() string
	Fetch(ctx context.Context, spec FetchSpec) (domain.PublishedRecord, error)
	Transcribe(input TranscriptionInput) (domain.ReferenceSeriesRegistrationSpec, error)
}

// SourceConnectorResolver 按绑定声明的连接器种类找实现。找不到答 false 不答 error：那是一格
// 治理答案（绑定声明了一种本部署还没装配的连接器——CFETS 段留 draft 期间正是如此），操作者
// 拿答案去改绑定或等连接器就位。
type SourceConnectorResolver interface {
	ConnectorFor(kind string) (SourceConnector, bool)
}

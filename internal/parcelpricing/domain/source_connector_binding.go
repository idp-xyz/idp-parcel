package domain

import "errors"

// ErrInvalidSourceConnectorBinding 表示来源连接器绑定立不住：租户、序列标识、绑定版本、
// 连接器种类、来源标识、来源定位符、登记责任方任一缺位，序列种类不在封闭集，汇率缺口径，
// 或免人工复核声明没有显式给出。
var ErrInvalidSourceConnectorBinding = errors.New("parcel pricing: invalid source connector binding")

// ReviewExemption 是租户对某来源的「免人工复核」声明（ADR-0099 决定六）。它是封闭三格而不是
// 一个布尔：`未声明`与`否`的续办相同（都要人复核），但前者是租户还没表态、后者是租户表了态
// 不放行，两者在治理上不是一回事——压成布尔会让「没配」看起来像「配了否」。
//
// 没有零值语义：构造门只认下面三个字面量，零值被拒。「未声明 = 需人工复核」是三格里的一格，
// 由租户显式写下，不是产品替它默认出来的（CONTEXT：产品不设默认）。
type ReviewExemption string

const (
	// ReviewExemptionGranted：租户声明信任该来源到免人工复核的程度。连接器登记的版本由系统
	// 代写复核记录，复核责任方为连接器身份、依据为该绑定的版本。
	ReviewExemptionGranted ReviewExemption = "EXEMPT"
	// ReviewExemptionWithheld：租户表态不放行，连接器登记的版本照常经人工复核进在用。
	ReviewExemptionWithheld ReviewExemption = "NOT_EXEMPT"
	// ReviewExemptionUndeclared：租户尚未表态；行为与`否`相同，只是治理上还欠一个决定。
	ReviewExemptionUndeclared ReviewExemption = "UNDECLARED"
)

func (exemption ReviewExemption) String() string { return string(exemption) }

func (exemption ReviewExemption) valid() bool {
	switch exemption {
	case ReviewExemptionGranted, ReviewExemptionWithheld, ReviewExemptionUndeclared:
		return true
	default:
		return false
	}
}

// RequiresManualReview 只在租户显式声明免复核时答否；未声明与否都答是。
func (exemption ReviewExemption) RequiresManualReview() bool {
	return exemption != ReviewExemptionGranted
}

// SourceConnectorBindingSpec 是登记一版来源连接器绑定的全部输入。它是实例半边的格：哪个租户、
// 哪条序列、由哪种连接器、从哪里抓、按什么口径、以谁的名义登记、多久抓一次、免不免复核——
// 每一格的取值都属租户，机制只提供格与构造门。出厂零绑定。
type SourceConnectorBindingSpec struct {
	Tenant TenantID
	// SeriesID 是绑定所服务的计价参考序列标识；一条序列一个绑定，绑定按（租户、序列）指名。
	SeriesID string
	// Version 是绑定自己的版本：改声明就登新版本，行只增不改。免复核时系统代写的复核记录
	// 以「该绑定的版本」为依据，所以它必须可指名。
	Version string
	// ConnectorKind 是连接器种类，绑定据以选连接器实现（FileConnector 为 FILE）。机制不把它
	// 做成封闭枚举：真实来源的连接器逐家另立，枚举会在接第一家时被撑破。
	ConnectorKind string
	// SourceIdentifier 是进登记的来源标识（CONTEXT 硬句四件之一）。
	SourceIdentifier string
	// SourceLocator 是连接器据以抓取的来源定位符：FileConnector 为受控目录内的相对路径，
	// 出网连接器为公布页地址。地址属绑定不属连接器——连接器里不写死任何真实来源。
	SourceLocator string
	SeriesKind    ReferenceSeriesKind
	// QuoteBasis 是声明取值口径的商业价格政策版本；汇率必备，燃油可缺。
	QuoteBasis VersionReference
	// Registrant 是登记责任方。连接器是它的转录代理（ADR-0099 决定六），不是第二种登记
	// 责任方；免复核时系统代写的复核由连接器身份署名，四眼门因此在结构上仍关得住。
	Registrant string
	// Cadence 是抓取节律，只登记、不解释：机制不调度，受控批量口手跑或由外部调度。
	Cadence         string
	ReviewExemption ReviewExemption
}

// SourceConnectorBinding 是一版立得住的来源连接器绑定。
type SourceConnectorBinding struct {
	tenant           TenantID
	seriesID         string
	version          string
	connectorKind    string
	sourceIdentifier string
	sourceLocator    string
	seriesKind       ReferenceSeriesKind
	quoteBasis       *VersionReference
	registrant       string
	cadence          string
	reviewExemption  ReviewExemption
}

func NewSourceConnectorBinding(spec SourceConnectorBindingSpec) (SourceConnectorBinding, error) {
	binding := SourceConnectorBinding{
		tenant:           spec.Tenant,
		seriesID:         spec.SeriesID,
		version:          spec.Version,
		connectorKind:    spec.ConnectorKind,
		sourceIdentifier: spec.SourceIdentifier,
		sourceLocator:    spec.SourceLocator,
		seriesKind:       spec.SeriesKind,
		registrant:       spec.Registrant,
		cadence:          spec.Cadence,
		reviewExemption:  spec.ReviewExemption,
	}
	if spec.QuoteBasis != (VersionReference{}) {
		basis := spec.QuoteBasis
		binding.quoteBasis = &basis
	}
	if !binding.valid() {
		return SourceConnectorBinding{}, ErrInvalidSourceConnectorBinding
	}
	return binding, nil
}

func (binding SourceConnectorBinding) Tenant() TenantID                { return binding.tenant }
func (binding SourceConnectorBinding) SeriesID() string                { return binding.seriesID }
func (binding SourceConnectorBinding) Version() string                 { return binding.version }
func (binding SourceConnectorBinding) ConnectorKind() string           { return binding.connectorKind }
func (binding SourceConnectorBinding) SourceIdentifier() string        { return binding.sourceIdentifier }
func (binding SourceConnectorBinding) SourceLocator() string           { return binding.sourceLocator }
func (binding SourceConnectorBinding) SeriesKind() ReferenceSeriesKind { return binding.seriesKind }
func (binding SourceConnectorBinding) Registrant() string              { return binding.registrant }
func (binding SourceConnectorBinding) ReviewExemption() ReviewExemption {
	return binding.reviewExemption
}

// QuoteBasis 只在声明了口径的绑定上给出（汇率必有，燃油可无）。
func (binding SourceConnectorBinding) QuoteBasis() (VersionReference, bool) {
	if binding.quoteBasis == nil {
		return VersionReference{}, false
	}
	return *binding.quoteBasis, true
}

// Cadence 只在声明了节律的绑定上给出。
func (binding SourceConnectorBinding) Cadence() (string, bool) {
	return binding.cadence, binding.cadence != ""
}

// Spec 是构造器的逆：交回能原样重建本绑定的声明。登记册按它比对同键第二份是重放还是冲突——
// 比对的是声明本身，不是某一列。
func (binding SourceConnectorBinding) Spec() SourceConnectorBindingSpec {
	spec := SourceConnectorBindingSpec{
		Tenant:           binding.tenant,
		SeriesID:         binding.seriesID,
		Version:          binding.version,
		ConnectorKind:    binding.connectorKind,
		SourceIdentifier: binding.sourceIdentifier,
		SourceLocator:    binding.sourceLocator,
		SeriesKind:       binding.seriesKind,
		Registrant:       binding.registrant,
		Cadence:          binding.cadence,
		ReviewExemption:  binding.reviewExemption,
	}
	if binding.quoteBasis != nil {
		spec.QuoteBasis = *binding.quoteBasis
	}
	return spec
}

// RequiresManualReview 转发免复核声明的答案：只有显式声明免复核的绑定，其登记的版本才由系统
// 代写复核。
func (binding SourceConnectorBinding) RequiresManualReview() bool {
	return binding.reviewExemption.RequiresManualReview()
}

// ConnectorIdentity 是系统代写复核时的复核责任方。按连接器种类而不按绑定取名：复核的是
// 「这类连接器的转录」，同一种连接器对不同序列是同一双眼睛；它与登记责任方（绑定声明的
// Registrant）不同名，四眼门（NewSeriesReview）才过得去——租户若把登记责任方也填成这个名字，
// 门会如实拒，那正是它该拒的。
func (binding SourceConnectorBinding) ConnectorIdentity() string {
	return "connector:" + binding.connectorKind
}

// ExemptionReviewBasis 是系统代写复核的依据：指回本绑定及其版本（ADR-0099 决定六「依据标为
// 该声明及其版本」）。读复核记录的人据此找得到是哪一版声明放行了这一版取值。
func (binding SourceConnectorBinding) ExemptionReviewBasis() string {
	return "免人工复核声明 ← 来源连接器绑定 " + binding.seriesID + "@" + binding.version
}

func (binding SourceConnectorBinding) valid() bool {
	if !binding.tenant.valid() ||
		!trimmed(binding.seriesID) ||
		!trimmed(binding.version) ||
		!trimmed(binding.connectorKind) ||
		!trimmed(binding.sourceIdentifier) ||
		!trimmed(binding.sourceLocator) ||
		!binding.seriesKind.valid() ||
		!trimmed(binding.registrant) ||
		!binding.reviewExemption.valid() {
		return false
	}
	if binding.cadence != "" && !trimmed(binding.cadence) {
		return false
	}
	if binding.quoteBasis != nil {
		if binding.quoteBasis.kind != ArtifactCommercialPolicy || !binding.quoteBasis.valid() {
			return false
		}
	}
	// 不接受未声明口径的裸汇率（CONTEXT）；绑定是口径的唯一来源，这里不守，登记那一步就
	// 只能拒，而那时文件已经抓回来了。
	if binding.seriesKind == ReferenceSeriesExchangeRate && binding.quoteBasis == nil {
		return false
	}
	return true
}

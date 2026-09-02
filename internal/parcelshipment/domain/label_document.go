package domain

import "time"

// 本文件是渠道交回的面单载荷在领域侧的落点（ADR-0092）。
//
// 落的是**引用**不是本体：定位符说本体在哪、摘要说我们收到过什么，两者分开。合成一格会让
// 「收到了但没处放」表达不出来，而在文件组件就位之前那是唯一走得到的分支。

// LabelDocumentGranularity 说一份件覆盖的是一件包裹还是一批。
//
// 它是本族里**唯一**的封闭集合：角色与格式都是渠道声明的自由取值，而粒度不是——它决定
// 「这份件对应哪些包裹」，是结构分叉。做成开放取值，一份批面单就只能被拆成假的逐件记录，
// 或被丢掉一部分覆盖范围。
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

func (granularity LabelDocumentGranularity) valid() bool {
	return granularity.String() != ""
}

// LabelDocumentRole 指名这份件在本次结果里充当什么角色。取值由渠道声明，机制不列举——
// 一次结果可能回面单之外的随附件、签名件与条码件，各家叫法不同（ADR-0092 决定四）。
type LabelDocumentRole struct{ requiredValue }

func NewLabelDocumentRole(value string) (LabelDocumentRole, error) {
	required, err := newRequiredValue("label document role", value)
	return LabelDocumentRole{required}, err
}

// LabelDocumentFormat 指名件的格式。同样由渠道声明：哪家回栅格、哪家回指令流、DPI 多少
// 一律属实例半边（`PAR-INT-02`／`PAR-SET-03`）。
type LabelDocumentFormat struct{ requiredValue }

func NewLabelDocumentFormat(value string) (LabelDocumentFormat, error) {
	required, err := newRequiredValue("label document format", value)
	return LabelDocumentFormat{required}, err
}

// LabelDocumentDigest 是收到本体那一刻算出的完整性摘要。
//
// 它**必备**，与本体是否已存放无关：它是「我们确实收到过这份件」的全部证据，也是日后判
// 「这一份和上一份是不是同一件」的唯一依据。漏算就永久失去了它，事后补不回来。
type LabelDocumentDigest struct{ requiredValue }

func NewLabelDocumentDigest(value string) (LabelDocumentDigest, error) {
	required, err := newRequiredValue("label document digest", value)
	return LabelDocumentDigest{required}, err
}

// LabelDocumentLocator 说本体此刻在哪。
//
// 它**可缺**，缺席即「本体尚无存放处」——文件能力属技术组件而那个组件今天不存在
// （ADR-0092 决定二）。缺席是一格如实的答复，不是失败，也不表示件没收到：那由摘要作证。
type LabelDocumentLocator struct{ requiredValue }

func NewLabelDocumentLocator(value string) (LabelDocumentLocator, error) {
	required, err := newRequiredValue("label document locator", value)
	return LabelDocumentLocator{required}, err
}

// RecordLabelDocumentSpec 是追加一条载荷记录的全部输入。Locator 留零值表示本体无存放处。
type RecordLabelDocumentSpec struct {
	Role           LabelDocumentRole
	Format         LabelDocumentFormat
	Granularity    LabelDocumentGranularity
	CoveredParcels []DeclaredParcelID
	Digest         LabelDocumentDigest
	Locator        LabelDocumentLocator
	ObservedAt     time.Time
}

// LabelDocumentRecord 是一条载荷记录。它只增不改：同一次结果的载荷不得被后来的调用顶替，
// 重打、换单与替换各自产生新的一条并保留原条（ADR-0092 决定三）。
type LabelDocumentRecord struct {
	role        LabelDocumentRole
	format      LabelDocumentFormat
	granularity LabelDocumentGranularity
	parcels     []DeclaredParcelID
	digest      LabelDocumentDigest
	locator     LabelDocumentLocator
	observedAt  time.Time
}

func (record LabelDocumentRecord) Role() LabelDocumentRole {
	return record.role
}

func (record LabelDocumentRecord) Format() LabelDocumentFormat {
	return record.format
}

func (record LabelDocumentRecord) Granularity() LabelDocumentGranularity {
	return record.granularity
}

// CoveredParcels 交回副本。批粒度那一条覆盖多件，逐件那一条恰一件——两者都逐件列出，读的
// 人不必按粒度去猜该看哪里。
func (record LabelDocumentRecord) CoveredParcels() []DeclaredParcelID {
	parcels := make([]DeclaredParcelID, len(record.parcels))
	copy(parcels, record.parcels)
	return parcels
}

func (record LabelDocumentRecord) Digest() LabelDocumentDigest {
	return record.digest
}

func (record LabelDocumentRecord) Locator() LabelDocumentLocator {
	return record.locator
}

func (record LabelDocumentRecord) ObservedAt() time.Time {
	return record.observedAt
}

// BodyStored 说本体此刻有没有存放处。它派生自定位符在不在，没有对应的存储字段——存一个
// 布尔就有了第二个来源，某次接上文件组件忘了改它，两处便各说各话（同 Finalized 的理由）。
func (record LabelDocumentRecord) BodyStored() bool {
	return record.locator.valid()
}

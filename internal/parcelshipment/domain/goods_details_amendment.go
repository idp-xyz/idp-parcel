package domain

import "fmt"

// 本文件是委托级资料组「货物明细」的修订半边（ADR-0373 决定四，票 declared-goods-lines/06）：资料修订以这一组整组替换——内件
// 性质、行与装箱分配一起；补充与更正交来完整的新货物明细，显式清空清掉整组。行号是版本内的序号，新版本从 1 重编，旧版本与
// 接受基线不动，所以（资料版本锚，委托，行号）这个引用本身稳定。

// goodsDetailsDataGroupWord 是本上下文对「货物明细」资料组的自有原词，与 declaredMeasurementDataGroupWord 同款：
// SourceDataGroupReference 是开放引用，`party-commercial` 的允许声明里用什么词是实例半边，那边不校验存在。
const goodsDetailsDataGroupWord = "GOODS_DETAILS"

// GoodsDetailsDataGroup 交回货物明细资料组的原词引用。它作用于整份委托，不按包裹指——每箱装了什么写在装箱分配里，是这一组
// 内容的一部分；按包裹切开就成了按行打补丁，ADR-0373 决定四不收。
func GoodsDetailsDataGroup() SourceDataGroupReference {
	return SourceDataGroupReference{requiredValue{value: goodsDetailsDataGroupWord}}
}

// NewGoodsDetailsContent 把一份报了的货物明细包成货物明细范围的内容；没报的不是内容，拒——清掉整组走显式清空，那一版
// 本来就不带内容。
func NewGoodsDetailsContent(goods GoodsDetails) (SourceDataVersionContent, error) {
	if !goods.Declared() {
		return SourceDataVersionContent{}, ErrInvalidGoodsDetails
	}
	return SourceDataVersionContent{goods: goods}, nil
}

// Goods 的第二个返回值为假即这份内容不是货物明细（测量、要素或缺席）。
func (content SourceDataVersionContent) Goods() (GoodsDetails, bool) {
	return content.goods, content.goods.Declared()
}

// fitsGoodsDetailsScope 是 FitsAmendment 在货物明细这一组上的判法：范围必须是整份委托；补充与更正必须带内容——整组替换下，
// 一份不带货物明细的更正读出来就是「清掉整组」，与显式清空同一个后果却换了个意图，矩阵对两者的允许可以相反（AT-PS-020）。
// 别的资料组可以不带内容（被改的也许是本上下文没有词条的条目），这一组整组都在词表里，没有那种情形。
func (content SourceDataVersionContent) fitsGoodsDetailsScope(scope SourceDataScope, intent AmendmentIntent) bool {
	if _, named := scope.DeclaredParcelID(); named {
		return false
	}
	_, declared := content.Goods()
	if intent == ExplicitClearIntent {
		return content.Empty()
	}
	return declared
}

// CheckSourceDataContentAgainstBaseline 核一份修订内容能不能落在这份委托的接受基线上：货物明细的装箱分配只能指基线成员，
// 报了就每个成员都报。指到基线之外的包裹交回 ErrParcelOutsideAcceptanceBaseline——与范围指名基线外成员同一个业务答案
// （AT-PS-023）：增删行或改分配不是增删委托成员，多出来的那一箱要走关联新委托；漏了成员交回 ErrInvalidPacking，是这份
// 内容自己没写全。
//
// 它单独暴露，判据同 SourceDataScopeOutsideAcceptanceBaseline：编排要在问矩阵之前就答这一拒，AmendCustomerSourceData
// 再核一次守住聚合，两处共用这一段判断。别的内容形不涉成员，恒过。
func (request ShipmentRequest) CheckSourceDataContentAgainstBaseline(content SourceDataVersionContent) error {
	goods, declared := content.Goods()
	if !declared || request.state != ShipmentRequestAccepted {
		return nil
	}
	for _, allocation := range goods.Packing() {
		if !request.baseline.covers(allocation.Parcel()) {
			return fmt.Errorf("%w: packing names parcel %q", ErrParcelOutsideAcceptanceBaseline, allocation.Parcel())
		}
	}
	return goodsFitMembers(goods, request.baseline.DeclaredParcelIDs())
}

// GoodsDetailsStanding 说「此刻在用的货物明细」出自哪里。
type GoodsDetailsStanding uint8

const (
	GoodsDetailsStandingInvalid GoodsDetailsStanding = iota
	// GoodsDetailsOnSubmission：货物明细范围上没有修订版本，在用的是当前提交版本上的那一份（接受后即接受基线那一版）。
	GoodsDetailsOnSubmission
	// GoodsDetailsOnAdoptedVersion：范围上的当前采用判断为`已采用`，在用的是那一版自己的内容；那一版是显式清空时货物明细
	// 如实缺席，不回退到提交版本——回退等于把客户已清掉的申报当现行申报。
	GoodsDetailsOnAdoptedVersion
	// GoodsDetailsUndetermined：当前采用判断为`待复核`（两条修订分叉），此刻说不出该按哪一版，不交货物明细。
	GoodsDetailsUndetermined
)

func (standing GoodsDetailsStanding) String() string {
	switch standing {
	case GoodsDetailsOnSubmission:
		return "ON_SUBMISSION"
	case GoodsDetailsOnAdoptedVersion:
		return "ON_ADOPTED_VERSION"
	case GoodsDetailsUndetermined:
		return "UNDETERMINED"
	default:
		return ""
	}
}

// GoodsDetailsInEffect 答这份委托此刻在用的货物明细，判法见同名函数。
func (request ShipmentRequest) GoodsDetailsInEffect() (GoodsDetails, GoodsDetailsStanding) {
	return GoodsDetailsInEffect(request.currentVersion.goods, request.sourceDataVersions, request.baseline.SubmissionVersionID())
}

// GoodsDetailsInEffect 答一份委托此刻在用的货物明细：submitted 是当前提交版本上的那一份，versions 是这份委托的全部客户原始
// 资料版本（按形成顺序），root 是接受基线那一版提交版本（还没接受时为零值，那时也不会有资料版本）。采用判断与 CurrentSourceDataAdoption 是同一段（adoptionAmong），不另立一套「最后到达者获胜」。
//
// 它不挂在 ShipmentRequest 上而收散的版本，是给查阅读面用的：读面按快照摊记录、不重建聚合，又不能自己再写一份采用判断。
func GoodsDetailsInEffect(submitted GoodsDetails, versions []CustomerSourceDataVersion, root SubmissionVersionID) (GoodsDetails, GoodsDetailsStanding) {
	var scoped []CustomerSourceDataVersion
	for _, version := range versions {
		if version.scope.DataGroup() == GoodsDetailsDataGroup() {
			if _, named := version.scope.DeclaredParcelID(); !named {
				scoped = append(scoped, version)
			}
		}
	}
	if len(scoped) == 0 {
		return submitted, GoodsDetailsOnSubmission
	}
	judgment, _ := adoptionAmong(scoped, scoped[0].scope, root)
	if judgment.outcome != SourceDataAdopted {
		return GoodsDetails{}, GoodsDetailsUndetermined
	}
	for _, version := range scoped {
		if version.versionID == judgment.adopted {
			goods, _ := version.content.Goods()
			return goods, GoodsDetailsOnAdoptedVersion
		}
	}
	return GoodsDetails{}, GoodsDetailsUndetermined
}

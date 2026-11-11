package domain

// AcceptedServiceProductFor 答覆盖这件包裹的已接受委托申报的服务产品：接受基线所指那一版提交上声明的那一个。委托没接受、包裹不在
// 接受基线的声明成员里、或那一版没申报，都答零值——正式 SELL 评价按它选售价卡（ADR-0378 决定三），三种情形在选卡上是同一个答案：
// 只看不声明产品的卡。取接受基线那一版而不是当前版本：接受之后客户的修订只改资料版本，不改委托申报的产品，计价与商业解析
// 认的是同一版。
func (request ShipmentRequest) AcceptedServiceProductFor(parcel DeclaredParcelID) DeclaredServiceProduct {
	if request.state != ShipmentRequestAccepted || !request.baseline.covers(parcel) {
		return DeclaredServiceProduct{}
	}
	version, found := request.submissionVersionByID(request.baseline.SubmissionVersionID())
	if !found {
		return DeclaredServiceProduct{}
	}
	return version.RequestedServiceProduct()
}

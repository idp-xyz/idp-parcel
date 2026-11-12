package domain

import (
	"errors"
	"fmt"
)

// ErrInvalidAcceptedServiceProduct 表示读回的委托答不出它申报的服务产品：接受基线所指的提交版本不在本委托上。那是读面坏了，
// 不是「没申报」——答成零值会让申报了 FBA 的委托静默落到不声明产品的售价卡上。
var ErrInvalidAcceptedServiceProduct = errors.New("parcel shipment: invalid accepted service product")

// AcceptedServiceProductFor 答覆盖这件包裹的已接受委托申报的服务产品：接受基线所指那一版提交上声明的那一个。委托没接受、包裹不在
// 接受基线的声明成员里、或那一版没申报，都答零值——正式 SELL 评价按它选售价卡（ADR-0382 决定三），三种情形在选卡上是同一个答案：
// 只看不声明产品的卡。取接受基线那一版而不是当前版本：接受之后客户的修订只改资料版本，不改委托申报的产品，计价与商业解析
// 认的是同一版。基线指向的版本找不到时报错，与 AddressElementsFor、DeclaredMeasurementFor 同一个处置。
func (request ShipmentRequest) AcceptedServiceProductFor(parcel DeclaredParcelID) (DeclaredServiceProduct, error) {
	if request.state != ShipmentRequestAccepted || !request.baseline.covers(parcel) {
		return DeclaredServiceProduct{}, nil
	}
	version, found := request.submissionVersionByID(request.baseline.SubmissionVersionID())
	if !found {
		return DeclaredServiceProduct{}, fmt.Errorf("%w: 接受基线指向的提交版本 %q 不在本委托上",
			ErrInvalidAcceptedServiceProduct, request.baseline.SubmissionVersionID())
	}
	return version.RequestedServiceProduct(), nil
}

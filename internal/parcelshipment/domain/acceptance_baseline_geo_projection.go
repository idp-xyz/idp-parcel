package domain

import (
	"errors"
	"time"
)

var (
	// ErrInvalidAcceptanceBaselineGeoProjection 是构造时缺格：委托、客户账户或基线版本的引用缺一个，固定时刻为零，或投影
	// 不是「带了」的那一形。
	ErrInvalidAcceptanceBaselineGeoProjection = errors.New("parcel shipment: invalid acceptance baseline geo projection")
	// ErrAcceptanceBaselineInconsistent 说一份已接受委托的接受基线自相矛盾：没有接受基线、基线没有固定时刻，或基线所指的
	// 那一版提交不在本委托的版本里。基线在接受时与版本同笔固定、此后不再改写，读到这三种只能是存储坏了，重投不会变
	// （ADR-0029）。读口把它与读不动分开交出，消费方据以分响亮失败与重投。
	ErrAcceptanceBaselineInconsistent = errors.New(
		"parcel shipment: the accepted shipment request carries an inconsistent acceptance baseline")
)

// AcceptanceBaselineGeoProjection 是一件当前已接受包裹的受理基线地理投影（network-routing 那一侧称接受基线为受理基线）：
// 声明它的委托与客户账户的引用、接受基线所指的那一版提交与基线固定的时刻，以及那一版提交上的地理解析投影。
//
// 只有这些（ADR-0301）。收件人身份、地址文本、申报测量、接受决定与商业依据都不在其中——这份值就是给不该经手它们的
// 消费方用的；要加一格，先问那个消费方凭什么需要它。
type AcceptanceBaselineGeoProjection struct {
	shipmentRequestID ShipmentRequestID
	customerAccountID CustomerAccountID
	baselineVersion   SubmissionVersionID
	fixedAt           time.Time
	geo               CarriedGeoProjection
}

type AcceptanceBaselineGeoProjectionSpec struct {
	ShipmentRequestID ShipmentRequestID
	CustomerAccountID CustomerAccountID
	// BaselineVersion 是接受基线所指的那一版提交，不是委托当前的提交版本。
	BaselineVersion SubmissionVersionID
	FixedAt         time.Time
	// Geo 取自 BaselineVersion 那一版（SubmissionVersion.GeoProjection 同一取法），一格都没报也是「带了」的一份。
	Geo CarriedGeoProjection
}

func NewAcceptanceBaselineGeoProjection(spec AcceptanceBaselineGeoProjectionSpec) (AcceptanceBaselineGeoProjection, error) {
	if !spec.ShipmentRequestID.valid() || !spec.CustomerAccountID.valid() || !spec.BaselineVersion.valid() ||
		spec.FixedAt.IsZero() || !spec.Geo.Carried() {
		return AcceptanceBaselineGeoProjection{}, ErrInvalidAcceptanceBaselineGeoProjection
	}
	return AcceptanceBaselineGeoProjection{
		shipmentRequestID: spec.ShipmentRequestID,
		customerAccountID: spec.CustomerAccountID,
		baselineVersion:   spec.BaselineVersion,
		fixedAt:           spec.FixedAt,
		geo:               spec.Geo,
	}, nil
}

func (projection AcceptanceBaselineGeoProjection) ShipmentRequestID() ShipmentRequestID {
	return projection.shipmentRequestID
}

func (projection AcceptanceBaselineGeoProjection) CustomerAccountID() CustomerAccountID {
	return projection.customerAccountID
}

// BaselineVersion 是接受基线所指的那一版提交。
func (projection AcceptanceBaselineGeoProjection) BaselineVersion() SubmissionVersionID {
	return projection.baselineVersion
}

// FixedAt 是接受基线固定的时刻，即委托接受的时刻。
func (projection AcceptanceBaselineGeoProjection) FixedAt() time.Time {
	return projection.fixedAt
}

func (projection AcceptanceBaselineGeoProjection) Geo() CarriedGeoProjection {
	return projection.geo
}

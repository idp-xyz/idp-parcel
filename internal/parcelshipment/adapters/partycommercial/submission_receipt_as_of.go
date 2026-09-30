package partycommercial

import (
	"context"
	"fmt"
	"time"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// submissionReceiptSemantics 是参考配置里可达性时点锚已经采用的那一格
// （SYN-ASOF-SUBMIT-TIME）：锚在本提交版本已记录的系统接收时刻。
//
// 它是判断方法，不是租户截点。截点、以及其余语义（含财务控制那格 SYN-ASOF-ACCEPT-TIME），
// 这里形不成值，交回未配置。不用本地时钟，也不用接收时刻去补 requestEffectiveAt。
const submissionReceiptSemantics = "SYN-ASOF-SUBMIT-TIME"

// submissionReceiptLookup 只取形成「提交接收」时点要用的那份委托。
type submissionReceiptLookup interface {
	FindBySourceIdentity(ctx context.Context, identity psdomain.SourceIdentity) (psdomain.ShipmentRequest, bool, error)
}

// submissionReceiptAsOf 按声明语义把「提交接收」折成该提交版本的 receivedAt。
type submissionReceiptAsOf struct {
	requests submissionReceiptLookup
}

func NewSubmissionReceiptAsOf(requests submissionReceiptLookup) (*submissionReceiptAsOf, error) {
	if requests == nil {
		return nil, fmt.Errorf("parcel shipment partycommercial adapter: submission receipt as-of: requests is nil")
	}
	return &submissionReceiptAsOf{requests: requests}, nil
}

func (source *submissionReceiptAsOf) FormAsOfValue(
	ctx context.Context,
	query psports.JudgmentAsOfQuery,
) (time.Time, bool, error) {
	if query.Declared.Semantics().String() != submissionReceiptSemantics {
		return time.Time{}, false, nil
	}
	request, found, err := source.requests.FindBySourceIdentity(ctx, query.Identity)
	if err != nil {
		return time.Time{}, false, fmt.Errorf("submission receipt as-of: %w", err)
	}
	if !found {
		return time.Time{}, false, fmt.Errorf("submission receipt as-of: shipment request not found")
	}
	version, ok := request.SubmissionVersionByID(query.SubmissionVersion)
	if !ok {
		return time.Time{}, false, fmt.Errorf("submission receipt as-of: submission version not on request")
	}
	received := version.SourceSubmission().ReceivedAt()
	if received.IsZero() {
		return time.Time{}, false, nil
	}
	return received, true, nil
}

package partycommercial

import (
	"context"
	"fmt"
	"time"

	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/referenceconfig"
)

// submissionReceiptReference 是「提交接收」这一形态的参考配置（ADR-0147）。
// 租户在规则包里选用的是它的引用串，不是租户自己起的名字。
const submissionReceiptReference = "parcel-shipment/as-of-semantics/submission-receipt@1"

// SubmissionReceiptAsOf 把「提交接收」折成该提交版本已记录的系统接收时间。
//
// 只认产品发布的那一版引用。租户在哪一格采用，就在哪一格形成。
// 演示种子的财务控制格没采用这一形态，所以那一格答未配置。
// 不用本地时钟，也不用系统接收时间去补 requestEffectiveAt。
type SubmissionReceiptAsOf struct {
	requests  shipmentRequestFinder
	semantics string
}

func NewSubmissionReceiptAsOf(requests shipmentRequestFinder) (*SubmissionReceiptAsOf, error) {
	if requests == nil {
		return nil, fmt.Errorf("parcel shipment partycommercial adapter: submission receipt as-of: requests is nil")
	}
	citation, err := SubmissionReceiptCitation()
	if err != nil {
		return nil, fmt.Errorf("parcel shipment partycommercial adapter: submission receipt as-of: %w", err)
	}
	return &SubmissionReceiptAsOf{requests: requests, semantics: citation}, nil
}

var _ AsOfValueSource = (*SubmissionReceiptAsOf)(nil)

// SubmissionReceiptCitation 交回写进时点策略语义格的产品引用。原文打不开就不交：
// 采用指向的必须是已发布的那一版。
func SubmissionReceiptCitation() (string, error) {
	reference, err := referenceconfig.ParseReference(submissionReceiptReference)
	if err != nil {
		return "", err
	}
	if _, err := referenceconfig.Open(reference); err != nil {
		return "", err
	}
	return reference.Citation(), nil
}

func (source *SubmissionReceiptAsOf) FormAsOfValue(
	ctx context.Context,
	query psports.JudgmentAsOfQuery,
) (time.Time, bool, error) {
	if query.Declared.Semantics().String() != source.semantics {
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
	at, err := submissionReceiptInstant(version.SourceSubmission().ReceivedAt())
	if err != nil {
		return time.Time{}, false, err
	}
	return at, true, nil
}

// submissionReceiptInstant 把已记录的系统接收时间交回去。零值过不了来源指纹的构造门，
// 走到这里是数据损坏，不是还没登记——答未配置会让人去补一份永远补不来的策略。
func submissionReceiptInstant(received time.Time) (time.Time, error) {
	if received.IsZero() {
		return time.Time{}, fmt.Errorf("submission receipt as-of: receivedAt is zero")
	}
	return received, nil
}

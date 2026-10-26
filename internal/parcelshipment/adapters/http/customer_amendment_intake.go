package shipmenthttp

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// CustomerAmendmentAuthenticator 认证一次首方客户的资料修订出示，交回修订请求自身的来源身份（信封的来源请求键带
// `source-data-amendment:` 前缀）。次序同补充口：accessRequestID 在令牌、册、授予与准入范围都过了之后才被调用。
type CustomerAmendmentAuthenticator interface {
	AuthenticateAmendment(
		ctx context.Context,
		credential PresentedClientCredential,
		accessRequestID func() (string, error),
	) (domain.SourceIdentity, error)
}

// CustomerAmendmentIntake 是首方客户渠道在资料修订口的 Intake（ADR-0139 决定四；ADR-0310 取舍 B；ADR-0311；票 first-party-api/10）。
//
// 指名的收法同补充口：原提交的接入请求标识（`originalAccessRequestId`）与信封的租户、客户账户、来源拼出原提交的来源身份，所以只可能
// 指到信封账户名下的委托，他账户的委托查不到、与不存在同答 404；载荷里的 `shipmentRequestId` 必填，由 NewCustomerAmendmentHandler 与
// 找到的委托核对，不符同样按找不到答。请求方（`Requester`）是信封的客户账户本人——直客 1:1，不涉委派。
//
// 摘要走 PAM-1（domain.CanonicalizeCustomerSourceDataAmendment），内容的要素构造与提交口同一个函数。客户是否有权修订不在这里判：
// 授权走 PC 裁定（编排的 Authorizer），Intake 不预判。
type CustomerAmendmentIntake struct {
	authenticator CustomerAmendmentAuthenticator
	clock         ports.Clock
}

var _ SourceDataAmendmentIntake = (*CustomerAmendmentIntake)(nil)

func NewCustomerAmendmentIntake(authenticator CustomerAmendmentAuthenticator, clock ports.Clock) (*CustomerAmendmentIntake, error) {
	if authenticator == nil || clock == nil {
		return nil, fmt.Errorf("parcel shipment http: customer amendment intake: needs an authenticator and a clock")
	}
	return &CustomerAmendmentIntake{authenticator: authenticator, clock: clock}, nil
}

// amendmentDraft 是首方资料修订口的载荷形状（PAM-1；以 docs/api 为准）。内容字段按资料范围取用：寄件 / 收件范围
// 取 postalCode、countryCode（空串即显式清空该要素），申报测量范围取重量与外廓；其余范围与显式清空不带内容字段。
type amendmentDraft struct {
	AccessRequestID         string `json:"accessRequestId"`
	OriginalAccessRequestID string `json:"originalAccessRequestId"`
	ShipmentRequestID       string `json:"shipmentRequestId"`
	DataGroup               string `json:"dataGroup"`
	ParcelID                string `json:"parcelId"`
	// BasisVersionID 缺省即以接受基线为基准（只有「补充」意图合法）；更正与按版本的清空必须带上前序版本标识：提交回执的
	// submissionVersionId，或前一次修订答复的 sourceDataVersionId。
	BasisVersionID     string  `json:"basisVersionId"`
	Intent             string  `json:"intent"`
	Reason             string  `json:"reason"`
	RequestEffectiveAt *string `json:"requestEffectiveAt"`

	PostalCode             *string `json:"postalCode"`
	CountryCode            *string `json:"countryCode"`
	DeclaredWeightValue    string  `json:"declaredWeightValue"`
	DeclaredWeightUnit     string  `json:"declaredWeightUnit"`
	DeclaredLength         string  `json:"declaredLength"`
	DeclaredWidth          string  `json:"declaredWidth"`
	DeclaredHeight         string  `json:"declaredHeight"`
	DeclaredDimensionsUnit string  `json:"declaredDimensionsUnit"`
}

func (draft amendmentDraft) parcelDraft() declaredParcelDraft {
	return declaredParcelDraft{
		DeclaredWeightValue: draft.DeclaredWeightValue, DeclaredWeightUnit: draft.DeclaredWeightUnit,
		DeclaredLength: draft.DeclaredLength, DeclaredWidth: draft.DeclaredWidth, DeclaredHeight: draft.DeclaredHeight,
		DeclaredDimensionsUnit: draft.DeclaredDimensionsUnit,
	}
}

func amendmentIntentOf(word string) (domain.AmendmentIntent, bool) {
	for _, intent := range []domain.AmendmentIntent{domain.SupplementIntent, domain.CorrectionIntent, domain.ExplicitClearIntent} {
		if intent.String() == word {
			return intent, true
		}
	}
	return domain.AmendmentIntentInvalid, false
}

// IntakeSourceDataAmendment 把一次首方资料修订译成修订命令。先认证、后读载荷；租户、客户账户与来源取自认证结果。
func (intake *CustomerAmendmentIntake) IntakeSourceDataAmendment(
	ctx context.Context,
	request *http.Request,
) (application.AmendCustomerSourceDataCommand, error) {
	none := application.AmendCustomerSourceDataCommand{}
	var draft amendmentDraft
	amendmentIdentity, err := intake.authenticator.AuthenticateAmendment(ctx, presentedClientCredential(request), func() (string, error) {
		if err := decodeJSONPayload(request.Body, &draft); err != nil {
			return "", err
		}
		return draft.AccessRequestID, nil
	})
	if err != nil {
		return none, err
	}

	originalText := strings.TrimSpace(draft.OriginalAccessRequestID)
	originalKey, err := domain.NewSourceRequestKey(originalText)
	if err != nil {
		return none, malformedField("originalAccessRequestId", err)
	}
	originalIdentity, err := domain.NewSourceIdentity(
		amendmentIdentity.TenantID(), amendmentIdentity.CustomerAccountID(), amendmentIdentity.Source(), originalKey)
	if err != nil {
		return none, err
	}
	shipmentRequestID, err := domain.NewShipmentRequestID(strings.TrimSpace(draft.ShipmentRequestID))
	if err != nil {
		return none, malformedField("shipmentRequestId", err)
	}
	group, err := domain.NewSourceDataGroupReference(strings.TrimSpace(draft.DataGroup))
	if err != nil {
		return none, malformedField("dataGroup", err)
	}
	var scope domain.SourceDataScope
	if parcelText := strings.TrimSpace(draft.ParcelID); parcelText != "" {
		parcel, err := domain.NewDeclaredParcelID(parcelText)
		if err != nil {
			return none, malformedField("parcelId", err)
		}
		scope, err = domain.NewParcelScopedSourceData(shipmentRequestID, parcel, group)
		if err != nil {
			return none, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
		}
	} else if scope, err = domain.NewShipmentScopedSourceData(shipmentRequestID, group); err != nil {
		return none, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	basis := domain.NewSupplementOnAcceptanceBaseline()
	if basisText := strings.TrimSpace(draft.BasisVersionID); basisText != "" {
		prior, err := domain.NewSourceDataVersionID(basisText)
		if err != nil {
			return none, malformedField("basisVersionId", err)
		}
		if basis, err = domain.NewAmendmentOfVersion(prior); err != nil {
			return none, malformedField("basisVersionId", err)
		}
	}
	intent, known := amendmentIntentOf(strings.TrimSpace(draft.Intent))
	if !known {
		return none, malformedField("intent", fmt.Errorf("must be one of SUPPLEMENT, CORRECTION, EXPLICIT_CLEAR"))
	}
	reason, err := domain.NewAmendmentReasonReference(strings.TrimSpace(draft.Reason))
	if err != nil {
		return none, malformedField("reason", err)
	}
	effectiveAt, err := declaredEffectiveAt(submissionDraft{RequestEffectiveAt: draft.RequestEffectiveAt})
	if err != nil {
		return none, err
	}
	spec, err := amendmentContent(draft, group, intent)
	if err != nil {
		return none, err
	}
	spec.OriginalAccessRequestID = originalText
	spec.Scope, spec.Basis, spec.Intent, spec.Reason, spec.EffectiveAt = scope, basis, intent, reason, effectiveAt
	canonical, err := domain.CanonicalizeCustomerSourceDataAmendment(spec)
	if err != nil {
		return none, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	requester, err := domain.NewRequesterReference("CUSTOMER_ACCOUNT:" + amendmentIdentity.CustomerAccountID().String())
	if err != nil {
		return none, err
	}

	now := intake.clock.Now().UTC()
	return application.AmendCustomerSourceDataCommand{
		Identity:          originalIdentity,
		AmendmentIdentity: amendmentIdentity,
		PayloadDigest:     canonical.Digest(),
		OccurredAt:        now,
		ReceivedAt:        now,
		Scope:             scope,
		Basis:             basis,
		Intent:            intent,
		Reason:            reason,
		Requester:         requester,
		EffectiveAt:       effectiveAt,
		Content:           canonical.Content(),
	}, nil
}

// amendmentContent 按资料范围取用内容字段，并拒绝落不进这个范围或这个意图的字段：静默丢掉客户填的值，
// 比答 400 更糟。显式清空不带任何内容字段。
func amendmentContent(draft amendmentDraft, group domain.SourceDataGroupReference, intent domain.AmendmentIntent) (domain.SourceDataAmendmentPayloadSpec, error) {
	var spec domain.SourceDataAmendmentPayloadSpec
	parcel := draft.parcelDraft()
	hasElements := draft.PostalCode != nil || draft.CountryCode != nil
	hasMeasurement := weightDeclared(parcel) || strings.TrimSpace(draft.DeclaredLength+draft.DeclaredWidth+draft.DeclaredHeight+draft.DeclaredDimensionsUnit) != ""
	if intent == domain.ExplicitClearIntent {
		if hasElements || hasMeasurement {
			return spec, fmt.Errorf("%w: an explicit clear carries no content", ErrMalformedRequest)
		}
		return spec, nil
	}
	switch group {
	case domain.SenderPlaceDataGroup(), domain.DeliveryPlaceDataGroup():
		if hasMeasurement {
			return spec, fmt.Errorf("%w: measurement fields do not belong to %s", ErrMalformedRequest, group)
		}
		for element, value := range map[domain.AddressElementName]*string{
			domain.PostalCodeElement: draft.PostalCode, domain.CountryCodeElement: draft.CountryCode,
		} {
			if value == nil {
				continue
			}
			entry, err := domain.NewCanonicalContentEntry(domain.AddressElementEntryName(group, element), *value)
			if err != nil {
				return spec, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
			}
			spec.Entries = append(spec.Entries, entry)
		}
	case domain.DeclaredMeasurementDataGroup():
		if hasElements {
			return spec, fmt.Errorf("%w: address element fields do not belong to %s", ErrMalformedRequest, group)
		}
		if hasMeasurement {
			measurement, err := declaredMeasurement(parcel)
			if err != nil {
				return spec, fmt.Errorf("%w: measurement: %v", ErrMalformedRequest, err)
			}
			spec.Measurement = measurement
		}
	default:
		if hasElements || hasMeasurement {
			return spec, fmt.Errorf("%w: %s carries no content fields", ErrMalformedRequest, group)
		}
	}
	return spec, nil
}

// NewCustomerAmendmentHandler 在编排前核对载荷的 shipmentRequestId 与按信封账户加原提交标识找到的委托是同一个，不一致按找不到答
// （ErrShipmentRequestNotVisible，与不存在同字节）。查找与编排用同一个来源身份。
func NewCustomerAmendmentHandler(requests ports.ShipmentRequestRepository, inner AmendmentHandler) AmendmentHandler {
	return namedAmendmentChecker{requests: requests, inner: inner}
}

type namedAmendmentChecker struct {
	requests ports.ShipmentRequestRepository
	inner    AmendmentHandler
}

func (checker namedAmendmentChecker) Handle(
	ctx context.Context,
	command application.AmendCustomerSourceDataCommand,
) (application.AmendCustomerSourceDataResult, error) {
	request, found, err := checker.requests.FindBySourceIdentity(ctx, command.Identity)
	if err == nil && found && request.ShipmentRequestID() != command.Scope.ShipmentRequestID() {
		return application.AmendCustomerSourceDataResult{}, fmt.Errorf("amend customer source data: %w", application.ErrShipmentRequestNotVisible)
	}
	return checker.inner.Handle(ctx, command)
}

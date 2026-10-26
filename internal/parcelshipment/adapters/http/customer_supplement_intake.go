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

// CustomerSupplementAuthenticator 认证一次首方客户的补充出示，交回补充请求自身的来源身份（补充口的信封，
// 来源请求键带 `supplement:` 前缀）。次序与 CustomerClientAuthenticator 相同：accessRequestID 在令牌、册与授予都过了
// 之后才被调用。
type CustomerSupplementAuthenticator interface {
	AuthenticateSupplement(
		ctx context.Context,
		credential PresentedClientCredential,
		accessRequestID func() (string, error),
	) (domain.SourceIdentity, error)
}

// CustomerSupplementIntake 是首方客户渠道在补充口的 Intake（ADR-0139 决定四；ADR-0310 取舍 B、E；票 first-party-api/08）。
//
// 补充请求里有三个「指名」，都只能由客户从提交回执带回，而它们各自被服务端用不同的办法收住：
//
//   - **委托**：载荷带原提交的接入请求标识（`originalAccessRequestId`）。原提交的来源身份由补充信封的租户、客户账户、客户端
//     主体加这个标识拼出，所以只可能指到信封账户名下、同一客户端提交的委托；他账户的委托查不到，编排答`统一不可见结果`，
//     与根本没有这个委托同字节。
//   - **基准版本**：载荷带回提交回执的 `submissionVersionId`；基准不是委托此刻的当前版本，编排答`基准过期`。
//   - **成员**：载荷逐件带回回执里的 `parcelId`。它们是编排比对成员集合的依据，不一致答`成员集合变更`；指着别的委托的
//     包裹号同样落在这一格，不会改到别人的委托。
//
// 载荷里的 `shipmentRequestId`（回执的委托号）只用于编排在未决时签续办引用，不参与任何判定；填错只会让客户自己拿到一个
// 对不上的引用。
//
// 摘要走 PSU-1（domain.CanonicalizeCustomerSupplement）：内容同提交的规范化，另带基准版本，补充请求标识换基准重发是
// 来源冲突。准入范围在铸造侧判（MintSupplement，ADR-0139 决定八），本 Intake 不预取归属：委托在提交时已过生产归属那一关，
// 补充只靠账户绑定、授予、准入范围与基准核对，不引入 PC 裁定（取舍 B）。
type CustomerSupplementIntake struct {
	authenticator CustomerSupplementAuthenticator
	clock         ports.Clock
}

var _ SupplementIntake = (*CustomerSupplementIntake)(nil)

func NewCustomerSupplementIntake(authenticator CustomerSupplementAuthenticator, clock ports.Clock) (*CustomerSupplementIntake, error) {
	if authenticator == nil || clock == nil {
		return nil, fmt.Errorf("parcel shipment http: customer supplement intake: needs an authenticator and a clock")
	}
	return &CustomerSupplementIntake{authenticator: authenticator, clock: clock}, nil
}

// supplementDraft 是首方补充口的载荷形状（PSU-1 的字段，ADR-0140 的契约生成接手正式发布前以 docs/api 为准）：重报整份提交内容
// （不从旧版本继承）加四个来自提交回执的指名。字段全部显式列出，不嵌套提交的载荷类型：提交口的封闭解码不能因补充多出
// 的字段而放宽。
type supplementDraft struct {
	// AccessRequestID 是补充请求自己的接入请求标识，铸成 `supplement:` 前缀的来源请求键。
	AccessRequestID string `json:"accessRequestId"`
	// OriginalAccessRequestID 是原提交的接入请求标识，指名要补充的委托。
	OriginalAccessRequestID string `json:"originalAccessRequestId"`
	// ShipmentRequestID 与 BasisSubmissionVersionID 是提交回执里的 shipmentRequestId 与 submissionVersionId。
	ShipmentRequestID        string `json:"shipmentRequestId"`
	BasisSubmissionVersionID string `json:"basisSubmissionVersionId"`

	CustomerShipmentReference string                  `json:"customerShipmentReference"`
	RequestedServiceProduct   string                  `json:"requestedServiceProduct"`
	RequestEffectiveAt        *string                 `json:"requestEffectiveAt"`
	SenderRelation            string                  `json:"senderRelation"`
	SenderAddress             string                  `json:"senderAddress"`
	SenderPostalCode          string                  `json:"senderPostalCode"`
	SenderCountryCode         string                  `json:"senderCountryCode"`
	RecipientRelation         string                  `json:"recipientRelation"`
	RecipientAddress          string                  `json:"recipientAddress"`
	RecipientPostalCode       string                  `json:"recipientPostalCode"`
	RecipientCountryCode      string                  `json:"recipientCountryCode"`
	DestinationServiceScope   string                  `json:"destinationServiceScope"`
	Parcels                   []supplementParcelDraft `json:"parcels"`
}

// supplementParcelDraft 是提交的包裹草案加回执里的 parcelId：补充不现签成员标识，成员就是回执里那几件。
type supplementParcelDraft struct {
	ParcelID string `json:"parcelId"`
	declaredParcelDraft
}

// content 把补充载荷里的提交内容部分交给提交口那一套译码函数——规范化词表只此一份，补充与提交不会各说各话。
func (draft supplementDraft) content() submissionDraft {
	parcels := make([]declaredParcelDraft, 0, len(draft.Parcels))
	for _, parcel := range draft.Parcels {
		parcels = append(parcels, parcel.declaredParcelDraft)
	}
	return submissionDraft{
		AccessRequestID:           draft.AccessRequestID,
		CustomerShipmentReference: draft.CustomerShipmentReference,
		RequestedServiceProduct:   draft.RequestedServiceProduct,
		RequestEffectiveAt:        draft.RequestEffectiveAt,
		SenderRelation:            draft.SenderRelation,
		SenderAddress:             draft.SenderAddress,
		SenderPostalCode:          draft.SenderPostalCode,
		SenderCountryCode:         draft.SenderCountryCode,
		RecipientRelation:         draft.RecipientRelation,
		RecipientAddress:          draft.RecipientAddress,
		RecipientPostalCode:       draft.RecipientPostalCode,
		RecipientCountryCode:      draft.RecipientCountryCode,
		DestinationServiceScope:   draft.DestinationServiceScope,
		Parcels:                   parcels,
	}
}

// IntakeSupplement 把一次首方补充译成补充命令。先认证、后读载荷；租户、客户账户与来源取自认证结果，载荷里自报的任何
// 账户 / 租户 / 操作者字段都是未知字段，整份拒。
func (intake *CustomerSupplementIntake) IntakeSupplement(
	ctx context.Context,
	request *http.Request,
) (application.FormNewSubmissionVersionCommand, error) {
	var draft supplementDraft
	supplementIdentity, err := intake.authenticator.AuthenticateSupplement(ctx, presentedClientCredential(request), func() (string, error) {
		if err := decodeJSONPayload(request.Body, &draft); err != nil {
			return "", err
		}
		return draft.AccessRequestID, nil
	})
	if err != nil {
		return application.FormNewSubmissionVersionCommand{}, err
	}

	originalKey, err := domain.NewSourceRequestKey(strings.TrimSpace(draft.OriginalAccessRequestID))
	if err != nil {
		return application.FormNewSubmissionVersionCommand{}, malformedField("originalAccessRequestId", err)
	}
	originalIdentity, err := domain.NewSourceIdentity(
		supplementIdentity.TenantID(), supplementIdentity.CustomerAccountID(), supplementIdentity.Source(), originalKey)
	if err != nil {
		return application.FormNewSubmissionVersionCommand{}, err
	}
	shipmentRequestID, err := domain.NewShipmentRequestID(strings.TrimSpace(draft.ShipmentRequestID))
	if err != nil {
		return application.FormNewSubmissionVersionCommand{}, malformedField("shipmentRequestId", err)
	}
	basis, err := domain.NewSubmissionVersionID(strings.TrimSpace(draft.BasisSubmissionVersionID))
	if err != nil {
		return application.FormNewSubmissionVersionCommand{}, malformedField("basisSubmissionVersionId", err)
	}

	content := draft.content()
	customerReference, customerParcels, err := customerDeclaredReferences(content)
	if err != nil {
		return application.FormNewSubmissionVersionCommand{}, err
	}
	members := make([]domain.DeclaredParcelID, 0, len(draft.Parcels))
	for index, parcel := range draft.Parcels {
		member, err := domain.NewDeclaredParcelID(strings.TrimSpace(parcel.ParcelID))
		if err != nil {
			return application.FormNewSubmissionVersionCommand{}, malformedField(fmt.Sprintf("parcels[%d].parcelId", index), err)
		}
		members = append(members, member)
	}
	effectiveAt, err := declaredEffectiveAt(content)
	if err != nil {
		return application.FormNewSubmissionVersionCommand{}, err
	}
	canonical, err := domain.CanonicalizeCustomerSupplement(domain.SubmissionPayloadSpec{
		RequestReference:  customerReference,
		BasisVersion:      basis,
		EffectiveAt:       effectiveAt,
		DeclaredParcelIDs: customerParcels,
		Scope:             scopeEntries(content),
		Service:           serviceEntries(content),
	})
	if err != nil {
		return application.FormNewSubmissionVersionCommand{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	profiles, err := declaredProfiles(content, members)
	if err != nil {
		return application.FormNewSubmissionVersionCommand{}, err
	}

	now := intake.clock.Now().UTC()
	return application.FormNewSubmissionVersionCommand{
		Identity:                originalIdentity,
		SupplementIdentity:      supplementIdentity,
		PayloadDigest:           canonical.Digest(),
		OccurredAt:              now,
		ReceivedAt:              now,
		ShipmentRequestID:       shipmentRequestID,
		BasisVersion:            basis,
		DeclaredParcelIDs:       members,
		DeclaredProfiles:        profiles,
		DeclaredElements:        canonical.DeclaredElements(),
		RequestedServiceProduct: canonical.RequestedServiceProduct(),
	}, nil
}

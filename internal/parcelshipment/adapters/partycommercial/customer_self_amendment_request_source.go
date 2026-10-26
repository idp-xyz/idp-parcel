package partycommercial

import (
	"context"
	"fmt"

	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// CustomerSelfServiceAuthorityLevel 是直客自助修订在 PC 授权判断里用的商业权限等级（ADR-0311），产品常量、只在这一处定义。
// 租户是否放行直客修订，由它在授权册里有没有登一条 `SOURCE_DATA_AMENDMENT` × 本等级的授予来决定；没登答`未配置`，登了范围
// 对不上答`不允许`。它不借信用政策的等级，也不借合同委派的受托等级：那两个等级各有自己的含义，借来就把两件事绑成一件。
const CustomerSelfServiceAuthorityLevel = "CUSTOMER_SELF_SERVICE"

// CustomerSelfAmendmentRequestSource 实现 SourceDataAmendmentAuthorizationRequestSource：首方直客（ADR-0310 取舍 B，请求方即
// 客户本人，不涉委派）的商业坐标映射。
//
//   - 责任法人、商业范围：读解析键登记面（ResolutionKeyStore），与受理链同一行登记；没登记交回「折不出来」，授权停在
//     `授权不可用`，不拿别的来源顶。
//   - 权限等级：CustomerSelfServiceAuthorityLevel。
//   - 原因：询问里的结构化修订原因原样转写。
//   - 证据：被修订的委托的来源身份加资料范围（`first-party-amendment:<租户>/<账户>/<来源>/<原提交请求键>#<资料组>[/<包裹>]`）。
//     询问不带修订请求自己的身份，修订请求的留痕在版本上（版本的请求指纹），这里的证据指向它所修订的对象。
//     因此 PC 侧**不按证据区分同一委托同一资料组上的两次修订**：两次请求给 PC 的是同一份证据，区分靠 PS 的资料版本。
//   - 时点：注入的时钟，不取载荷。
//   - 请求方种类：客户账户。
type CustomerSelfAmendmentRequestSource struct {
	keys  ResolutionKeyStore
	clock psports.Clock
}

var _ SourceDataAmendmentAuthorizationRequestSource = (*CustomerSelfAmendmentRequestSource)(nil)

func NewCustomerSelfAmendmentRequestSource(keys ResolutionKeyStore, clock psports.Clock) (*CustomerSelfAmendmentRequestSource, error) {
	if keys == nil || clock == nil {
		return nil, fmt.Errorf("parcel shipment party commercial: customer self amendment request source needs a key store and a clock")
	}
	return &CustomerSelfAmendmentRequestSource{keys: keys, clock: clock}, nil
}

func (source *CustomerSelfAmendmentRequestSource) FormRequestCoordinates(
	ctx context.Context,
	query psports.SourceDataAmendmentAuthorizationQuery,
) (SourceDataAmendmentRequestCoordinates, bool, error) {
	none := SourceDataAmendmentRequestCoordinates{}
	row, found, err := source.keys.FindResolutionKey(ctx, query.Identity.TenantID().String(), query.Identity.CustomerAccountID().String())
	if err != nil {
		return none, false, fmt.Errorf("form amendment coordinates: %w", err)
	}
	if !found {
		return none, false, nil
	}
	var coordinates SourceDataAmendmentRequestCoordinates
	coordinates.RequesterKind = pcdomain.CustomerAccountRequester
	if coordinates.LegalEntity, err = pcdomain.NewLegalEntityReference(row.LegalEntity); err != nil {
		return none, false, fmt.Errorf("form amendment coordinates: %w", err)
	}
	if coordinates.Scope, err = pcdomain.NewCommercialScopeReference(row.Scope); err != nil {
		return none, false, fmt.Errorf("form amendment coordinates: %w", err)
	}
	if coordinates.Level, err = pcdomain.NewAuthorityLevel(CustomerSelfServiceAuthorityLevel); err != nil {
		return none, false, fmt.Errorf("form amendment coordinates: %w", err)
	}
	if coordinates.Reason, err = pcdomain.NewStructuredReason(query.Reason.String()); err != nil {
		return none, false, fmt.Errorf("form amendment coordinates: %w", err)
	}
	evidence := fmt.Sprintf("first-party-amendment:%s/%s/%s/%s#%s",
		query.Identity.TenantID(), query.Identity.CustomerAccountID(), query.Identity.Source(), query.Identity.RequestKey(),
		query.Scope.DataGroup())
	if parcel, named := query.Scope.DeclaredParcelID(); named {
		evidence += "/" + parcel.String()
	}
	if coordinates.Evidence, err = pcdomain.NewEvidenceReference(evidence); err != nil {
		return none, false, fmt.Errorf("form amendment coordinates: %w", err)
	}
	coordinates.At = source.clock.Now().UTC()
	return coordinates, true, nil
}

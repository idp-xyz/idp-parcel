package visibilityhttp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 按运单号查询那一口的查询串只认这两格（ADR-0356 决定一）：documentNumber 恰一格，after 至多一格。after 与 ADR-0144 的翻页参数
// 同名同义。
const (
	shipmentTrackingNumberParameter = "documentNumber"
	shipmentTrackingAfterParameter  = "after"
)

// customerShipmentTrackingPageSize 是一页至多列几件：产品策略，随首方契约发布（ADR-0356 决定三、越权风险点 3）。调用方不给页大小，
// 由 Intake 按渠道契约定（ADR-0144 决定二）；改它是契约变更。
const customerShipmentTrackingPageSize = 50

// ShipmentTrackingQuery 是一次已授权的按运单号查询：租户与账户来自认证结果，号来自请求，页大小来自渠道契约。翻页游标是调用方的
// 查询意图、不属授权，由端点处理器解（ADR-0144 决定六）。
type ShipmentTrackingQuery struct {
	Tenant   domain.TenantID
	Customer domain.CustomerAccountReference
	Number   domain.ShipmentDocumentNumberReference
	PageSize int
}

// ShipmentTrackingIntake 把一次已认证的按运单号查询请求翻译成查询键。理由同 QueryIntake：账户只来自认证结果，本包不带采信实现。
type ShipmentTrackingIntake interface {
	IntakeShipmentTrackingQuery(ctx context.Context, request *http.Request) (ShipmentTrackingQuery, error)
}

// CustomerShipmentTrackingAuthenticator 认证一次按运单号查询的首方出示。它与 CustomerTrackingViewAuthenticator 分开，是因为两口各核
// 各的授予（一口一词，ADR-0356 决定一）：同一个认证方拿按包裹查的授予放行这一口，就是复用了词。
type CustomerShipmentTrackingAuthenticator interface {
	AuthenticateShipmentTrackingQuery(ctx context.Context, credential PresentedCustomerCredential) (CustomerViewer, error)
}

// CustomerShipmentTrackingIntake 是首方客户渠道在 `/customer-shipment-tracking` 的 Intake（ADR-0356 决定一；ADR-0139 决定四）。
//
// 先认证、后读查询串，理由同 CustomerTrackingViewIntake。查询串封闭：documentNumber 与 after 之外带任何一格都答坏请求，自报的
// 账户、租户与 ADR-0144 的 sort、q 都落在这一类——本口不开排序与筛选，悄悄忽略会让客户系统以为它们起了作用。号逐字收下：不去
// 空白、不折大小写，全空白立不起一个号也答坏请求；这一判只看输入，与号存不存在无关。
type CustomerShipmentTrackingIntake struct {
	authenticator CustomerShipmentTrackingAuthenticator
}

var _ ShipmentTrackingIntake = (*CustomerShipmentTrackingIntake)(nil)

func NewCustomerShipmentTrackingIntake(authenticator CustomerShipmentTrackingAuthenticator) (*CustomerShipmentTrackingIntake, error) {
	if authenticator == nil {
		return nil, errors.New("visibility http: customer shipment tracking intake needs an authenticator")
	}
	return &CustomerShipmentTrackingIntake{authenticator: authenticator}, nil
}

func (intake *CustomerShipmentTrackingIntake) IntakeShipmentTrackingQuery(ctx context.Context, request *http.Request) (ShipmentTrackingQuery, error) {
	viewer, err := intake.authenticator.AuthenticateShipmentTrackingQuery(ctx, presentedCustomerCredential(request))
	if err != nil {
		return ShipmentTrackingQuery{}, err
	}
	parameters, err := url.ParseQuery(request.URL.RawQuery)
	if err != nil {
		return ShipmentTrackingQuery{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	for name := range parameters {
		if name != shipmentTrackingNumberParameter && name != shipmentTrackingAfterParameter {
			return ShipmentTrackingQuery{}, fmt.Errorf("%w: query parameter %q is not part of the shipment tracking query", ErrMalformedRequest, name)
		}
	}
	numbers := parameters[shipmentTrackingNumberParameter]
	if len(numbers) != 1 {
		return ShipmentTrackingQuery{}, fmt.Errorf("%w: exactly one %s is required", ErrMalformedRequest, shipmentTrackingNumberParameter)
	}
	if len(parameters[shipmentTrackingAfterParameter]) > 1 {
		return ShipmentTrackingQuery{}, fmt.Errorf("%w: at most one %s is allowed", ErrMalformedRequest, shipmentTrackingAfterParameter)
	}
	number, err := domain.NewShipmentDocumentNumberReference(numbers[0])
	if err != nil {
		return ShipmentTrackingQuery{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	return ShipmentTrackingQuery{
		Tenant:   viewer.Tenant,
		Customer: viewer.Customer,
		Number:   number,
		PageSize: customerShipmentTrackingPageSize,
	}, nil
}

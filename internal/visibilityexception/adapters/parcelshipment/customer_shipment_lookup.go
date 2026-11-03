package parcelshipment

import (
	"context"
	"errors"
	"fmt"

	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// ErrCustomerShipmentUntranslatable 是端口哨兵在本译名包的同名引用，理由同 ErrCustomerAccountUntranslatable。
var ErrCustomerShipmentUntranslatable = veports.ErrCustomerShipmentUntranslatable

// CustomerShipmentLookup 把 PS 的按运单号读口（ReadCustomerShipmentByDocumentNumberHandler）译成 VE 的整单读口
// （veports.CustomerShipmentTrackingView；ADR-0356 决定四）。只翻译不判断：委托属不属请求方、处在哪一步、当前有效包裹是哪几件，
// 都由 PS 答；账户作为查询条件交过去，「存在但不是你的」这一位 PS 本就不给，本适配器也无从泄出。不缓存、不写 PS 的库。
type CustomerShipmentLookup struct {
	reads *psapplication.ReadCustomerShipmentByDocumentNumberHandler
}

func NewCustomerShipmentLookup(
	reads *psapplication.ReadCustomerShipmentByDocumentNumberHandler,
) (*CustomerShipmentLookup, error) {
	if reads == nil {
		return nil, errors.New("visibility exception parcelshipment adapter: customer shipment read is nil")
	}
	return &CustomerShipmentLookup{reads: reads}, nil
}

var _ veports.CustomerShipmentTrackingView = (*CustomerShipmentLookup)(nil)

// LocateCustomerShipment 照端口注释的全函数翻译作答。PS 读不动原样上抛（包一层定位前缀）：它不是四种里的任何一种，译成查无
// 此单会把「没查成」说成「没有这单」。PS 的哨兵不外泄，译不成的一律包本上下文的具名哨兵，理由同 FindCustomerAccount。
func (lookup *CustomerShipmentLookup) LocateCustomerShipment(
	ctx context.Context,
	tenant vedomain.TenantID,
	customer vedomain.CustomerAccountReference,
	number vedomain.ShipmentDocumentNumberReference,
	page veports.ShipmentParcelPage,
) (veports.LocatedCustomerShipment, error) {
	none := veports.LocatedCustomerShipment{}
	psTenant, err := psdomain.NewTenantID(tenant.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrCustomerShipmentUntranslatable, err)
	}
	psCustomer, err := psdomain.NewCustomerAccountID(customer.String())
	if err != nil {
		return none, fmt.Errorf("%w: customer account: %v", ErrCustomerShipmentUntranslatable, err)
	}
	psNumber, err := psdomain.NewShipmentDocumentNumber(number.String())
	if err != nil {
		return none, fmt.Errorf("%w: document number: %v", ErrCustomerShipmentUntranslatable, err)
	}

	shipment, err := lookup.reads.Handle(ctx, psapplication.ReadCustomerShipmentByDocumentNumberQuery{
		Tenant:        psTenant,
		Customer:      psCustomer,
		Number:        psNumber,
		AfterPosition: page.AfterPosition,
		Limit:         page.Limit,
	})
	if err != nil {
		return none, fmt.Errorf("visibility exception parcelshipment adapter: read customer shipment by document number: %w", err)
	}
	if !shipment.Found() {
		return veports.LocatedCustomerShipment{Standing: veports.ShipmentNotFound}, nil
	}
	switch shipment.State() {
	case psdomain.ShipmentRequestSubmitted:
		return veports.LocatedCustomerShipment{Standing: veports.ShipmentAwaitingAcceptance}, nil
	case psdomain.ShipmentRequestRejected, psdomain.ShipmentRequestWithdrawn:
		return veports.LocatedCustomerShipment{Standing: veports.ShipmentClosedWithoutService}, nil
	case psdomain.ShipmentRequestAccepted:
		parcels, err := shipmentParcelsOf(shipment.Parcels())
		if err != nil {
			return none, err
		}
		return veports.LocatedCustomerShipment{
			Standing: veports.ShipmentParcelsListed,
			Parcels:  parcels,
			Total:    shipment.Total(),
			More:     shipment.More(),
		}, nil
	default:
		// 不兜底：PS 加了一个状态而本适配器没跟上，答哪一格都是替 PS 编一个续办。
		return none, fmt.Errorf("%w: shipment request state %d", ErrCustomerShipmentUntranslatable, shipment.State())
	}
}

func shipmentParcelsOf(effective []psdomain.EffectiveParcel) ([]veports.ShipmentParcel, error) {
	parcels := make([]veports.ShipmentParcel, 0, len(effective))
	for _, member := range effective {
		parcel, err := vedomain.NewTrackedParcelReference(member.Parcel().String())
		if err != nil {
			return nil, fmt.Errorf("%w: parcel: %v", ErrCustomerShipmentUntranslatable, err)
		}
		translated := veports.ShipmentParcel{Parcel: parcel, Position: member.Position()}
		if number, declared := member.CustomerParcelNumber(); declared {
			reference, err := vedomain.NewCustomerParcelReference(number.String())
			if err != nil {
				return nil, fmt.Errorf("%w: customer parcel number: %v", ErrCustomerShipmentUntranslatable, err)
			}
			translated.CustomerParcelReference = reference
			translated.HasCustomerParcelReference = true
		}
		for _, source := range member.Replaces() {
			replaced, err := vedomain.NewTrackedParcelReference(source.String())
			if err != nil {
				return nil, fmt.Errorf("%w: replaced parcel: %v", ErrCustomerShipmentUntranslatable, err)
			}
			translated.Replaces = append(translated.Replaces, replaced)
		}
		parcels = append(parcels, translated)
	}
	return parcels, nil
}

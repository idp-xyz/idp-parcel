package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ErrInvalidEffectiveParcelPage 说按页取的请求本身不成立：页大小非正，或起点位次为负。那是调用方的编程错误，不是业务答案；静默
// 答一页会把「忘了传」变成一个没人决定过的页大小。
var ErrInvalidEffectiveParcelPage = errors.New("parcel shipment: invalid effective parcel page")

// ReadCustomerShipmentByDocumentNumberQuery 是一次客户按运单号的读（ADR-0356 决定四）。租户与账户来自消费方已认证的请求方，
// 本读口不判它们从哪来，只把它们当查询条件。
type ReadCustomerShipmentByDocumentNumberQuery struct {
	Tenant   domain.TenantID
	Customer domain.CustomerAccountID
	Number   domain.ShipmentDocumentNumber
	// AfterPosition 是上一页末件的位次（EffectiveParcel.Position）；0 即第一页。
	AfterPosition int
	// Limit 是一页至多几件，由接入面按渠道契约定。
	Limit int
}

// CustomerShipmentByDocumentNumber 是读的结果：找没找到；找到时委托所处的状态；已接受时另带本页的当前有效包裹、全集件数，与
// 本页之后还有没有。
type CustomerShipmentByDocumentNumber struct {
	found   bool
	state   domain.ShipmentRequestState
	parcels []domain.EffectiveParcel
	total   int
	more    bool
}

// Found 为假即本租户下请求方账户名下没有这个号：查无、属别的账户、委托没有单据号，同此一格。
func (shipment CustomerShipmentByDocumentNumber) Found() bool {
	return shipment.found
}

func (shipment CustomerShipmentByDocumentNumber) State() domain.ShipmentRequestState {
	return shipment.state
}

// Parcels 交回本页的当前有效包裹（拷贝），按位次升序；不是已接受即为空。
func (shipment CustomerShipmentByDocumentNumber) Parcels() []domain.EffectiveParcel {
	return append([]domain.EffectiveParcel(nil), shipment.parcels...)
}

// Total 是这份委托此刻的当前有效包裹件数，与本页出自同一次读。
func (shipment CustomerShipmentByDocumentNumber) Total() int {
	return shipment.total
}

// More 答本页之后是否还有件。位次在谱系落地后不连续，调用方不能拿末件位次与 Total 比出来。
func (shipment CustomerShipmentByDocumentNumber) More() bool {
	return shipment.more
}

// ReadCustomerShipmentByDocumentNumberHandler 是客户按运单号追踪时本上下文那一半的应用读口：委托所处的状态，已接受时按页取
// 当前有效包裹。
//
// 账户隔离在端口的键上，不在这里：端口对号属别的账户与查无同答，本读口也就答不出前者。按页取在这里而不下推到读口，是因为
// 当前有效包裹今天出自委托快照的一次读，整份读出再切页不多一次往返；谱系落成独立的表之后，下推是读口内部的事，本读口的
// 签名不变。
type ReadCustomerShipmentByDocumentNumberHandler struct {
	shipments ports.CustomerShipmentByDocumentNumberView
}

func NewReadCustomerShipmentByDocumentNumberHandler(
	shipments ports.CustomerShipmentByDocumentNumberView,
) (*ReadCustomerShipmentByDocumentNumberHandler, error) {
	if shipments == nil {
		return nil, errors.New("parcel shipment: customer shipment by document number view is nil")
	}
	return &ReadCustomerShipmentByDocumentNumberHandler{shipments: shipments}, nil
}

func (handler *ReadCustomerShipmentByDocumentNumberHandler) Handle(
	ctx context.Context,
	query ReadCustomerShipmentByDocumentNumberQuery,
) (CustomerShipmentByDocumentNumber, error) {
	if query.Limit < 1 || query.AfterPosition < 0 {
		return CustomerShipmentByDocumentNumber{}, fmt.Errorf("%w: limit %d, after position %d",
			ErrInvalidEffectiveParcelPage, query.Limit, query.AfterPosition)
	}
	record, found, err := handler.shipments.FindCustomerShipmentByDocumentNumber(ctx, query.Tenant, query.Customer, query.Number)
	if err != nil {
		return CustomerShipmentByDocumentNumber{}, fmt.Errorf("read customer shipment by document number: %w", err)
	}
	if !found {
		return CustomerShipmentByDocumentNumber{}, nil
	}
	if record.State != domain.ShipmentRequestAccepted {
		if len(record.EffectiveParcels) != 0 {
			return CustomerShipmentByDocumentNumber{}, fmt.Errorf("%w: a %s shipment request carries effective parcels",
				domain.ErrInconsistentEffectiveParcels, record.State)
		}
		return CustomerShipmentByDocumentNumber{found: true, state: record.State}, nil
	}

	all := record.EffectiveParcels
	if len(all) == 0 {
		return CustomerShipmentByDocumentNumber{}, fmt.Errorf("%w: an accepted shipment request has no effective parcel",
			domain.ErrInconsistentEffectiveParcels)
	}
	start := len(all)
	for index, parcel := range all {
		// 位次必须严格升序：游标按位次续读，乱序时「严格排在其后」就不再是一段连续的尾巴，下一页会跳件或重出。
		if index > 0 && parcel.Position() <= all[index-1].Position() {
			return CustomerShipmentByDocumentNumber{}, fmt.Errorf("%w: positions are not strictly ascending",
				domain.ErrInconsistentEffectiveParcels)
		}
		if start == len(all) && parcel.Position() > query.AfterPosition {
			start = index
		}
	}
	page := all[start:]
	more := len(page) > query.Limit
	if more {
		page = page[:query.Limit]
	}
	return CustomerShipmentByDocumentNumber{
		found:   true,
		state:   record.State,
		parcels: append([]domain.EffectiveParcel(nil), page...),
		total:   len(all),
		more:    more,
	}, nil
}

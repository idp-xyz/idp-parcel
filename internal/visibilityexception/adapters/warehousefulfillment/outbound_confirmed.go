// Package warehousefulfillment 消费仓配履约交出的出库确认，译成已接受源事实再派生投影。
// 里程碑映射没登记时派生结果是未归类，不编默认映射。
package warehousefulfillment

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

const (
	outboundConfirmedEventType = "warehouse-fulfillment.outbound-confirmed"
	consumerName               = "visibility-exception/warehouse-outbound-confirmed"
	factKind                   = "warehouse-outbound-confirmed"
)

// OutboundConfirmedEventType 与仓配履约交出的事件类型同一串。
const OutboundConfirmedEventType = outboundConfirmedEventType

type projectionDeriver interface {
	Handle(ctx context.Context, command veapplication.DeriveProjectionCommand) (veapplication.DeriveProjectionResult, error)
}

type confirmedParcel struct {
	TenantID         string    `json:"tenantId"`
	FactID           string    `json:"factId"`
	DeclaredParcelID string    `json:"declaredParcelId"`
	OccurredAt       time.Time `json:"occurredAt"`
	Version          string    `json:"version"`
	Supersedes       string    `json:"supersedes"`
	RecordedAt       time.Time `json:"-"`
}

func NewOutboundConfirmedConsumer(transactor bentoapp.Transactor, store *inbox.Store, derive projectionDeriver) (*inboxconsume.Gate[confirmedParcel], error) {
	if derive == nil {
		return nil, fmt.Errorf("visibility exception warehouse fulfillment: projection deriver is nil")
	}
	return inboxconsume.New(inboxconsume.Spec[confirmedParcel]{
		Transactor: transactor, Store: store, Name: consumerName, EventType: outboundConfirmedEventType,
		DecodeEnvelope: func(envelope eventing.Envelope) (confirmedParcel, error) {
			var body confirmedParcel
			if err := json.Unmarshal(envelope.Payload, &body); err != nil {
				return confirmedParcel{}, err
			}
			if body.TenantID == "" || body.FactID == "" || body.DeclaredParcelID == "" || body.OccurredAt.IsZero() {
				return confirmedParcel{}, fmt.Errorf("outbound confirmation is missing a parcel")
			}
			body.RecordedAt = envelope.RecordedAt
			return body, nil
		},
		Handle: func(ctx context.Context, body confirmedParcel) error {
			command, err := projectionCommand(body)
			if err != nil {
				return err
			}
			result, err := derive.Handle(ctx, command)
			if err != nil {
				return err
			}
			return veconsume.Consumption(result)
		},
		UnexpectedType: "visibility exception warehouse fulfillment", HandleVerb: "derive outbound confirmation",
	})
}

func projectionCommand(body confirmedParcel) (veapplication.DeriveProjectionCommand, error) {
	var none veapplication.DeriveProjectionCommand
	tenant, err := vedomain.NewTenantID(body.TenantID)
	if err != nil {
		return none, err
	}
	parcel, err := vedomain.NewTrackedParcelReference(body.DeclaredParcelID)
	if err != nil {
		return none, err
	}
	fact, err := vedomain.NewSourceFactReference(body.FactID)
	if err != nil {
		return none, err
	}
	versionText := body.Version
	if versionText == "" {
		versionText = "1"
	}
	version, err := vedomain.NewSourceFactVersion(versionText)
	if err != nil {
		return none, err
	}
	var supersedes vedomain.SourceFactVersion
	if body.Supersedes != "" {
		supersedes, err = vedomain.NewSourceFactVersion(body.Supersedes)
		if err != nil {
			return none, err
		}
	}
	kind, err := vedomain.NewSourceFactKind(factKind)
	if err != nil {
		return none, err
	}
	occurred := body.OccurredAt.UTC()
	received := body.RecordedAt.UTC()
	if received.IsZero() {
		received = occurred
	}
	return veapplication.DeriveProjectionCommand{
		TenantID: tenant,
		Fact: vedomain.AcceptedSourceFactSpec{
			Source: vedomain.SourceWarehouseFulfillment, Parcel: parcel, Fact: fact, Kind: kind, Version: version, Supersedes: supersedes,
			OccurredAt: occurred, EffectiveAt: occurred, ReceivedAt: received,
		},
	}, nil
}

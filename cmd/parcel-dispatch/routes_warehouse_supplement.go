package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
	wfinbox "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/inbox"
	whpostgres "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	warehouseapp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
)

const customerSupplementEventType eventing.EventType = "parcel-shipment.customer-supplement.required"

// errWarehouseSupplementWhileDisabled 是开关关着时，来源为仓配出库的待补充信封的显式失败。
// 别的来源由收工路入账。关着却还有仓配出库在途，是部署方没排空。
var errWarehouseSupplementWhileDisabled = errors.New("parcel-dispatch: warehouse outbound supplement while warehouse fulfillment is off")

// 开关关着：只登收工路。开关开着：收工路不登，由仓配那一路接。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		if sources.settings.warehouseFulfillment {
			return nil, nil
		}
		consumer, err := newSupplementCloseOut(sources.db.Transactor(), sources.inbox)
		if err != nil {
			return nil, err
		}
		return []routeLeg{{
			event: customerSupplementEventType, position: positionPrimary, consumer: consumer,
			untranslated: true, label: "customer supplement close-out",
		}}, nil
	})
	registerWarehouseFulfillmentRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		waves, err := whpostgres.NewWaves(sources.db)
		if err != nil {
			return nil, err
		}
		consumer, err := wfinbox.NewCustomerSupplementConsumer(sources.db.Transactor(), sources.inbox, warehouseapp.NewSupplementHandler(waves, sources.clock))
		if err != nil {
			return nil, err
		}
		return []routeLeg{{
			event: wfinbox.CustomerSupplementRequiredEventType, position: positionPrimary, consumer: consumer,
			untranslated: true, label: "warehouse supplement",
		}}, nil
	})
}

type supplementCloseOut struct {
	gate *inboxconsume.Gate[supplementNotice]
}

func newSupplementCloseOut(transactor bentoapp.Transactor, store *inbox.Store) (*supplementCloseOut, error) {
	gate, err := inboxconsume.New(inboxconsume.Spec[supplementNotice]{
		Transactor: transactor, Store: store, Name: "parcel-dispatch/customer-supplement-close-out",
		EventType: customerSupplementEventType, Decode: decodeSupplementNotice,
		Handle: closeOutSupplement, UnexpectedType: "parcel-dispatch", HandleVerb: "close out customer supplement",
	})
	if err != nil {
		return nil, err
	}
	return &supplementCloseOut{gate: gate}, nil
}

func (consumer *supplementCloseOut) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

type supplementNotice struct {
	Source string
}

func decodeSupplementNotice(payload []byte) (supplementNotice, error) {
	var body struct {
		Source string `json:"source"`
	}
	if err := json.Unmarshal(payload, &body); err != nil || body.Source == "" {
		return supplementNotice{}, fmt.Errorf("parcel-dispatch: customer supplement payload has no source")
	}
	return supplementNotice{Source: body.Source}, nil
}

func closeOutSupplement(_ context.Context, notice supplementNotice) error {
	if notice.Source == "WAREHOUSE_OUTBOUND" {
		return errWarehouseSupplementWhileDisabled
	}
	return nil
}

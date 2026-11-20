package inbox

import (
	"context"
	"encoding/json"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/inboxconsume"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

const (
	sourceDataEventType      eventing.EventType = "parcel-shipment.source-data-version.formed"
	cancellationEventType    eventing.EventType = "parcel-shipment.parcel-cancellation.recorded"
	sourceDataConsumerName                      = "warehouse-fulfillment/source-data-version"
	cancellationConsumerName                    = "warehouse-fulfillment/parcel-cancellation"
)

// SourceDataFormedEventType 与 CancellationRecordedEventType 是本消费者认的事件类型，字符串与小包托运交出的相同。
const (
	SourceDataFormedEventType     = sourceDataEventType
	CancellationRecordedEventType = cancellationEventType
)

type followUpRecorder interface {
	RecordSourceData(ctx context.Context, command application.SourceDataCommand) (application.FollowUpResult, error)
	RecordCancellation(ctx context.Context, command application.CancellationCommand) (application.FollowUpResult, error)
}

func NewSourceDataConsumer(transactor bentoapp.Transactor, store *inbox.Store, recorder followUpRecorder) (*inboxconsume.Gate[application.SourceDataCommand], error) {
	return inboxconsume.New(inboxconsume.Spec[application.SourceDataCommand]{
		Transactor: transactor, Store: store, Name: sourceDataConsumerName, EventType: sourceDataEventType,
		Decode: decodeSourceData, Handle: func(ctx context.Context, command application.SourceDataCommand) error {
			return finishFollowUp(recorder.RecordSourceData(ctx, command))
		},
		UnexpectedType: "warehouse fulfillment inbox", HandleVerb: "record source data version",
	})
}

func NewCancellationConsumer(transactor bentoapp.Transactor, store *inbox.Store, recorder followUpRecorder) (*inboxconsume.Gate[application.CancellationCommand], error) {
	return inboxconsume.New(inboxconsume.Spec[application.CancellationCommand]{
		Transactor: transactor, Store: store, Name: cancellationConsumerName, EventType: cancellationEventType,
		Decode: decodeCancellation, Handle: func(ctx context.Context, command application.CancellationCommand) error {
			return finishFollowUp(recorder.RecordCancellation(ctx, command))
		},
		UnexpectedType: "warehouse fulfillment inbox", HandleVerb: "record parcel cancellation",
	})
}

func finishFollowUp(result application.FollowUpResult, err error) error {
	if err != nil {
		return err
	}
	switch result.Outcome() {
	case application.FollowUpAdopted, application.FollowUpAwaitingReview, application.FollowUpCancelled, application.FollowUpPosted, application.FollowUpExisting, application.FollowUpConflict:
		return nil
	default:
		return fmt.Errorf("%w: %s", ErrPoisonEnvelope, result.Outcome())
	}
}

func decodeSourceData(payload []byte) (application.SourceDataCommand, error) {
	var body struct {
		TenantID         string `json:"tenantId"`
		Source           string `json:"source"`
		SourceRequestKey string `json:"sourceRequestKey"`
		Version          string `json:"version"`
		AdoptionOutcome  string `json:"adoptionOutcome"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return application.SourceDataCommand{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	tenant, err := domain.NewTenantID(body.TenantID)
	if err != nil || body.Source == "" || body.SourceRequestKey == "" || body.Version == "" || body.AdoptionOutcome == "" {
		return application.SourceDataCommand{}, fmt.Errorf("%w: missing source data fields", ErrPoisonEnvelope)
	}
	return application.SourceDataCommand{
		Tenant: tenant, Version: body.Version, Source: body.Source, SourceRequestKey: body.SourceRequestKey, Adoption: body.AdoptionOutcome,
	}, nil
}

func decodeCancellation(payload []byte) (application.CancellationCommand, error) {
	var body struct {
		TenantID   string `json:"tenantId"`
		RequestKey string `json:"requestKey"`
		Parcel     string `json:"parcel"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return application.CancellationCommand{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	tenant, err := domain.NewTenantID(body.TenantID)
	if err != nil || body.RequestKey == "" || body.Parcel == "" {
		return application.CancellationCommand{}, fmt.Errorf("%w: missing cancellation fields", ErrPoisonEnvelope)
	}
	return application.CancellationCommand{Tenant: tenant, RequestKey: body.RequestKey, ParcelID: body.Parcel}, nil
}

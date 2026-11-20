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

const customerSupplementConsumerName = "warehouse-fulfillment/customer-supplement"

// CustomerSupplementRequiredEventType 是本消费者认的事件类型。字符串与小包托运交出的那一封相同，本包自己写。
const CustomerSupplementRequiredEventType eventing.EventType = "parcel-shipment.customer-supplement.required"

// CustomerSupplementNotice 是待受控补充信封上的引用。
type CustomerSupplementNotice struct {
	TenantID          string
	Source            string
	SourceRequestKey  string
	SubmissionVersion string
}

// SupplementRecorder 是待补充入账。
type SupplementRecorder interface {
	Record(ctx context.Context, command application.SupplementCommand) (application.SupplementResult, error)
}

// CustomerSupplementConsumer 把待受控补充信封推进仓配的消费门。
type CustomerSupplementConsumer struct {
	gate *inboxconsume.Gate[CustomerSupplementNotice]
}

func NewCustomerSupplementConsumer(transactor bentoapp.Transactor, store *inbox.Store, recorder SupplementRecorder) (*CustomerSupplementConsumer, error) {
	if recorder == nil {
		return nil, fmt.Errorf("warehouse fulfillment inbox: supplement recorder is nil")
	}
	gate, err := inboxconsume.New(inboxconsume.Spec[CustomerSupplementNotice]{
		Transactor: transactor, Store: store, Name: customerSupplementConsumerName,
		EventType: CustomerSupplementRequiredEventType, Decode: decodeCustomerSupplement,
		Handle: recordSupplement(recorder), UnexpectedType: "warehouse fulfillment inbox",
		HandleVerb: "record customer supplement",
	})
	if err != nil {
		return nil, err
	}
	return &CustomerSupplementConsumer{gate: gate}, nil
}

func (consumer *CustomerSupplementConsumer) Consume(ctx context.Context, envelope eventing.Envelope) error {
	return consumer.gate.Consume(ctx, envelope)
}

func recordSupplement(recorder SupplementRecorder) func(context.Context, CustomerSupplementNotice) error {
	return func(ctx context.Context, notice CustomerSupplementNotice) error {
		tenant, err := domain.NewTenantID(notice.TenantID)
		if err != nil {
			return fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
		}
		result, err := recorder.Record(ctx, application.SupplementCommand{
			Tenant: tenant, SubmissionVersion: notice.SubmissionVersion, Source: notice.Source, SourceRequestKey: notice.SourceRequestKey,
		})
		if err != nil {
			return err
		}
		switch result.Outcome() {
		case application.SupplementAwaiting, application.SupplementPosted, application.SupplementExisting, application.SupplementConflict:
			return nil
		default:
			return fmt.Errorf("%w: %s", ErrPoisonEnvelope, result.Outcome())
		}
	}
}

func decodeCustomerSupplement(payload []byte) (CustomerSupplementNotice, error) {
	var body struct {
		TenantID          string `json:"tenantId"`
		Source            string `json:"source"`
		SourceRequestKey  string `json:"sourceRequestKey"`
		SubmissionVersion string `json:"submissionVersion"`
	}
	if err := json.Unmarshal(payload, &body); err != nil {
		return CustomerSupplementNotice{}, fmt.Errorf("%w: %v", ErrPoisonEnvelope, err)
	}
	if body.TenantID == "" || body.Source == "" || body.SourceRequestKey == "" || body.SubmissionVersion == "" {
		return CustomerSupplementNotice{}, fmt.Errorf("%w: missing customer supplement fields", ErrPoisonEnvelope)
	}
	return CustomerSupplementNotice{
		TenantID: body.TenantID, Source: body.Source, SourceRequestKey: body.SourceRequestKey, SubmissionVersion: body.SubmissionVersion,
	}, nil
}

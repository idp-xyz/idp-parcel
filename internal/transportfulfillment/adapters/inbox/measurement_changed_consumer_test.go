package tfinbox_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
)

type changedMeasurementHandler struct {
	calls []tfinbox.ChangedMeasurement
}

func (handler *changedMeasurementHandler) HandleChangedMeasurement(_ context.Context, changed tfinbox.ChangedMeasurement) error {
	handler.calls = append(handler.calls, changed)
	return nil
}

func measurementConsumer(t *testing.T) (*tfinbox.MeasurementChangedConsumer, *changedMeasurementHandler) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	store, err := inbox.NewStore(db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	handler := &changedMeasurementHandler{}
	consumer, err := tfinbox.NewMeasurementChangedConsumer(db.Transactor(), store, handler)
	if err != nil {
		t.Fatalf("构造消费者：%v", err)
	}
	return consumer, handler
}

func measurementEnvelope(eventID, payload string) eventing.Envelope {
	now := time.Date(2026, 10, 21, 2, 0, 0, 0, time.UTC)
	return eventing.Envelope{
		SpecVersion:  eventing.SpecVersion,
		ID:           eventing.EventID(eventID),
		Source:       "idp-parcel/node-operations",
		Type:         tfinbox.ActualMeasurementChangedEventType,
		Version:      1,
		Scope:        "SYN-TENANT-01",
		Subject:      "SYN-MEASURE-1",
		PartitionKey: "SYN-TENANT-01/measurement/SYN-HU-1",
		OccurredAt:   now,
		RecordedAt:   now,
		ContentType:  eventing.JSONContentType,
		Payload:      json.RawMessage(payload),
	}
}

// Covers: 四个字段都在才交给处理方。信封上没有业务时间也不毒：本路用实测自己的发生时刻。
func TestAMeasurementChangeCarriesItsFourFields(t *testing.T) {
	consumer, handler := measurementConsumer(t)
	envelope := measurementEnvelope("measure-1",
		`{"tenantId":"SYN-TENANT-01","sourceId":"SYN-MEASURE-1","objectKind":"HANDLING_UNIT","objectId":"SYN-HU-1"}`)
	envelope.OccurredAt = time.Time{}
	if err := consumer.Consume(t.Context(), envelope); err != nil {
		t.Fatalf("投递：%v", err)
	}
	want := tfinbox.ChangedMeasurement{
		TenantID: "SYN-TENANT-01", SourceID: "SYN-MEASURE-1", ObjectKind: "HANDLING_UNIT", ObjectID: "SYN-HU-1",
	}
	if len(handler.calls) != 1 || handler.calls[0] != want {
		t.Fatalf("处理 = %+v", handler.calls)
	}
}

// Covers: 四个字段各缺一个都是毒信封，入账不交处理方。
func TestAMeasurementChangeMissingAnyFieldIsPoison(t *testing.T) {
	consumer, handler := measurementConsumer(t)
	payloads := []string{
		`{"sourceId":"SYN-MEASURE-1","objectKind":"HANDLING_UNIT","objectId":"SYN-HU-1"}`,
		`{"tenantId":"SYN-TENANT-01","objectKind":"HANDLING_UNIT","objectId":"SYN-HU-1"}`,
		`{"tenantId":"SYN-TENANT-01","sourceId":"SYN-MEASURE-1","objectId":"SYN-HU-1"}`,
		`{"tenantId":"SYN-TENANT-01","sourceId":"SYN-MEASURE-1","objectKind":"HANDLING_UNIT"}`,
	}
	for index, payload := range payloads {
		if err := consumer.Consume(t.Context(), measurementEnvelope("poison-"+string(rune('a'+index)), payload)); err != nil {
			t.Fatalf("缺字段 %d 应入账不报错：%v", index, err)
		}
	}
	if len(handler.calls) != 0 {
		t.Fatalf("毒信封交给了处理方：%+v", handler.calls)
	}
}

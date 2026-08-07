package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func TestOpaqueValuesRejectBlankInputWithoutNormalizing(t *testing.T) {
	tests := []struct {
		name string
		new  func(string) (string, error)
	}{
		{"tenant ID", func(value string) (string, error) { got, err := domain.NewTenantID(value); return got.String(), err }},
		{"customer account ID", func(value string) (string, error) {
			got, err := domain.NewCustomerAccountID(value)
			return got.String(), err
		}},
		{"source", func(value string) (string, error) { got, err := domain.NewSource(value); return got.String(), err }},
		{"source request key", func(value string) (string, error) {
			got, err := domain.NewSourceRequestKey(value)
			return got.String(), err
		}},
		{"payload digest", func(value string) (string, error) {
			got, err := domain.NewPayloadDigest(value)
			return got.String(), err
		}},
		{"submission batch ID", func(value string) (string, error) {
			got, err := domain.NewSubmissionBatchID(value)
			return got.String(), err
		}},
		{"shipment request ID", func(value string) (string, error) {
			got, err := domain.NewShipmentRequestID(value)
			return got.String(), err
		}},
		{"declared parcel ID", func(value string) (string, error) {
			got, err := domain.NewDeclaredParcelID(value)
			return got.String(), err
		}},
	}

	for _, test := range tests {
		t.Run(test.name+" rejects blank", func(t *testing.T) {
			_, err := test.new(" \t ")
			if !errors.Is(err, domain.ErrBlankValue) {
				t.Fatalf("error = %v, want blank value", err)
			}
		})
		t.Run(test.name+" remains opaque", func(t *testing.T) {
			const value = "  opaque/value  "
			got, err := test.new(value)
			if err != nil {
				t.Fatalf("new value: %v", err)
			}
			if got != value {
				t.Fatalf("value = %q, want exact %q", got, value)
			}
		})
	}
}

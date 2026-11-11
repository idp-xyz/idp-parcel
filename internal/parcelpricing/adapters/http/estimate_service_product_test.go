package pricinghttp_test

import (
	"errors"
	"strings"
	"testing"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 试算载荷的 serviceProduct（ADR-0378 决定二、三）：可缺，缺席即只看不声明产品的卡；有值必须立得成服务产品引用，立不住是畸形
// 请求（400），不是零值。
func TestEstimatePayloadTranslatesTheOptionalServiceProduct(t *testing.T) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	const head = `{"scope":"SYN-SCOPE-01","direction":"SELL","basisAt":"2026-06-01T08:00:00Z","weight":{"value":"1","unit":"KG"}`

	payload, err := pricinghttp.DecodeEstimatePayload(strings.NewReader(head + `,"serviceProduct":"SYN-PROD-CN-SG-FBA"}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	command, err := payload.Command(tenant)
	if err != nil || command.ServiceProduct.String() != "SYN-PROD-CN-SG-FBA" {
		t.Fatalf("command = %+v err=%v", command, err)
	}

	absent, err := pricinghttp.DecodeEstimatePayload(strings.NewReader(head + `}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if command, err := absent.Command(tenant); err != nil || command.ServiceProduct.Declared() {
		t.Fatalf("没带产品要留空：%+v err=%v", command, err)
	}

	padded, err := pricinghttp.DecodeEstimatePayload(strings.NewReader(head + `,"serviceProduct":" SYN-PROD-CN-SG-FBA"}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, err := padded.Command(tenant); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
		t.Fatalf("带首尾空白的产品：err = %v, want ErrMalformedRequest", err)
	}
}

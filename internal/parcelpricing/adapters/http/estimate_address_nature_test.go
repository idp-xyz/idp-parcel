package pricinghttp_test

import (
	"errors"
	"strings"
	"testing"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 试算载荷的 addressNature（ADR-0347 决定四）：可缺，缺席即未声明；有值必须是封闭词 COMMERCIAL / RESIDENTIAL，
// 不认识的值是畸形请求（400），不是默认值，也不是业务答案。键名不是 addressType——计价引擎里「地址类型」那一格承载偏远档位。
func TestEstimatePayloadTranslatesTheOptionalAddressNature(t *testing.T) {
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	const head = `{"scope":"SYN-SCOPE-01","direction":"SELL","basisAt":"2026-06-01T08:00:00Z","weight":{"value":"1","unit":"KG"}`

	for nature, want := range map[string]domain.AddressNature{
		"COMMERCIAL":  domain.AddressNatureCommercial,
		"RESIDENTIAL": domain.AddressNatureResidential,
	} {
		payload, err := pricinghttp.DecodeEstimatePayload(strings.NewReader(head + `,"addressNature":"` + nature + `"}`))
		if err != nil {
			t.Fatalf("decode %s: %v", nature, err)
		}
		command, err := payload.Command(tenant)
		if err != nil || command.AddressNature == nil || *command.AddressNature != want {
			t.Fatalf("command for %s = %+v err=%v", nature, command, err)
		}
	}

	absent, err := pricinghttp.DecodeEstimatePayload(strings.NewReader(head + `}`))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	command, err := absent.Command(tenant)
	if err != nil || command.AddressNature != nil {
		t.Fatalf("an absent nature must stay undeclared: %+v err=%v", command, err)
	}

	for name, raw := range map[string]string{
		"小写":   head + `,"addressNature":"residential"}`,
		"拼错":   head + `,"addressNature":"RESIDENTAL"}`,
		"不在词表": head + `,"addressNature":"MIXED"}`,
	} {
		t.Run(name, func(t *testing.T) {
			payload, err := pricinghttp.DecodeEstimatePayload(strings.NewReader(raw))
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if _, err := payload.Command(tenant); !errors.Is(err, pricinghttp.ErrMalformedRequest) {
				t.Fatalf("err = %v, want ErrMalformedRequest", err)
			}
		})
	}

	if _, err := pricinghttp.DecodeEstimatePayload(strings.NewReader(head + `,"addressType":"RESIDENTIAL"}`)); err == nil {
		t.Fatal("an addressType key was accepted; the wire key for the nature is addressNature")
	}
}

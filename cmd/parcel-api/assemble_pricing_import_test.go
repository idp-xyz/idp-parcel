package main

import (
	"bytes"
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/pricecardtemplate"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 装配点接的是真读口：空白模板读得出模板版本、读不出方案身份，答未受理并带那一条问题——替身
// 读口不会知道模板长什么样。
func TestAssembledPriceCardPreviewReadsTheRealTemplate(t *testing.T) {
	preview, err := buildPriceCardImportPreview()
	if err != nil {
		t.Fatal(err)
	}
	var blank bytes.Buffer
	if err := pricecardtemplate.WriteTemplate(&blank); err != nil {
		t.Fatal(err)
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	answer, err := preview.Handle(context.Background(), pricingapp.PreviewPriceCardImportCommand{
		Tenant: tenant, FileName: pricecardtemplate.TemplateFileName, Raw: blank.Bytes(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if answer.Outcome != pricingapp.PriceCardImportNotAccepted || answer.TemplateVersion != pricecardtemplate.TemplateVersion ||
		len(answer.Problems) != 1 || answer.Problems[0].Code != ports.TemplatePlanIdentityUnreadable {
		t.Fatalf("answer = %+v", answer)
	}
}

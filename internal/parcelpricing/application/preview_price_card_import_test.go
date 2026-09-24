package application_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

// templateReaderDouble 按给定读法作答，并记下读到的字节。
type templateReaderDouble struct {
	reading ports.PriceCardTemplateReading
	read    []byte
}

func (double *templateReaderDouble) ReadPriceCardTemplate(raw []byte) ports.PriceCardTemplateReading {
	double.read = raw
	return double.reading
}

func previewHandler(t *testing.T, reader ports.PriceCardTemplateReader) *application.PreviewPriceCardImportHandler {
	t.Helper()
	handler, err := application.NewPreviewPriceCardImportHandler(reader)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func previewTenant(t *testing.T) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	return tenant
}

// 三种读法各对一格答复；源文件身份按上传字节由服务端算出，读口读到的就是那串字节。
func TestPreviewAnswersEachReadingAndComputesTheSourceIdentity(t *testing.T) {
	plan := pricingPlanForPreview(t)
	content := &ports.PriceCardTemplateContent{Plan: plan}
	problem := ports.PriceCardTemplateProblem{Sheet: "tables", Row: 2, Column: "currency", Code: ports.TemplateCellInvalid, Message: "x"}
	cases := map[string]struct {
		reading ports.PriceCardTemplateReading
		want    application.PreviewPriceCardImportOutcome
	}{
		"已校验": {ports.PriceCardTemplateReading{TemplateVersion: "PPT-1", Accepted: true, Plan: plan.Reference(), Content: content},
			application.PriceCardImportValidated},
		"带问题": {ports.PriceCardTemplateReading{TemplateVersion: "PPT-1", Accepted: true, Plan: plan.Reference(),
			Problems: []ports.PriceCardTemplateProblem{problem}}, application.PriceCardImportHasProblems},
		"整份不收": {ports.PriceCardTemplateReading{TemplateVersion: "PPT-1",
			Problems: []ports.PriceCardTemplateProblem{{Code: ports.TemplateFileNotWorkbook, Message: "x"}}}, application.PriceCardImportNotAccepted},
	}
	raw := []byte("workbook bytes")
	digest := sha256.Sum256(raw)
	for name, testCase := range cases {
		reader := &templateReaderDouble{reading: testCase.reading}
		preview, err := previewHandler(t, reader).Handle(context.Background(), application.PreviewPriceCardImportCommand{
			Tenant: previewTenant(t), FileName: "card.xlsx", Raw: raw,
		})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if preview.Outcome != testCase.want || preview.Outcome.String() == "" {
			t.Errorf("%s: outcome = %v", name, preview.Outcome)
		}
		if preview.SourceFile.Name() != "card.xlsx" || preview.SourceFile.SHA256() != hex.EncodeToString(digest[:]) {
			t.Errorf("%s: source = %s %s", name, preview.SourceFile.Name(), preview.SourceFile.SHA256())
		}
		if string(reader.read) != string(raw) || preview.TemplateVersion != "PPT-1" || len(preview.Problems) != len(testCase.reading.Problems) {
			t.Errorf("%s: preview = %+v", name, preview)
		}
		if (preview.Content != nil) != (testCase.want == application.PriceCardImportValidated) {
			t.Errorf("%s: content present = %v", name, preview.Content != nil)
		}
		if testCase.want != application.PriceCardImportNotAccepted && !preview.Plan.SameIdentity(plan.Reference()) {
			t.Errorf("%s: plan = %v", name, preview.Plan)
		}
	}
}

// 命令立不住时不读文件：缺租户、文件名不合格都答未受理。
func TestPreviewRefusesACommandWithoutTenantOrFileName(t *testing.T) {
	for name, command := range map[string]application.PreviewPriceCardImportCommand{
		"缺租户":    {FileName: "card.xlsx", Raw: []byte("x")},
		"文件名为空":  {Tenant: previewTenant(t), Raw: []byte("x")},
		"文件名带空白": {Tenant: previewTenant(t), FileName: " card.xlsx", Raw: []byte("x")},
	} {
		reader := &templateReaderDouble{}
		preview, err := previewHandler(t, reader).Handle(context.Background(), command)
		if err != nil || preview.Outcome != application.PriceCardImportNotAccepted || reader.read != nil {
			t.Errorf("%s: outcome=%v err=%v read=%v", name, preview.Outcome, err, reader.read != nil)
		}
	}
}

func TestPreviewHandlerNeedsAReader(t *testing.T) {
	if _, err := application.NewPreviewPriceCardImportHandler(nil); err == nil {
		t.Fatal("nil reader accepted")
	}
}

func pricingPlanForPreview(t *testing.T) domain.PricingPlanVersion {
	t.Helper()
	return pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, domain.ArtifactPricingPlan, "SYN-PLAN-PREVIEW", "v1"),
		TableReference:        pptest.IdentityReference(t, domain.ArtifactRateTable, "SYN-TABLE-PREVIEW", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, domain.ArtifactWeightPolicy, "SYN-WEIGHT-PREVIEW", "v1"),
		Scope:                 "SYN-SCOPE-01",
		Direction:             domain.PricingDirectionBuy,
		Purpose:               domain.PricingPurposeSupplierCost,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "CNY",
		Period:                pptest.Period{StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)},
		RateEntryID:           "SYN-RATE-PREVIEW",
		RateZone:              "Z1",
		MinimumKilograms:      "0",
		MaximumKilograms:      "30",
		RateAmount:            "30",
		WeightRounding:        domain.RoundingCeiling,
		WeightStepKilograms:   "0.5",
	})
}

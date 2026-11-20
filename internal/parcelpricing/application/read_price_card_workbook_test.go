package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

func TestWorkbookReadNeedsItsDependencies(t *testing.T) {
	registered, drafts, projector := &registeredCardDouble{}, &draftByReferenceDouble{}, &workbookProjectorDouble{}
	for _, build := range []func() error{
		func() error {
			_, err := application.NewReadPriceCardWorkbookHandler(nil, drafts, projector)
			return err
		},
		func() error {
			_, err := application.NewReadPriceCardWorkbookHandler(registered, nil, projector)
			return err
		},
		func() error {
			_, err := application.NewReadPriceCardWorkbookHandler(registered, drafts, nil)
			return err
		},
	} {
		if build() == nil {
			t.Fatal("依赖缺席也立起来了")
		}
	}
}

func TestWorkbookReadPrefersTheRegisteredCard(t *testing.T) {
	plan := workbookPlan(t)
	auth := pptest.IdentityReference(t, domain.ArtifactCommercialAuthorization, "SYN-AUTH-WB", "v1")
	registered := &registeredCardDouble{subject: ports.PriceCardWorkbookSubject{Plan: plan, DirectionAuthorization: auth}, found: true}
	drafts := &draftByReferenceDouble{draft: workbookDraft(t, plan, auth, false)}
	projector := &workbookProjectorDouble{body: []byte(`{"templateVersion":"PPT-1"}`)}
	handler := workbookHandler(t, registered, drafts, projector)

	result, err := handler.Handle(context.Background(), workbookQuery(t, ports.PriceCardWorkbookSpreadsheet))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != application.PriceCardWorkbookReady || result.Origin != application.PriceCardWorkbookFromRegister {
		t.Fatalf("outcome %s origin %s", result.Outcome, result.Origin)
	}
	if result.ContentDigest != plan.ContentDigest() || result.Canonicalization != plan.CanonicalizationVersion() {
		t.Fatalf("摘要 %s %s", result.ContentDigest, result.Canonicalization)
	}
	if drafts.calls != 0 {
		t.Fatal("已登记的卡不该再去翻草稿")
	}
	if projector.carrier != ports.PriceCardWorkbookSpreadsheet || string(result.Body) != string(projector.body) {
		t.Fatalf("载体 %s 正文 %q", projector.carrier, result.Body)
	}
}

func TestWorkbookReadUsesADraftWhenNothingIsRegistered(t *testing.T) {
	plan := workbookPlan(t)
	auth := pptest.IdentityReference(t, domain.ArtifactCommercialAuthorization, "SYN-AUTH-WB", "v1")
	drafts := &draftByReferenceDouble{draft: workbookDraft(t, plan, auth, false), found: true}
	projector := &workbookProjectorDouble{body: []byte(`{"ok":true}`)}
	handler := workbookHandler(t, &registeredCardDouble{}, drafts, projector)

	result, err := handler.Handle(context.Background(), workbookQuery(t, ports.PriceCardWorkbookStructured))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != application.PriceCardWorkbookReady || result.Origin != application.PriceCardWorkbookFromDraft {
		t.Fatalf("outcome %s origin %s", result.Outcome, result.Origin)
	}
	if result.ContentDigest != plan.ContentDigest() {
		t.Fatalf("摘要 %s", result.ContentDigest)
	}
}

func TestWorkbookReadNamesADraftThatOnlyHasProblems(t *testing.T) {
	plan := workbookPlan(t)
	auth := pptest.IdentityReference(t, domain.ArtifactCommercialAuthorization, "SYN-AUTH-WB", "v1")
	drafts := &draftByReferenceDouble{draft: workbookDraft(t, plan, auth, true), found: true}
	projector := &workbookProjectorDouble{}
	handler := workbookHandler(t, &registeredCardDouble{}, drafts, projector)

	result, err := handler.Handle(context.Background(), workbookQuery(t, ports.PriceCardWorkbookStructured))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != application.PriceCardWorkbookDraftWithoutContent || result.Where != "tables.currency" {
		t.Fatalf("outcome %s where %s conclusion %s", result.Outcome, result.Where, result.Conclusion)
	}
	if !strings.Contains(result.Conclusion, "问题清单") || projector.calls != 0 || len(result.Body) != 0 {
		t.Fatalf("不该编出工作簿：%s calls=%d", result.Conclusion, projector.calls)
	}
}

func TestWorkbookReadNamesWhatTheTemplateCannotExpress(t *testing.T) {
	plan := workbookPlan(t)
	auth := pptest.IdentityReference(t, domain.ArtifactCommercialAuthorization, "SYN-AUTH-WB", "v1")
	projector := &workbookProjectorDouble{gap: ports.PriceCardWorkbookGap{Where: "rate-table SYN-TABLE-PREVIEW/v1", Conclusion: "版本引用带指纹"}}
	handler := workbookHandler(t, &registeredCardDouble{
		subject: ports.PriceCardWorkbookSubject{Plan: plan, DirectionAuthorization: auth}, found: true,
	}, &draftByReferenceDouble{}, projector)

	result, err := handler.Handle(context.Background(), workbookQuery(t, ports.PriceCardWorkbookStructured))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != application.PriceCardWorkbookNotRepresentable || result.Where != projector.gap.Where || len(result.Body) != 0 {
		t.Fatalf("outcome %s where %s body %d", result.Outcome, result.Where, len(result.Body))
	}
}

func TestWorkbookReadReportsCanonicalizationItCannotRebuild(t *testing.T) {
	registered := &registeredCardDouble{err: fmtWrap(domain.ErrCanonicalizationVersionUnsupported)}
	handler := workbookHandler(t, registered, &draftByReferenceDouble{}, &workbookProjectorDouble{})
	result, err := handler.Handle(context.Background(), workbookQuery(t, ports.PriceCardWorkbookStructured))
	if err != nil || result.Outcome != application.PriceCardWorkbookCanonicalizationUnsupported {
		t.Fatalf("err %v outcome %s", err, result.Outcome)
	}
}

func TestWorkbookReadAnswersNotFoundWhenBothBooksAreEmpty(t *testing.T) {
	handler := workbookHandler(t, &registeredCardDouble{}, &draftByReferenceDouble{}, &workbookProjectorDouble{})
	result, err := handler.Handle(context.Background(), workbookQuery(t, ports.PriceCardWorkbookStructured))
	if err != nil || result.Outcome != application.PriceCardWorkbookNotFound {
		t.Fatalf("err %v outcome %s", err, result.Outcome)
	}
}

type registeredCardDouble struct {
	subject ports.PriceCardWorkbookSubject
	found   bool
	err     error
}

func (double *registeredCardDouble) ReadRegisteredPriceCard(
	context.Context, domain.TenantID, domain.VersionReference,
) (ports.PriceCardWorkbookSubject, bool, error) {
	if double.err != nil {
		return ports.PriceCardWorkbookSubject{}, false, double.err
	}
	return double.subject, double.found, nil
}

type draftByReferenceDouble struct {
	draft domain.PriceCardDraft
	found bool
	calls int
}

func (double *draftByReferenceDouble) LoadPriceCardDraft(
	context.Context, domain.TenantID, domain.VersionReference,
) (domain.PriceCardDraft, bool, error) {
	double.calls++
	return double.draft, double.found, nil
}

type workbookProjectorDouble struct {
	body    []byte
	gap     ports.PriceCardWorkbookGap
	carrier ports.PriceCardWorkbookCarrier
	calls   int
}

func (double *workbookProjectorDouble) ProjectPriceCardWorkbook(
	_ domain.PricingPlanVersion,
	_ domain.VersionReference,
	carrier ports.PriceCardWorkbookCarrier,
) (ports.PriceCardWorkbookProjection, error) {
	double.calls++
	double.carrier = carrier
	if double.gap.Where != "" {
		return ports.PriceCardWorkbookProjection{Gap: double.gap}, nil
	}
	return ports.PriceCardWorkbookProjection{Document: ports.PriceCardWorkbookDocument{
		TemplateVersion: "PPT-1", Body: double.body, MediaType: "application/json", FileName: "price-card.xlsx",
	}}, nil
}

func workbookHandler(
	t *testing.T,
	registered *registeredCardDouble,
	drafts *draftByReferenceDouble,
	projector *workbookProjectorDouble,
) *application.ReadPriceCardWorkbookHandler {
	t.Helper()
	handler, err := application.NewReadPriceCardWorkbookHandler(registered, drafts, projector)
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

func workbookQuery(t *testing.T, carrier ports.PriceCardWorkbookCarrier) application.ReadPriceCardWorkbookQuery {
	t.Helper()
	return application.ReadPriceCardWorkbookQuery{
		Tenant:  pptest.Value(t, domain.NewTenantID, "SYN-TENANT-01"),
		Plan:    pptest.IdentityReference(t, domain.ArtifactPricingPlan, "SYN-PLAN-PREVIEW", "v1"),
		Carrier: carrier,
	}
}

func workbookPlan(t *testing.T) domain.PricingPlanVersion {
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

func workbookDraft(t *testing.T, plan domain.PricingPlanVersion, auth domain.VersionReference, problems bool) domain.PriceCardDraft {
	t.Helper()
	submission := domain.PriceCardDraftSubmission{
		Tenant: pptest.Value(t, domain.NewTenantID, "SYN-TENANT-01"),
		Plan:   plan.Reference(),
		Source: pptest.Value(t, func(name string) (domain.SourceFileIdentity, error) {
			return domain.NewSourceFileIdentity(name, strings.Repeat("ab", 32))
		}, "SYN-CARD.xlsx"),
		Submitter:   "https://id.syn.example/dex#SYN-OPERATOR-01",
		SubmittedAt: time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	}
	if problems {
		submission.Problems = []domain.PriceCardDraftProblem{
			pptest.Value(t, func(message string) (domain.PriceCardDraftProblem, error) {
				return domain.NewPriceCardDraftProblem("tables", 2, "currency", "CELL_INVALID", message)
			}, "币种立不住"),
		}
	} else {
		submission.Content = &domain.PriceCardDraftContent{Plan: plan, DirectionAuthorization: auth}
	}
	draft, err := domain.SubmitPriceCardDraft(submission)
	if err != nil {
		t.Fatal(err)
	}
	return draft
}

func fmtWrap(err error) error { return errors.Join(errors.New("read registered price card"), err) }

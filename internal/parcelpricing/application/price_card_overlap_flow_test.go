package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

// 预览、存草稿、批准、发布只把同时适用的在册版本放进答复。重叠不改那一格结果：校验通过仍是校验通过，
// 批准仍是批准，发布仍是已发布。查找口没装时这一格是空的，装上之后按候选卡的范围、方向、目的与适用期去问。

type overlapFinderFake struct {
	overlaps  []domain.PriceCardOverlap
	err       error
	calls     int
	tenant    domain.TenantID
	plan      domain.VersionReference
	scope     domain.PricingScopeID
	direction domain.PricingDirection
	purpose   domain.PricingPurpose
	period    domain.EffectivePeriod
	products  int
}

func (fake *overlapFinderFake) FindOverlappingInForce(
	_ context.Context,
	tenant domain.TenantID,
	plan domain.VersionReference,
	scope domain.PricingScopeID,
	direction domain.PricingDirection,
	purpose domain.PricingPurpose,
	period domain.EffectivePeriod,
	products []domain.ServiceProductReference,
) ([]domain.PriceCardOverlap, error) {
	fake.calls++
	fake.tenant, fake.plan, fake.scope = tenant, plan, scope
	fake.direction, fake.purpose, fake.period, fake.products = direction, purpose, period, len(products)
	if fake.err != nil {
		return nil, fake.err
	}
	if fake.overlaps == nil {
		return []domain.PriceCardOverlap{}, nil
	}
	return fake.overlaps, nil
}

func successorCutOverlap(t *testing.T, registered domain.VersionReference) domain.PriceCardOverlap {
	t.Helper()
	january := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	june := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	older, err := domain.NewEffectivePeriod(january, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	newer, err := domain.NewEffectivePeriod(june, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	candidate, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, registered.ID(), "v2")
	if err != nil {
		t.Fatal(err)
	}
	overlap, ok := domain.ClassifyPriceCardOverlap(candidate, newer, nil, registered, older, nil)
	if !ok || overlap.Resolution() != domain.PriceCardOverlapSuccessorCut {
		t.Fatalf("夹具重叠没分成后继截止：ok=%v %s", ok, overlap.Resolution())
	}
	return overlap
}

func TestPreviewKeepsValidatedAndReportsTheRegisteredOverlap(t *testing.T) {
	plan := pricingPlanForPreview(t)
	registered, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, plan.Reference().ID(), "v0")
	if err != nil {
		t.Fatal(err)
	}
	overlap := successorCutOverlap(t, registered)
	finder := &overlapFinderFake{overlaps: []domain.PriceCardOverlap{overlap}}
	handler, err := application.NewPreviewPriceCardImportHandler(
		&templateReaderDouble{reading: validatedReading(t, "30")},
		application.WithPriceCardOverlapFinder(finder),
	)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := handler.Handle(context.Background(), application.PreviewPriceCardImportCommand{
		Tenant: previewTenant(t), FileName: "card.xlsx", Raw: []byte("card bytes"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if preview.Outcome != application.PriceCardImportValidated || preview.Outcome.String() != "VALIDATED" {
		t.Fatalf("重叠不该改预览结果：%s", preview.Outcome)
	}
	if finder.calls != 1 || finder.tenant != previewTenant(t) || !finder.plan.SameIdentity(plan.Reference()) ||
		finder.scope != plan.Scope() || finder.direction != plan.Direction() || finder.purpose != plan.Purpose() ||
		!finder.period.StartsAt().Equal(plan.EffectivePeriod().StartsAt()) || finder.products != len(plan.ServiceProducts()) {
		t.Fatalf("查找口没按这张卡去问：calls=%d plan=%s/%s scope=%s", finder.calls, finder.plan.ID(), finder.plan.Version(), finder.scope)
	}
	if len(preview.Overlaps) != 1 || preview.Overlaps[0].Resolution() != domain.PriceCardOverlapSuccessorCut ||
		!preview.Overlaps[0].Plan().SameIdentity(registered) {
		t.Fatalf("预览没带回在册的那一版：%d", len(preview.Overlaps))
	}
}

func TestPreviewDoesNotAskAboutAReadingThatIsNotValidated(t *testing.T) {
	finder := &overlapFinderFake{}
	handler, err := application.NewPreviewPriceCardImportHandler(
		&templateReaderDouble{reading: problemReading(t, ports.PriceCardTemplateProblem{
			Sheet: "tables", Row: 2, Column: "currency", Code: ports.TemplateCellInvalid, Message: "币种立不住",
		})},
		application.WithPriceCardOverlapFinder(finder),
	)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := handler.Handle(context.Background(), application.PreviewPriceCardImportCommand{
		Tenant: previewTenant(t), FileName: "card.xlsx", Raw: []byte("x"),
	})
	if err != nil || preview.Outcome != application.PriceCardImportHasProblems || finder.calls != 0 || len(preview.Overlaps) != 0 {
		t.Fatalf("带问题仍去问了册：outcome=%s calls=%d err=%v", preview.Outcome, finder.calls, err)
	}
}

func TestPreviewSurfacesAFinderFailureInsteadOfABusinessAnswer(t *testing.T) {
	broken := errors.New("价卡册读不动")
	handler, err := application.NewPreviewPriceCardImportHandler(
		&templateReaderDouble{reading: validatedReading(t, "30")},
		application.WithPriceCardOverlapFinder(&overlapFinderFake{err: broken}),
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = handler.Handle(context.Background(), application.PreviewPriceCardImportCommand{
		Tenant: previewTenant(t), FileName: "card.xlsx", Raw: []byte("x"),
	})
	if !errors.Is(err, broken) {
		t.Fatalf("查找失败应上抛，err=%v", err)
	}
}

func TestSubmittingAValidatedDraftKeepsTheLandingAndReportsTheOverlap(t *testing.T) {
	plan := pricingPlanForPreview(t)
	registered, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, plan.Reference().ID(), "v0")
	if err != nil {
		t.Fatal(err)
	}
	finder := &overlapFinderFake{overlaps: []domain.PriceCardOverlap{successorCutOverlap(t, registered)}}
	register := newDraftRegisterFake()
	handler, err := application.NewSubmitPriceCardDraftHandler(
		&templateReaderDouble{reading: validatedReading(t, "30")}, register, fixedClock{at: draftClockAt},
		application.WithPriceCardOverlapFinder(finder),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := handler.Handle(context.Background(), submitCommand(t, "card bytes"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != application.PriceCardDraftSubmitted || result.Outcome.String() != "DRAFT_SUBMITTED" || !result.HasDraft {
		t.Fatalf("重叠不该改录入落点：%s", result.Outcome)
	}
	if len(result.Reading.Overlaps) != 1 || !result.Reading.Overlaps[0].SamePlan() || finder.calls != 1 || len(register.rows) != 1 {
		t.Fatalf("存草稿没带回重叠：overlaps=%d calls=%d rows=%d", len(result.Reading.Overlaps), finder.calls, len(register.rows))
	}
}

func TestApprovalKeepsApprovedAndReportsTheOverlap(t *testing.T) {
	validated := validatedPriceCardDraft(t)
	content, ok := validated.Content()
	if !ok {
		t.Fatal("已校验草稿没有内容")
	}
	registered, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, content.Plan.Reference().ID(), "v0")
	if err != nil {
		t.Fatal(err)
	}
	finder := &overlapFinderFake{overlaps: []domain.PriceCardOverlap{successorCutOverlap(t, registered)}}
	drafts := newDraftProgressFake(validated)
	handler, err := application.NewApprovePriceCardDraftHandler(
		drafts, configuredRule(t, true, ""), fixedClock{at: progressClockAt},
		application.WithPriceCardOverlapFinder(finder),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := handler.Handle(context.Background(), application.ApprovePriceCardDraftCommand{
		Tenant: draftTenant(t), Plan: validated.Plan(), Approver: subject(t, draftApprover),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != application.PriceCardDraftApproved || result.Outcome.String() != "DRAFT_APPROVED" || !result.HasDraft {
		t.Fatalf("重叠不该改批准结果：%s", result.Outcome)
	}
	if len(result.Overlaps) != 1 || result.Overlaps[0].Resolution().String() != "SUCCESSOR_CUT" || finder.calls != 1 {
		t.Fatalf("批准没带回重叠：%d calls=%d", len(result.Overlaps), finder.calls)
	}
}

func TestPublicationKeepsPublishedAndReportsTheOverlap(t *testing.T) {
	approved := approvedPriceCardDraft(t)
	content, _ := approved.Content()
	registered, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, content.Plan.Reference().ID(), "v0")
	if err != nil {
		t.Fatal(err)
	}
	finder := &overlapFinderFake{overlaps: []domain.PriceCardOverlap{successorCutOverlap(t, registered)}}
	drafts := newDraftProgressFake(approved)
	registrar := application.NewRegisterPriceCardHandler(application.RegisterPriceCardDeps{
		Catalog: &priceCardCatalogDouble{outcome: ports.PriceCardRegistered}, Registered: &pricingParameterDouble{},
	})
	handler, err := application.NewPublishPriceCardDraftHandler(
		drafts, registrar, fixedClock{at: progressClockAt.Add(time.Minute)},
		application.WithPriceCardOverlapFinder(finder),
	)
	if err != nil {
		t.Fatal(err)
	}
	result, err := handler.Handle(context.Background(), application.PublishPriceCardDraftCommand{
		Tenant: draftTenant(t), Plan: approved.Plan(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != application.PriceCardDraftPublished || result.Outcome.String() != "DRAFT_PUBLISHED" ||
		result.Registration != application.PriceCardRecorded || !result.HasDraft {
		t.Fatalf("重叠不该改发布结果：%s / %s", result.Outcome, result.Registration)
	}
	if len(result.Overlaps) != 1 || !result.Overlaps[0].Plan().SameIdentity(registered) || finder.calls != 1 {
		t.Fatalf("发布没带回重叠：%d calls=%d", len(result.Overlaps), finder.calls)
	}
}

func TestAnUnwiredOverlapFinderLeavesTheAnswersUntouched(t *testing.T) {
	preview, err := previewHandler(t, &templateReaderDouble{reading: validatedReading(t, "30")}).Handle(context.Background(), application.PreviewPriceCardImportCommand{
		Tenant: previewTenant(t), FileName: "card.xlsx", Raw: []byte("card bytes"),
	})
	if err != nil || preview.Outcome != application.PriceCardImportValidated || len(preview.Overlaps) != 0 {
		t.Fatalf("没装查找口的预览：%s overlaps=%d err=%v", preview.Outcome, len(preview.Overlaps), err)
	}
	direction := pptest.IdentityReference(t, domain.ArtifactCommercialAuthorization, "SYN-AUTH-COST-DIR-01", "v1")
	if direction.ID() == "" {
		t.Fatal("方向授权夹具立不住")
	}
}

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 生产装配把价卡册装成查找口：真库上预览、存草稿、批准都答出在册的那一版，发布仍是 201。
// 发布之后、收尾之前，交集内的适用冲突候选就是提示里的旧版加上刚发布的这一版。截止之后旧版不再出现。
// 输入是隔离合成，只记 S。

type fixedPublicationIntake struct {
	command pricingapp.PublishPriceCardDraftCommand
}

func (intake fixedPublicationIntake) IntakePriceCardDraftPublication(context.Context, *http.Request) (pricingapp.PublishPriceCardDraftCommand, error) {
	return intake.command, nil
}

func TestWiredFlowsReportOverlapAndPublicationStaysCreated(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	publishDirectionAuthorizationForPricingSeed(t, db, "price-card-cn-sg-full-03.json")
	preview, err := buildPriceCardImportPreview(db)
	if err != nil {
		t.Fatal(err)
	}
	submission, err := buildPriceCardDraftSubmission(db)
	if err != nil {
		t.Fatal(err)
	}
	approval, err := buildPriceCardDraftApproval(db)
	if err != nil {
		t.Fatal(err)
	}
	publication, err := buildPriceCardDraftPublication(db, true)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := pppostgres.NewPriceCards(db)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := pppostgres.NewPriceCardApprovalDutyRules(db)
	if err != nil {
		t.Fatal(err)
	}
	workbook, err := os.ReadFile(full03WorkbookJS)
	if err != nil {
		t.Fatal(err)
	}
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		t.Fatal(err)
	}
	command := pricingapp.PreviewPriceCardImportCommand{Tenant: tenant, FromForm: true, Raw: workbook}
	first, err := preview.Handle(t.Context(), command)
	if err != nil || first.Outcome != pricingapp.PriceCardImportValidated || first.Content == nil {
		t.Fatalf("空白册上的预览 = %s err=%v problems=%v", first.Outcome, err, first.Problems)
	}
	if len(first.Overlaps) != 0 {
		t.Fatalf("册上还没有卡，不该提示重叠：%d", len(first.Overlaps))
	}

	predecessor := predecessorCovering(t, first.Content.Plan)
	if err := registerPredecessor(db, tenant, cards, predecessor); err != nil {
		t.Fatalf("登记旧版：%v", err)
	}

	again, err := preview.Handle(t.Context(), command)
	if err != nil || again.Outcome != pricingapp.PriceCardImportValidated || !overlapIsSuccessorCut(again.Overlaps, predecessor, first.Content.Plan) {
		t.Fatalf("预览应仍是已校验，并提示旧版后继截止：%s overlaps=%d err=%v", again.Outcome, len(again.Overlaps), err)
	}

	submitted, err := submission.Handle(t.Context(), pricingapp.SubmitPriceCardDraftCommand{
		Tenant: tenant, Submitter: "https://id.syn.example/dex#SYN-OPERATOR-01", FromForm: true, Raw: workbook,
	})
	if err != nil || submitted.Outcome != pricingapp.PriceCardDraftSubmitted || !overlapIsSuccessorCut(submitted.Reading.Overlaps, predecessor, first.Content.Plan) {
		t.Fatalf("存草稿 = %s overlaps=%d err=%v", submitted.Outcome, len(submitted.Reading.Overlaps), err)
	}

	rule, err := domain.NewPriceCardApprovalDutyRule(tenant, true, domain.OperatorGrant{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, saveErr := rules.SavePriceCardApprovalDutyRule(ctx, rule)
		return saveErr
	}); err != nil {
		t.Fatalf("登规则：%v", err)
	}
	approver, err := domain.NewOperatorSubject("https://id.syn.example/dex#SYN-OPERATOR-02", nil)
	if err != nil {
		t.Fatal(err)
	}
	approved, err := approval.Handle(t.Context(), pricingapp.ApprovePriceCardDraftCommand{Tenant: tenant, Plan: submitted.Draft.Plan(), Approver: approver})
	if err != nil || approved.Outcome != pricingapp.PriceCardDraftApproved || !overlapIsSuccessorCut(approved.Overlaps, predecessor, first.Content.Plan) {
		t.Fatalf("批准 = %s overlaps=%d err=%v", approved.Outcome, len(approved.Overlaps), err)
	}

	recorder := httptest.NewRecorder()
	pricinghttp.NewPublishPriceCardDraftEndpoint(fixedPublicationIntake{command: pricingapp.PublishPriceCardDraftCommand{
		Tenant: tenant, Plan: submitted.Draft.Plan(),
	}}, publication).ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/pricing-price-card-draft-publications", nil))
	var body struct {
		Outcome      string `json:"outcome"`
		Registration string `json:"registration"`
		Overlaps     []struct {
			Plan struct {
				Version string `json:"version"`
			} `json:"plan"`
			SamePlan   bool   `json:"samePlan"`
			Resolution string `json:"resolution"`
		} `json:"overlaps"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("发布答复不是 JSON：%s", recorder.Body)
	}
	if recorder.Code != http.StatusCreated || body.Outcome != "DRAFT_PUBLISHED" || body.Registration != "RECORDED" ||
		len(body.Overlaps) != 1 || body.Overlaps[0].Plan.Version != "v0" || !body.Overlaps[0].SamePlan || body.Overlaps[0].Resolution != "SUCCESSOR_CUT" {
		t.Fatalf("发布仍应 201：%d %s", recorder.Code, recorder.Body)
	}

	at := first.Content.Plan.EffectivePeriod().StartsAt().Add(24 * time.Hour)
	var declared domain.ServiceProductReference
	if products := first.Content.Plan.ServiceProducts(); len(products) > 0 {
		declared = products[0]
	}
	resolution, err := cards.ResolveInForce(t.Context(), tenant, first.Content.Plan.Scope(), first.Content.Plan.Direction(), first.Content.Plan.Purpose(), at, declared)
	if err != nil {
		t.Fatal(err)
	}
	versions := map[string]bool{}
	for _, candidate := range resolution.Candidates {
		versions[candidate.Version()] = true
	}
	if resolution.Outcome != ports.PriceCardApplicabilityConflict || len(resolution.Candidates) != 2 || !versions["v0"] || !versions[first.Content.Plan.Reference().Version()] {
		t.Fatalf("收尾前的适用冲突应是旧版加刚发布的这一版：%s %v", resolution.Outcome, versions)
	}

	if err := cutPredecessor(db, cards, tenant, predecessor.Reference(), first.Content.Plan.Reference()); err != nil {
		t.Fatalf("截止旧版：%v", err)
	}
	left, err := cards.FindOverlappingInForce(
		t.Context(), tenant, first.Content.Plan.Reference(), first.Content.Plan.Scope(), first.Content.Plan.Direction(),
		first.Content.Plan.Purpose(), first.Content.Plan.EffectivePeriod(), first.Content.Plan.ServiceProducts(),
	)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range left {
		if item.Plan().Version() == "v0" {
			t.Fatal("旧版已截止到新版起点，不该再出现在重叠里")
		}
	}
}

func overlapIsSuccessorCut(overlaps []domain.PriceCardOverlap, predecessor, candidate domain.PricingPlanVersion) bool {
	if len(overlaps) != 1 {
		return false
	}
	item := overlaps[0]
	return item.SamePlan() && item.Resolution() == domain.PriceCardOverlapSuccessorCut &&
		item.Plan().SameIdentity(predecessor.Reference()) &&
		item.Overlap().StartsAt().Equal(candidate.EffectivePeriod().StartsAt()) && item.Overlap().EndsAt().IsZero()
}

func registerPredecessor(db *bentopg.DB, tenant domain.TenantID, cards *pppostgres.PriceCards, plan domain.PricingPlanVersion) error {
	source, err := domain.NewSourceFileIdentity("SYN-OVERLAP-V0.xlsx", "9edaf27ef93004e00f73a65471897f2cf7064d5d4df05014934ef7ac5861d33d")
	if err != nil {
		return err
	}
	grant, err := domain.NewVersionReferenceIdentity(domain.ArtifactCommercialAuthorization, "SYN-OVERLAP-GRANT", "v1")
	if err != nil {
		return err
	}
	registration, err := domain.NewPriceCardRegistration(tenant, plan, source, grant, "SYN-OVERLAP-GOV")
	if err != nil {
		return err
	}
	return db.Transactor().WithinTransaction(context.Background(), func(ctx context.Context) error {
		saved, regErr := cards.Register(ctx, registration)
		if regErr != nil {
			return regErr
		}
		if saved != ports.PriceCardRegistered {
			return fmt.Errorf("登记落点 %v", saved)
		}
		return nil
	})
}

func predecessorCovering(t *testing.T, candidate domain.PricingPlanVersion) domain.PricingPlanVersion {
	t.Helper()
	start := candidate.EffectivePeriod().StartsAt().AddDate(-1, 0, 0)
	period, err := domain.NewEffectivePeriod(start, candidate.EffectivePeriod().EndsAt())
	if err != nil {
		t.Fatal(err)
	}
	reference, err := domain.NewVersionReferenceIdentity(domain.ArtifactPricingPlan, candidate.Reference().ID(), "v0")
	if err != nil {
		t.Fatal(err)
	}
	currency := mustValue(t, domain.NewCurrency, "USD")
	amount, err := domain.NewMoneyFromString("10", currency)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := domain.NewRateEntry(
		mustValue(t, domain.NewRateEntryID, "SYN-OVERLAP-ENTRY"),
		"Z1", syntheticPricingWeight(t, "0"), syntheticPricingWeight(t, "10"), amount,
	)
	if err != nil {
		t.Fatal(err)
	}
	table, err := domain.NewRateTableVersion(
		syntheticPricingReference(t, domain.ArtifactRateTable, "SYN-OVERLAP-TABLE", "v0"),
		domain.RateTableFamilyWeightZone, currency, domain.WeightUnitKilogram, period, []domain.RateEntry{entry},
	)
	if err != nil {
		t.Fatal(err)
	}
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, syntheticPricingWeight(t, "0.5"))
	if err != nil {
		t.Fatal(err)
	}
	weight, err := domain.NewPricingWeightPolicy(
		syntheticPricingReference(t, domain.ArtifactWeightPolicy, "SYN-OVERLAP-WEIGHT", "v0"),
		domain.PricingWeightActualOnly, rounding, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := domain.NewPricingPlanVersion(
		reference, candidate.Scope(), candidate.Direction(), candidate.Purpose(), candidate.BaseChargeCode(),
		period, table, weight, nil, domain.PricingPlanStructures{},
	)
	if err != nil {
		t.Fatal(err)
	}
	if products := candidate.ServiceProducts(); len(products) > 0 {
		plan, err = plan.WithServiceProducts(products...)
		if err != nil {
			t.Fatal(err)
		}
	}
	return plan
}

func cutPredecessor(db *bentopg.DB, cards *pppostgres.PriceCards, tenant domain.TenantID, predecessor, successor domain.VersionReference) error {
	return db.Transactor().WithinTransaction(context.Background(), func(ctx context.Context) error {
		old, oldOK, err := cards.LoadPriceCardStanding(ctx, tenant, predecessor)
		if err != nil || !oldOK {
			return fmt.Errorf("读旧版 ok=%v: %w", oldOK, err)
		}
		next, nextOK, err := cards.LoadPriceCardStanding(ctx, tenant, successor)
		if err != nil || !nextOK {
			return fmt.Errorf("读后继 ok=%v: %w", nextOK, err)
		}
		reference, err := domain.NewPriceCardClosureReference("SYN-OVERLAP-CUT")
		if err != nil {
			return err
		}
		decision, replay, err := old.CutForSuccessor(next, reference, time.Now().UTC())
		if err != nil || replay {
			return fmt.Errorf("截止 replay=%v: %w", replay, err)
		}
		outcome, err := cards.SavePriceCardCut(ctx, decision)
		if err != nil || outcome != ports.PriceCardClosureSaved {
			return fmt.Errorf("落下截止 %v: %w", outcome, err)
		}
		return nil
	})
}

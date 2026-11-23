package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 发布依据门：要求接受而没带依据、带了对不上的依据，都不入册、草稿留在已批准。
// 放行时带了依据就记下那一条，没带就不记。依据是不透明引用，这里不解释它。

type publicationBasisDouble struct {
	standing ports.PublicationBasisStanding
	queries  []ports.PublicationBasisQuery
}

func (double *publicationBasisDouble) JudgePublicationBasis(_ context.Context, query ports.PublicationBasisQuery) (ports.PublicationBasisStanding, error) {
	double.queries = append(double.queries, query)
	return double.standing, nil
}

type publicationBasisRecordDouble struct {
	saved []string
}

func (double *publicationBasisRecordDouble) SavePublicationBasis(_ context.Context, _ domain.TenantID, planID, planVersion, basis string) error {
	double.saved = append(double.saved, planID+"/"+planVersion+"/"+basis)
	return nil
}

func TestThePublicationBasisGateDecidesBeforeTheRegister(t *testing.T) {
	approved := approvedPriceCardDraft(t)
	content, _ := approved.Content()
	publish := func(standing ports.PublicationBasisStanding, basis string) (application.PublishPriceCardDraftResult, *draftProgressFake, *priceCardCatalogDouble, *publicationBasisDouble, *publicationBasisRecordDouble) {
		t.Helper()
		drafts := newDraftProgressFake(approved)
		catalog := &priceCardCatalogDouble{outcome: ports.PriceCardRegistered}
		reader := &publicationBasisDouble{standing: standing}
		records := &publicationBasisRecordDouble{}
		registrar := application.NewRegisterPriceCardHandler(registerDeps(catalog, &pricingParameterDouble{}, false, nil))
		handler, err := application.NewPublishPriceCardDraftHandler(drafts, registrar, fixedClock{at: progressClockAt.Add(time.Minute)},
			application.WithPublicationBasis(reader, records))
		if err != nil {
			t.Fatalf("构造发布编排：%v", err)
		}
		result, err := handler.Handle(context.Background(), application.PublishPriceCardDraftCommand{
			Tenant: draftTenant(t), Plan: approved.Plan(), Basis: basis,
		})
		if err != nil {
			t.Fatalf("发布编排：%v", err)
		}
		return result, drafts, catalog, reader, records
	}

	for name, refusal := range map[string]struct {
		standing ports.PublicationBasisStanding
		basis    string
		outcome  string
	}{
		"要求接受而没带依据": {ports.PublicationBasisAcceptanceRequired, "", "ACCEPTANCE_REQUIRED"},
		"依据对不上":     {ports.PublicationBasisCitationRejected, "QTS-OTHER", "CITATION_REJECTED"},
	} {
		result, drafts, catalog, reader, records := publish(refusal.standing, refusal.basis)
		if result.Outcome.String() != refusal.outcome || result.HasRegistration || result.HasDraft {
			t.Fatalf("%s：outcome = %v registration=%v draft=%v", name, result.Outcome, result.HasRegistration, result.HasDraft)
		}
		if len(catalog.registered) != 0 || len(drafts.advanced) != 0 || len(records.saved) != 0 {
			t.Fatalf("%s：登记 %d 份，推进 %d 次，依据 %d 条，want 都是 0", name, len(catalog.registered), len(drafts.advanced), len(records.saved))
		}
		if len(reader.queries) != 1 || reader.queries[0].ContentDigest != content.Plan.ContentDigest() || reader.queries[0].Basis != refusal.basis {
			t.Fatalf("%s：门收到的问句 = %+v", name, reader.queries)
		}
	}

	result, drafts, catalog, _, records := publish(ports.PublicationBasisAllow, "QTS-1")
	if result.Outcome != application.PriceCardDraftPublished || len(catalog.registered) != 1 || len(drafts.advanced) != 1 {
		t.Fatalf("带依据放行：outcome = %v 登记 %d 推进 %d", result.Outcome, len(catalog.registered), len(drafts.advanced))
	}
	want := approved.Plan().ID() + "/" + approved.Plan().Version() + "/QTS-1"
	if len(records.saved) != 1 || records.saved[0] != want {
		t.Fatalf("记下的依据 = %v，want [%s]", records.saved, want)
	}

	result, _, catalog, _, records = publish(ports.PublicationBasisAllow, "")
	if result.Outcome != application.PriceCardDraftPublished || len(catalog.registered) != 1 || len(records.saved) != 0 {
		t.Fatalf("不带依据放行：outcome = %v 登记 %d 依据 %d", result.Outcome, len(catalog.registered), len(records.saved))
	}
}

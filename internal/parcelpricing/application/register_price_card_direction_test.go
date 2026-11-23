package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件证价卡入册前核对它引用的价格方向授权：三格不入册，读不回是未决，同一份已入册内容的重放不再被这道门打成拒绝。

func TestRegisteringAPriceCardRefusesTheThreeDirectionAuthorizationGaps(t *testing.T) {
	cases := []struct {
		name       string
		assessment ports.DirectionAuthorizationAssessment
		want       application.RegisterPriceCardOutcome
	}{
		{"不在册或未发布", ports.DirectionAuthorizationNotPublished, application.PriceCardRegistrationDirectionAuthorizationNotPublished},
		{"已到期或已收尾", ports.DirectionAuthorizationNotInForce, application.PriceCardRegistrationDirectionAuthorizationNotInForce},
		{"方向不符", ports.DirectionAuthorizationDirectionMismatch, application.PriceCardRegistrationDirectionAuthorizationDirectionMismatch},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			catalog := &priceCardCatalogDouble{outcome: ports.PriceCardRegistered}
			reader := &directionAuthorizationDouble{assessment: test.assessment}
			handler := application.NewRegisterPriceCardHandler(registerDeps(catalog, &pricingParameterDouble{}, false, reader))
			outcome, err := handler.Handle(context.Background(), application.RegisterPriceCardCommand{Registration: registrationFixture(t)})
			if err != nil {
				t.Fatalf("登记：%v", err)
			}
			if outcome != test.want || outcome.String() == "" {
				t.Fatalf("outcome = %s，want %s", outcome, test.want)
			}
			if len(catalog.registered) != 0 || reader.calls != 1 {
				t.Fatalf("仓储 %d 次，读口 %d 次", len(catalog.registered), reader.calls)
			}
		})
	}
}

func TestADirectionAuthorizationReadFailureStaysUndecided(t *testing.T) {
	boom := errors.New("商业册读不动")
	catalog := &priceCardCatalogDouble{outcome: ports.PriceCardRegistered}
	handler := application.NewRegisterPriceCardHandler(registerDeps(catalog, &pricingParameterDouble{}, false, &directionAuthorizationDouble{err: boom}))
	outcome, err := handler.Handle(context.Background(), application.RegisterPriceCardCommand{Registration: registrationFixture(t)})
	if !errors.Is(err, boom) || outcome != application.PriceCardRegistrationUndecided || len(catalog.registered) != 0 {
		t.Fatalf("outcome = %s err = %v 仓储 %d", outcome, err, len(catalog.registered))
	}

	missing := application.NewRegisterPriceCardHandler(application.RegisterPriceCardDeps{
		Catalog: catalog, Registered: &pricingParameterDouble{},
		Clock: fixedClock{at: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)},
	})
	outcome, err = missing.Handle(context.Background(), application.RegisterPriceCardCommand{Registration: registrationFixture(t)})
	if !errors.Is(err, application.ErrPriceCardDirectionAuthorizationUnreadable) || outcome != application.PriceCardRegistrationUndecided {
		t.Fatalf("没装读口：outcome = %s err = %v", outcome, err)
	}
}

func TestReplayingTheSameRegisteredContentSkipsTheDirectionAuthorizationGate(t *testing.T) {
	registration := registrationFixture(t)
	catalog := &priceCardCatalogDouble{outcome: ports.PriceCardAlreadyRegistered}
	reader := &directionAuthorizationDouble{assessment: ports.DirectionAuthorizationNotPublished}
	deps := registerDeps(catalog, &pricingParameterDouble{}, false, reader)
	deps.RegisteredContent = &registeredContentDouble{plan: registration.Plan(), found: true}
	outcome, err := application.NewRegisterPriceCardHandler(deps).Handle(context.Background(),
		application.RegisterPriceCardCommand{Registration: registration})
	if err != nil || outcome != application.PriceCardAlreadyOnRegister {
		t.Fatalf("重放：outcome = %s err = %v", outcome, err)
	}
	if reader.calls != 0 || len(catalog.registered) != 1 {
		t.Fatalf("重放不该问授权：读口 %d 次，仓储 %d 次", reader.calls, len(catalog.registered))
	}

	different := registrationFixtureOf(t, minimalPlanPriced(t, "11"))
	catalog = &priceCardCatalogDouble{outcome: ports.PriceCardContentConflict}
	reader = &directionAuthorizationDouble{assessment: ports.DirectionAuthorizationNotPublished}
	deps = registerDeps(catalog, &pricingParameterDouble{}, false, reader)
	deps.RegisteredContent = &registeredContentDouble{plan: registration.Plan(), found: true}
	outcome, err = application.NewRegisterPriceCardHandler(deps).Handle(context.Background(),
		application.RegisterPriceCardCommand{Registration: different})
	if err != nil || outcome != application.PriceCardRegistrationDirectionAuthorizationNotPublished || len(catalog.registered) != 0 {
		t.Fatalf("内容不同：outcome = %s err = %v 仓储 %d", outcome, err, len(catalog.registered))
	}
}

func TestPreviewRefusesAnUnpublishedDirectionAuthorizationInsteadOfSayingValidated(t *testing.T) {
	reader := &directionAuthorizationDouble{assessment: ports.DirectionAuthorizationNotPublished}
	handler, err := application.NewPreviewPriceCardImportHandler(
		&templateReaderDouble{reading: validatedReading(t, "30")},
		application.WithDirectionAuthorizationReader(reader, fixedClock{at: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}),
	)
	if err != nil {
		t.Fatal(err)
	}
	preview, err := handler.Handle(context.Background(), application.PreviewPriceCardImportCommand{
		Tenant: previewTenant(t), FileName: "card.xlsx", Raw: []byte("card bytes"),
	})
	if err != nil {
		t.Fatalf("预览：%v", err)
	}
	if preview.Outcome != application.PriceCardImportDirectionAuthorizationNotPublished ||
		preview.Outcome.String() != "DIRECTION_AUTHORIZATION_NOT_PUBLISHED" || preview.Content == nil {
		t.Fatalf("预览 = %s content=%v", preview.Outcome, preview.Content != nil)
	}
	if reader.calls != 1 {
		t.Fatalf("读口 %d 次", reader.calls)
	}

	broken := errors.New("商业册读不动")
	handler, err = application.NewPreviewPriceCardImportHandler(
		&templateReaderDouble{reading: validatedReading(t, "30")},
		application.WithDirectionAuthorizationReader(&directionAuthorizationDouble{err: broken}, fixedClock{at: time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)}),
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = handler.Handle(context.Background(), application.PreviewPriceCardImportCommand{
		Tenant: previewTenant(t), FileName: "card.xlsx", Raw: []byte("card bytes"),
	}); !errors.Is(err, broken) {
		t.Fatalf("读不回：%v", err)
	}
}

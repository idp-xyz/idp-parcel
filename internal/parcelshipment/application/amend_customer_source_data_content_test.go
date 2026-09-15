package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// deliveryPlaceDataScope 是收件资料范围（委托级、本上下文原词）——pp-seams/05 里带地址要素内容的修订落在这一格。
func deliveryPlaceDataScope(t *testing.T) domain.SourceDataScope {
	t.Helper()
	scope, err := domain.NewShipmentScopedSourceData(
		mustValue(t, domain.NewShipmentRequestID, "request-1"), domain.DeliveryPlaceDataGroup())
	if err != nil {
		t.Fatalf("new delivery place scope: %v", err)
	}
	return scope
}

func deliveryPostalCodeContent(t *testing.T, postal string) domain.SourceDataVersionContent {
	t.Helper()
	entry, err := domain.NewCanonicalContentEntry(
		domain.AddressElementEntryName(domain.DeliveryPlaceDataGroup(), domain.PostalCodeElement), postal)
	if err != nil {
		t.Fatalf("new content entry: %v", err)
	}
	return domain.NewAddressElementsContent(
		domain.AddressElementsOf(domain.DeliveryPlaceDataGroup(), []domain.CanonicalContentEntry{entry}))
}

func weightOnlyContent(t *testing.T, raw string) domain.SourceDataVersionContent {
	t.Helper()
	weight, err := domain.NewDeclaredWeight(
		mustValue(t, domain.NewMeasurementValue, raw), mustValue(t, domain.NewMeasurementUnitReference, "KG"))
	if err != nil {
		t.Fatalf("new declared weight: %v", err)
	}
	measurement, err := domain.NewDeclaredMeasurement(weight, domain.DeclaredDimensions{})
	if err != nil {
		t.Fatalf("new declared measurement: %v", err)
	}
	content, err := domain.NewMeasurementContent(measurement)
	if err != nil {
		t.Fatalf("new measurement content: %v", err)
	}
	return content
}

// Covers: pp-seams/05 裁决 3「命令两样一起带，应用层不重算」与判据 (2)——修订命令带的内容原样进版本，被采用后读口的
// 已采用格答那一版的值。编排不碰内容：不重算摘要、不核对、不改值。
func TestAnAmendmentCarryingContentFormsAVersionThatKeepsIt(t *testing.T) {
	fixture := newAmendmentFixture(t)
	command := fixture.command(t)
	command.Scope = deliveryPlaceDataScope(t)
	command.Content = deliveryPostalCodeContent(t, "SYN-200095")

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	version, formed := result.Version()
	if !formed {
		t.Fatal("the amendment formed no version")
	}
	if version.Content() != command.Content {
		t.Fatalf("content = %#v, want the content the command carried（应用层不改值）", version.Content())
	}
	if fixture.requests.saved == nil {
		t.Fatal("a version was formed but never saved")
	}
	destination, err := fixture.requests.saved.AddressElementsFor(mustValue(t, domain.NewDeclaredParcelID, "parcel-1"))
	if err != nil {
		t.Fatalf("address elements for parcel-1: %v", err)
	}
	if destination.Destination().Outcome() != domain.AddressElementsAnchoredOnAdoptedVersion {
		t.Fatalf("destination outcome = %q, want ANCHORED_ON_ADOPTED_VERSION", destination.Destination().Outcome())
	}
	elements, present := destination.Destination().Elements()
	if postal, declared := elements.PostalCode(); !present || !declared || postal != "SYN-200095" {
		t.Fatalf("postal = %q present = %v declared = %v, want SYN-200095", postal, present, declared)
	}
}

// Covers: 裁决 3「形与范围不配 → 未受理」「显式清空时内容必须为空，非空 → 未受理」——本编排里「调用方对世界的判断错了」
// 走的是与意图未声明同一格：上抛 ErrInvalidCustomerSourceDataVersion（接 HTTP 归`没形成答案`），在授权之前就拒、不签
// 版本号、不形成版本；请求到达过这件事由来源保全留痕。
func TestAmendmentContentThatDoesNotFitTheScopeOrIntentIsRefusedBeforeAuthorization(t *testing.T) {
	cases := map[string]func(*testing.T, *application.AmendCustomerSourceDataCommand){
		"收件范围带测量内容": func(t *testing.T, command *application.AmendCustomerSourceDataCommand) {
			command.Scope = deliveryPlaceDataScope(t)
			command.Content = weightOnlyContent(t, "1.00")
		},
		"开放范围带要素内容": func(t *testing.T, command *application.AmendCustomerSourceDataCommand) {
			command.Content = deliveryPostalCodeContent(t, "SYN-200095")
		},
		"显式清空带要素内容": func(t *testing.T, command *application.AmendCustomerSourceDataCommand) {
			command.Scope = deliveryPlaceDataScope(t)
			command.Intent = domain.ExplicitClearIntent
			command.Content = deliveryPostalCodeContent(t, "SYN-200095")
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newAmendmentFixture(t)
			command := fixture.command(t)
			mutate(t, &command)

			result, err := fixture.handler.Handle(context.Background(), command)
			if !errors.Is(err, domain.ErrInvalidCustomerSourceDataVersion) {
				t.Fatalf("err = %v, want ErrInvalidCustomerSourceDataVersion", err)
			}
			if _, formed := result.Version(); formed {
				t.Fatal("a refused command still formed a version")
			}
			if fixture.authorizer.calls != 0 {
				t.Fatalf("authorized %d times; a command that cannot stand should be refused before asking about authority", fixture.authorizer.calls)
			}
			if fixture.identities.issued != 0 {
				t.Fatalf("issued %d version IDs for a refused command", fixture.identities.issued)
			}
			if fixture.requests.saved != nil {
				t.Fatal("a refused command wrote onto the accepted request")
			}
			if _, preserved := fixture.sources.stored(t, amendmentSourceIdentity(t)); !preserved {
				t.Fatal("the refused request still arrived; its source fact must be preserved")
			}
		})
	}
}

// Covers: 裁决 3「清空就是那一版上要素缺席」——显式清空不带内容照旧成版本，被采用后已采用格带锚不带值。
func TestAnExplicitClearWithoutContentIsAdoptedWithoutAValue(t *testing.T) {
	fixture := newAmendmentFixture(t)
	command := fixture.command(t)
	command.Scope = deliveryPlaceDataScope(t)
	command.Intent = domain.ExplicitClearIntent

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if version, formed := result.Version(); !formed || !version.Content().Empty() {
		t.Fatalf("version = %#v formed = %v, want a formed version without content", version, formed)
	}
	destination, err := fixture.requests.saved.AddressElementsFor(mustValue(t, domain.NewDeclaredParcelID, "parcel-2"))
	if err != nil {
		t.Fatalf("address elements: %v", err)
	}
	if destination.Destination().Outcome() != domain.AddressElementsAnchoredOnAdoptedVersion {
		t.Fatalf("destination outcome = %q, want ANCHORED_ON_ADOPTED_VERSION", destination.Destination().Outcome())
	}
	if _, present := destination.Destination().Elements(); present {
		t.Fatal("清空版本被采用后交出了要素")
	}
}

package parcelshipment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/parcelshipment"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件证 parcel-pricing 读 parcel-shipment 提交版本申报的消费侧适配器（ADR-0179 决定四）：逐件译测量、两段邮编拼路线，单位折进
// 计价单位表、表外不猜；引用立不起来与提供方不可见同答不可见。委托真经领域建单，假状态钉不住读取路径。

var submittedAt = time.Date(2026, 8, 7, 9, 0, 0, 0, time.UTC)

func value[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

type declarationViewDouble struct {
	request psdomain.ShipmentRequest
	err     error
	calls   int
}

func (double *declarationViewDouble) LoadSubmissionVersionDeclaration(
	_ context.Context,
	tenant psdomain.TenantID,
	shipmentRequestID psdomain.ShipmentRequestID,
	submissionVersion psdomain.SubmissionVersionID,
) (psdomain.SubmissionVersionDeclaration, bool, error) {
	double.calls++
	if double.err != nil {
		return psdomain.SubmissionVersionDeclaration{}, false, double.err
	}
	if tenant.String() != "tenant-1" || shipmentRequestID != double.request.ShipmentRequestID() {
		return psdomain.SubmissionVersionDeclaration{}, false, nil
	}
	declaration, found := double.request.SubmissionVersionDeclarationFor(submissionVersion)
	return declaration, found, nil
}

func profileFor(t *testing.T, parcel, weightRaw, unit string, sides ...string) psdomain.DeclaredParcelProfile {
	t.Helper()
	weight, err := psdomain.NewDeclaredWeight(
		value(t, psdomain.NewMeasurementValue, weightRaw),
		value(t, psdomain.NewMeasurementUnitReference, unit),
	)
	if err != nil {
		t.Fatalf("declared weight: %v", err)
	}
	var dimensions psdomain.DeclaredDimensions
	if len(sides) == 4 {
		dimensions, err = psdomain.NewDeclaredDimensions(
			value(t, psdomain.NewMeasurementValue, sides[0]),
			value(t, psdomain.NewMeasurementValue, sides[1]),
			value(t, psdomain.NewMeasurementValue, sides[2]),
			value(t, psdomain.NewMeasurementUnitReference, sides[3]),
		)
		if err != nil {
			t.Fatalf("declared dimensions: %v", err)
		}
	}
	measurement, err := psdomain.NewDeclaredMeasurement(weight, dimensions)
	if err != nil {
		t.Fatalf("declared measurement: %v", err)
	}
	profile, err := psdomain.NewDeclaredParcelProfile(value(t, psdomain.NewDeclaredParcelID, parcel), measurement)
	if err != nil {
		t.Fatalf("declared profile: %v", err)
	}
	return profile
}

func postalCodes(t *testing.T, origin, destination string) psdomain.DeclaredAddressElements {
	t.Helper()
	group := func(scope psdomain.SourceDataGroupReference, postal string) psdomain.AddressElements {
		if postal == "" {
			return psdomain.AddressElementsOf(scope, nil)
		}
		entry, err := psdomain.NewCanonicalContentEntry(psdomain.AddressElementEntryName(scope, psdomain.PostalCodeElement), postal)
		if err != nil {
			t.Fatalf("content entry: %v", err)
		}
		return psdomain.AddressElementsOf(scope, []psdomain.CanonicalContentEntry{entry})
	}
	return psdomain.NewDeclaredAddressElements(
		group(psdomain.SenderPlaceDataGroup(), origin),
		group(psdomain.DeliveryPlaceDataGroup(), destination),
	)
}

// submittedRequest 造一份已提交委托。成员名单与画像分开给：缺画像的成员正是要演练的那一格。
func submittedRequest(t *testing.T, parcelIDs []string, elements psdomain.DeclaredAddressElements, profiles ...psdomain.DeclaredParcelProfile) psdomain.ShipmentRequest {
	t.Helper()
	parcels := make([]psdomain.DeclaredParcelID, 0, len(parcelIDs))
	for _, raw := range parcelIDs {
		parcels = append(parcels, value(t, psdomain.NewDeclaredParcelID, raw))
	}
	identity, err := psdomain.NewSourceIdentity(
		value(t, psdomain.NewTenantID, "tenant-1"),
		value(t, psdomain.NewCustomerAccountID, "customer-1"),
		value(t, psdomain.NewSource, "source-a"),
		value(t, psdomain.NewSourceRequestKey, "key-1"),
	)
	if err != nil {
		t.Fatalf("source identity: %v", err)
	}
	fingerprint, err := psdomain.NewSourceSubmissionFingerprint(identity, value(t, psdomain.NewPayloadDigest, "digest-1"),
		submittedAt.Add(-time.Hour), submittedAt.Add(-time.Hour+time.Second))
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	candidate, err := psdomain.NewSubmissionCandidate(fingerprint, value(t, psdomain.NewSubmissionBatchID, "batch-1"),
		value(t, psdomain.NewShipmentRequestID, "request-1"), parcels)
	if err != nil {
		t.Fatalf("candidate: %v", err)
	}
	gate, err := psdomain.EvaluateFutureSubmissionGate(allowedOwnership(t), value(t, psdomain.NewAdmissionScopeDigest, "scope-1"),
		value(t, psdomain.NewProductionOwnershipRevision, "rev-1"), submittedAt.Add(-time.Minute))
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	request, err := psdomain.SubmitShipmentRequest(psdomain.SubmitShipmentRequestSpec{
		Candidate:   candidate,
		Gate:        gate,
		VersionID:   value(t, psdomain.NewSubmissionVersionID, "version-1"),
		TaskID:      value(t, psdomain.NewAcceptanceDecisionTaskID, "task-1"),
		SubmittedAt: submittedAt,
		Profiles:    profiles,
		Elements:    elements,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	return request
}

func allowedOwnership(t *testing.T) psdomain.ProductionOwnershipDecision {
	t.Helper()
	scope, err := psdomain.NewAdmissionScope(
		value(t, psdomain.NewAdmissionScopeReference, "scope-ref-1"),
		value(t, psdomain.NewAdmissionScopeDigest, "scope-1"),
	)
	if err != nil {
		t.Fatalf("admission scope: %v", err)
	}
	validity, err := psdomain.NewOwnershipValidityInterval(submittedAt.Add(-24*time.Hour), submittedAt.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("validity: %v", err)
	}
	decision, err := psdomain.NewProductionOwnershipDecision(psdomain.ProductionOwnershipDecisionSpec{
		DecisionID:       value(t, psdomain.NewProductionOwnershipDecisionID, "decision-1"),
		Scope:            scope,
		Authority:        psdomain.ProductionAuthorityIDPParcel,
		AdmissionControl: psdomain.AdmissionControlOpen,
		RuleVersion:      value(t, psdomain.NewProductionOwnershipRuleVersion, "rule-1"),
		AsOf:             submittedAt.Add(-time.Hour),
		Validity:         validity,
		Revision:         value(t, psdomain.NewProductionOwnershipRevision, "rev-1"),
		DecisionAt:       submittedAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("ownership decision: %v", err)
	}
	return decision
}

func newAdapter(t *testing.T, view *declarationViewDouble) *parcelshipment.SubmissionDeclarations {
	t.Helper()
	adapter, err := parcelshipment.NewSubmissionDeclarations(view)
	if err != nil {
		t.Fatalf("adapter: %v", err)
	}
	return adapter
}

// 逐件译、与声明成员同序：报了测量的那件带重量与外廓，单位按原词进计价单位表；没报的那件重量为空、不拿别件顶。两段邮编拼成
// 一条路线，各件共用。
func TestASubmissionDeclarationTranslatesEachParcelAndTheRoute(t *testing.T) {
	request := submittedRequest(t, []string{"parcel-1", "parcel-2"}, postalCodes(t, "200000", "90210"),
		profileFor(t, "parcel-1", "1.50", "KG", "20", "15", "10", "CM"))
	adapter := newAdapter(t, &declarationViewDouble{request: request})

	declaration, found, err := adapter.LoadSubmissionDeclaration(context.Background(),
		value(t, ppdomain.NewTenantID, "tenant-1"), "request-1", "version-1")
	if err != nil || !found {
		t.Fatalf("found = %t err = %v, 想要读到", found, err)
	}
	if len(declaration.Parcels) != 2 || declaration.Parcels[0].Parcel != "parcel-1" || declaration.Parcels[1].Parcel != "parcel-2" {
		t.Fatalf("parcels = %+v, 想要 parcel-1、parcel-2 同序", declaration.Parcels)
	}
	first := declaration.Parcels[0]
	if first.Weight == nil || first.Weight.Value().String() != "1.5" || first.Weight.Unit() != ppdomain.WeightUnitKilogram {
		t.Fatalf("parcel-1 weight = %+v, 想要 1.5 KG", first.Weight)
	}
	if first.Dimensions == nil || first.Dimensions.Unit() != ppdomain.LengthUnitCentimeter ||
		first.Dimensions.LongestSide().Value().String() != "20" {
		t.Fatalf("parcel-1 dimensions = %+v, 想要 20 × 15 × 10 CM", first.Dimensions)
	}
	if second := declaration.Parcels[1]; second.Weight != nil || second.Dimensions != nil {
		t.Fatalf("parcel-2 = %+v, 想要没报测量即为空", second)
	}
	if declaration.Route == nil || declaration.Route.Origin() != "200000" || declaration.Route.Destination() != "90210" {
		t.Fatalf("route = %+v, 想要 200000 → 90210", declaration.Route)
	}
}

// 单位不在计价单位表内不猜：整件当没有测量交回。重量单位与外廓单位任一在表外都是这一格。表按原词认，小写的 kg、cm 也在表外：
// 正式 SELL 评价的输入解析（transportfulfillment 适配器的 weightUnit / lengthUnit）同样按原词认、对小写答表外，受理前若折叠，
// 同一份申报在受理前估得出冻结额、受理后却评不出客户费用。
func TestAnUntranslatableUnitLeavesTheParcelUnmeasured(t *testing.T) {
	for name, profile := range map[string]psdomain.DeclaredParcelProfile{
		"重量单位在表外": profileFor(t, "parcel-1", "3", "STONE"),
		"外廓单位在表外": profileFor(t, "parcel-1", "3", "KG", "1", "1", "1", "FATHOM"),
		"重量单位小写":  profileFor(t, "parcel-1", "3", "kg"),
		"外廓单位小写":  profileFor(t, "parcel-1", "3", "KG", "1", "1", "1", "cm"),
	} {
		t.Run(name, func(t *testing.T) {
			request := submittedRequest(t, []string{"parcel-1"}, postalCodes(t, "200000", "90210"), profile)
			declaration, found, err := newAdapter(t, &declarationViewDouble{request: request}).LoadSubmissionDeclaration(
				context.Background(), value(t, ppdomain.NewTenantID, "tenant-1"), "request-1", "version-1")
			if err != nil || !found || len(declaration.Parcels) != 1 {
				t.Fatalf("declaration = %+v found = %t err = %v", declaration, found, err)
			}
			if parcel := declaration.Parcels[0]; parcel.Weight != nil || parcel.Dimensions != nil {
				t.Fatalf("parcel = %+v, 想要整件当没有测量", parcel)
			}
		})
	}
}

// 只报了收件邮编时按目的邮编拼路线；收件邮编没报即没有路线，寄件邮编单独在场也不拼。
func TestARouteNeedsTheDestinationPostalCode(t *testing.T) {
	for name, tc := range map[string]struct {
		origin, destination string
		routed              bool
	}{
		"只报收件": {destination: "90210", routed: true},
		"只报寄件": {origin: "200000"},
		"都没报":  {},
	} {
		t.Run(name, func(t *testing.T) {
			request := submittedRequest(t, []string{"parcel-1"}, postalCodes(t, tc.origin, tc.destination),
				profileFor(t, "parcel-1", "1", "KG"))
			declaration, found, err := newAdapter(t, &declarationViewDouble{request: request}).LoadSubmissionDeclaration(
				context.Background(), value(t, ppdomain.NewTenantID, "tenant-1"), "request-1", "version-1")
			if err != nil || !found {
				t.Fatalf("found = %t err = %v", found, err)
			}
			if (declaration.Route != nil) != tc.routed {
				t.Fatalf("route = %+v, 想要有路线 = %t", declaration.Route, tc.routed)
			}
			if tc.routed && (declaration.Route.Origin() != "" || declaration.Route.Destination() != tc.destination) {
				t.Fatalf("route = %+v, 想要只按目的邮编拼", declaration.Route)
			}
		})
	}
}

// 引用在提供方构造门上立不起来时答不可见、一次都不问；提供方不可见照答不可见；读面坏了上抛。
func TestUnbuildableReferencesAndProviderGapsAnswerInvisible(t *testing.T) {
	request := submittedRequest(t, []string{"parcel-1"}, postalCodes(t, "", ""), profileFor(t, "parcel-1", "1", "KG"))
	tenant := value(t, ppdomain.NewTenantID, "tenant-1")

	view := &declarationViewDouble{request: request}
	if _, found, err := newAdapter(t, view).LoadSubmissionDeclaration(context.Background(), tenant, "", "version-1"); err != nil || found || view.calls != 0 {
		t.Fatalf("空委托引用：found = %t err = %v calls = %d, 想要不可见且不问", found, err, view.calls)
	}
	if _, found, err := newAdapter(t, view).LoadSubmissionDeclaration(context.Background(), tenant, "request-1", "version-9"); err != nil || found {
		t.Fatalf("不在本委托上的版本：found = %t err = %v, 想要不可见", found, err)
	}
	broken := &declarationViewDouble{request: request, err: errors.New("shipment read unreachable")}
	if _, _, err := newAdapter(t, broken).LoadSubmissionDeclaration(context.Background(), tenant, "request-1", "version-1"); err == nil {
		t.Fatal("读面坏了却没报错")
	}
	if _, err := parcelshipment.NewSubmissionDeclarations(nil); err == nil {
		t.Fatal("缺读口却装配成功")
	}
}

package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func lifetimeNumber(t *testing.T, code, number string) domain.LifetimeRegistrationNumber {
	t.Helper()
	built, err := domain.NewLifetimeRegistrationNumber(
		commercialValue(t, domain.NewRegistrationNumberTypeCode, code),
		commercialValue(t, domain.NewRegistrationNumber, number),
	)
	if err != nil {
		t.Fatalf("new lifetime registration number: %v", err)
	}
	return built
}

func identityLayer(t *testing.T, country string, numbers ...domain.LifetimeRegistrationNumber) domain.LegalEntityIdentityLayer {
	t.Helper()
	layer, err := domain.NewLegalEntityIdentityLayer(
		commercialValue(t, domain.NewRegistrationCountryCode, country), numbers,
	)
	if err != nil {
		t.Fatalf("new identity layer: %v", err)
	}
	return layer
}

func legalEntityRevision(t *testing.T, revision int) domain.LegalEntityRegistration {
	t.Helper()
	entity, err := domain.NewResponsibleLegalEntity(
		commercialValue(t, domain.NewTenantID, "tenant-1"),
		commercialValue(t, domain.NewLegalEntityReference, "le-1"),
		party(t, "party-1", "本租参与方"),
	)
	if err != nil {
		t.Fatalf("new legal entity: %v", err)
	}
	registration, err := domain.NewLegalEntityRegistration(
		entity, revision, commercialValue(t, domain.NewIdentityBasisReference, "basis-1"), identityLifecycle(t),
	)
	if err != nil {
		t.Fatalf("new legal entity registration: %v", err)
	}
	return registration
}

func withIdentity(
	t *testing.T,
	registration domain.LegalEntityRegistration,
	layer domain.LegalEntityIdentityLayer,
	correction string,
) domain.LegalEntityRegistration {
	t.Helper()
	var basis *domain.IdentityBasisReference
	if correction != "" {
		value := commercialValue(t, domain.NewIdentityBasisReference, correction)
		basis = &value
	}
	carried, err := registration.WithIdentityLayer(layer, basis)
	if err != nil {
		t.Fatalf("with identity layer: %v", err)
	}
	return carried
}

// Covers: ADR-0145 决定一——身份层两格缺一即不成立；一类只收一个号。
func TestLegalEntityIdentityLayerNeedsCountryAndOneNumberPerType(t *testing.T) {
	if _, err := domain.NewLegalEntityIdentityLayer(domain.RegistrationCountryCode{}, []domain.LifetimeRegistrationNumber{
		lifetimeNumber(t, "SYN-XA-LIFETIME", "SYN-XA-000001"),
	}); !errors.Is(err, domain.ErrInvalidLegalEntityIdentityLayer) {
		t.Fatalf("missing country: error = %v, want ErrInvalidLegalEntityIdentityLayer", err)
	}
	if _, err := domain.NewLegalEntityIdentityLayer(
		commercialValue(t, domain.NewRegistrationCountryCode, "XA"), nil,
	); !errors.Is(err, domain.ErrInvalidLegalEntityIdentityLayer) {
		t.Fatalf("missing numbers: error = %v, want ErrInvalidLegalEntityIdentityLayer", err)
	}
	if _, err := domain.NewLegalEntityIdentityLayer(
		commercialValue(t, domain.NewRegistrationCountryCode, "XA"),
		[]domain.LifetimeRegistrationNumber{
			lifetimeNumber(t, "SYN-XA-LIFETIME", "SYN-XA-000001"),
			lifetimeNumber(t, "SYN-XA-LIFETIME", "SYN-XA-000002"),
		},
	); !errors.Is(err, domain.ErrInvalidLegalEntityIdentityLayer) {
		t.Fatalf("same type twice: error = %v, want ErrInvalidLegalEntityIdentityLayer", err)
	}
}

// Covers: 号按类型代码规整——录入次序不同的同一组号是同一个身份层，登记册按内容比对重放时答法不变。
func TestLegalEntityIdentityLayerOrdersNumbersByType(t *testing.T) {
	first := identityLayer(t, "XA",
		lifetimeNumber(t, "SYN-XA-B", "SYN-B-1"), lifetimeNumber(t, "SYN-XA-A", "SYN-A-1"))
	second := identityLayer(t, "XA",
		lifetimeNumber(t, "SYN-XA-A", "SYN-A-1"), lifetimeNumber(t, "SYN-XA-B", "SYN-B-1"))
	if !first.Equal(second) {
		t.Fatal("same numbers in a different entry order must be the same identity layer")
	}
	if got := first.Numbers()[0].TypeCode().String(); got != "SYN-XA-A" {
		t.Fatalf("first number type = %s, want SYN-XA-A", got)
	}
	if first.Equal(identityLayer(t, "XB",
		lifetimeNumber(t, "SYN-XA-A", "SYN-A-1"), lifetimeNumber(t, "SYN-XA-B", "SYN-B-1"))) {
		t.Fatal("a different country is a different identity layer")
	}
}

// Covers: ADR-0145 决定二——改号必须是带身份更正依据的更正；没改不许带；历史修订第一次补登是补登不是更正。
func TestLegalEntityIdentitySuccession(t *testing.T) {
	original := identityLayer(t, "XA", lifetimeNumber(t, "SYN-XA-LIFETIME", "SYN-XA-000001"))
	corrected := identityLayer(t, "XA", lifetimeNumber(t, "SYN-XA-LIFETIME", "SYN-XA-000009"))
	latest := withIdentity(t, legalEntityRevision(t, 1), original, "")
	legacy := legalEntityRevision(t, 1)

	cases := []struct {
		name   string
		latest domain.LegalEntityRegistration
		next   domain.LegalEntityRegistration
		want   error
	}{
		{"改号不带更正依据", latest, withIdentity(t, legalEntityRevision(t, 2), corrected, ""), domain.ErrIdentityLayerChangedWithoutCorrection},
		{"改号带更正依据", latest, withIdentity(t, legalEntityRevision(t, 2), corrected, "SYN-CORRECTION-01"), nil},
		{"没改却带更正依据", latest, withIdentity(t, legalEntityRevision(t, 2), original, "SYN-CORRECTION-01"), domain.ErrIdentityCorrectionWithoutChange},
		{"没改不带", latest, withIdentity(t, legalEntityRevision(t, 2), original, ""), nil},
		{"历史修订补登", legacy, withIdentity(t, legalEntityRevision(t, 2), original, ""), nil},
		{"历史修订补登却带更正依据", legacy, withIdentity(t, legalEntityRevision(t, 2), original, "SYN-CORRECTION-01"), domain.ErrIdentityCorrectionWithoutChange},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := domain.CheckLegalEntityIdentitySuccession(testCase.latest, testCase.next)
			if testCase.want == nil && err != nil {
				t.Fatalf("error = %v, want nil", err)
			}
			if testCase.want != nil && !errors.Is(err, testCase.want) {
				t.Fatalf("error = %v, want %v", err, testCase.want)
			}
		})
	}

	if err := domain.CheckLegalEntityIdentitySuccession(latest, legalEntityRevision(t, 2)); !errors.Is(err, domain.ErrInvalidLegalEntityIdentityLayer) {
		t.Fatalf("successor without identity layer: error = %v, want ErrInvalidLegalEntityIdentityLayer", err)
	}
}

// Covers: ADR-0145 决定二——首笔登记之前没有登记过的号：修订 1 带身份更正依据即不成立，拒因与「没改却带」同一个；
// 修订 2 起带不带由接续门比对前一笔判，这里不拦。
func TestFirstLegalEntityRevisionCarriesNoCorrection(t *testing.T) {
	layer := identityLayer(t, "XA", lifetimeNumber(t, "SYN-XA-LIFETIME", "SYN-XA-000001"))
	correction := commercialValue(t, domain.NewIdentityBasisReference, "SYN-CORRECTION-01")

	if _, err := legalEntityRevision(t, 1).WithIdentityLayer(layer, &correction); !errors.Is(err, domain.ErrIdentityCorrectionWithoutChange) {
		t.Fatalf("revision 1 with a correction basis: error = %v, want ErrIdentityCorrectionWithoutChange", err)
	}
	carried, err := legalEntityRevision(t, 2).WithIdentityLayer(layer, &correction)
	if err != nil {
		t.Fatalf("revision 2 with a correction basis: error = %v, want nil", err)
	}
	if basis, has := carried.IdentityCorrectionBasis(); !has || basis != correction {
		t.Fatalf("revision 2 correction basis = %q (has %v)", basis, has)
	}
}

// Covers: 停用修订原样沿用身份层、不沿用身份更正依据；身份更正依据不能以零值出现。
func TestLegalEntityDeactivationKeepsTheIdentityLayer(t *testing.T) {
	layer := identityLayer(t, "XA", lifetimeNumber(t, "SYN-XA-LIFETIME", "SYN-XA-000001"))
	corrected := withIdentity(t, legalEntityRevision(t, 2), layer, "SYN-CORRECTION-01")

	deactivated, err := corrected.Deactivate(commercialValue(t, domain.NewIdentityBasisReference, "SYN-DEACT-01"), deactivationAt)
	if err != nil {
		t.Fatalf("deactivate: %v", err)
	}
	if deactivated.Revision() != 3 {
		t.Fatalf("revision = %d, want 3", deactivated.Revision())
	}
	kept, has := deactivated.IdentityLayer()
	if !has || !kept.Equal(layer) {
		t.Fatalf("identity layer after deactivation = %+v (has %v), want the same layer", kept, has)
	}
	if _, has := deactivated.IdentityCorrectionBasis(); has {
		t.Fatal("a deactivation revision must not carry the previous revision's correction basis")
	}

	if _, has := legalEntityRevision(t, 1).IdentityLayer(); has {
		t.Fatal("a revision registered without the identity layer must answer false")
	}
	zero := domain.IdentityBasisReference{}
	if _, err := legalEntityRevision(t, 1).WithIdentityLayer(layer, &zero); !errors.Is(err, domain.ErrInvalidIdentityRegistration) {
		t.Fatalf("zero correction basis: error = %v, want ErrInvalidIdentityRegistration", err)
	}
}

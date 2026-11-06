package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

func partyRevision(t *testing.T, revision int) domain.BusinessPartyRegistration {
	t.Helper()
	registration, err := domain.NewBusinessPartyRegistration(
		party(t, "party-1", "往来单位一号"), revision,
		commercialValue(t, domain.NewIdentityBasisReference, "basis-1"), identityLifecycle(t),
	)
	if err != nil {
		t.Fatalf("new business party registration: %v", err)
	}
	return registration
}

func identified(
	t *testing.T,
	registration domain.BusinessPartyRegistration,
	layer *domain.LegalEntityIdentityLayer,
	correction string,
	references ...domain.ExternalReference,
) domain.BusinessPartyRegistration {
	t.Helper()
	var basis *domain.IdentityBasisReference
	if correction != "" {
		value := commercialValue(t, domain.NewIdentityBasisReference, correction)
		basis = &value
	}
	next, err := registration.WithIdentification(layer, basis, references)
	if err != nil {
		t.Fatalf("with identification: %v", err)
	}
	return next
}

func reference(t *testing.T, system, purpose, code string) domain.ExternalReference {
	t.Helper()
	built, err := domain.ParseExternalReference(system, purpose, code)
	if err != nil {
		t.Fatalf("parse external reference: %v", err)
	}
	return built
}

// Covers: 外部编码的形状——系统标识不收空白与怪字符、用途封闭三值、码不收首尾空白；同一（系统，用途）给两个码拒；
// 次序按（系统，用途）规整。
func TestExternalReferenceShapes(t *testing.T) {
	for _, bad := range [][3]string{
		{"KD ERP", "CUSTOMER", "C-1"},
		{"-ERP", "CUSTOMER", "C-1"},
		{"SYN-ERP", "VENDOR", "C-1"},
		{"SYN-ERP", "CUSTOMER", " C-1"},
		{"SYN-ERP", "CUSTOMER", ""},
	} {
		if _, err := domain.ParseExternalReference(bad[0], bad[1], bad[2]); !errors.Is(err, domain.ErrInvalidExternalReference) {
			t.Errorf("%v: err = %v, want ErrInvalidExternalReference", bad, err)
		}
	}
	_, err := partyRevision(t, 1).WithIdentification(nil, nil, []domain.ExternalReference{
		reference(t, "SYN-ERP", "CUSTOMER", "C-1"), reference(t, "SYN-ERP", "CUSTOMER", "C-2"),
	})
	if !errors.Is(err, domain.ErrInvalidExternalReference) {
		t.Fatalf("同一系统同一用途两个码：err = %v", err)
	}
	ordered := identified(t, partyRevision(t, 1), nil, "",
		reference(t, "SYN-ERP-B", "CUSTOMER", "C-9"), reference(t, "SYN-ERP-A", "SUPPLIER", "V-1"), reference(t, "SYN-ERP-A", "CUSTOMER", "C-1"))
	references := ordered.ExternalReferences()
	if references[0].Code() != "C-1" || references[1].Code() != "V-1" || references[2].Code() != "C-9" {
		t.Fatalf("次序 = %+v", references)
	}
}

// Covers: 接续判法——补登不带更正依据、改号与去号要带、没改却带拒；修订 1 不收更正依据。
func TestBusinessPartyIdentitySuccession(t *testing.T) {
	first := identityLayer(t, "CN", lifetimeNumber(t, "SYN-LIFETIME", "SYN-000001"))
	second := identityLayer(t, "CN", lifetimeNumber(t, "SYN-LIFETIME", "SYN-000002"))
	none := partyRevision(t, 1)
	numbered := identified(t, partyRevision(t, 2), &first, "")

	cases := []struct {
		name         string
		latest, next domain.BusinessPartyRegistration
		want         error
	}{
		{"补登", none, identified(t, partyRevision(t, 2), &first, ""), nil},
		{"补登带更正依据", none, identified(t, partyRevision(t, 2), &first, "SYN-FIX"), domain.ErrIdentityCorrectionWithoutChange},
		{"照带原号", numbered, identified(t, partyRevision(t, 3), &first, ""), nil},
		{"照带原号却带更正依据", numbered, identified(t, partyRevision(t, 3), &first, "SYN-FIX"), domain.ErrIdentityCorrectionWithoutChange},
		{"改号", numbered, identified(t, partyRevision(t, 3), &second, ""), domain.ErrBusinessPartyIdentityChangedWithoutCorrection},
		{"改号带更正依据", numbered, identified(t, partyRevision(t, 3), &second, "SYN-FIX"), nil},
		{"去号", numbered, identified(t, partyRevision(t, 3), nil, ""), domain.ErrBusinessPartyIdentityChangedWithoutCorrection},
		{"去号带更正依据", numbered, identified(t, partyRevision(t, 3), nil, "SYN-FIX"), nil},
		{"一直没号", none, partyRevision(t, 2), nil},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := domain.CheckBusinessPartyIdentitySuccession(testCase.latest, testCase.next); !errors.Is(err, testCase.want) {
				t.Fatalf("err = %v, want %v", err, testCase.want)
			}
		})
	}

	correction := commercialValue(t, domain.NewIdentityBasisReference, "SYN-FIX")
	if _, err := partyRevision(t, 1).WithIdentification(&first, &correction, nil); !errors.Is(err, domain.ErrIdentityCorrectionWithoutChange) {
		t.Fatalf("修订 1 带更正依据：err = %v", err)
	}
}

// Covers: 停用修订原样沿用号与外部编码，不沿用更正依据。
func TestBusinessPartyDeactivationKeepsIdentification(t *testing.T) {
	layer := identityLayer(t, "CN", lifetimeNumber(t, "SYN-LIFETIME", "SYN-000001"))
	corrected := identified(t, partyRevision(t, 2), &layer, "SYN-FIX", reference(t, "SYN-ERP", "PARTY", "P-1"))
	stopped, err := corrected.Deactivate(commercialValue(t, domain.NewIdentityBasisReference, "basis-stop"), time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if got, has := stopped.IdentityLayer(); !has || !got.Equal(layer) {
		t.Fatalf("停用修订的号 = %+v has=%v", got, has)
	}
	if _, has := stopped.IdentityCorrectionBasis(); has {
		t.Fatal("停用修订不该带更正依据")
	}
	if references := stopped.ExternalReferences(); len(references) != 1 || references[0].Code() != "P-1" {
		t.Fatalf("停用修订的外部编码 = %+v", references)
	}
}

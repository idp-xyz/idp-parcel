package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// Covers: `AT-NO-126` 的领域面（ADR-0220）——更正链只由这一版与它沿 TF 更正回指上溯的各前版组成：同一（对象，范围）下的另一版不在
// 链上，除非它被列为前版；首登版的链上只有它自己。
func TestACorrectionChainHoldsOnlyTheVersionsItWasGiven(t *testing.T) {
	firstRegistration := correctionChain(t, "v2")
	if !firstRegistration.Holds(handoverRef(t, "v2")) || firstRegistration.Holds(handoverRef(t, "v1")) {
		t.Fatal("首登版的链只该有它自己：同一（对象，范围）下的 v1 不是它的前版")
	}
	corrected := correctionChain(t, "v3", "v2", "v1")
	for _, version := range []string{"v3", "v2", "v1"} {
		if !corrected.Holds(handoverRef(t, version)) {
			t.Fatalf("链上缺 %s", version)
		}
	}
	if corrected.Holds(handoverRef(t, "v0")) {
		t.Fatal("没列为前版的 v0 不在链上")
	}
	members := corrected.Members()
	if len(members) != 3 || members[0].Version() != "v3" || members[1].Version() != "v2" || members[2].Version() != "v1" {
		t.Fatalf("成员 = %v, want 这一版在前、前版近者在前", members)
	}
	members[0] = handoverRef(t, "v9")
	if corrected.Head().Version() != "v3" {
		t.Fatal("改 Members 交回的切片动了链")
	}
}

// Covers: ADR-0220——给的不是一条 TF 更正链就立不起来：前版换了范围或对象、版本重复、这一版缺格。TF 的更正只换版本，链线性。
func TestACorrectionChainRefusesWhatTransportFulfillmentCannotChain(t *testing.T) {
	otherScope, err := domain.NewAuthoritativeHandoverReference("SYN-PARCEL-61", "SYN-SCOPE-OTHER", "v1")
	if err != nil {
		t.Fatal(err)
	}
	otherObject, err := domain.NewAuthoritativeHandoverReference("SYN-PARCEL-OTHER", "SYN-SCOPE-61", "v1")
	if err != nil {
		t.Fatal(err)
	}
	for name, attempt := range map[string]func() error{
		"前版换了范围": func() error {
			_, err := domain.NewHandoverCorrectionChain(handoverRef(t, "v2"), otherScope)
			return err
		},
		"前版换了对象": func() error {
			_, err := domain.NewHandoverCorrectionChain(handoverRef(t, "v2"), otherObject)
			return err
		},
		"前版就是这一版": func() error {
			_, err := domain.NewHandoverCorrectionChain(handoverRef(t, "v2"), handoverRef(t, "v2"))
			return err
		},
		"前版重复": func() error {
			_, err := domain.NewHandoverCorrectionChain(handoverRef(t, "v3"), handoverRef(t, "v2"), handoverRef(t, "v2"))
			return err
		},
		"这一版缺格": func() error {
			_, err := domain.NewHandoverCorrectionChain(domain.AuthoritativeHandoverReference{})
			return err
		},
		"前版缺格": func() error {
			_, err := domain.NewHandoverCorrectionChain(handoverRef(t, "v2"), domain.AuthoritativeHandoverReference{})
			return err
		},
	} {
		if err := attempt(); !errors.Is(err, domain.ErrInvalidHandoverCorrectionChain) {
			t.Fatalf("%s: err = %v, want ErrInvalidHandoverCorrectionChain", name, err)
		}
	}
}

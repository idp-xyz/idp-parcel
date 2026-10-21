package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// adoptionOf 是一段对 commission 的采用连线，开始依据与时刻照给。
func adoptionOf(t *testing.T, commission domain.TransportCommission, segment, basis string, at time.Time) domain.SegmentCommissionAdoption {
	t.Helper()
	adoption, err := domain.RehydrateSegmentCommissionAdoption(domain.RehydrateSegmentCommissionAdoptionSpec{
		TenantID: commission.TenantID(), Segment: mustValue(t, domain.NewFulfillmentSegmentReference, segment),
		Commission: commission.Commission(), Provider: commission.Provider(),
		StartBasis: mustValue(t, domain.NewParticipationBasisReference, basis), StartedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	return adoption
}

// Covers: AT-TF-307、AT-TF-308 的领域半边（ADR-0232 决定二、三）——解除一段，委托的开始按其余各段重新派生：一段不剩撤回到已提交，
// 剩下的段里取最早的那一条（比原来晚也照取），解除的不是开始所依的那一段时不动；之前的开始与变法随解除留下。
func TestWithdrawingAnAdoptionRederivesTheCommissionStartFromTheRemainingSegments(t *testing.T) {
	commission := submittedCommission(t)
	earlyAt, lateAt := commissionSubmittedAt.Add(time.Hour), commissionSubmittedAt.Add(3*time.Hour)
	started, err := commission.MarkTransportStarted(mustValue(t, domain.NewParticipationBasisReference, "HO/early"), earlyAt)
	if err != nil {
		t.Fatal(err)
	}
	early := adoptionOf(t, started, "SEG-EARLY", "HO/early", earlyAt)
	late := adoptionOf(t, started, "SEG-LATE", "HO/late", lateAt)
	by := mustValue(t, domain.NewRegistrantReference, "https://id.syn.example/dex#SYN-OPERATOR-01")

	withdrawn, alone, err := domain.WithdrawSegmentCommissionAdoption(early, 1, started, nil, by)
	if err != nil || withdrawn.StartChange() != domain.CommissionStartWithdrawn {
		t.Fatalf("唯一一段 = %v %v", withdrawn.StartChange(), err)
	}
	if _, _, still := alone.TransportStarted(); still {
		t.Fatal("唯一一段解除后委托仍是已开始")
	}
	if _, _, present := withdrawn.StartAfter(); present {
		t.Fatal("撤回之后还有开始")
	}
	if basis, at := withdrawn.StartBefore(); basis.String() != "HO/early" || !at.Equal(earlyAt) {
		t.Fatalf("之前的开始 = (%s, %s)", basis, at)
	}
	if _, err := alone.Cancel(lateAt); err != nil {
		t.Fatalf("撤回到已提交的委托取消不了：%v", err)
	}

	rederived, moved, err := domain.WithdrawSegmentCommissionAdoption(early, 1, started, []domain.SegmentCommissionAdoption{late}, by)
	basis, at, _ := moved.TransportStarted()
	if err != nil || rederived.StartChange() != domain.CommissionStartRederived || basis.String() != "HO/late" || !at.Equal(lateAt) {
		t.Fatalf("重新派生 = %v (%s, %s) %v", rederived.StartChange(), basis, at, err)
	}

	kept, same, err := domain.WithdrawSegmentCommissionAdoption(late, 1, started, []domain.SegmentCommissionAdoption{early}, by)
	basis, at, _ = same.TransportStarted()
	if err != nil || kept.StartChange() != domain.CommissionStartUnchanged || basis.String() != "HO/early" || !at.Equal(earlyAt) {
		t.Fatalf("不是开始所依的段 = %v (%s, %s) %v", kept.StartChange(), basis, at, err)
	}

	tie := adoptionOf(t, started, "SEG-TIE", "HO/a-tie", lateAt)
	_, tied, err := domain.WithdrawSegmentCommissionAdoption(early, 1, started, []domain.SegmentCommissionAdoption{late, tie}, by)
	if basis, _, _ := tied.TransportStarted(); err != nil || basis.String() != "HO/a-tie" {
		t.Fatalf("同刻按依据字面序取 = %s %v", basis, err)
	}
}

func TestAWithdrawalRefusesInputsThatContradictTheRegister(t *testing.T) {
	commission := submittedCommission(t)
	at := commissionSubmittedAt.Add(time.Hour)
	started, err := commission.MarkTransportStarted(mustValue(t, domain.NewParticipationBasisReference, "HO/1"), at)
	if err != nil {
		t.Fatal(err)
	}
	adoption := adoptionOf(t, started, "SEG-1", "HO/1", at)
	by := mustValue(t, domain.NewRegistrantReference, "https://id.syn.example/dex#SYN-OPERATOR-01")
	for name, attempt := range map[string]func() error{
		"委托未开始": func() error {
			_, _, err := domain.WithdrawSegmentCommissionAdoption(adoption, 1, commission, nil, by)
			return err
		},
		"其余里混进这一段自己": func() error {
			_, _, err := domain.WithdrawSegmentCommissionAdoption(adoption, 1, started, []domain.SegmentCommissionAdoption{adoption}, by)
			return err
		},
		"没有代次": func() error {
			_, _, err := domain.WithdrawSegmentCommissionAdoption(adoption, 0, started, nil, by)
			return err
		},
		"没有解除人": func() error {
			_, _, err := domain.WithdrawSegmentCommissionAdoption(adoption, 1, started, nil, domain.RegistrantReference{})
			return err
		},
	} {
		if err := attempt(); !errors.Is(err, domain.ErrInvalidAdoptionWithdrawal) {
			t.Fatalf("%s：err = %v", name, err)
		}
	}
	if _, err := domain.ParseCommissionStartCorrection("MOVED"); err == nil {
		t.Fatal("集外的变法认成了一格")
	}
}

// Covers: AT-TF-308 的同刻并列那一格（评审复现）——委托的开始依的是先采用、同刻、依据字面序较大的那一段；解除一段不相干的段，开始所依的
// 那一段还在，记 KEPT、开始原样，不按字面序换成另一条同刻的依据。
func TestWithdrawingAnUnrelatedSegmentUnderATieKeepsTheStart(t *testing.T) {
	commission := submittedCommission(t)
	tieAt, otherAt := commissionSubmittedAt.Add(time.Hour), commissionSubmittedAt.Add(3*time.Hour)
	started, err := commission.MarkTransportStarted(mustValue(t, domain.NewParticipationBasisReference, "HO/z-first-adopted"), tieAt)
	if err != nil {
		t.Fatal(err)
	}
	first := adoptionOf(t, started, "SEG-FIRST", "HO/z-first-adopted", tieAt)
	second := adoptionOf(t, started, "SEG-SECOND", "HO/a-second-adopted", tieAt)
	unrelated := adoptionOf(t, started, "SEG-UNRELATED", "HO/unrelated", otherAt)
	by := mustValue(t, domain.NewRegistrantReference, "https://id.syn.example/dex#SYN-OPERATOR-01")
	withdrawal, after, err := domain.WithdrawSegmentCommissionAdoption(unrelated, 1, started, []domain.SegmentCommissionAdoption{first, second}, by)
	basis, _, _ := after.TransportStarted()
	if err != nil || withdrawal.StartChange() != domain.CommissionStartUnchanged || basis.String() != "HO/z-first-adopted" {
		t.Fatalf("变法 %s，之后的依据 %s，err=%v；want KEPT、HO/z-first-adopted", withdrawal.StartChange(), basis, err)
	}
	// 开始所依的那一段被解除、剩下同刻的另一段：照字面序取它，记 REDERIVED。
	rederived, moved, err := domain.WithdrawSegmentCommissionAdoption(first, 1, started, []domain.SegmentCommissionAdoption{second, unrelated}, by)
	if basis, _, _ := moved.TransportStarted(); err != nil || rederived.StartChange() != domain.CommissionStartRederived || basis.String() != "HO/a-second-adopted" {
		t.Fatalf("解除开始所依的那一段 = %s %s %v", rederived.StartChange(), basis, err)
	}
}

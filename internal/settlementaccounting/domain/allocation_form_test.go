package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

func TestApportionByWeightUsesLargestRemainder(t *testing.T) {
	// 来源 10，权重 1、2、3。取整后是 1、3、5，余 1 个最小货币单位。
	// 余数分别是 4、2、0，所以加在 parcel-a 上：2、3、5。
	apportionment := apportionWeight(t, 10, basis("parcel-b", 2), basis("parcel-a", 1), basis("parcel-c", 3))
	if apportionment.Form() != domain.AllocationByWeight || apportionment.Denominator() != 6 || apportionment.UnallocatedMinor() != 0 {
		t.Fatalf("展开头 = form %s den %d unallocated %d", apportionment.Form(), apportionment.Denominator(), apportionment.UnallocatedMinor())
	}
	shares := apportionment.Shares()
	if len(shares) != 3 {
		t.Fatalf("展开行数 = %d", len(shares))
	}
	expectShare(t, shares[0], "parcel-a", 1, 10, 6, 1, 4, 2, true)
	expectShare(t, shares[1], "parcel-b", 2, 20, 6, 3, 2, 3, false)
	expectShare(t, shares[2], "parcel-c", 3, 30, 6, 5, 0, 5, false)
	expectPortions(t, apportionment, "parcel-a", 2, "parcel-b", 3, "parcel-c", 5)
}

func TestApportionTieBreaksByTargetIdentity(t *testing.T) {
	// 来源 5，两个权重都是 1。取整各 2，余数都是 1，剩下 1。
	// 目标标识升序，parcel-a 承接，不是输入里先写的 parcel-b。
	apportionment := apportionWeight(t, 5, basis("parcel-b", 1), basis("parcel-a", 1))
	shares := apportionment.Shares()
	expectShare(t, shares[0], "parcel-a", 1, 5, 2, 2, 1, 3, true)
	expectShare(t, shares[1], "parcel-b", 1, 5, 2, 2, 1, 2, false)
	expectPortions(t, apportionment, "parcel-a", 3, "parcel-b", 2)
}

func TestByPieceAndByRevenueKeepTheSameArithmetic(t *testing.T) {
	bases := []domain.AllocationBasis{basis("parcel-a", 1), basis("parcel-b", 1)}
	byPiece, err := domain.Apportion(domain.AllocationByPiece, 5, bases)
	if err != nil {
		t.Fatal(err)
	}
	byRevenue, err := domain.Apportion(domain.AllocationByRevenue, 5, bases)
	if err != nil {
		t.Fatal(err)
	}
	if byPiece.Form() != domain.AllocationByPiece || byRevenue.Form() != domain.AllocationByRevenue {
		t.Fatalf("分法被抹成同一种：%s %s", byPiece.Form(), byRevenue.Form())
	}
	if byPiece.Shares()[0].ActualMinor != 3 || byRevenue.Shares()[0].ActualMinor != 3 {
		t.Fatalf("按件 %d 按收入 %d，算术应相同", byPiece.Shares()[0].ActualMinor, byRevenue.Shares()[0].ActualMinor)
	}
}

func TestApportionWithNoPositiveBasisLeavesTheSourceUnallocated(t *testing.T) {
	none, err := domain.Apportion(domain.AllocationByWeight, 8, nil)
	if err != nil {
		t.Fatal(err)
	}
	if none.UnallocatedMinor() != 8 || len(none.Portions()) != 0 || none.Denominator() != 0 {
		t.Fatalf("没有对象却分出去了：unallocated=%d portions=%d den=%d", none.UnallocatedMinor(), len(none.Portions()), none.Denominator())
	}
	zero, err := domain.Apportion(domain.AllocationByPiece, 8, []domain.AllocationBasis{basis("parcel-z", 0)})
	if err != nil {
		t.Fatal(err)
	}
	if zero.UnallocatedMinor() != 8 || len(zero.Portions()) != 0 {
		t.Fatalf("分母为零仍分出了份额：unallocated=%d portions=%d", zero.UnallocatedMinor(), len(zero.Portions()))
	}
}

func TestApportionKeepsAZeroActualShareOutOfPortions(t *testing.T) {
	// 来源 1，权重 1 与 100。两个取整都是 0，余数 1 与 100，1 个单位加给 parcel-b。
	// parcel-a 的实际是 0：展开还在，份额里没有它。
	apportionment := apportionWeight(t, 1, basis("parcel-a", 1), basis("parcel-b", 100))
	shares := apportionment.Shares()
	expectShare(t, shares[0], "parcel-a", 1, 1, 101, 0, 1, 0, false)
	expectShare(t, shares[1], "parcel-b", 100, 100, 101, 0, 100, 1, true)
	expectPortions(t, apportionment, "parcel-b", 1)
	if apportionment.UnallocatedMinor() != 0 {
		t.Fatalf("unallocated = %d", apportionment.UnallocatedMinor())
	}
}

func TestApportionRejectsDuplicatesAndAMissingForm(t *testing.T) {
	_, err := domain.Apportion(domain.AllocationByWeight, 10, []domain.AllocationBasis{basis("parcel-a", 1), basis("parcel-a", 2)})
	if !errors.Is(err, domain.ErrInvalidAllocationForm) {
		t.Fatalf("重复目标 err = %v", err)
	}
	_, err = domain.Apportion(domain.AllocationNotApplicable, 10, []domain.AllocationBasis{basis("parcel-a", 1)})
	if !errors.Is(err, domain.ErrInvalidAllocationForm) {
		t.Fatalf("不适用不能拿来分摊 err = %v", err)
	}
	_, err = domain.Apportion(domain.AllocationByRevenue, 10, []domain.AllocationBasis{basis("parcel-a", -1)})
	if !errors.Is(err, domain.ErrInvalidAllocationForm) {
		t.Fatalf("负权重 err = %v", err)
	}
}

func apportionWeight(t *testing.T, source int64, bases ...domain.AllocationBasis) domain.AllocationApportionment {
	t.Helper()
	apportionment, err := domain.Apportion(domain.AllocationByWeight, source, bases)
	if err != nil {
		t.Fatal(err)
	}
	var allocated int64
	for _, portion := range apportionment.Portions() {
		allocated += portion.AmountMinor
	}
	if allocated+apportionment.UnallocatedMinor() != source {
		t.Fatalf("不守恒：allocated %d + unallocated %d != %d", allocated, apportionment.UnallocatedMinor(), source)
	}
	return apportionment
}

func basis(target string, weight int64) domain.AllocationBasis {
	reference, err := domain.NewAllocationTargetReference(target)
	if err != nil {
		panic(err)
	}
	return domain.AllocationBasis{Target: reference, Basis: weight}
}

func expectShare(
	t *testing.T,
	share domain.AllocationShare,
	target string,
	weight, numerator, denominator, floor, remainder, actual int64,
	carried bool,
) {
	t.Helper()
	if share.Target.String() != target || share.Basis != weight ||
		share.TheoreticalNumerator != numerator || share.TheoreticalDenominator != denominator ||
		share.FloorMinor != floor || share.RemainderNumerator != remainder ||
		share.ActualMinor != actual || share.Carried != carried {
		t.Fatalf("份额 %s 走样：got target=%s basis=%d num=%d den=%d floor=%d rem=%d actual=%d carried=%v",
			target, share.Target, share.Basis, share.TheoreticalNumerator, share.TheoreticalDenominator,
			share.FloorMinor, share.RemainderNumerator, share.ActualMinor, share.Carried)
	}
}

func expectPortions(t *testing.T, apportionment domain.AllocationApportionment, pairs ...any) {
	t.Helper()
	portions := apportionment.Portions()
	if len(portions)*2 != len(pairs) {
		t.Fatalf("份额数 = %d，期望 %d", len(portions), len(pairs)/2)
	}
	for index := 0; index < len(portions); index++ {
		target := pairs[index*2].(string)
		amount := pairs[index*2+1].(int)
		if portions[index].Target.String() != target || portions[index].AmountMinor != int64(amount) {
			t.Fatalf("份额[%d] = %s %d，期望 %s %d", index, portions[index].Target, portions[index].AmountMinor, target, amount)
		}
	}
}

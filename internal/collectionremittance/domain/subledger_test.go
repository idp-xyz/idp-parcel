package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
)

func instant(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("解析时刻 %q：%v", value, err)
	}
	return parsed
}

func currency(t *testing.T, code string) domain.CurrencyCode {
	t.Helper()
	value, err := domain.NewCurrencyCode(code)
	if err != nil {
		t.Fatalf("构造币种 %q：%v", code, err)
	}
	return value
}

func money(t *testing.T, code string, amountMinor int64) domain.Money {
	t.Helper()
	value, err := domain.NewMoney(currency(t, code), amountMinor)
	if err != nil {
		t.Fatalf("构造金额 %s %d：%v", code, amountMinor, err)
	}
	return value
}

func ledgerKey(t *testing.T, code string) domain.SubledgerKey {
	t.Helper()
	customer, err := domain.NewCustomerReference("SYN-CUST-1")
	if err != nil {
		t.Fatalf("构造客户引用：%v", err)
	}
	entity, err := domain.NewLegalEntityReference("SYN-ENTITY-1")
	if err != nil {
		t.Fatalf("构造法人引用：%v", err)
	}
	channel, err := domain.NewCollectionChannelReference("SYN-CHANNEL-1")
	if err != nil {
		t.Fatalf("构造渠道引用：%v", err)
	}
	key, err := domain.NewSubledgerKey(customer, entity, currency(t, code), channel)
	if err != nil {
		t.Fatalf("构造分户账键：%v", err)
	}
	return key
}

func postingID(t *testing.T, value string) domain.PostingID {
	t.Helper()
	id, err := domain.NewPostingID(value)
	if err != nil {
		t.Fatalf("构造记账标识 %q：%v", value, err)
	}
	return id
}

func basisRef(t *testing.T, value string) domain.BasisReference {
	t.Helper()
	ref, err := domain.NewBasisReference(value)
	if err != nil {
		t.Fatalf("构造依据引用 %q：%v", value, err)
	}
	return ref
}

func posting(t *testing.T, spec domain.SubledgerPostingSpec) domain.SubledgerPosting {
	t.Helper()
	recorded, err := domain.RecordSubledgerPosting(spec)
	if err != nil {
		t.Fatalf("形成记账 %s：%v", spec.ID, err)
	}
	return recorded
}

// intake 造一笔入账：外部来源 → 指定位置，凭代收事实。
func intake(t *testing.T, key domain.SubledgerKey, id string, amountMinor int64, to domain.FundPosition) domain.SubledgerPosting {
	t.Helper()
	return posting(t, domain.SubledgerPostingSpec{
		ID:        postingID(t, id),
		Ledger:    key,
		From:      domain.PositionExternalSource,
		To:        to,
		Amount:    money(t, key.Currency().String(), amountMinor),
		BasisKind: domain.BasisCollectionFact,
		Basis:     basisRef(t, "SYN-FACT-"+id),
		PostedAt:  instant(t, "2026-08-24T01:00:00Z"),
	})
}

func TestAMoneyAmountMustBePositiveAndCarryACurrency(t *testing.T) {
	if _, err := domain.NewMoney(currency(t, "EUR"), 0); err == nil {
		t.Fatal("零金额被接受——账上一笔什么也没说的记账会让「已处置」看起来成立")
	}
	if _, err := domain.NewMoney(currency(t, "EUR"), -1); err == nil {
		t.Fatal("负金额被接受")
	}
	if _, err := domain.NewMoney(domain.CurrencyCode{}, 100); err == nil {
		t.Fatal("无币种金额被接受")
	}
}

func TestASubledgerKeyNeedsAllFourDimensions(t *testing.T) {
	customer, _ := domain.NewCustomerReference("SYN-CUST-1")
	entity, _ := domain.NewLegalEntityReference("SYN-ENTITY-1")
	channel, _ := domain.NewCollectionChannelReference("SYN-CHANNEL-1")

	if _, err := domain.NewSubledgerKey(domain.CustomerReference{}, entity, currency(t, "EUR"), channel); err == nil {
		t.Fatal("缺客户维的分户账键被接受")
	}
	if _, err := domain.NewSubledgerKey(customer, domain.LegalEntityReference{}, currency(t, "EUR"), channel); err == nil {
		t.Fatal("缺责任法人维的分户账键被接受")
	}
	if _, err := domain.NewSubledgerKey(customer, entity, domain.CurrencyCode{}, channel); err == nil {
		t.Fatal("缺币种维的分户账键被接受")
	}
	if _, err := domain.NewSubledgerKey(customer, entity, currency(t, "EUR"), domain.CollectionChannelReference{}); err == nil {
		t.Fatal("缺代收渠道维的分户账键被接受")
	}
}

// 三条依据门：入账凭代收事实、进`已汇付`凭回汇批次、进`短款`/`溢款`凭差异事项。
func TestAPostingMustCiteTheBasisItsDestinationRequires(t *testing.T) {
	key := ledgerKey(t, "EUR")
	base := domain.SubledgerPostingSpec{
		ID:       postingID(t, "SYN-POST-1"),
		Ledger:   key,
		Amount:   money(t, "EUR", 1000),
		Basis:    basisRef(t, "SYN-BASIS-1"),
		PostedAt: instant(t, "2026-08-24T01:00:00Z"),
	}

	intakeWithWrongBasis := base
	intakeWithWrongBasis.From = domain.PositionExternalSource
	intakeWithWrongBasis.To = domain.PositionInTransitAtChannel
	intakeWithWrongBasis.BasisKind = domain.BasisCorrection
	if _, err := domain.RecordSubledgerPosting(intakeWithWrongBasis); err == nil {
		t.Fatal("入账不凭代收事实也被接受——账上会出现无来源的本金")
	}

	remitWithWrongBasis := base
	remitWithWrongBasis.From = domain.PositionPayableToCustomer
	remitWithWrongBasis.To = domain.PositionRemitted
	remitWithWrongBasis.BasisKind = domain.BasisCollectionFact
	if _, err := domain.RecordSubledgerPosting(remitWithWrongBasis); err == nil {
		t.Fatal("汇付不凭回汇批次也被接受")
	}

	shortfallWithWrongBasis := base
	shortfallWithWrongBasis.From = domain.PositionAwaitingAllocation
	shortfallWithWrongBasis.To = domain.PositionShortfall
	shortfallWithWrongBasis.BasisKind = domain.BasisCorrection
	if _, err := domain.RecordSubledgerPosting(shortfallWithWrongBasis); err == nil {
		t.Fatal("短款不凭差异事项也被接受——差额自动落账正是本上下文不给的那条路")
	}

	surplusWithWrongBasis := shortfallWithWrongBasis
	surplusWithWrongBasis.To = domain.PositionSurplus
	if _, err := domain.RecordSubledgerPosting(surplusWithWrongBasis); err == nil {
		t.Fatal("溢款不凭差异事项也被接受")
	}
}

func TestNoPostingMovesPrincipalOutOfTheLedgerOrAcrossCurrencies(t *testing.T) {
	key := ledgerKey(t, "EUR")
	outward := domain.SubledgerPostingSpec{
		ID:        postingID(t, "SYN-POST-OUT"),
		Ledger:    key,
		From:      domain.PositionPayableToCustomer,
		To:        domain.PositionExternalSource,
		Amount:    money(t, "EUR", 1000),
		BasisKind: domain.BasisCorrection,
		Basis:     basisRef(t, "SYN-BASIS-1"),
		PostedAt:  instant(t, "2026-08-24T01:00:00Z"),
	}
	if _, err := domain.RecordSubledgerPosting(outward); err == nil {
		t.Fatal("去向为账外的记账被接受——全账总额恒等于入账之和这条就不成立了")
	}

	crossCurrency := outward
	crossCurrency.To = domain.PositionRemitted
	crossCurrency.BasisKind = domain.BasisRemittanceBatch
	crossCurrency.Amount = money(t, "USD", 1000)
	if _, err := domain.RecordSubledgerPosting(crossCurrency); err == nil {
		t.Fatal("记账币种与分户账币种不同也被接受——一本账内不该发生换算")
	}

	sameSpot := outward
	sameSpot.From = domain.PositionAwaitingAllocation
	sameSpot.To = domain.PositionAwaitingAllocation
	if _, err := domain.RecordSubledgerPosting(sameSpot); err == nil {
		t.Fatal("来源与去向相同的记账被接受")
	}
}

func TestABalanceIsDerivedFromPostingsAndStaysConserved(t *testing.T) {
	key := ledgerKey(t, "EUR")
	postings := []domain.SubledgerPosting{
		intake(t, key, "1", 1000, domain.PositionInTransitAtChannel),
		intake(t, key, "2", 500, domain.PositionInTransitAtChannel),
		posting(t, domain.SubledgerPostingSpec{
			ID:        postingID(t, "SYN-POST-3"),
			Ledger:    key,
			From:      domain.PositionInTransitAtChannel,
			To:        domain.PositionAwaitingAllocation,
			Amount:    money(t, "EUR", 1200),
			BasisKind: domain.BasisCorrection,
			Basis:     basisRef(t, "SYN-POST-1"),
			PostedAt:  instant(t, "2026-08-24T02:00:00Z"),
		}),
	}

	balance, err := domain.DeriveSubledgerBalance(key, postings)
	if err != nil {
		t.Fatalf("派生余额：%v", err)
	}
	if got := balance.At(domain.PositionInTransitAtChannel); got != 300 {
		t.Fatalf("渠道在途余额 = %d，要 300", got)
	}
	if got := balance.At(domain.PositionAwaitingAllocation); got != 1200 {
		t.Fatalf("待清分余额 = %d，要 1200", got)
	}
	if got := balance.IntakeTotal(); got != 1500 {
		t.Fatalf("入账总额 = %d，要 1500", got)
	}
	if got := balance.PostingCount(); got != 3 {
		t.Fatalf("记账笔数 = %d，要 3", got)
	}
	// 分配守恒：账内各位置之和恒等于入账之和。
	held := balance.At(domain.PositionInTransitAtChannel) +
		balance.At(domain.PositionAwaitingAllocation) +
		balance.At(domain.PositionPayableToCustomer) +
		balance.At(domain.PositionRemitted) +
		balance.At(domain.PositionShortfall) +
		balance.At(domain.PositionSurplus)
	if held != balance.IntakeTotal() {
		t.Fatalf("账内合计 %d 与入账总额 %d 不等——分配守恒破了", held, balance.IntakeTotal())
	}
}

// 「已开立但当期无记账」是一格如实答案：余额各位置皆零、笔数为零，而不是派生失败。
func TestAnOpenedLedgerWithNoPostingsDerivesAZeroBalance(t *testing.T) {
	key := ledgerKey(t, "EUR")
	balance, err := domain.DeriveSubledgerBalance(key, nil)
	if err != nil {
		t.Fatalf("零记账账面派生失败：%v", err)
	}
	if balance.PostingCount() != 0 || balance.IntakeTotal() != 0 {
		t.Fatalf("零记账账面不为零：笔数 %d 入账 %d", balance.PostingCount(), balance.IntakeTotal())
	}
	if got := balance.At(domain.PositionPayableToCustomer); got != 0 {
		t.Fatalf("未出现过的位置余额 = %d，要 0", got)
	}
}

func TestDerivingRefusesAForeignPostingAndABrokenLedger(t *testing.T) {
	key := ledgerKey(t, "EUR")
	other := ledgerKey(t, "USD")
	if _, err := domain.DeriveSubledgerBalance(key, []domain.SubledgerPosting{
		intake(t, other, "9", 100, domain.PositionInTransitAtChannel),
	}); !errors.Is(err, domain.ErrInvalidPosting) {
		t.Fatalf("别本账的记账被算进来了：%v", err)
	}

	// 只有支出没有入账：账内位置为负，说明账面本身坏了。
	drain := posting(t, domain.SubledgerPostingSpec{
		ID:        postingID(t, "SYN-POST-DRAIN"),
		Ledger:    key,
		From:      domain.PositionPayableToCustomer,
		To:        domain.PositionRemitted,
		Amount:    money(t, "EUR", 100),
		BasisKind: domain.BasisRemittanceBatch,
		Basis:     basisRef(t, "SYN-BATCH-1"),
		PostedAt:  instant(t, "2026-08-24T03:00:00Z"),
	})
	if _, err := domain.DeriveSubledgerBalance(key, []domain.SubledgerPosting{drain}); !errors.Is(
		err, domain.ErrConservationBroken) {
		t.Fatalf("负余额账面没被拦住：%v", err)
	}
}

// 余额不足与输入不合法是两回事：前者等实收或先清分，后者改请求。
func TestAnUnderfundedPositionIsNotAnInvalidRequest(t *testing.T) {
	key := ledgerKey(t, "EUR")
	balance, err := domain.DeriveSubledgerBalance(key, []domain.SubledgerPosting{
		intake(t, key, "1", 1000, domain.PositionAwaitingAllocation),
	})
	if err != nil {
		t.Fatalf("派生余额：%v", err)
	}

	tooMuch := posting(t, domain.SubledgerPostingSpec{
		ID:        postingID(t, "SYN-POST-BIG"),
		Ledger:    key,
		From:      domain.PositionAwaitingAllocation,
		To:        domain.PositionPayableToCustomer,
		Amount:    money(t, "EUR", 1001),
		BasisKind: domain.BasisCorrection,
		Basis:     basisRef(t, "SYN-POST-1"),
		PostedAt:  instant(t, "2026-08-24T04:00:00Z"),
	})
	if err := balance.Admit(tooMuch); !errors.Is(err, domain.ErrPositionUnderfunded) {
		t.Fatalf("透支被放行：%v", err)
	}

	exact := posting(t, domain.SubledgerPostingSpec{
		ID:        postingID(t, "SYN-POST-EXACT"),
		Ledger:    key,
		From:      domain.PositionAwaitingAllocation,
		To:        domain.PositionPayableToCustomer,
		Amount:    money(t, "EUR", 1000),
		BasisKind: domain.BasisCorrection,
		Basis:     basisRef(t, "SYN-POST-1"),
		PostedAt:  instant(t, "2026-08-24T04:00:00Z"),
	})
	next, err := balance.Apply(exact)
	if err != nil {
		t.Fatalf("恰好足额的记账被拒：%v", err)
	}
	if next.At(domain.PositionPayableToCustomer) != 1000 || next.At(domain.PositionAwaitingAllocation) != 0 {
		t.Fatalf("落账后账面不对：应付客户 %d 待清分 %d",
			next.At(domain.PositionPayableToCustomer), next.At(domain.PositionAwaitingAllocation))
	}
	// 余额是派生值，没有就地改写的入口：原快照不因 Apply 而变。
	if balance.At(domain.PositionAwaitingAllocation) != 1000 {
		t.Fatalf("Apply 改写了原快照：待清分 %d", balance.At(domain.PositionAwaitingAllocation))
	}
}

func TestOpeningASubledgerNeedsACustodyBasis(t *testing.T) {
	key := ledgerKey(t, "EUR")
	basis, err := domain.NewCustodyBasisReference("SYN-COD-RESPONSIBILITY-V1")
	if err != nil {
		t.Fatalf("构造受托依据：%v", err)
	}
	if _, err := domain.OpenSubledger(key, domain.CustodyBasisReference{}, instant(t, "2026-08-24T00:00:00Z")); err == nil {
		t.Fatal("无受托依据的分户账被开立——账上看不出这笔钱凭什么由运营企业代管")
	}
	if _, err := domain.OpenSubledger(key, basis, time.Time{}); err == nil {
		t.Fatal("无开立时刻的分户账被接受")
	}
	ledger, err := domain.OpenSubledger(key, basis, instant(t, "2026-08-24T00:00:00Z"))
	if err != nil {
		t.Fatalf("开立分户账：%v", err)
	}
	if ledger.Key() != key {
		t.Fatal("开立后的分户账键与输入不同")
	}
}

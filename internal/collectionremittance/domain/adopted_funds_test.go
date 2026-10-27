package domain_test

import (
	"errors"
	"math"
	"testing"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
)

func bankCredit(t *testing.T, id, instruction, funds string, amountMinor int64) domain.CollectionFact {
	t.Helper()
	return bankCreditIn(t, "CNY", id, instruction, funds, amountMinor)
}

func bankCreditIn(t *testing.T, code, id, instruction, funds string, amountMinor int64) domain.CollectionFact {
	t.Helper()
	fact, err := domain.AcceptCollectionFact(domain.CollectionFactSpec{
		ID:          mustFactID(t, id),
		Instruction: instructionID(t, instruction),
		Layer:       domain.OperatorBankCredit,
		Evidence:    mustEvidence(t, funds),
		Amount:      money(t, code, amountMinor),
		OccurredAt:  instant(t, "2026-10-01T00:00:00Z"),
	})
	if err != nil {
		t.Fatalf("接受真实到账 %s：%v", id, err)
	}
	return fact
}

func adoptedFunds(t *testing.T, code string, amountMinor int64) domain.AdoptedFunds {
	t.Helper()
	version, err := domain.NewAdoptedFundsVersion("BANK-RECEIPT-1/v1")
	if err != nil {
		t.Fatalf("构造采用版本：%v", err)
	}
	funds, err := domain.NewAdoptedFunds(version, money(t, code, amountMinor))
	if err != nil {
		t.Fatalf("构造已采用事实：%v", err)
	}
	return funds
}

// Covers: AT-CR-020——真实到账的证据引用就是结算已采用事实的引用；读到的链头版本只钉在真实到账这一层上，
// 别的三层带版本、或给非真实到账钉版本，都不成立。
func TestOnlyAnOperatorBankCreditReferencesAdoptedFundsAndPinsItsVersion(t *testing.T) {
	credit := bankCredit(t, "SYN-FACT-CREDIT", "SYN-INSTR-1", "BANK-RECEIPT-1", 96)
	reference, referencing := credit.AdoptedFundsReference()
	if !referencing || reference.String() != "BANK-RECEIPT-1" {
		t.Fatalf("真实到账的采用引用 = (%q, %v)", reference, referencing)
	}
	if _, pinned := credit.AdoptedFundsVersion(); pinned {
		t.Fatal("还没按引用读结算就带了版本")
	}

	version, _ := domain.NewAdoptedFundsVersion("BANK-RECEIPT-1/v1")
	pinned, err := credit.PinnedToAdoptedFunds(version)
	if err != nil {
		t.Fatalf("钉版本：%v", err)
	}
	if got, ok := pinned.AdoptedFundsVersion(); !ok || got != version {
		t.Fatalf("钉后版本 = (%q, %v)", got, ok)
	}
	if _, ok := credit.AdoptedFundsVersion(); ok {
		t.Fatal("钉版本改了原值：事实是值语义")
	}
	if pinned.ID() != credit.ID() || pinned.Amount() != credit.Amount() || pinned.Evidence() != credit.Evidence() {
		t.Fatal("钉版本改了版本以外的内容")
	}
	if _, err := credit.PinnedToAdoptedFunds(domain.AdoptedFundsVersion{}); err == nil {
		t.Fatal("钉了空版本")
	}

	for _, layer := range []domain.CollectionSourceLayer{
		domain.RecipientPayment, domain.ChannelCollectionReport, domain.ChannelRemittanceNotice,
	} {
		other := acceptedFact(t, layer)
		if _, referencing := other.AdoptedFundsReference(); referencing {
			t.Fatalf("%s 被当成引用结算已采用事实的一层", layer)
		}
		if _, err := other.PinnedToAdoptedFunds(version); err == nil {
			t.Fatalf("%s 钉了结算事实的版本", layer)
		}
		if _, err := domain.AcceptCollectionFact(domain.CollectionFactSpec{
			ID:                  mustFactID(t, "SYN-FACT-X"),
			Instruction:         instructionID(t, "SYN-INSTR-1"),
			Layer:               layer,
			Evidence:            mustEvidence(t, "SYN-EVIDENCE-1"),
			Amount:              money(t, "CNY", 1),
			OccurredAt:          instant(t, "2026-10-01T00:00:00Z"),
			AdoptedFundsVersion: version,
		}); err == nil {
			t.Fatalf("%s 接受时带了结算事实的版本", layer)
		}
	}
}

// Covers: AT-CR-020 的金额面——引用同一条已采用事实的各条真实到账，金额之和不超过该事实此刻的金额；一条已采用事实对
// 同一条指令至多支撑一条真实到账；币种与事实不同不受理；重放自己不算两次。
func TestAdoptedFundsBacksArrivalsUpToItsAmountOnce(t *testing.T) {
	funds := adoptedFunds(t, "CNY", 100_000)
	first := bankCredit(t, "SYN-FACT-A", "SYN-INSTR-1", "BANK-RECEIPT-1", 60_000)
	second := bankCredit(t, "SYN-FACT-B", "SYN-INSTR-2", "BANK-RECEIPT-1", 40_000)

	for name, testCase := range map[string]struct {
		referenced []domain.CollectionFact
		candidate  domain.CollectionFact
		want       error
	}{
		"首条到账":       {nil, first, nil},
		"拆分合计恰好等于事实": {[]domain.CollectionFact{first}, second, nil},
		"多一分都超": {
			[]domain.CollectionFact{first, second},
			bankCredit(t, "SYN-FACT-C", "SYN-INSTR-3", "BANK-RECEIPT-1", 1),
			domain.ErrAdoptedFundsExceeded,
		},
		"换事实标识重登同一指令": {
			[]domain.CollectionFact{first},
			bankCredit(t, "SYN-FACT-A2", "SYN-INSTR-1", "BANK-RECEIPT-1", 60_000),
			domain.ErrAdoptedFundsInstructionTaken,
		},
		"同一指令少一点也不行": {
			[]domain.CollectionFact{first},
			bankCredit(t, "SYN-FACT-A3", "SYN-INSTR-1", "BANK-RECEIPT-1", 100),
			domain.ErrAdoptedFundsInstructionTaken,
		},
		"重放自己不算两次": {[]domain.CollectionFact{first, second}, first, nil},
		"币种与事实不同": {
			nil,
			bankCreditIn(t, "USD", "SYN-FACT-U", "SYN-INSTR-9", "BANK-RECEIPT-1", 10),
			domain.ErrAdoptedFundsCurrency,
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := domain.AdmitAdoptedFundsReference(funds, testCase.referenced, testCase.candidate)
			if !errors.Is(err, testCase.want) || (testCase.want == nil && err != nil) {
				t.Fatalf("err = %v，要 %v", err, testCase.want)
			}
		})
	}

	t.Run("金额相加不回绕", func(t *testing.T) {
		huge := adoptedFunds(t, "CNY", math.MaxInt64)
		half := bankCredit(t, "SYN-FACT-H1", "SYN-INSTR-1", "BANK-RECEIPT-1", math.MaxInt64/2+1)
		other := bankCredit(t, "SYN-FACT-H2", "SYN-INSTR-2", "BANK-RECEIPT-1", math.MaxInt64/2+1)
		if err := domain.AdmitAdoptedFundsReference(huge, []domain.CollectionFact{half}, other); !errors.Is(err, domain.ErrAdoptedFundsExceeded) {
			t.Fatalf("两个超过一半的到账相加回绕成了没超额：err = %v", err)
		}
	})
}

// Covers: AT-CR-027——被结算更正的事实：此刻的链头金额低于已引用的合计时，已登记的到账不再被事实撑住（AdoptedFundsCover 答超额），
// 新到账也进不来；链头不低于合计则照旧。
func TestACorrectedFactStopsBackingArrivalsItNoLongerCovers(t *testing.T) {
	first := bankCredit(t, "SYN-FACT-A", "SYN-INSTR-1", "BANK-RECEIPT-1", 60_000)
	second := bankCredit(t, "SYN-FACT-B", "SYN-INSTR-2", "BANK-RECEIPT-1", 40_000)
	referenced := []domain.CollectionFact{first, second}

	if err := domain.AdoptedFundsCover(adoptedFunds(t, "CNY", 100_000), referenced); err != nil {
		t.Fatalf("链头恰好等于合计也算撑住：%v", err)
	}
	if err := domain.AdoptedFundsCover(adoptedFunds(t, "CNY", 120_000), referenced); err != nil {
		t.Fatalf("更正上调后撑不住：%v", err)
	}
	lowered := adoptedFunds(t, "CNY", 90_000)
	if err := domain.AdoptedFundsCover(lowered, referenced); !errors.Is(err, domain.ErrAdoptedFundsExceeded) {
		t.Fatalf("更正下调到 90000，已引用 100000 却仍算撑住：err = %v", err)
	}
	if err := domain.AdoptedFundsCover(lowered, []domain.CollectionFact{first}); err != nil {
		t.Fatalf("下调后合计仍不超：%v", err)
	}
	third := bankCredit(t, "SYN-FACT-C", "SYN-INSTR-3", "BANK-RECEIPT-1", 10)
	if err := domain.AdmitAdoptedFundsReference(lowered, referenced, third); !errors.Is(err, domain.ErrAdoptedFundsExceeded) {
		t.Fatalf("事实被下调到容不下既有到账时又收了新到账：err = %v", err)
	}
}

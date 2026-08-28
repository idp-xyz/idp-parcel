package postgres_test

import (
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本包用例共用的夹具。领域值一律走构造门造出来——不用零值，是因为好几个写口把「封闭
// 枚举之外即拒」的入参门放在事务守卫之前，零值会在守卫之前就被那道门拦下，于是负向
// 证据证到的不是它声称的那件事。

func newDB(t *testing.T) *bentopg.DB {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	return db
}

func fixtureTenant(t *testing.T) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID("SYN-T1")
	if err != nil {
		t.Fatalf("构造租户标识：%v", err)
	}
	return tenant
}

func fixtureAt(t *testing.T, value string) time.Time {
	t.Helper()
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		t.Fatalf("解析时刻 %q：%v", value, err)
	}
	return parsed
}

func fixtureLedgerKey(t *testing.T) domain.SubledgerKey {
	t.Helper()
	customer, err := domain.NewCustomerReference("SYN-CUST-1")
	if err != nil {
		t.Fatalf("构造客户引用：%v", err)
	}
	entity, err := domain.NewLegalEntityReference("SYN-ENTITY-1")
	if err != nil {
		t.Fatalf("构造法人引用：%v", err)
	}
	currency, err := domain.NewCurrencyCode("EUR")
	if err != nil {
		t.Fatalf("构造币种：%v", err)
	}
	channel, err := domain.NewCollectionChannelReference("SYN-CHANNEL-1")
	if err != nil {
		t.Fatalf("构造渠道引用：%v", err)
	}
	key, err := domain.NewSubledgerKey(customer, entity, currency, channel)
	if err != nil {
		t.Fatalf("构造分户账键：%v", err)
	}
	return key
}

func fixtureMoney(t *testing.T, amountMinor int64) domain.Money {
	t.Helper()
	currency, err := domain.NewCurrencyCode("EUR")
	if err != nil {
		t.Fatalf("构造币种：%v", err)
	}
	amount, err := domain.NewMoney(currency, amountMinor)
	if err != nil {
		t.Fatalf("构造金额：%v", err)
	}
	return amount
}

func fixtureSubledger(t *testing.T) domain.Subledger {
	t.Helper()
	basis, err := domain.NewCustodyBasisReference("SYN-COD-RESPONSIBILITY-V1")
	if err != nil {
		t.Fatalf("构造受托依据：%v", err)
	}
	ledger, err := domain.OpenSubledger(fixtureLedgerKey(t), basis, fixtureAt(t, "2026-08-24T00:00:00Z"))
	if err != nil {
		t.Fatalf("开立分户账：%v", err)
	}
	return ledger
}

func fixtureInstructionID(t *testing.T, value string) domain.CollectionInstructionID {
	t.Helper()
	id, err := domain.NewCollectionInstructionID(value)
	if err != nil {
		t.Fatalf("构造指令标识 %q：%v", value, err)
	}
	return id
}

func fixtureInstruction(t *testing.T, id string, amountMinor int64) domain.CollectionInstruction {
	t.Helper()
	parcel, err := domain.NewParcelReference("SYN-PARCEL-1")
	if err != nil {
		t.Fatalf("构造包裹引用：%v", err)
	}
	requirement, err := domain.NewServiceRequirementReference("SYN-COD-REQ-V1")
	if err != nil {
		t.Fatalf("构造服务要求引用：%v", err)
	}
	instruction, err := domain.IssueCollectionInstruction(domain.CollectionInstructionSpec{
		ID:           fixtureInstructionID(t, id),
		Parcel:       parcel,
		Requirement:  requirement,
		Ledger:       fixtureLedgerKey(t),
		Amount:       fixtureMoney(t, amountMinor),
		InstructedAt: fixtureAt(t, "2026-08-24T00:30:00Z"),
	})
	if err != nil {
		t.Fatalf("成立代收指令：%v", err)
	}
	return instruction
}

func fixtureFactID(t *testing.T, value string) domain.CollectionFactID {
	t.Helper()
	id, err := domain.NewCollectionFactID(value)
	if err != nil {
		t.Fatalf("构造事实标识 %q：%v", value, err)
	}
	return id
}

func fixtureFact(
	t *testing.T, id, instruction string, layer domain.CollectionSourceLayer, amountMinor int64,
) domain.CollectionFact {
	t.Helper()
	evidence, err := domain.NewEvidenceReference("SYN-EVIDENCE-" + id)
	if err != nil {
		t.Fatalf("构造证据引用：%v", err)
	}
	fact, err := domain.AcceptCollectionFact(domain.CollectionFactSpec{
		ID:          fixtureFactID(t, id),
		Instruction: fixtureInstructionID(t, instruction),
		Layer:       layer,
		Evidence:    evidence,
		Amount:      fixtureMoney(t, amountMinor),
		OccurredAt:  fixtureAt(t, "2026-08-24T01:00:00Z"),
	})
	if err != nil {
		t.Fatalf("接受代收事实：%v", err)
	}
	return fact
}

func fixtureDiscrepancy(t *testing.T, id, instruction string, amountMinor int64) domain.DiscrepancyItem {
	t.Helper()
	itemID, err := domain.NewDiscrepancyItemID(id)
	if err != nil {
		t.Fatalf("构造差异标识：%v", err)
	}
	basis, err := domain.NewBasisReference("SYN-CHANNEL-REPORT-1")
	if err != nil {
		t.Fatalf("构造依据引用：%v", err)
	}
	item, err := domain.RegisterDiscrepancyItem(domain.DiscrepancyItemSpec{
		ID:          itemID,
		Instruction: fixtureInstructionID(t, instruction),
		Kind:        domain.DiscrepancyShortfall,
		Amount:      fixtureMoney(t, amountMinor),
		Basis:       basis,
		ObservedAt:  fixtureAt(t, "2026-08-25T00:00:00Z"),
	})
	if err != nil {
		t.Fatalf("登记差异事项：%v", err)
	}
	return item
}

func fixtureBatchID(t *testing.T, value string) domain.RemittanceBatchID {
	t.Helper()
	id, err := domain.NewRemittanceBatchID(value)
	if err != nil {
		t.Fatalf("构造批次标识 %q：%v", value, err)
	}
	return id
}

func fixtureBatch(t *testing.T, id string) domain.RemittanceBatch {
	t.Helper()
	batch, err := domain.FormRemittanceBatch(domain.RemittanceBatchSpec{
		ID:               fixtureBatchID(t, id),
		Ledger:           fixtureLedgerKey(t),
		CollectedThrough: fixtureAt(t, "2026-08-31T00:00:00Z"),
		FormedAt:         fixtureAt(t, "2026-08-31T01:00:00Z"),
	})
	if err != nil {
		t.Fatalf("形成批次：%v", err)
	}
	return batch
}

func fixturePosting(
	t *testing.T,
	id string,
	from, to domain.FundPosition,
	amountMinor int64,
	kind domain.PostingBasisKind,
	basis string,
	postedAt string,
) domain.SubledgerPosting {
	t.Helper()
	postingID, err := domain.NewPostingID(id)
	if err != nil {
		t.Fatalf("构造记账标识：%v", err)
	}
	basisRef, err := domain.NewBasisReference(basis)
	if err != nil {
		t.Fatalf("构造依据引用：%v", err)
	}
	posting, err := domain.RecordSubledgerPosting(domain.SubledgerPostingSpec{
		ID:        postingID,
		Ledger:    fixtureLedgerKey(t),
		From:      from,
		To:        to,
		Amount:    fixtureMoney(t, amountMinor),
		BasisKind: kind,
		Basis:     basisRef,
		PostedAt:  fixtureAt(t, postedAt),
	})
	if err != nil {
		t.Fatalf("形成记账：%v", err)
	}
	return posting
}

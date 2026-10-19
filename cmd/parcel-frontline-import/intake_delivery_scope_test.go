package main

import (
	"context"
	"slices"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件证收寄模板多出的交付范围依据引用一列（ADR-0212 决定七；票 ps-external-mark-relations/11）：模板换版 INTAKE-2，带这一列；
// INTAKE-1 照旧认、读作没有依据。一格写「种类:标识」，可以空。合成值（S 级）。

const scopedTemplateHeader = "templateVersion,batchRef,factRef,operator,node,deliveredBy,handlingUnit,mark,claim,evidenceRef,refusalReason,occurredAt,deliveryScopeBasis\n"

func scopedReceivedRow(factRef, unit, basis string) string {
	return "INTAKE-2,SYN-BATCH-PSXM11," + factRef + ",SYN-CLERK-A,SYN-NODE-SZX,SYN-CUSTOMER-1," + unit + ",SYN-MARK-PSXM11,RECEIVED,SYN-RECEIPT-" +
		factRef + ",," + "2026-09-08T09:15:00+08:00," + basis + "\n"
}

// Covers: ADR-0212 决定七「受控批量口的收寄模板多一个可选格，原样存」——INTAKE-2 的依据一格按「种类:标识」译进命令，标识冒号之后逐字；
// 空格即没有依据。换了模板版本，两处来源标记也换成 INTAKE-2：同一个号下两种列形状会让落库的标记说谎。
func TestTheIntake2TemplateCarriesAnOptionalDeliveryScopeBasis(t *testing.T) {
	batch, err := decodeIntakeTemplate(synTenant(t), []byte(scopedTemplateHeader+
		scopedReceivedRow("SYN-SLIP-PSXM11-1", "SYN-HU-PSXM11-1", "DELIVERY_PRE_ADVICE:PREADV-PSXM11")+
		scopedReceivedRow("SYN-SLIP-PSXM11-2", "SYN-HU-PSXM11-2", "DELIVERY_HANDOVER_NOTE:SYN-NOTE:7")+
		scopedReceivedRow("SYN-SLIP-PSXM11-3", "SYN-HU-PSXM11-3", "")))
	if err != nil {
		t.Fatalf("INTAKE-2 译装失败：%v", err)
	}
	want := []struct {
		kind       domain.DeliveryScopeBasisKind
		identifier string
	}{
		{domain.BasisDeliveryPreAdvice, "PREADV-PSXM11"},
		{domain.BasisDeliveryHandoverNote, "SYN-NOTE:7"},
		{domain.DeliveryScopeBasisKindInvalid, ""},
	}
	for index, row := range batch.Rows {
		basis := row.Command.ScopeBasis
		if basis.Kind() != want[index].kind || basis.Identifier() != want[index].identifier {
			t.Fatalf("第 %d 行依据 = %s %q, want %s %q", index+1, basis.Kind(), basis.Identifier(), want[index].kind, want[index].identifier)
		}
	}
	first := batch.Rows[0].Command
	if first.SourceID != "FTI/INTAKE-2/SYN-BATCH-PSXM11/SYN-SLIP-PSXM11-1" || first.Evidence.String() != "FTI/INTAKE-2/SYN-CLERK-A/SYN-RECEIPT-SYN-SLIP-PSXM11-1" {
		t.Fatalf("来源标记 = %q / %q, want 带 INTAKE-2", first.SourceID, first.Evidence)
	}
	if batch.Version != "INTAKE-2" {
		t.Fatalf("批次模板版本 = %q, want INTAKE-2", batch.Version)
	}

	legacy, err := decodeIntakeTemplate(synTenant(t), sampleTemplate(t))
	if err != nil || legacy.Version != "INTAKE-1" || legacy.Rows[0].Command.ScopeBasis.Present() {
		t.Fatalf("INTAKE-1 样例 = %q 依据在场 %v err %v, want 照旧认、没有依据", legacy.Version, legacy.Rows[0].Command.ScopeBasis.Present(), err)
	}
}

// Covers: 依据一格给了就要给全，模板填错整批拒——缺种类、种类不在词表、标识空；列形状与行上的版本号对不上同样拒（INTAKE-1 的表头配
// INTAKE-2 的行、反过来也一样）。
func TestTheIntake2TemplateRefusesAMalformedBasisOrAMismatchedVersion(t *testing.T) {
	for name, raw := range map[string]string{
		"缺种类":       scopedTemplateHeader + scopedReceivedRow("SYN-SLIP-PSXM11-1", "SYN-HU-PSXM11-1", "PREADV-PSXM11"),
		"种类不在词表":    scopedTemplateHeader + scopedReceivedRow("SYN-SLIP-PSXM11-1", "SYN-HU-PSXM11-1", "PRE_ADVICE:PREADV-PSXM11"),
		"标识空":       scopedTemplateHeader + scopedReceivedRow("SYN-SLIP-PSXM11-1", "SYN-HU-PSXM11-1", "DELIVERY_PRE_ADVICE:"),
		"新表头配旧版本的行": scopedTemplateHeader + strings.Replace(scopedReceivedRow("SYN-SLIP-PSXM11-1", "SYN-HU-PSXM11-1", ""), "INTAKE-2", "INTAKE-1", 1),
		"旧表头配新版本的行": templateHeader + strings.Replace(receivedRow("SYN-SLIP-PSXM11-1", "SYN-HU-PSXM11-1"), "INTAKE-1", "INTAKE-2", 1),
	} {
		if _, err := decodeIntakeTemplate(synTenant(t), []byte(raw)); err == nil {
			t.Fatalf("%s：译装成功了", name)
		}
	}
}

// scopedIdentityView 照 PS 答一个只经客户包裹件号命中的候选：带了 SYN-CUSTOMER-1 才范围相容（ADR-0212 决定四）。
type scopedIdentityView struct {
	parcel domain.ParcelAssociationReference
}

func (view scopedIdentityView) ResolveParcelIdentity(
	_ context.Context, _ domain.TenantID, _ ports.ExternalMarkObservation, scope []domain.CustomerAccountReference,
) (ports.ParcelIdentityAnswer, error) {
	compatible := slices.ContainsFunc(scope, func(account domain.CustomerAccountReference) bool { return account.String() == "SYN-CUSTOMER-1" })
	return ports.ParcelIdentityAnswer{
		Scoped:     len(scope) > 0,
		Candidates: []ports.ParcelIdentityCandidate{{Parcel: view.parcel, ScopeCompatible: compatible}},
	}, nil
}

// Covers: 受控批量口的真库面（ADR-0212 决定四、七；`AT-NO-063` 的受控批量口面）——生产装配的导入链按模板里的依据读 NO 的预报册与时窗册（真库）推导范围：带客户
// 账户一预报的那一行唯一识别、落地；同一批里没带依据的那一行答待识别、核对态未证；依据原样落在来源文档里。
func TestTheIntakeImportJudgesTheScopeFromTheTemplateBasis(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	preAdviseForImport(t, db)
	parcel, err := domain.NewParcelAssociationReference("SYN-PARCEL-PSXM11")
	if err != nil {
		t.Fatal(err)
	}
	importer, err := buildIntakeImporter(db, fixedClock{at: mustInstant(t, "2026-09-08T12:00:00+08:00")}, scopedIdentityView{parcel: parcel})
	if err != nil {
		t.Fatalf("装配导入口：%v", err)
	}
	results := importTemplate(t, importer, []byte(scopedTemplateHeader+
		scopedReceivedRow("SYN-SLIP-PSXM11-1", "SYN-HU-PSXM11-1", "DELIVERY_PRE_ADVICE:PREADV-PSXM11-IMPORT")+
		scopedReceivedRow("SYN-SLIP-PSXM11-2", "SYN-HU-PSXM11-2", "")))
	assertDisposition(t, results, 0, rowLanded, "INTAKE_FORMED")
	assertDisposition(t, results, 1, rowLanded, "PENDING_IDENTIFICATION")

	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	advised, found := findReception(t, receptions, "FTI/INTAKE-2/SYN-BATCH-PSXM11/SYN-SLIP-PSXM11-1")
	if !found || advised.Source.ScopeBasis.Identifier() != "PREADV-PSXM11-IMPORT" || advised.DeliveryScopeCheck.State() != domain.DeliveryScopeVerified {
		t.Fatalf("带预报那一行 = %+v, want 来源带预报、核对态已证", advised)
	}
	unadvised, found := findReception(t, receptions, "FTI/INTAKE-2/SYN-BATCH-PSXM11/SYN-SLIP-PSXM11-2")
	if !found || unadvised.IdentityConflict || unadvised.DeliveryScopeCheck.Reason() != domain.ScopeNoBasis {
		t.Fatalf("没带依据那一行 = %+v, want 不记冲突、核对态未证·没有依据", unadvised)
	}
}

// preAdviseForImport 在模板的节点与业务时刻上给 SYN-CUSTOMER-1 登一份列着模板号码的预报，并登一行时窗。
func preAdviseForImport(t *testing.T, db *bentopg.DB) {
	t.Helper()
	tenant := synTenant(t)
	expected := mustInstant(t, "2026-09-08T09:00:00+08:00")
	number, err := domain.NewPreAdvisedParcelNumber("SYN-MARK-PSXM11")
	if err != nil {
		t.Fatal(err)
	}
	preAdvice, err := domain.FormDeliveryPreAdvice(domain.DeliveryPreAdviceSpec{
		TenantID:          tenant,
		ID:                mustDomain(t, domain.NewDeliveryPreAdviceID, "PREADV-PSXM11-IMPORT"),
		CustomerAccount:   mustDomain(t, domain.NewCustomerAccountReference, "SYN-CUSTOMER-1"),
		Node:              mustDomain(t, domain.NewNodeReference, "SYN-NODE-SZX"),
		ExpectedArrivalAt: expected,
		Numbers:           []domain.PreAdvisedParcelNumber{number},
		Source:            domain.CustomerRequestSource{Channel: "SYN-CLIENT-PSXM11", RequestKey: "SYN-KEY-PSXM11", ContentDigest: "NOC-1:syn-psxm11"},
		FormedAt:          expected.Add(-24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("预报：%v", err)
	}
	window, err := domain.FormPreAdviceValidityWindow(domain.PreAdviceValidityWindowSpec{
		TenantID: tenant, Lead: 24 * time.Hour, Grace: 12 * time.Hour,
		EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Basis: "SYN-WINDOW-BASIS-PSXM11",
		RegisteredAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("时窗：%v", err)
	}
	preAdvices, err := nopostgres.NewDeliveryPreAdvices(db)
	if err != nil {
		t.Fatal(err)
	}
	windows, err := nopostgres.NewPreAdviceWindows(db)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if _, _, err := windows.RegisterPreAdviceWindow(txCtx, window); err != nil {
			return err
		}
		_, _, err := preAdvices.RecordDeliveryPreAdvice(txCtx, preAdvice)
		return err
	}); err != nil {
		t.Fatalf("登预报与时窗：%v", err)
	}
}

func mustDomain[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return value
}

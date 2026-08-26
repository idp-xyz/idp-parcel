package postgres_test

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

type receiptFixture struct {
	db        *bentopg.DB
	registrar *adapter.MaterialReceiptRegistrar
	view      *adapter.ClaimMaterialReceipts
}

func newReceiptFixture(t *testing.T) *receiptFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registrar, err := adapter.NewMaterialReceiptRegistrar(db)
	if err != nil {
		t.Fatalf("构造归集面写入方：%v", err)
	}
	view, err := adapter.NewClaimMaterialReceipts(db)
	if err != nil {
		t.Fatalf("构造证据视图：%v", err)
	}
	return &receiptFixture{db: db, registrar: registrar, view: view}
}

func materialReceipt(t *testing.T, batch, item, material string, at time.Time, by string) ports.MaterialReceipt {
	t.Helper()
	return ports.MaterialReceipt{
		Batch:      projectionValue(t, domain.NewClaimBatchReference, batch),
		Item:       projectionValue(t, domain.NewClaimItemID, item),
		Material:   projectionValue(t, domain.NewMaterialRequirementReference, material),
		ReceivedAt: at,
		ReceivedBy: by,
	}
}

func revocationOf(t *testing.T, receipt ports.MaterialReceipt, by string, at time.Time) ports.MaterialReceiptRevocation {
	t.Helper()
	return ports.MaterialReceiptRevocation{
		Batch:      receipt.Batch,
		Item:       receipt.Item,
		Material:   receipt.Material,
		ReceivedAt: receipt.ReceivedAt,
		RevokedBy:  by,
		RevokedAt:  at,
	}
}

// register / revoke 各包一笔事务跑写入方：写口按框架合同无事务即拒（守卫另有测试），
// 生产装配里事务由 CLI 入口开启，这里替它开。
func (fixture *receiptFixture) register(t *testing.T, tenant string, receipt ports.MaterialReceipt) ports.MaterialReceiptWriteOutcome {
	t.Helper()
	tenantID := projectionValue(t, domain.NewTenantID, tenant)
	var outcome ports.MaterialReceiptWriteOutcome
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var registerErr error
		outcome, registerErr = fixture.registrar.RegisterReceipt(txCtx, tenantID, receipt)
		return registerErr
	}); err != nil {
		t.Fatalf("登记收讫：%v", err)
	}
	return outcome
}

func (fixture *receiptFixture) revoke(t *testing.T, tenant string, revocation ports.MaterialReceiptRevocation) ports.MaterialReceiptWriteOutcome {
	t.Helper()
	tenantID := projectionValue(t, domain.NewTenantID, tenant)
	var outcome ports.MaterialReceiptWriteOutcome
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var revokeErr error
		outcome, revokeErr = fixture.registrar.RevokeReceipt(txCtx, tenantID, revocation)
		return revokeErr
	}); err != nil {
		t.Fatalf("撤销收讫：%v", err)
	}
	return outcome
}

func (fixture *receiptFixture) read(t *testing.T, tenant, batch, item string) []string {
	t.Helper()
	received, known, err := fixture.view.ReceivedMaterials(t.Context(),
		projectionValue(t, domain.NewTenantID, tenant),
		projectionValue(t, domain.NewClaimBatchReference, batch),
		projectionValue(t, domain.NewClaimItemID, item))
	if err != nil {
		t.Fatalf("读现存集：%v", err)
	}
	if !known {
		t.Fatal("归集面已建，known 必须恒为 true——false 那格（无从查起）随桩退役")
	}
	values := make([]string, 0, len(received))
	for _, material := range received {
		values = append(values, material.String())
	}
	return values
}

func sameStrings(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for index := range got {
		if got[index] != want[index] {
			return false
		}
	}
	return true
}

// Covers: 票 ve-claims-read-seams/02 的读侧口径——收讫减撤销、按收讫行逐次抵扣、
// 去重排序交付；零行是「查过了，一件都没收到」的有效事实（known=true + 零件，与
// 退役前桩的 known=false 语义不同）；租户与索赔身份各自隔离。
func TestReceivedMaterialsAnswerReceiptsMinusRevocations(t *testing.T) {
	fixture := newReceiptFixture(t)
	firstAt := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	secondAt := time.Date(2026, 8, 21, 9, 30, 0, 0, time.UTC)

	photoFirst := materialReceipt(t, "batch-1", "item-1", "MAT-PHOTO", firstAt, "operator-a")
	photoSecond := materialReceipt(t, "batch-1", "item-1", "MAT-PHOTO", secondAt, "operator-b")
	invoice := materialReceipt(t, "batch-1", "item-1", "MAT-INVOICE", firstAt, "operator-a")
	for _, receipt := range []ports.MaterialReceipt{photoFirst, photoSecond, invoice} {
		if outcome := fixture.register(t, "tenant-a", receipt); outcome != ports.MaterialReceiptRecorded {
			t.Fatalf("登记收讫应成功，实得 %d", outcome)
		}
	}

	if got := fixture.read(t, "tenant-a", "batch-1", "item-1"); !sameStrings(got, []string{"MAT-INVOICE", "MAT-PHOTO"}) {
		t.Fatalf("现存集 = %v，要去重排序的 [MAT-INVOICE MAT-PHOTO]", got)
	}

	// 撤销第一次 PHOTO 收讫：第二次收讫仍在手，这一格不变——撤销按收讫行逐次抵扣，
	// 不是按材料整类划掉。
	revokedAt := time.Date(2026, 8, 22, 10, 0, 0, 0, time.UTC)
	if outcome := fixture.revoke(t, "tenant-a", revocationOf(t, photoFirst, "supervisor-a", revokedAt)); outcome != ports.MaterialReceiptRevocationRecorded {
		t.Fatalf("撤销应成功，实得 %d", outcome)
	}
	if got := fixture.read(t, "tenant-a", "batch-1", "item-1"); !sameStrings(got, []string{"MAT-INVOICE", "MAT-PHOTO"}) {
		t.Fatalf("撤销一次收讫后另一次仍应在手：%v", got)
	}

	if outcome := fixture.revoke(t, "tenant-a", revocationOf(t, photoSecond, "supervisor-a", revokedAt)); outcome != ports.MaterialReceiptRevocationRecorded {
		t.Fatalf("撤销第二次收讫应成功，实得 %d", outcome)
	}
	if got := fixture.read(t, "tenant-a", "batch-1", "item-1"); !sameStrings(got, []string{"MAT-INVOICE"}) {
		t.Fatalf("两次收讫都撤销后 PHOTO 应退出现存集：%v", got)
	}

	// 零行两格：别的租户、别的索赔项都答「查过了，一件都没收到」——known=true 而不是
	// 无从查起，差集等于整份清单、限期补充照常推进。
	if got := fixture.read(t, "tenant-b", "batch-1", "item-1"); len(got) != 0 {
		t.Fatalf("跨租户读到了收讫行：%v", got)
	}
	if got := fixture.read(t, "tenant-a", "batch-1", "item-2"); len(got) != 0 {
		t.Fatalf("别的索赔项读到了收讫行：%v", got)
	}
}

// Covers: 写侧代数——同五件重登幂等（AlreadyRecorded，先登的经手声明留在行上）；
// 撤销无收讫行答 Unknown（治理答案，不是故障）；重复撤销答 RevocationAlreadyRecorded。
func TestReceiptWriteAlgebraIsIdempotentAndGuardsRevocation(t *testing.T) {
	fixture := newReceiptFixture(t)
	receivedAt := time.Date(2026, 8, 20, 8, 0, 0, 0, time.UTC)
	receipt := materialReceipt(t, "batch-1", "item-1", "MAT-PHOTO", receivedAt, "operator-a")

	if outcome := fixture.register(t, "tenant-a", receipt); outcome != ports.MaterialReceiptRecorded {
		t.Fatalf("首登应 Recorded，实得 %d", outcome)
	}
	replay := receipt
	replay.ReceivedBy = "operator-b"
	if outcome := fixture.register(t, "tenant-a", replay); outcome != ports.MaterialReceiptAlreadyRecorded {
		t.Fatalf("同五件重登应 AlreadyRecorded，实得 %d", outcome)
	}

	unknown := materialReceipt(t, "batch-1", "item-1", "MAT-INVOICE", receivedAt, "operator-a")
	revokedAt := receivedAt.Add(24 * time.Hour)
	if outcome := fixture.revoke(t, "tenant-a", revocationOf(t, unknown, "supervisor-a", revokedAt)); outcome != ports.MaterialReceiptUnknown {
		t.Fatalf("撤销不在册的收讫应 Unknown，实得 %d", outcome)
	}

	if outcome := fixture.revoke(t, "tenant-a", revocationOf(t, receipt, "supervisor-a", revokedAt)); outcome != ports.MaterialReceiptRevocationRecorded {
		t.Fatalf("首撤应 RevocationRecorded，实得 %d", outcome)
	}
	if outcome := fixture.revoke(t, "tenant-a", revocationOf(t, receipt, "supervisor-b", revokedAt.Add(time.Hour))); outcome != ports.MaterialReceiptRevocationAlreadyRecorded {
		t.Fatalf("重复撤销应 RevocationAlreadyRecorded，实得 %d", outcome)
	}
}

// Covers: 读口的接线守卫——三件身份缺一按「依赖调不通」报错，不答业务格：编排在取
// 证据之前已校验三件非空，走到这里还缺是接线错误，答「零件」会立出一个没被问对的
// 「一件都没收到」。
func TestReceivedMaterialsRefuseBlankIdentity(t *testing.T) {
	fixture := newReceiptFixture(t)
	tenant := projectionValue(t, domain.NewTenantID, "tenant-a")
	batch := projectionValue(t, domain.NewClaimBatchReference, "batch-1")
	item := projectionValue(t, domain.NewClaimItemID, "item-1")

	if _, _, err := fixture.view.ReceivedMaterials(t.Context(), domain.TenantID{}, batch, item); err == nil {
		t.Fatal("空租户没有按接线错误报错")
	}
	if _, _, err := fixture.view.ReceivedMaterials(t.Context(), tenant, domain.ClaimBatchReference{}, item); err == nil {
		t.Fatal("空批次没有按接线错误报错")
	}
	if _, _, err := fixture.view.ReceivedMaterials(t.Context(), tenant, batch, domain.ClaimItemID{}); err == nil {
		t.Fatal("空索赔项没有按接线错误报错")
	}
}

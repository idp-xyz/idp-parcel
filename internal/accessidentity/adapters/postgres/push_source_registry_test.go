package postgres_test

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	adapter "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证推送源册与重放账（access_identity 0006、ADR-0149 决定三第三条）：绑定落在集成客户端册
// 的一行上，窗口与时钟偏差不设缺省，密钥只存引用，同一份签名原文只收下一次。

const pushSourceName = "SYN-PUSH-SOURCE/bank-01"

type pushRegister struct {
	sources    *adapter.PushSourceRegistry
	clients    *adapter.IntegrationClientRegistry
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newPushRegister(t *testing.T) pushRegister {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	sources, err := adapter.NewPushSourceRegistry(db)
	if err != nil {
		t.Fatalf("构造推送源册：%v", err)
	}
	clients, err := adapter.NewIntegrationClientRegistry(db)
	if err != nil {
		t.Fatalf("构造集成客户端册：%v", err)
	}
	return pushRegister{sources: sources, clients: clients, transactor: db.Transactor(), pool: pool}
}

func (register pushRegister) within(
	t *testing.T,
	act func(ctx context.Context) (accessidentity.PushSourceRegistrationOutcome, error),
) accessidentity.PushSourceRegistrationOutcome {
	t.Helper()
	var outcome accessidentity.PushSourceRegistrationOutcome
	if err := register.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var actErr error
		outcome, actErr = act(txCtx)
		return actErr
	}); err != nil {
		t.Fatalf("登记事务：%v", err)
	}
	return outcome
}

func (register pushRegister) bindClient(t *testing.T, tenant, sub string) accessidentity.IntegrationClientSubject {
	t.Helper()
	subject := clientSubjectOf(t, syntheticIssuer, sub)
	var outcome accessidentity.IntegrationClientRegistrationOutcome
	if err := register.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var actErr error
		outcome, actErr = register.clients.RegisterIntegrationClient(txCtx, clientBindingOf(t, subject, defaultClientBinding(tenant)))
		return actErr
	}); err != nil {
		t.Fatalf("登客户端：%v", err)
	}
	if outcome != accessidentity.IntegrationClientRegistrationRecorded {
		t.Fatalf("登客户端：答复 = %s", outcome)
	}
	return subject
}

func (register pushRegister) grantFunds(t *testing.T, tenant string, subject accessidentity.IntegrationClientSubject) {
	t.Helper()
	grant := clientGrantOf(t, tenant, "SYN-CLIENT-GRANT-01", subject, accessidentity.FactExternalFunds, grantStartsAt, time.Time{})
	var outcome accessidentity.IntegrationClientRegistrationOutcome
	if err := register.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var actErr error
		outcome, actErr = register.clients.RegisterIntegrationClientGrant(txCtx, grant)
		return actErr
	}); err != nil {
		t.Fatalf("授事实：%v", err)
	}
	if outcome != accessidentity.IntegrationClientRegistrationRecorded {
		t.Fatalf("授事实：答复 = %s", outcome)
	}
}

func pushSourceID(t *testing.T, name string) accessidentity.PushSourceID {
	t.Helper()
	source, err := accessidentity.NewPushSourceID(name)
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func pushBinding(t *testing.T, tenant, name string, subject accessidentity.IntegrationClientSubject, window, skew time.Duration, basis string) accessidentity.PushSourceBinding {
	t.Helper()
	binding, err := accessidentity.NewPushSourceBinding(pushSourceID(t, name), subject, tenant, window, skew, basis)
	if err != nil {
		t.Fatal(err)
	}
	return binding
}

func pushKeyRow(
	t *testing.T,
	tenant string,
	source accessidentity.PushSourceID,
	id string,
	form accessidentity.PushSignatureForm,
	startsAt, endsAt time.Time,
) accessidentity.PushSigningKey {
	t.Helper()
	reference, err := accessidentity.NewCredentialReference("SYN-PUSH-KEY-REF/" + id)
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(startsAt, endsAt)
	if err != nil {
		t.Fatal(err)
	}
	key, err := accessidentity.NewPushSigningKey(tenant, source, id, form, reference, interval, "SYN-PUSH-KEY-BASIS-"+id)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func expectPushOutcome(t *testing.T, label string, got, want accessidentity.PushSourceRegistrationOutcome) {
	t.Helper()
	if got != want {
		t.Fatalf("%s：答复 = %s, want %s", label, got, want)
	}
}

// Covers: 绑定是登记行不是部署参数——首登落册，窗口、时钟偏差、所绑客户端原样读回；同内容重放答原结果；异窗口、异时钟
// 偏差、异客户端、异依据答冲突且行不动；标识被别的租户占住答跨租户，不说是哪个；本租户没有那个客户端答未登记。
func TestPushSourceRegistrationRecordsReplaysAndNeverOverwrites(t *testing.T) {
	register := newPushRegister(t)
	subject := register.bindClient(t, tenantOne, "SYN-CLIENT-01")
	other := register.bindClient(t, tenantOne, "SYN-CLIENT-02")
	foreignClient := register.bindClient(t, tenantTwo, "SYN-CLIENT-T2")

	binding := pushBinding(t, tenantOne, pushSourceName, subject, 5*time.Minute, 30*time.Second, "SYN-PUSH-SOURCE-BASIS-01")
	expectPushOutcome(t, "首登", register.within(t, func(ctx context.Context) (accessidentity.PushSourceRegistrationOutcome, error) {
		return register.sources.RegisterPushSource(ctx, binding)
	}), accessidentity.PushSourceRegistrationRecorded)
	expectPushOutcome(t, "重放", register.within(t, func(ctx context.Context) (accessidentity.PushSourceRegistrationOutcome, error) {
		return register.sources.RegisterPushSource(ctx, binding)
	}), accessidentity.PushSourceRegistrationAlreadyRegistered)

	for name, changed := range map[string]accessidentity.PushSourceBinding{
		"异窗口":   pushBinding(t, tenantOne, pushSourceName, subject, 10*time.Minute, 30*time.Second, "SYN-PUSH-SOURCE-BASIS-01"),
		"异时钟偏差": pushBinding(t, tenantOne, pushSourceName, subject, 5*time.Minute, 0, "SYN-PUSH-SOURCE-BASIS-01"),
		"异客户端":  pushBinding(t, tenantOne, pushSourceName, other, 5*time.Minute, 30*time.Second, "SYN-PUSH-SOURCE-BASIS-01"),
		"异依据":   pushBinding(t, tenantOne, pushSourceName, subject, 5*time.Minute, 30*time.Second, "SYN-PUSH-SOURCE-BASIS-02"),
	} {
		expectPushOutcome(t, name, register.within(t, func(ctx context.Context) (accessidentity.PushSourceRegistrationOutcome, error) {
			return register.sources.RegisterPushSource(ctx, changed)
		}), accessidentity.PushSourceRegistrationContentConflict)
	}

	standing, found, err := register.sources.FindPushSource(t.Context(), binding.Source())
	if err != nil || !found || standing.Binding() != binding || len(standing.Keys()) != 0 {
		t.Fatalf("册上现状 found %v err %v keys %d, want 原登记", found, err, len(standing.Keys()))
	}

	foreign := pushBinding(t, tenantTwo, pushSourceName, foreignClient, 5*time.Minute, 30*time.Second, "SYN-PUSH-SOURCE-BASIS-01")
	expectPushOutcome(t, "跨租户占标识", register.within(t, func(ctx context.Context) (accessidentity.PushSourceRegistrationOutcome, error) {
		return register.sources.RegisterPushSource(ctx, foreign)
	}), accessidentity.PushSourceBoundToAnotherTenant)

	missing := pushBinding(t, tenantOne, "SYN-PUSH-SOURCE/ghost", clientSubjectOf(t, syntheticIssuer, "SYN-CLIENT-GHOST"), 5*time.Minute, 30*time.Second, "SYN-BASIS")
	expectPushOutcome(t, "客户端未登记", register.within(t, func(ctx context.Context) (accessidentity.PushSourceRegistrationOutcome, error) {
		return register.sources.RegisterPushSource(ctx, missing)
	}), accessidentity.PushSourceClientNotRegistered)

	if _, found, err := register.sources.FindPushSource(t.Context(), pushSourceID(t, "SYN-PUSH-SOURCE/ghost")); found || err != nil {
		t.Fatalf("不在册：found %v, err %v", found, err)
	}
}

// Covers: 密钥轮换期新旧并存可配置——新钥与旧钥的停用都是登记行，重叠多久由行上的时刻决定；只存引用，不存本体。
// 推送源未登记时登不了钥，钥未登记时停不了。
func TestPushSigningKeyRotationIsARegistration(t *testing.T) {
	register := newPushRegister(t)
	subject := register.bindClient(t, tenantOne, "SYN-CLIENT-01")
	binding := pushBinding(t, tenantOne, pushSourceName, subject, 5*time.Minute, 30*time.Second, "SYN-PUSH-SOURCE-BASIS-01")
	expectPushOutcome(t, "登推送源", register.within(t, func(ctx context.Context) (accessidentity.PushSourceRegistrationOutcome, error) {
		return register.sources.RegisterPushSource(ctx, binding)
	}), accessidentity.PushSourceRegistrationRecorded)

	source := binding.Source()
	orphan := pushKeyRow(t, tenantOne, pushSourceID(t, "SYN-PUSH-SOURCE/ghost"), "SYN-K0", accessidentity.PushSignatureHMACSHA256, grantStartsAt, time.Time{})
	expectPushOutcome(t, "推送源未登记", register.within(t, func(ctx context.Context) (accessidentity.PushSourceRegistrationOutcome, error) {
		return register.sources.RegisterPushSigningKey(ctx, orphan)
	}), accessidentity.PushSourceNotRegistered)

	oldKey := pushKeyRow(t, tenantOne, source, "SYN-K-OLD", accessidentity.PushSignatureHMACSHA256, grantStartsAt, time.Time{})
	newStarts := grantStartsAt.Add(time.Hour)
	newKey := pushKeyRow(t, tenantOne, source, "SYN-K-NEW", accessidentity.PushSignatureEd25519, newStarts, time.Time{})
	expectPushOutcome(t, "登旧钥", register.within(t, func(ctx context.Context) (accessidentity.PushSourceRegistrationOutcome, error) {
		return register.sources.RegisterPushSigningKey(ctx, oldKey)
	}), accessidentity.PushSourceRegistrationRecorded)
	expectPushOutcome(t, "旧钥重放", register.within(t, func(ctx context.Context) (accessidentity.PushSourceRegistrationOutcome, error) {
		return register.sources.RegisterPushSigningKey(ctx, oldKey)
	}), accessidentity.PushSourceRegistrationAlreadyRegistered)
	changed := pushKeyRow(t, tenantOne, source, "SYN-K-OLD", accessidentity.PushSignatureEd25519, grantStartsAt, time.Time{})
	expectPushOutcome(t, "同标识异形态", register.within(t, func(ctx context.Context) (accessidentity.PushSourceRegistrationOutcome, error) {
		return register.sources.RegisterPushSigningKey(ctx, changed)
	}), accessidentity.PushSourceRegistrationContentConflict)
	expectPushOutcome(t, "登新钥", register.within(t, func(ctx context.Context) (accessidentity.PushSourceRegistrationOutcome, error) {
		return register.sources.RegisterPushSigningKey(ctx, newKey)
	}), accessidentity.PushSourceRegistrationRecorded)

	retiresAt := newStarts.Add(time.Hour)
	retirement, err := accessidentity.NewPushSigningKeyRetirement(tenantOne, source, "SYN-K-OLD", retiresAt, "SYN-PUSH-KEY-RETIREMENT-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	missingKey, err := accessidentity.NewPushSigningKeyRetirement(tenantOne, source, "SYN-K-GHOST", retiresAt, "SYN-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	expectPushOutcome(t, "停不存在的钥", register.within(t, func(ctx context.Context) (accessidentity.PushSourceRegistrationOutcome, error) {
		return register.sources.RegisterPushSigningKeyRetirement(ctx, missingKey)
	}), accessidentity.PushSigningKeyNotRegistered)
	expectPushOutcome(t, "停旧钥", register.within(t, func(ctx context.Context) (accessidentity.PushSourceRegistrationOutcome, error) {
		return register.sources.RegisterPushSigningKeyRetirement(ctx, retirement)
	}), accessidentity.PushSourceRegistrationRecorded)
	expectPushOutcome(t, "停用重放", register.within(t, func(ctx context.Context) (accessidentity.PushSourceRegistrationOutcome, error) {
		return register.sources.RegisterPushSigningKeyRetirement(ctx, retirement)
	}), accessidentity.PushSourceRegistrationAlreadyRegistered)

	standing, found, err := register.sources.FindPushSource(t.Context(), source)
	if err != nil || !found || len(standing.Keys()) != 2 {
		t.Fatalf("读回：found %v err %v keys %d", found, err, len(standing.Keys()))
	}
	byID := map[string]accessidentity.RecordedPushSigningKey{}
	for _, recorded := range standing.Keys() {
		byID[recorded.Key().KeyID()] = recorded
		if recorded.Key().Reference().String() == "" || len(recorded.Key().Reference().String()) < 8 {
			t.Fatalf("引用丢失：%s", recorded.Key().KeyID())
		}
	}
	if byID["SYN-K-OLD"].EffectiveAt(retiresAt.Add(-time.Microsecond)) == false || byID["SYN-K-OLD"].EffectiveAt(retiresAt) {
		t.Fatal("旧钥应在停用时刻前有效、停用时刻起无效")
	}
	if byID["SYN-K-NEW"].EffectiveAt(newStarts.Add(-time.Microsecond)) || !byID["SYN-K-NEW"].EffectiveAt(newStarts) {
		t.Fatal("新钥应从自己的起点起有效")
	}
	if !byID["SYN-K-OLD"].EffectiveAt(newStarts) || !byID["SYN-K-NEW"].EffectiveAt(newStarts) {
		t.Fatal("重叠期内两把都应有效")
	}
}

// Covers: 重放账以签名原文摘要为键——同一份再记答 first=false，换时间戳重签是另一行；写必须在事务里。
func TestPushReceiptRecognisesTheSameSignedContentOnce(t *testing.T) {
	register := newPushRegister(t)
	subject := register.bindClient(t, tenantOne, "SYN-CLIENT-01")
	binding := pushBinding(t, tenantOne, pushSourceName, subject, 5*time.Minute, 30*time.Second, "SYN-PUSH-SOURCE-BASIS-01")
	expectPushOutcome(t, "登推送源", register.within(t, func(ctx context.Context) (accessidentity.PushSourceRegistrationOutcome, error) {
		return register.sources.RegisterPushSource(ctx, binding)
	}), accessidentity.PushSourceRegistrationRecorded)

	signedAt := time.Date(2026, 10, 9, 2, 0, 0, 0, time.UTC)
	push := signedPushForReceipt(t, "msg_SYN01", signedAt, `{"fact":"SYN-FUNDS-FACT-01"}`)
	receipt, err := accessidentity.NewPushReceipt(tenantOne, binding.Source(), push, signedAt.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	var first, second bool
	if err := register.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var actErr error
		first, actErr = register.sources.RecordPushReceipt(txCtx, receipt)
		if actErr != nil {
			return actErr
		}
		second, actErr = register.sources.RecordPushReceipt(txCtx, receipt)
		return actErr
	}); err != nil {
		t.Fatal(err)
	}
	if !first || second {
		t.Fatalf("first %v second %v, want true then false", first, second)
	}

	retry := signedPushForReceipt(t, "msg_SYN01", signedAt.Add(time.Second), `{"fact":"SYN-FUNDS-FACT-01"}`)
	retryReceipt, err := accessidentity.NewPushReceipt(tenantOne, binding.Source(), retry, signedAt.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	var retryFirst bool
	if err := register.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var actErr error
		retryFirst, actErr = register.sources.RecordPushReceipt(txCtx, retryReceipt)
		return actErr
	}); err != nil {
		t.Fatal(err)
	}
	if !retryFirst {
		t.Fatal("换时间戳重签应是另一行")
	}

	if _, err := register.sources.RecordPushReceipt(t.Context(), receipt); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务记重放：err = %v, want ErrTransactionRequired", err)
	}
}

// Covers: PBC-08——登记三口只经框架事务，拿不到事务句柄时答 ErrTransactionRequired，不退回连接池自动提交。
func TestPushSourceWritesRefuseToRunOutsideATransaction(t *testing.T) {
	register := newPushRegister(t)
	subject := clientSubjectOf(t, syntheticIssuer, "SYN-CLIENT-01")
	binding := pushBinding(t, tenantOne, pushSourceName, subject, time.Minute, 0, "SYN-BASIS")
	key := pushKeyRow(t, tenantOne, binding.Source(), "SYN-K", accessidentity.PushSignatureHMACSHA256, grantStartsAt, time.Time{})
	retirement, err := accessidentity.NewPushSigningKeyRetirement(tenantOne, binding.Source(), "SYN-K", grantStartsAt.Add(time.Hour), "SYN-BASIS")
	if err != nil {
		t.Fatal(err)
	}
	ctx := t.Context()
	if _, err := register.sources.RegisterPushSource(ctx, binding); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记推送源：err = %v, want ErrTransactionRequired", err)
	}
	if _, err := register.sources.RegisterPushSigningKey(ctx, key); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记密钥：err = %v, want ErrTransactionRequired", err)
	}
	if _, err := register.sources.RegisterPushSigningKeyRetirement(ctx, retirement); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务停用密钥：err = %v, want ErrTransactionRequired", err)
	}
	if _, found, err := register.sources.FindPushSource(ctx, binding.Source()); found || err != nil {
		t.Fatalf("无事务登记之后：found %v, err %v; want 未落册", found, err)
	}
}

// Covers: 表结构自己守得住——窗口缺省、未知签名形态、停不存在的钥、重放账挂到没有的推送源，都进不了库。
func TestPushSourceTablesGuardTheInvariantsThemselves(t *testing.T) {
	register := newPushRegister(t)
	register.bindClient(t, tenantOne, "SYN-CLIENT-01")
	const insertSource = `INSERT INTO access_identity.push_source
		(source_id, tenant_id, issuer, subject, window_seconds, clock_skew_seconds, basis_ref)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	expectSQLState(t, register.pool, "窗口为零", checkViolation, insertSource,
		"SYN-PUSH-SOURCE/zero", tenantOne, syntheticIssuer, "SYN-CLIENT-01", int64(0), int64(0), "SYN-X")
	expectSQLState(t, register.pool, "时钟偏差为负", checkViolation, insertSource,
		"SYN-PUSH-SOURCE/neg", tenantOne, syntheticIssuer, "SYN-CLIENT-01", int64(60), int64(-1), "SYN-X")
	expectSQLState(t, register.pool, "客户端不存在", foreignKeyViolation, insertSource,
		"SYN-PUSH-SOURCE/ghost", tenantOne, syntheticIssuer, "SYN-CLIENT-GHOST", int64(60), int64(0), "SYN-X")

	binding := pushBinding(t, tenantOne, pushSourceName, clientSubjectOf(t, syntheticIssuer, "SYN-CLIENT-01"), time.Minute, 0, "SYN-BASIS")
	expectPushOutcome(t, "登推送源", register.within(t, func(ctx context.Context) (accessidentity.PushSourceRegistrationOutcome, error) {
		return register.sources.RegisterPushSource(ctx, binding)
	}), accessidentity.PushSourceRegistrationRecorded)
	const insertKey = `INSERT INTO access_identity.push_signing_key
		(tenant_id, source_id, key_id, form, key_ref, effective_starts_at, basis_ref)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	expectSQLState(t, register.pool, "未知形态", checkViolation, insertKey,
		tenantOne, pushSourceName, "SYN-K", "RSA", "SYN-REF", grantStartsAt, "SYN-X")
	const insertRetirement = `INSERT INTO access_identity.push_signing_key_retirement
		(tenant_id, source_id, key_id, retires_at, basis_ref) VALUES ($1, $2, $3, $4, $5)`
	expectSQLState(t, register.pool, "停不存在的钥", foreignKeyViolation, insertRetirement,
		tenantOne, pushSourceName, "SYN-K-GHOST", grantStartsAt, "SYN-X")
	const insertReceipt = `INSERT INTO access_identity.push_receipt
		(source_id, digest, tenant_id, push_id, signed_at, retain_until)
		VALUES ($1, $2, $3, $4, $5, $6)`
	expectSQLState(t, register.pool, "重放账没有推送源", foreignKeyViolation, insertReceipt,
		"SYN-PUSH-SOURCE/ghost", "sha256:ab", tenantOne, "msg", grantStartsAt, grantStartsAt)
}

func signedPushForReceipt(t *testing.T, id string, signedAt time.Time, body string) accessidentity.SignedPush {
	t.Helper()
	secret := make([]byte, 32)
	if _, err := rand.Read(secret); err != nil {
		t.Fatal(err)
	}
	timestamp := strconv.FormatInt(signedAt.Unix(), 10)
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(id + "." + timestamp + "." + body))
	signature := "v1," + base64.StdEncoding.EncodeToString(mac.Sum(nil))
	push, err := accessidentity.ParseStandardWebhook(id, timestamp, signature, []byte(body))
	if err != nil {
		t.Fatal(err)
	}
	return push
}

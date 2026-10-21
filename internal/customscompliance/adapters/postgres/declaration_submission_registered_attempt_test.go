package postgres_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// Covers: AT-CC-441（持久化半边）——`已登记待发送`的首次尝试落库时 sent_at 是 NULL，读回仍是待发送、没有发送时间
// （迁移 0027）；库上那两条配对 CHECK 拦得住给待发送编发送时间、给已发出的抹发送时间、以及停在待发送的受控重发。
func TestARegisteredAttemptIsStoredWithoutASendTime(t *testing.T) {
	fixture := newDeclarationFixture(t)
	ctx := t.Context()

	record := submissionRecord(t, "tenant-a", "unit-1", "EXPORT_STANDARD", "version-1")
	registered, err := domain.InitialAttempt(record.Version, "customs-gateway/prod", domain.AttemptRegisteredPendingSend, time.Time{})
	if err != nil {
		t.Fatalf("构造待发送尝试：%v", err)
	}
	record.Attempt = registered
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.submissions.Save(txCtx, record)
		return err
	})

	var sentAtIsNull bool
	if err := fixture.pool.QueryRow(ctx,
		`SELECT sent_at IS NULL FROM customs_compliance.submission_attempt WHERE tenant_id = 'tenant-a' AND version_id = 'version-1'`,
	).Scan(&sentAtIsNull); err != nil {
		t.Fatalf("读尝试行：%v", err)
	}
	if !sentAtIsNull {
		t.Fatal("待发送尝试在库里带着发送时间")
	}

	found, present, err := fixture.submissions.FindByKey(ctx, record.Key)
	if err != nil || !present {
		t.Fatalf("读回提交申报：present=%v err=%v", present, err)
	}
	if found.Attempt.Result() != domain.AttemptRegisteredPendingSend || !found.Attempt.SentAt().IsZero() {
		t.Fatalf("读回尝试 = %s @ %s，want REGISTERED_PENDING_SEND 无发送时间", found.Attempt.Result(), found.Attempt.SentAt())
	}

	for label, statement := range map[string]string{
		"待发送带发送时间": `UPDATE customs_compliance.submission_attempt SET sent_at = now()
		  WHERE tenant_id = 'tenant-a' AND version_id = 'version-1' AND sequence = 1`,
		"已发出却没有发送时间": `INSERT INTO customs_compliance.submission_attempt
			(tenant_id, version_id, sequence, target, result, safe_resend_ref, sent_at)
		 VALUES ('tenant-a', 'version-1', 2, 'gateway', 'ACKNOWLEDGED', 'resend/safe-1', NULL)`,
		"受控重发停在待发送": `INSERT INTO customs_compliance.submission_attempt
			(tenant_id, version_id, sequence, target, result, safe_resend_ref, sent_at)
		 VALUES ('tenant-a', 'version-1', 2, 'gateway', 'REGISTERED_PENDING_SEND', 'resend/safe-1', NULL)`,
	} {
		if _, err := fixture.pool.Exec(context.Background(), statement); err == nil {
			t.Fatalf("%s：库接受了", label)
		}
	}
}

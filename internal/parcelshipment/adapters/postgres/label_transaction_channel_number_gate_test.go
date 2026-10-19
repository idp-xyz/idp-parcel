package postgres_test

import (
	"context"
	"math/rand/v2"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件对真实 PostgreSQL 证面单渠道结果这条写册路上「号码存得下」的门（票 ps-external-mark-relations/06 裁定二）：真编排、
// 真仓储、真关联册。夹具全为合成（S 级）。号码用不可压缩的串：Postgres 写索引行前会先压缩，一长串同一个字符压得下去，
// 撞不到 btree 索引行的长度上限，证不出要证的那一格。

// incompressibleNumber 交回 length 字节、按固定种子取字母数字的串：同一长度每次都是同一个号，又没有可压缩的重复。
func incompressibleNumber(length int) string {
	const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	source := rand.New(rand.NewPCG(uint64(length), 0x0605))
	var builder strings.Builder
	for builder.Len() < length {
		builder.WriteByte(alphabet[source.IntN(len(alphabet))])
	}
	return builder.String()
}

func externalIdentifierRelationRows(t *testing.T, pool *pgxpool.Pool, tenant string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM parcel_shipment.external_identifier_relation WHERE tenant_id = $1`, tenant,
	).Scan(&count); err != nil {
		t.Fatalf("数关联册：%v", err)
	}
	return count
}

// Covers: 票 ps-external-mark-relations/06 裁定二与完成判据「面单渠道结果照该路坏结果答法，不 500」——渠道结果里受理的那件包裹
// 带回的渠道包裹件号存不进册（513 字节、带 NUL、远超索引行上限）时，记录结果答`未受理`、不报错：交易仍停在`已提交渠道`，
// 册上一行不多。修前三格各自撞库：513 字节的照收进册，带 NUL 的在交易文档那一步报错，远超上限的在关联册的索引那一步报错，
// 整笔回滚、调用方只拿到错误，重放同一份结果永远一样。恰 512 字节的不可压缩号照收：落册，并能经按号核对读口原样查回。
func TestAChannelResultWhoseNumberCannotEnterTheRegisterIsNotAccepted(t *testing.T) {
	handoff, db, pool := newLabelTransactionJudgmentHandoffFixture(t)
	repository, err := adapter.NewLabelTransactions(db)
	if err != nil {
		t.Fatalf("构造面单交易仓储：%v", err)
	}
	clock := handoffClock{at: time.Date(2026, 9, 10, 18, 0, 0, 0, time.UTC)}
	handler := newLabelTransactionHandlerWithRealViews(t, db, repository, handoff, clock)
	ctx := t.Context()
	transactor := db.Transactor()
	tenant := mustBuild(t, domain.NewTenantID, "tenant-a")

	submit := func(id string) domain.LabelTransactionID {
		t.Helper()
		transactionID := mustBuild(t, domain.NewLabelTransactionID, id)
		mustInsertLabelTransaction(t, transactor, ctx, repository,
			establishedLabelTransactionFixture(t, "tenant-a", id, "parcel-1"))
		mustWithinTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			_, err := handler.SubmitToChannel(txCtx, psapplication.SubmitLabelTransactionCommand{
				Tenant: tenant, TransactionID: transactionID,
			})
			return err
		})
		return transactionID
	}
	record := func(transactionID domain.LabelTransactionID, identifier string) (psapplication.LabelTransactionResult, error) {
		t.Helper()
		var result psapplication.LabelTransactionResult
		err := transactor.WithinTransaction(ctx, func(txCtx context.Context) (err error) {
			result, err = handler.RecordChannelResult(txCtx, psapplication.RecordLabelChannelResultCommand{
				Tenant:        tenant,
				TransactionID: transactionID,
				Outcome:       domain.LabelTransactionSucceeded,
				ParcelResults: []domain.LabelTransactionParcelResultSpec{{
					Parcel:     mustBuild(t, domain.NewDeclaredParcelID, "parcel-1"),
					Accepted:   true,
					Identifier: mustBuild(t, domain.NewChannelParcelIdentifier, identifier),
				}},
				ObservedAt: clock.at.Add(2 * time.Minute),
			})
			return err
		})
		return result, err
	}

	for _, refused := range []struct{ name, transaction, identifier string }{
		{"513 字节", "LT-GATE-513", incompressibleNumber(513)},
		{"带 NUL", "LT-GATE-NUL", "SYN-CH\x00-0001"},
		{"远超索引行上限", "LT-GATE-4000", incompressibleNumber(4000)},
	} {
		t.Run(refused.name, func(t *testing.T) {
			transactionID, identifier := submit(refused.transaction), refused.identifier
			result, err := record(transactionID, identifier)
			if err != nil {
				t.Fatalf("记录结果报错 %v, want 答未受理——坏结果不是依赖故障", err)
			}
			if result.Outcome() != psapplication.LabelTransactionNotAccepted {
				t.Fatalf("outcome = %q, want NOT_ACCEPTED", result.Outcome())
			}
			if state := storedLabelTransactionState(t, transactor, ctx, repository, tenant, transactionID); state != domain.LabelTransactionSubmitted {
				t.Fatalf("交易状态 = %q, want SUBMITTED——拒收的结果不该落库", state)
			}
			if rows := externalIdentifierRelationRows(t, pool, "tenant-a"); rows != 0 {
				t.Fatalf("关联册 %d 行, want 0", rows)
			}
		})
	}

	longest := incompressibleNumber(512)
	transactionID := submit("LT-GATE-512")
	result, err := record(transactionID, longest)
	if err != nil || result.Outcome() != psapplication.LabelTransactionApplied {
		t.Fatalf("512 字节：outcome = %q err = %v, want APPLIED", result.Outcome(), err)
	}
	view, err := adapter.NewExternalIdentifiers(db)
	if err != nil {
		t.Fatalf("构造按号核对读口：%v", err)
	}
	found, err := view.FindParcelsByExternalIdentifier(ctx, tenant, mustBuild(t, domain.NewExternalIdentifierLookupNumber, longest))
	if err != nil || len(found.Parcels) != 1 || found.Parcels[0].String() != "parcel-1" {
		t.Fatalf("按 512 字节的号查回 %+v err = %v, want 恰 parcel-1", found, err)
	}
}

package parcelpricing_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pricingpostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/pricecardtemplate"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	quotedprice "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/parcelpricing"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 先手在事务里已经插入、还没提交；后手必须撞上这把未提交的主键，才放开先手。
// 传进这里的回调不调用 t.Fatal，失败用返回值带出来。

type quotationDesk struct {
	pool     *pgxpool.Pool
	db       *bentopg.DB
	tenant   domain.TenantID
	party    domain.PartyID
	entity   domain.LegalEntityReference
	drafts   *pricingpostgres.PriceCardDrafts
	sends    *pcpostgres.QuotationSends
	replies  *pcpostgres.QuotationReplies
	sender   *application.SendQuotationHandler
	recorder *application.RecordQuotationReplyHandler
	sentAt   time.Time
}

func newQuotationDesk(t *testing.T) quotationDesk {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	tenant := quoteValue(t, domain.NewTenantID, "SYN-TENANT-01")
	party := quoteValue(t, domain.NewPartyID, "SYN-PARTY-SHIPPER-01")
	entity := quoteValue(t, domain.NewLegalEntityReference, "SYN-LE-01")
	sentAt := time.Date(2026, 11, 23, 4, 0, 0, 0, time.UTC)
	mustQuotationTx(t, db, func(txCtx context.Context) error {
		return saveQuotationIdentities(txCtx, db, tenant, party, entity, sentAt.Add(-24*time.Hour))
	})
	drafts, err := pricingpostgres.NewPriceCardDrafts(db)
	if err != nil {
		t.Fatal(err)
	}
	cards, err := pricingpostgres.NewPriceCards(db)
	if err != nil {
		t.Fatal(err)
	}
	prices, err := quotedprice.NewQuotedPrices(drafts, cards, pricecardtemplate.Reader{})
	if err != nil {
		t.Fatal(err)
	}
	sends, err := pcpostgres.NewQuotationSends(db)
	if err != nil {
		t.Fatal(err)
	}
	replies, err := pcpostgres.NewQuotationReplies(db)
	if err != nil {
		t.Fatal(err)
	}
	identities, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		t.Fatal(err)
	}
	sender, err := application.NewSendQuotationHandler(prices, identities, sends)
	if err != nil {
		t.Fatal(err)
	}
	recorder, err := application.NewRecordQuotationReplyHandler(sends, replies, prices)
	if err != nil {
		t.Fatal(err)
	}
	return quotationDesk{
		pool: pool, db: db, tenant: tenant, party: party, entity: entity,
		drafts: drafts, sends: sends, replies: replies, sender: sender, recorder: recorder, sentAt: sentAt,
	}
}

func (desk quotationDesk) approve(t *testing.T, planID, amount string) ppdomain.PricingPlanVersion {
	t.Helper()
	plan := quoteSellPlan(t, planID, amount)
	validated := quoteValidatedDraft(t, desk.tenant, plan, quoteSource(t, planID+".xlsx"), "SYN-AUTH-"+planID, desk.sentAt.Add(-time.Hour))
	approved := quoteApprove(t, validated, desk.sentAt.Add(-30*time.Minute))
	mustQuotationTx(t, desk.db, func(txCtx context.Context) error {
		if _, submitErr := desk.drafts.SubmitDraft(txCtx, validated); submitErr != nil {
			return submitErr
		}
		outcome, advanceErr := desk.drafts.AdvancePriceCardDraft(txCtx, approved)
		if advanceErr != nil {
			return advanceErr
		}
		if outcome != ppports.PriceCardDraftAdvanced {
			return errQuote(outcome.String())
		}
		return nil
	})
	return plan
}

// raceQuotationTransactions 让两笔事务撞上同一把主键。先手的回调返回 nil 时，行已经插进它的事务、尚未提交。
// 后手要出现未授予的锁（正在等这把主键）才提交先手。回调本身不调用 t.Fatal。
func raceQuotationTransactions(t *testing.T, pool *pgxpool.Pool, db *bentopg.DB, first, second func(context.Context) error) (error, error) {
	t.Helper()
	watcher, err := pgx.ConnectConfig(t.Context(), pool.Config().ConnConfig.Copy())
	if err != nil {
		t.Fatalf("开观察连接：%v", err)
	}
	t.Cleanup(func() { _ = watcher.Close(context.Background()) })

	inserted := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	letGo := func() { releaseOnce.Do(func() { close(release) }) }
	defer letGo()

	firstDone := make(chan error, 1)
	go func() {
		firstDone <- db.Transactor().WithinTransaction(context.Background(), func(txCtx context.Context) error {
			if err := first(txCtx); err != nil {
				return err
			}
			close(inserted)
			<-release
			return nil
		})
	}()

	select {
	case <-inserted:
	case err := <-firstDone:
		return err, errors.New("先手在落行前就结束了")
	case <-time.After(15 * time.Second):
		return errors.New("先手十五秒内没有落行"), errors.New("后手没有启动")
	}

	secondDone := make(chan error, 1)
	go func() {
		secondDone <- db.Transactor().WithinTransaction(context.Background(), func(txCtx context.Context) error {
			return second(txCtx)
		})
	}()

	deadline := time.Now().Add(15 * time.Second)
	for {
		if quotationLockWaiting(watcher) {
			break
		}
		select {
		case secondErr := <-secondDone:
			letGo()
			firstErr := <-firstDone
			return firstErr, fmt.Errorf("后手在撞上未提交的主键之前就结束了：%w", secondErr)
		case <-time.After(20 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			letGo()
			firstErr := <-firstDone
			var secondErr error
			select {
			case secondErr = <-secondDone:
			case <-time.After(15 * time.Second):
				secondErr = errors.New("超时后后手仍没有结束")
			}
			return firstErr, fmt.Errorf("后手十五秒内没有等在锁上：%w", secondErr)
		}
	}

	letGo()
	var firstErr error
	select {
	case firstErr = <-firstDone:
	case <-time.After(15 * time.Second):
		return errors.New("放开后先手没走完"), nil
	}
	var secondErr error
	select {
	case secondErr = <-secondDone:
	case <-time.After(15 * time.Second):
		return firstErr, errors.New("先手提交后后手没走完")
	}
	return firstErr, secondErr
}

func quotationLockWaiting(watcher *pgx.Conn) bool {
	var waiting int
	err := watcher.QueryRow(context.Background(),
		`SELECT count(*) FROM pg_stat_activity
		  WHERE datname = current_database()
		    AND wait_event_type = 'Lock'
		    AND state = 'active'`,
	).Scan(&waiting)
	return err == nil && waiting > 0
}

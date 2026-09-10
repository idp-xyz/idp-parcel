package main

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pscommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	shipmentapp "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	shipmentdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	shipmentports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outbound"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证面单渠道写链的组合根（票 label-channel/28 判据 4）：生产装配下链在择优那一格如实停在
// 「未配置」，不建立交易、不写决定记录；把缝配上合成替身后，择优落定 → 翻译 → Establish → Submit → 07 壳答未配置、
// 停在出向缝，每个停点断言到具名错误或结果格。测试输入全是隔离合成，只记 `S`，不进生产装配。

var labelChannelSelectionAt = time.Date(2026, 9, 10, 12, 0, 0, 0, time.UTC)

func labelChannelTestDB(t *testing.T) *bentopg.DB {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	return db
}

func labelChannelCommand(t *testing.T, tenant, transaction string) shipmentapp.EstablishSelectedLabelTransactionCommand {
	t.Helper()
	return shipmentapp.EstablishSelectedLabelTransactionCommand{
		Tenant:        mustValue(t, shipmentdomain.NewTenantID, tenant),
		TransactionID: mustValue(t, shipmentdomain.NewLabelTransactionID, transaction),
		CoveredParcels: []shipmentdomain.DeclaredParcelID{
			mustValue(t, shipmentdomain.NewDeclaredParcelID, "SYN-PARCEL-LC28-1"),
		},
		Selection: shipmentports.ChannelSelectionQuery{
			Tenant:  mustValue(t, shipmentdomain.NewTenantID, tenant),
			Scope:   mustValue(t, shipmentdomain.NewCommercialScopeReference, "SYN-SCOPE-LC28"),
			Mapping: mustValue(t, shipmentdomain.NewProductChannelMappingReference, "SYN-MAPPING-LC28"),
			At:      labelChannelSelectionAt,
		},
	}
}

// Covers: 判据 4 前半——三取数口未配置时链在择优装配那一格停在「未配置」：错误既是前置步的择优段标，又是装配适配器
// 具名的 ErrChannelConstraintNotConfigured（约束先问、先停）；库里没有决定记录、没有交易。07 壳独立于链答未配置。
func TestTheProductionLabelChannelChainStopsHonestlyAtTheUnconfiguredSources(t *testing.T) {
	db := labelChannelTestDB(t)
	chain, err := buildLabelChannelOrchestration(db)
	if err != nil {
		t.Fatalf("装配面单渠道链：%v", err)
	}
	command := labelChannelCommand(t, "SYN-TENANT-LC28-UNCONFIGURED", "SYN-LT-LC28-1")

	_, err = chain.Flow.Establish(t.Context(), command)
	if !errors.Is(err, shipmentapp.ErrChannelSelectionStopped) || !errors.Is(err, pscommercial.ErrChannelConstraintNotConfigured) {
		t.Fatalf("error = %v, want 择优段标 + ErrChannelConstraintNotConfigured", err)
	}
	assertNoDecisionAndNoTransaction(t, db, command)

	submitted, err := chain.Gateway.SubmitLabelRequest(t.Context(), shipmentports.LabelChannelRequest{
		Tenant: command.Tenant, TransactionID: command.TransactionID, CoveredParcels: command.CoveredParcels,
	})
	if err != nil || submitted.Outcome.Disposition() != outbound.NotConfigured {
		t.Fatalf("07 壳 = %v / %v，want NOT_CONFIGURED 且不发起调用", submitted.Outcome.Disposition(), err)
	}
}

// Covers: 判据 4 后半——缝配上合成替身：择优落定（决定记录 SELECTED 落库）→ 翻译 → Establish（交易 ESTABLISHED 落库）
// → Submit（SUBMITTED）→ 07 壳答未配置、停在出向缝；以及两个中途停点——翻译停下时决定记录仍在、交易不建立；并列
// 冲突记 TIED、不建立。
func TestTheLabelChannelChainWalksToTheOutboundSeamOnceTheSeamsAreConfigured(t *testing.T) {
	db := labelChannelTestDB(t)
	tenant := "SYN-TENANT-LC28"
	winner := pricedChannelCandidate(t, "SYN-CAND-LC28-A", "9.00", "SYN-EVAL-LC28-A")
	winner, err := winner.WithRate(mustValue(t, shipmentdomain.NewChannelRateReference, "SYN-BUY-PLAN-LC28/v1"))
	if err != nil {
		t.Fatalf("赢家带费率：%v", err)
	}
	runnerUp := pricedChannelCandidate(t, "SYN-CAND-LC28-B", "12.00", "SYN-EVAL-LC28-B")
	basis := labelChannelBasis(t)

	t.Run("selected → translated → established → submitted → outbound unconfigured", func(t *testing.T) {
		chain, err := buildLabelChannelOrchestrationWith(db, labelChannelSeams{
			Assembly:   labelChannelAssemblyDouble{candidates: []shipmentdomain.ChannelCandidateID{winner.Candidate(), runnerUp.Candidate()}},
			Costs:      labelChannelCostsDouble{costs: []shipmentdomain.ChannelCandidateCost{winner, runnerUp}},
			Translator: labelChannelTranslatorDouble{basis: basis},
		})
		if err != nil {
			t.Fatalf("装配：%v", err)
		}
		command := labelChannelCommand(t, tenant, "SYN-LT-LC28-WALK")

		result, err := chain.Flow.Establish(t.Context(), command)
		if err != nil {
			t.Fatalf("前置步：%v", err)
		}
		if result.Outcome() != shipmentapp.SelectedLabelTransactionEstablished {
			t.Fatalf("outcome = %q, want ESTABLISHED", result.Outcome())
		}
		if selected, present := result.Selected(); !present || selected.Candidate() != winner.Candidate() {
			t.Fatalf("选中 = %v/%v，want %s", selected, present, winner.Candidate())
		}
		establishment, present := result.Establishment()
		if !present || establishment.Outcome() != shipmentapp.LabelTransactionApplied {
			t.Fatalf("建立 = %v/%v，want APPLIED", establishment.Outcome(), present)
		}
		decisions := decisionsFor(t, db, command)
		if len(decisions) != 1 || decisions[0].Conclusion() != shipmentdomain.ChannelSelectionConcludedSelected {
			t.Fatalf("决定记录 = %d 条 / %v，want 一条 SELECTED", len(decisions), conclusionsOf(decisions))
		}
		stored := transactionFor(t, db, command)
		if stored.State() != shipmentdomain.LabelTransactionEstablished || stored.Rate() != basis.Rate() {
			t.Fatalf("库里的交易 state = %q rate = %q，want ESTABLISHED / 择优结果里的费率", stored.State(), stored.Rate())
		}

		submitted, err := chain.Transactions.SubmitToChannel(t.Context(), shipmentapp.SubmitLabelTransactionCommand{
			Tenant: command.Tenant, TransactionID: command.TransactionID,
		})
		if err != nil || submitted.Outcome() != shipmentapp.LabelTransactionApplied {
			t.Fatalf("提交渠道 = %v / %v，want APPLIED", submitted.Outcome(), err)
		}
		if transactionFor(t, db, command).State() != shipmentdomain.LabelTransactionSubmitted {
			t.Fatal("提交渠道没落库成 SUBMITTED")
		}

		channelAnswer, err := chain.Gateway.SubmitLabelRequest(t.Context(), shipmentports.LabelChannelRequest{
			Tenant: command.Tenant, TransactionID: command.TransactionID, CoveredParcels: command.CoveredParcels,
		})
		if err != nil || channelAnswer.Outcome.Disposition() != outbound.NotConfigured {
			t.Fatalf("07 壳 = %v / %v，want NOT_CONFIGURED——链停在出向缝", channelAnswer.Outcome.Disposition(), err)
		}
		if channelAnswer.Outcome.AdmitsResend() {
			t.Fatal("未配置不是「发过了没答」，不该准许重发")
		}
	})

	t.Run("a translation stop keeps the decision and establishes nothing", func(t *testing.T) {
		refused := errors.New("synthetic: acceptance resolution source not configured")
		chain, err := buildLabelChannelOrchestrationWith(db, labelChannelSeams{
			Assembly:   labelChannelAssemblyDouble{candidates: []shipmentdomain.ChannelCandidateID{winner.Candidate()}},
			Costs:      labelChannelCostsDouble{costs: []shipmentdomain.ChannelCandidateCost{winner}},
			Translator: labelChannelTranslatorDouble{err: refused},
		})
		if err != nil {
			t.Fatalf("装配：%v", err)
		}
		command := labelChannelCommand(t, tenant+"-TRANSLATION", "SYN-LT-LC28-T")

		_, err = chain.Flow.Establish(t.Context(), command)
		if !errors.Is(err, shipmentapp.ErrChannelBasisTranslationStopped) || !errors.Is(err, refused) {
			t.Fatalf("error = %v, want 翻译段标 + 成因", err)
		}
		if decisions := decisionsFor(t, db, command); len(decisions) != 1 {
			t.Fatalf("翻译停下应保留择优留痕：决定记录 %d 条", len(decisions))
		}
		if _, found := findTransaction(t, db, command); found {
			t.Fatal("翻译停下还建立了交易")
		}
	})

	t.Run("a tie is recorded and establishes nothing", func(t *testing.T) {
		tiedA := pricedChannelCandidate(t, "SYN-CAND-LC28-T1", "10.00", "SYN-EVAL-LC28-T1")
		tiedB := pricedChannelCandidate(t, "SYN-CAND-LC28-T2", "10.00", "SYN-EVAL-LC28-T2")
		chain, err := buildLabelChannelOrchestrationWith(db, labelChannelSeams{
			Assembly:   labelChannelAssemblyDouble{candidates: []shipmentdomain.ChannelCandidateID{tiedA.Candidate(), tiedB.Candidate()}},
			Costs:      labelChannelCostsDouble{costs: []shipmentdomain.ChannelCandidateCost{tiedA, tiedB}},
			Translator: labelChannelTranslatorDouble{basis: basis},
		})
		if err != nil {
			t.Fatalf("装配：%v", err)
		}
		command := labelChannelCommand(t, tenant+"-TIED", "SYN-LT-LC28-TIED")

		result, err := chain.Flow.Establish(t.Context(), command)
		if err != nil || result.Outcome() != shipmentapp.ChannelSelectionTied {
			t.Fatalf("outcome = %q / %v，want SELECTION_TIED", result.Outcome(), err)
		}
		decisions := decisionsFor(t, db, command)
		if len(decisions) != 1 || decisions[0].Conclusion() != shipmentdomain.ChannelSelectionConcludedTied {
			t.Fatalf("决定记录 = %d 条 / %v，want 一条 TIED", len(decisions), conclusionsOf(decisions))
		}
		if _, found := findTransaction(t, db, command); found {
			t.Fatal("并列冲突还建立了交易")
		}
	})
}

func assertNoDecisionAndNoTransaction(t *testing.T, db *bentopg.DB, command shipmentapp.EstablishSelectedLabelTransactionCommand) {
	t.Helper()
	if decisions := decisionsFor(t, db, command); len(decisions) != 0 {
		t.Fatalf("未配置停点写了决定记录：%d 条", len(decisions))
	}
	if _, found := findTransaction(t, db, command); found {
		t.Fatal("未配置停点建立了交易")
	}
}

func decisionsFor(t *testing.T, db *bentopg.DB, command shipmentapp.EstablishSelectedLabelTransactionCommand) []shipmentdomain.ChannelSelectionDecision {
	t.Helper()
	registry, err := pspostgres.NewChannelSelectionDecisions(db)
	if err != nil {
		t.Fatalf("构造决定登记册：%v", err)
	}
	subject, err := shipmentdomain.NewChannelSelectionSubject(command.Selection.Scope, command.Selection.Mapping)
	if err != nil {
		t.Fatalf("造对象引用：%v", err)
	}
	decisions, err := registry.ListBySubject(t.Context(), command.Tenant, subject)
	if err != nil {
		t.Fatalf("读决定记录：%v", err)
	}
	return decisions
}

func conclusionsOf(decisions []shipmentdomain.ChannelSelectionDecision) []string {
	conclusions := make([]string, 0, len(decisions))
	for _, decision := range decisions {
		conclusions = append(conclusions, decision.Conclusion().String())
	}
	return conclusions
}

func findTransaction(t *testing.T, db *bentopg.DB, command shipmentapp.EstablishSelectedLabelTransactionCommand) (shipmentdomain.LabelTransaction, bool) {
	t.Helper()
	repository, err := pspostgres.NewLabelTransactions(db)
	if err != nil {
		t.Fatalf("构造交易仓储：%v", err)
	}
	transaction, found, err := repository.FindByID(t.Context(), command.Tenant, command.TransactionID)
	if err != nil {
		t.Fatalf("读交易：%v", err)
	}
	return transaction, found
}

func transactionFor(t *testing.T, db *bentopg.DB, command shipmentapp.EstablishSelectedLabelTransactionCommand) shipmentdomain.LabelTransaction {
	t.Helper()
	transaction, found := findTransaction(t, db, command)
	if !found {
		t.Fatalf("交易 %s 没落库", command.TransactionID)
	}
	return transaction
}

func labelChannelBasis(t *testing.T) shipmentdomain.SelectedChannelBasis {
	t.Helper()
	basis, err := shipmentdomain.NewSelectedChannelBasis(shipmentdomain.SelectedChannelBasisSpec{
		Candidate:              mustValue(t, shipmentdomain.NewChannelCandidateID, "SYN-CAND-LC28-A"),
		ChannelAccount:         mustValue(t, shipmentdomain.NewChannelAccountReference, "SYN-ACCT-LC28"),
		AccountHolder:          mustValue(t, shipmentdomain.NewChannelAccountHolderReference, "SYN-HOLDER-LC28"),
		ServiceProvider:        mustValue(t, shipmentdomain.NewChannelServiceProviderReference, "SYN-PROVIDER-LC28"),
		SettlementCounterparty: mustValue(t, shipmentdomain.NewSettlementCounterpartyReference, "SYN-COUNTERPARTY-LC28"),
		Contract:               mustValue(t, shipmentdomain.NewChannelContractReference, "SYN-CONTRACT-LC28"),
		Rate:                   mustValue(t, shipmentdomain.NewChannelRateReference, "SYN-BUY-PLAN-LC28/v1"),
		ResponsibilityBasis:    mustValue(t, shipmentdomain.NewResponsibilityBasisSnapshotReference, "SYN-RESOLUTION-LC28"),
	})
	if err != nil {
		t.Fatalf("造择优结果：%v", err)
	}
	return basis
}

// 三个端口级合成替身：只给测试用（labelChannelSeams 头注），不进生产装配。

type labelChannelAssemblyDouble struct {
	candidates []shipmentdomain.ChannelCandidateID
}

func (double labelChannelAssemblyDouble) AssembleChannelCandidates(
	context.Context, shipmentports.ChannelSelectionQuery,
) ([]shipmentdomain.ChannelCandidateID, error) {
	return double.candidates, nil
}

type labelChannelCostsDouble struct {
	costs []shipmentdomain.ChannelCandidateCost
}

func (double labelChannelCostsDouble) ChannelCandidateCosts(
	context.Context, shipmentports.ChannelSelectionQuery, []shipmentdomain.ChannelCandidateID,
) ([]shipmentdomain.ChannelCandidateCost, error) {
	return double.costs, nil
}

type labelChannelTranslatorDouble struct {
	basis shipmentdomain.SelectedChannelBasis
	err   error
}

func (double labelChannelTranslatorDouble) TranslateSelectedCandidate(
	context.Context, shipmentports.ChannelSelectionQuery, shipmentdomain.SelectedChannelCandidate,
) (shipmentdomain.SelectedChannelBasis, error) {
	return double.basis, double.err
}

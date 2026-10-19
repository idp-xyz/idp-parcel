package postgres_test

import (
	"context"
	"fmt"
	"slices"
	"sync"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件对真实 PostgreSQL 证身份核对读口带上可验证交付范围之后的答法（ADR-0212 决定四、决定八；PS CONTEXT「范围相容」「歧义信号」）：
// 候选照旧在租户内全部交回，逐候选答范围相容；歧义信号带了范围时只认范围内客户账户的`已提交`委托；三样出自同一条语句。夹具全为
// 合成引用（S 级）。

// scopedAnswerOf 带着客户账户集合 scope 按号核对，候选写成「包裹:范围相容」，按包裹定序。
func (fixture externalIdentifierFixture) scopedAnswerOf(t *testing.T, tenant, number string, scope ...string) ([]string, bool) {
	t.Helper()
	answer, err := fixture.identifiers.FindParcelsByExternalIdentifier(t.Context(),
		mustBuild(t, domain.NewTenantID, tenant), mustBuild(t, domain.NewExternalIdentifierLookupNumber, number),
		customerAccounts(t, scope...))
	if err != nil {
		t.Fatalf("带范围 %v 按号码查 %s：%v", scope, number, err)
	}
	return candidateCells(answer.Candidates), answer.DeclaredBySubmittedRequest
}

func customerAccounts(t *testing.T, values ...string) []domain.CustomerAccountID {
	t.Helper()
	accounts := make([]domain.CustomerAccountID, 0, len(values))
	for _, value := range values {
		accounts = append(accounts, mustBuild(t, domain.NewCustomerAccountID, value))
	}
	return accounts
}

func candidateCells(candidates []ports.ExternalIdentifierCandidate) []string {
	cells := make([]string, 0, len(candidates))
	for _, candidate := range candidates {
		cells = append(cells, fmt.Sprintf("%s:%v", candidate.Parcel, candidate.ScopeCompatible))
	}
	slices.Sort(cells)
	return cells
}

// Covers: ADR-0212 决定四「候选照旧在租户内按号码逐字取、全部交回（ADR-0184 决定六不废），逐候选带范围相容」与「比的是适用范围、
// 不是分配方」（`AT-NO-064` 的 PS 读口半边）——两个客户账户的已接受委托用了同一个号：带客户账户一核对，两件都交回，只有客户账户一那件范围相容；两个账户都带，
// 两件都相容；不带范围，只经客户包裹件号命中的两件都不相容。别的租户、大小写不同照旧零候选。
func TestAScopedCheckReturnsEveryCandidateAndAnswersCompatibilityPerCandidate(t *testing.T) {
	fixture := newExternalIdentifierFixture(t)
	fixture.insertAndAccept(t, numberedRequest(t, "tenant-1", "customer-1", "req-key-a", "request-a",
		[]string{"parcel-a"}, map[string]string{"parcel-a": "SCOPED-01"}))
	fixture.insertAndAccept(t, numberedRequest(t, "tenant-1", "customer-2", "req-key-b", "request-b",
		[]string{"parcel-b"}, map[string]string{"parcel-b": "SCOPED-01"}))

	for name, testCase := range map[string]struct {
		scope []string
		want  []string
	}{
		"带客户账户一":    {[]string{"customer-1"}, []string{"parcel-a:true", "parcel-b:false"}},
		"带客户账户二":    {[]string{"customer-2"}, []string{"parcel-a:false", "parcel-b:true"}},
		"两个都带":      {[]string{"customer-1", "customer-2"}, []string{"parcel-a:true", "parcel-b:true"}},
		"不带范围":      {nil, []string{"parcel-a:false", "parcel-b:false"}},
		"范围是别的客户账户": {[]string{"customer-9"}, []string{"parcel-a:false", "parcel-b:false"}},
	} {
		got, signalled := fixture.scopedAnswerOf(t, "tenant-1", "SCOPED-01", testCase.scope...)
		if !slices.Equal(got, testCase.want) || signalled {
			t.Fatalf("%s：答 %v / 信号 %v, want %v 不带信号——范围收窄了候选，或相容没按适用范围比", name, got, signalled, testCase.want)
		}
	}
	if got, _ := fixture.scopedAnswerOf(t, "tenant-2", "SCOPED-01", "customer-1"); len(got) != 0 {
		t.Fatalf("别的租户带范围答 %v, want 零候选", got)
	}
	if got, _ := fixture.scopedAnswerOf(t, "tenant-1", "scoped-01", "customer-1"); len(got) != 0 {
		t.Fatalf("大小写不同带范围答 %v, want 零候选——范围不改逐字比对", got)
	}
}

// Covers: ADR-0212 决定四「渠道包裹件号不需要范围」（`AT-NO-067` 的 PS 读口半边）——渠道签发的号在渠道服务方内算数，带不带范围、带的是哪个客户账户，它的命中都
// 范围相容。
func TestAChannelNumberHitIsAlwaysScopeCompatible(t *testing.T) {
	fixture := newChannelFixture(t)
	fixture.recordChannel(t, establishedLabelTransactionFixture(t, "tenant-a", "label-txn-scope", "parcel-1"),
		labelEstablishedAt.Add(2*time.Minute), acceptedChannelResult(t, "parcel-1", "CH-SCOPE-0001"))
	for name, scope := range map[string][]string{
		"不带范围":    nil,
		"带一个客户账户": {"customer-1"},
		"带两个客户账户": {"customer-1", "customer-2"},
	} {
		answer, err := fixture.identifiers.FindParcelsByExternalIdentifier(t.Context(),
			mustBuild(t, domain.NewTenantID, "tenant-a"), mustBuild(t, domain.NewExternalIdentifierLookupNumber, "CH-SCOPE-0001"),
			customerAccounts(t, scope...))
		if err != nil {
			t.Fatalf("%s：%v", name, err)
		}
		if got := candidateCells(answer.Candidates); !slices.Equal(got, []string{"parcel-1:true"}) || answer.DeclaredBySubmittedRequest {
			t.Fatalf("%s：答 %v / 信号 %v, want [parcel-1:true] 不带信号", name, got, answer.DeclaredBySubmittedRequest)
		}
	}
}

// Covers: ADR-0212 决定四「歧义信号：给了客户账户时，只认这些账户的`已提交`委托所作的声明；没给时照 ADR-0195」与决定八「范围内
// 同一客户账户两份委托重复用号，照旧是冲突」（`AT-NO-064` 的信号半边）——客户账户一那件在册，客户账户二另有一份`已提交`委托也报了它：带客户账户一核对不带
// 信号，带客户账户二、两个都带或不带范围都带信号。客户账户一自己另交一份`已提交`委托重复用号，带客户账户一也带信号。
func TestTheSignalNarrowsToTheScopeAccounts(t *testing.T) {
	fixture := newExternalIdentifierFixture(t)
	ctx := t.Context()
	fixture.insertAndAccept(t, numberedRequest(t, "tenant-1", "customer-1", "req-key-a", "request-a",
		[]string{"parcel-a"}, map[string]string{"parcel-a": "SIGNAL-01"}))
	mustInsert(t, fixture.transactor, ctx, fixture.requests, numberedRequest(t, "tenant-1", "customer-2", "req-key-b", "request-b",
		[]string{"parcel-b"}, map[string]string{"parcel-b": "SIGNAL-01"}))

	for name, testCase := range map[string]struct {
		scope  []string
		signal bool
	}{
		"范围只有持号一方": {[]string{"customer-1"}, false},
		"范围是声明一方":  {[]string{"customer-2"}, true},
		"两个都在范围里":  {[]string{"customer-1", "customer-2"}, true},
		"不带范围":     {nil, true},
	} {
		got, signalled := fixture.scopedAnswerOf(t, "tenant-1", "SIGNAL-01", testCase.scope...)
		if signalled != testCase.signal {
			t.Fatalf("%s：信号 %v, want %v（候选 %v）", name, signalled, testCase.signal, got)
		}
		if len(got) != 1 {
			t.Fatalf("%s：候选 %v, want 恰一件——声明不进候选", name, got)
		}
	}

	mustInsert(t, fixture.transactor, ctx, fixture.requests, numberedRequest(t, "tenant-1", "customer-1", "req-key-c", "request-c",
		[]string{"parcel-c"}, map[string]string{"parcel-c": "SIGNAL-01"}))
	if got, signalled := fixture.scopedAnswerOf(t, "tenant-1", "SIGNAL-01", "customer-1"); !signalled ||
		!slices.Equal(got, []string{"parcel-a:true"}) {
		t.Fatalf("范围内同一客户账户另一份已提交委托重复用号：答 %v / 信号 %v, want [parcel-a:true] 带信号", got, signalled)
	}
}

// Covers: ADR-0212 决定四「候选、范围相容与信号出自同一条语句、同一个快照」（ADR-0195 决定二照旧）——一份`已提交`委托的接受与带
// 范围的核对并发：接受之前答零候选带信号，接受之后答那一件范围相容、不带信号；分两次读，就可能在两次之间看见接受，答零候选又不带
// 信号，那一刻的实物就落成没有歧义的待识别。逐个号码让接受与连续核对赛跑，一次也不许两头漏。
func TestCandidatesCompatibilityAndSignalComeFromOneSnapshot(t *testing.T) {
	fixture := newExternalIdentifierFixture(t)
	ctx := t.Context()
	const rounds = 12
	for round := range rounds {
		number := fmt.Sprintf("RACE-%02d", round)
		key, parcel := fmt.Sprintf("req-key-race-%02d", round), fmt.Sprintf("parcel-race-%02d", round)
		submitted := numberedRequest(t, "tenant-1", "customer-1", key, "request-race-"+number, []string{parcel},
			map[string]string{parcel: number})
		mustInsert(t, fixture.transactor, ctx, fixture.requests, submitted)
		identity := submitted.CurrentSubmissionVersion().SourceSubmission().Identity()
		loaded, found, err := fixture.requests.FindBySourceIdentity(ctx, identity)
		if err != nil || !found {
			t.Fatalf("读回：found = %v, err = %v", found, err)
		}
		accepted := accept(t, loaded)

		var (
			wait      sync.WaitGroup
			acceptErr error
			done      = make(chan struct{})
		)
		wait.Add(1)
		go func() {
			defer wait.Done()
			defer close(done)
			acceptErr = fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
				_, err := fixture.requests.Save(txCtx, identity, accepted)
				return err
			})
		}()
		reads := 0
		for finished := false; !finished; reads++ {
			select {
			case <-done:
				finished = true
			default:
			}
			answer, err := fixture.identifiers.FindParcelsByExternalIdentifier(ctx,
				mustBuild(t, domain.NewTenantID, "tenant-1"), mustBuild(t, domain.NewExternalIdentifierLookupNumber, number),
				customerAccounts(t, "customer-1"))
			if err != nil {
				t.Fatalf("第 %d 轮核对：%v", round, err)
			}
			if len(answer.Candidates) == 0 && !answer.DeclaredBySubmittedRequest {
				t.Fatalf("第 %d 轮第 %d 次核对零候选又不带信号——候选与信号不是同一个快照", round, reads)
			}
			if len(answer.Candidates) == 1 && answer.DeclaredBySubmittedRequest {
				t.Fatalf("第 %d 轮第 %d 次核对既有候选又带信号——同一份委托两头都被算了", round, reads)
			}
		}
		wait.Wait()
		if acceptErr != nil {
			t.Fatalf("第 %d 轮接受：%v", round, acceptErr)
		}
		if got, signalled := fixture.scopedAnswerOf(t, "tenant-1", number, "customer-1"); !slices.Equal(got, []string{parcel + ":true"}) || signalled {
			t.Fatalf("第 %d 轮接受之后答 %v / %v", round, got, signalled)
		}
	}
}

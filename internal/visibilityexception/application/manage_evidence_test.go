package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var evidenceSubmittedAt = time.Date(2026, 8, 12, 14, 0, 0, 0, time.UTC)

type providerDigestKey struct {
	tenant   domain.TenantID
	provider string
	digest   string
}

type disclosureVersionKey struct {
	tenant   domain.TenantID
	item     domain.EvidenceItemID
	redacted string
	scope    string
}

type evidenceStoreDouble struct {
	byID        map[domain.EvidenceItemID]domain.EvidenceItem
	byProvider  map[providerDigestKey]domain.EvidenceItem
	disclosures map[disclosureVersionKey]domain.EvidenceDisclosureVersion
	findErr     error
	saveErr     error
	saved       int
	prepared    int
}

func newEvidenceStore() *evidenceStoreDouble {
	return &evidenceStoreDouble{
		byID:        map[domain.EvidenceItemID]domain.EvidenceItem{},
		byProvider:  map[providerDigestKey]domain.EvidenceItem{},
		disclosures: map[disclosureVersionKey]domain.EvidenceDisclosureVersion{},
	}
}

func (double *evidenceStoreDouble) FindByID(
	_ context.Context,
	_ domain.TenantID,
	id domain.EvidenceItemID,
) (domain.EvidenceItem, bool, error) {
	if double.findErr != nil {
		return domain.EvidenceItem{}, false, double.findErr
	}
	item, found := double.byID[id]
	return item, found, nil
}

func (double *evidenceStoreDouble) FindByProviderDigest(
	_ context.Context,
	tenant domain.TenantID,
	provider domain.EvidenceProviderReference,
	digest domain.EvidenceContentDigest,
) (domain.EvidenceItem, bool, error) {
	if double.findErr != nil {
		return domain.EvidenceItem{}, false, double.findErr
	}
	item, found := double.byProvider[providerDigestKey{tenant: tenant, provider: provider.String(), digest: digest.String()}]
	return item, found, nil
}

func (double *evidenceStoreDouble) Save(
	_ context.Context,
	tenant domain.TenantID,
	item domain.EvidenceItem,
) (ports.EvidenceSaveOutcome, error) {
	if double.saveErr != nil {
		return ports.EvidenceSaveOutcomeInvalid, double.saveErr
	}
	key := providerDigestKey{tenant: tenant, provider: item.Provider().String(), digest: item.Digest().String()}
	if _, found := double.byProvider[key]; found {
		return ports.EvidenceAlreadyRecorded, nil
	}
	double.byProvider[key] = item
	double.byID[item.ID()] = item
	double.saved++
	return ports.EvidenceSaved, nil
}

func (double *evidenceStoreDouble) FindDisclosure(
	_ context.Context,
	tenant domain.TenantID,
	item domain.EvidenceItemID,
	redacted domain.EvidenceContentDigest,
	scope string,
) (domain.EvidenceDisclosureVersion, bool, error) {
	if double.findErr != nil {
		return domain.EvidenceDisclosureVersion{}, false, double.findErr
	}
	version, found := double.disclosures[disclosureVersionKey{tenant: tenant, item: item, redacted: redacted.String(), scope: scope}]
	return version, found, nil
}

func (double *evidenceStoreDouble) SaveDisclosure(
	_ context.Context,
	tenant domain.TenantID,
	version domain.EvidenceDisclosureVersion,
) (ports.EvidenceSaveOutcome, error) {
	if double.saveErr != nil {
		return ports.EvidenceSaveOutcomeInvalid, double.saveErr
	}
	key := disclosureVersionKey{tenant: tenant, item: version.Item(), redacted: version.Redacted().String(), scope: version.Scope()}
	if _, found := double.disclosures[key]; found {
		return ports.EvidenceAlreadyRecorded, nil
	}
	double.disclosures[key] = version
	double.prepared++
	return ports.EvidenceSaved, nil
}

type evidenceIdentityDouble struct {
	next int
	err  error
}

func (double *evidenceIdentityDouble) NextEvidenceItemID(_ context.Context) (domain.EvidenceItemID, error) {
	if double.err != nil {
		return domain.EvidenceItemID{}, double.err
	}
	double.next++
	return domain.NewEvidenceItemID("evidence-" + string(rune('0'+double.next)))
}

type evidenceFixture struct {
	handler    *application.ManageEvidenceHandler
	store      *evidenceStoreDouble
	identities *evidenceIdentityDouble
}

func newEvidenceFixture(t *testing.T) *evidenceFixture {
	t.Helper()
	fixture := &evidenceFixture{store: newEvidenceStore(), identities: &evidenceIdentityDouble{}}
	fixture.handler = application.NewManageEvidenceHandler(application.ManageEvidenceDeps{
		Evidence:   fixture.store,
		Identities: fixture.identities,
		Clock:      fixedClock{at: evidenceSubmittedAt.Add(time.Hour)},
	})
	return fixture
}

func submitCommand(t *testing.T) application.SubmitEvidenceCommand {
	t.Helper()
	return application.SubmitEvidenceCommand{
		TenantID:    mustValue(t, domain.NewTenantID, "tenant-1"),
		Provider:    mustValue(t, domain.NewEvidenceProviderReference, "customer-1"),
		Digest:      mustValue(t, domain.NewEvidenceContentDigest, "sha256:damage-photo"),
		SubmittedAt: evidenceSubmittedAt,
	}
}

// Covers: `AT-VE-113`「证据材料收到但尚未核实→形成证据项，不形成事实或责任」与 CONTEXT
// 「客户或合作伙伴提交证据只表示材料已经收到，不证明其陈述、事实或责任成立」——到达
// 形成证据项，评价起点是`已收到`且不由调用方指定（命令上根本没有评价字段），证据项落库。
func TestEvidenceArrivalFormsAReceivedItemWithoutAppraisal(t *testing.T) {
	fixture := newEvidenceFixture(t)

	result, err := fixture.handler.SubmitEvidence(context.Background(), submitCommand(t))
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	if result.Outcome() != application.EvidenceSubmitted {
		t.Fatalf("outcome = %q, want EVIDENCE_SUBMITTED", result.Outcome())
	}
	item, present := result.Evidence()
	if !present {
		t.Fatal("a submitted result carries no evidence item")
	}
	if item.Appraisal() != domain.EvidenceReceived {
		t.Fatalf("appraisal = %s, want RECEIVED; receipt is not credit", item.Appraisal())
	}
	if _, appraised := item.AppraisalBasis(); appraised {
		t.Fatal("a freshly received item carries an appraisal basis")
	}
	if item.Provider().String() != "customer-1" || item.Digest().String() != "sha256:damage-photo" ||
		!item.SubmittedAt().Equal(evidenceSubmittedAt) {
		t.Fatalf("item = %+v; provider, digest and receipt time must come from the arrival", item.Snapshot())
	}
	if fixture.store.saved != 1 || fixture.identities.next != 1 {
		t.Fatalf("saved %d items with %d identities, want 1 and 1", fixture.store.saved, fixture.identities.next)
	}
}

// Covers: 幂等——同一提供方再次提交同一份材料是同一证据项：返回原项、不签新标识、不写库。
// 另一提供方提交同样的字节是另一项（证据项保存来源与提供方）。
func TestTheSameProviderAndDigestIsTheSameEvidenceItem(t *testing.T) {
	fixture := newEvidenceFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.SubmitEvidence(ctx, submitCommand(t))
	if err != nil {
		t.Fatalf("first submit: %v", err)
	}
	firstItem, _ := first.Evidence()

	replay, err := fixture.handler.SubmitEvidence(ctx, submitCommand(t))
	if err != nil {
		t.Fatalf("replay submit: %v", err)
	}
	if replay.Outcome() != application.EvidenceExistingResult {
		t.Fatalf("outcome = %q, want EVIDENCE_EXISTING_RESULT", replay.Outcome())
	}
	replayed, _ := replay.Evidence()
	if replayed.ID() != firstItem.ID() || fixture.identities.next != 1 || fixture.store.saved != 1 {
		t.Fatal("a replay minted a new identity or recorded a second item")
	}

	other := submitCommand(t)
	other.Provider = mustValue(t, domain.NewEvidenceProviderReference, "carrier-9")
	separate, err := fixture.handler.SubmitEvidence(ctx, other)
	if err != nil {
		t.Fatalf("other provider submit: %v", err)
	}
	otherItem, _ := separate.Evidence()
	if separate.Outcome() != application.EvidenceSubmitted || otherItem.ID() == firstItem.ID() {
		t.Fatal("the same bytes from another provider must form a separate evidence item")
	}
}

// Covers: 受理与未决半边——要件缺一即未受理，不读依赖；证据库读不回或标识签不出停在未决，
// 各自占格，且未决不写库。
func TestIncompleteOrBlockedEvidenceArrivalDoesNotFormAnItem(t *testing.T) {
	fixture := newEvidenceFixture(t)

	missingDigest := submitCommand(t)
	missingDigest.Digest = domain.EvidenceContentDigest{}
	result, err := fixture.handler.SubmitEvidence(context.Background(), missingDigest)
	if err != nil {
		t.Fatalf("submit without digest: %v", err)
	}
	if result.Outcome() != application.ManageEvidenceNotAccepted || fixture.store.saved != 0 {
		t.Fatalf("outcome = %q, want NOT_ACCEPTED without touching the store", result.Outcome())
	}

	fixture.identities.err = errors.New("identity unavailable")
	undecided, err := fixture.handler.SubmitEvidence(context.Background(), submitCommand(t))
	if err != nil {
		t.Fatalf("submit without identity: %v", err)
	}
	if undecided.Outcome() != application.ManageEvidenceUndecided ||
		undecided.UndecidedReason() != application.EvidenceIdentityUnavailable {
		t.Fatalf("outcome/reason = %q/%q, want UNDECIDED/EVIDENCE_IDENTITY_UNAVAILABLE",
			undecided.Outcome(), undecided.UndecidedReason())
	}

	fixture.identities.err = nil
	fixture.store.findErr = errors.New("store unavailable")
	blocked, err := fixture.handler.SubmitEvidence(context.Background(), submitCommand(t))
	if err != nil {
		t.Fatalf("submit with unreadable store: %v", err)
	}
	if blocked.UndecidedReason() != application.EvidenceStoreUnavailable || fixture.identities.next != 0 {
		t.Fatal("an unreadable store must be undecided before an identity is consumed")
	}
}

func (fixture *evidenceFixture) submittedItem(t *testing.T) domain.EvidenceItem {
	t.Helper()
	result, err := fixture.handler.SubmitEvidence(context.Background(), submitCommand(t))
	if err != nil {
		t.Fatalf("submit: %v", err)
	}
	item, _ := result.Evidence()
	return item
}

func prepareCommand(t *testing.T, item domain.EvidenceItemID) application.PrepareEvidenceDisclosureCommand {
	t.Helper()
	return application.PrepareEvidenceDisclosureCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Evidence: item,
		Redacted: mustValue(t, domain.NewEvidenceContentDigest, "sha256:damage-photo-redacted"),
		Scope:    "claim-counterparty/insurer-1",
	}
}

// Covers: CONTEXT「对外披露必须形成明确披露范围或脱敏版本，不能复制出来源不明、内容不一致
// 的附件」与 `AT-VE-132`「通知内容和证据已准备但尚未对外提交→保持准备完成」——披露版本
// 锚定原件指纹、带范围、只记准备完成；同一脱敏版本重复准备返回原版本（`AT-VE-147` 受控
// 复用：同一证据被多处引用，不复制不一致附件）。
func TestPreparingADisclosureVersionAnchorsTheOriginalAndIsIdempotent(t *testing.T) {
	fixture := newEvidenceFixture(t)
	item := fixture.submittedItem(t)

	result, err := fixture.handler.PrepareDisclosure(context.Background(), prepareCommand(t, item.ID()))
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}

	if result.Outcome() != application.EvidenceDisclosurePrepared {
		t.Fatalf("outcome = %q, want DISCLOSURE_PREPARED", result.Outcome())
	}
	version, present := result.Disclosure()
	if !present {
		t.Fatal("a prepared result carries no disclosure version")
	}
	if version.Item() != item.ID() || version.Original() != item.Digest() {
		t.Fatal("the disclosure version must anchor the evidence item and its original digest")
	}
	if version.Redacted().String() != "sha256:damage-photo-redacted" || version.Scope() != "claim-counterparty/insurer-1" {
		t.Fatalf("version = %+v; redacted digest and scope must come from the preparer", version)
	}
	if version.PreparedAt().IsZero() {
		t.Fatal("preparation time was not fixed")
	}

	replay, err := fixture.handler.PrepareDisclosure(context.Background(), prepareCommand(t, item.ID()))
	if err != nil {
		t.Fatalf("replay prepare: %v", err)
	}
	if replay.Outcome() != application.EvidenceDisclosureExistingResult || fixture.store.prepared != 1 {
		t.Fatalf("outcome = %q with %d versions; the same redacted version must be reused, not re-prepared",
			replay.Outcome(), fixture.store.prepared)
	}

	// 一个披露版本是「范围 + 脱敏版本」这一对：同一份脱敏内容对另一相对方是另一个版本，
	// 不是复用——复用了就把准备方要的范围静默换成了别人的。
	otherScope := prepareCommand(t, item.ID())
	otherScope.Scope = "claim-counterparty/carrier-9"
	second, err := fixture.handler.PrepareDisclosure(context.Background(), otherScope)
	if err != nil {
		t.Fatalf("prepare for another scope: %v", err)
	}
	secondVersion, _ := second.Disclosure()
	if second.Outcome() != application.EvidenceDisclosurePrepared || fixture.store.prepared != 2 ||
		secondVersion.Scope() != "claim-counterparty/carrier-9" {
		t.Fatalf("outcome = %q with %d versions, scope %q; another scope must be its own version",
			second.Outcome(), fixture.store.prepared, secondVersion.Scope())
	}
}

// Covers: 领域门「脱敏指纹不得与原件指纹相同（相同即原件外流）」与「披露范围必备」——
// 两种准备方给错的输入都答未受理而不是撞错，不写库；证据项不在场同样未受理。
func TestADisclosureThatLeaksTheOriginalOrLacksScopeIsNotAccepted(t *testing.T) {
	fixture := newEvidenceFixture(t)
	item := fixture.submittedItem(t)

	leaking := prepareCommand(t, item.ID())
	leaking.Redacted = item.Digest()
	result, err := fixture.handler.PrepareDisclosure(context.Background(), leaking)
	if err != nil {
		t.Fatalf("prepare with original digest: %v", err)
	}
	if result.Outcome() != application.ManageEvidenceNotAccepted {
		t.Fatalf("outcome = %q, want NOT_ACCEPTED; a redacted digest equal to the original leaks the original", result.Outcome())
	}

	unscoped := prepareCommand(t, item.ID())
	unscoped.Scope = "   "
	noScope, err := fixture.handler.PrepareDisclosure(context.Background(), unscoped)
	if err != nil {
		t.Fatalf("prepare without scope: %v", err)
	}
	if noScope.Outcome() != application.ManageEvidenceNotAccepted {
		t.Fatalf("outcome = %q, want NOT_ACCEPTED; a disclosure needs an explicit scope", noScope.Outcome())
	}

	unknown, err := fixture.handler.PrepareDisclosure(context.Background(),
		prepareCommand(t, mustValue(t, domain.NewEvidenceItemID, "evidence-never-submitted")))
	if err != nil {
		t.Fatalf("prepare for unknown item: %v", err)
	}
	if unknown.Outcome() != application.ManageEvidenceNotAccepted {
		t.Fatalf("outcome = %q, want NOT_ACCEPTED for an item that was never received", unknown.Outcome())
	}
	if fixture.store.prepared != 0 {
		t.Fatal("a refused preparation still recorded a version")
	}
}

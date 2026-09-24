package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/pilotgovernance/application"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
)

// 固定候选版本组：首次落册、同内容重放、同标识异内容拒（组不可扩张）、零值组未受理、存储读写
// 失败答未决而不是折成「没在册」。

type fixCandidateSetStore struct {
	byID    map[domain.CandidateVersionSetID]domain.CandidateVersionSet
	findErr error
	saveErr error
	saves   int
}

func (store *fixCandidateSetStore) FindByID(
	_ context.Context,
	id domain.CandidateVersionSetID,
) (domain.CandidateVersionSet, bool, error) {
	if store.findErr != nil {
		return domain.CandidateVersionSet{}, false, store.findErr
	}
	set, found := store.byID[id]
	return set, found, nil
}

func (store *fixCandidateSetStore) Save(_ context.Context, set domain.CandidateVersionSet) error {
	store.saves++
	if store.saveErr != nil {
		return store.saveErr
	}
	store.byID[set.ID()] = set
	return nil
}

var candidateFormedAt = time.Date(2025, 12, 20, 0, 0, 0, 0, time.UTC)

func fixedCandidateSet(t *testing.T, parameters string) domain.CandidateVersionSet {
	t.Helper()
	set, err := domain.FixCandidateVersionSet(
		mustValue(t, domain.NewCandidateVersionSetID, "candidate-set-1"),
		mustValue(t, domain.NewScopeVersionReference, "pilot-scope/intake@v1"),
		mustValue(t, domain.NewParameterSnapshotReference, parameters),
		mustValue(t, domain.NewRuleVersionsReference, "rules/intake-v1"),
		candidateFormedAt,
	)
	if err != nil {
		t.Fatalf("FixCandidateVersionSet：%v", err)
	}
	return set
}

func TestFixCandidateSetFixesOnceAndReplaysTheSameContent(t *testing.T) {
	store := &fixCandidateSetStore{byID: map[domain.CandidateVersionSetID]domain.CandidateVersionSet{}}
	handler := application.NewFixCandidateSetHandler(store)
	set := fixedCandidateSet(t, "params/intake-v1")

	for round, want := range []application.FixCandidateSetOutcome{
		application.CandidateSetFixed, application.CandidateSetAlreadyFixed,
	} {
		result, err := handler.Handle(t.Context(), set)
		if err != nil || result.Outcome() != want {
			t.Fatalf("第 %d 次 = (%s, %v)，要 %s", round+1, result.Outcome(), err, want)
		}
	}
	if store.saves != 1 {
		t.Fatalf("写口被调 %d 次，要 1（重放不再写）", store.saves)
	}
}

// 同标识换了参数快照就是另一组：不改这一组，要换新标识。
func TestFixCandidateSetRefusesToWidenAFixedSet(t *testing.T) {
	store := &fixCandidateSetStore{byID: map[domain.CandidateVersionSetID]domain.CandidateVersionSet{}}
	handler := application.NewFixCandidateSetHandler(store)
	if result, _ := handler.Handle(t.Context(), fixedCandidateSet(t, "params/intake-v1")); result.Outcome() != application.CandidateSetFixed {
		t.Fatalf("首次 = %s", result.Outcome())
	}
	result, err := handler.Handle(t.Context(), fixedCandidateSet(t, "params/intake-v2"))
	if err != nil || result.Outcome() != application.CandidateSetContentConflict {
		t.Fatalf("同标识异内容 = (%s, %v)，要 CONTENT_CONFLICT", result.Outcome(), err)
	}
	if store.saves != 1 {
		t.Fatalf("写口被调 %d 次，要 1", store.saves)
	}
}

func TestFixCandidateSetAnswersUndecidedOnStoreFailureAndRefusesZeroSets(t *testing.T) {
	failingFind := &fixCandidateSetStore{byID: map[domain.CandidateVersionSetID]domain.CandidateVersionSet{}, findErr: errors.New("db down")}
	if result, err := application.NewFixCandidateSetHandler(failingFind).Handle(t.Context(), fixedCandidateSet(t, "params/intake-v1")); err != nil || result.Outcome() != application.CandidateSetUndecided {
		t.Fatalf("读失败 = (%s, %v)，要 UNDECIDED", result.Outcome(), err)
	}
	failingSave := &fixCandidateSetStore{byID: map[domain.CandidateVersionSetID]domain.CandidateVersionSet{}, saveErr: errors.New("already fixed")}
	if result, err := application.NewFixCandidateSetHandler(failingSave).Handle(t.Context(), fixedCandidateSet(t, "params/intake-v1")); err != nil || result.Outcome() != application.CandidateSetUndecided {
		t.Fatalf("写失败 = (%s, %v)，要 UNDECIDED", result.Outcome(), err)
	}
	empty := &fixCandidateSetStore{byID: map[domain.CandidateVersionSetID]domain.CandidateVersionSet{}}
	if result, err := application.NewFixCandidateSetHandler(empty).Handle(t.Context(), domain.CandidateVersionSet{}); err != nil || result.Outcome() != application.CandidateSetNotAccepted {
		t.Fatalf("零值组 = (%s, %v)，要 NOT_ACCEPTED", result.Outcome(), err)
	}
	if empty.saves != 0 {
		t.Fatalf("零值组碰了写口")
	}
}

package domain

import (
	"errors"
	"slices"
	"time"
)

// 委托协议换版（ADR-0231 决定三、四）。
//
// 委托提交时固定一版供应商协议的快照与采购责任法人（ADR-0202 决定三），委托下形成的成本按它判（ADR-0204 决定一「委托快照是采购
// 的权威」）。那一版此后被截止或退役（ADR-0233）、或租户另发一版补上收费发生条件，委托不会自己跟上：快照不随 party-commercial
// 的新版改指，改指是替委托作了一个它没作的采购决定。换版是运营人员显式作出的那个决定：自某一业务时刻起，这份委托改按同一协议对象
// 的后继版本采购。旧版不改写、留作历史，那一刻之前的事实仍按它判。
//
// 换版是委托上的一串版本，不是另一份委托：服务方、条件、角色、责任与成员都不随换版变，变的只有协议版本、它的快照与采购责任法人。
// 提交那一版是第 1 版，换版从第 2 版起依次编号。

var (
	ErrInvalidCommissionAgreementRevision = errors.New("transport fulfillment: invalid commission agreement revision")
	// ErrRevisionAppliesBeforeSubmission：换版自何时起早于委托提交。委托下的事实都不早于提交，早于它的起点说的不是这份委托下的事。
	ErrRevisionAppliesBeforeSubmission = errors.New("transport fulfillment: a commission agreement revision cannot apply before the commission was submitted")
	// ErrRevisionAgreementObjectMismatch：后继与委托指名的不是同一个协议对象。换版换的是同一份协议的版本；换一份协议是换了采购
	// 安排，该另起委托。
	ErrRevisionAgreementObjectMismatch = errors.New("transport fulfillment: a commission agreement revision must name a version of the same agreement object")
)

// submittedAgreementRevision 是提交那一版的号。
const submittedAgreementRevision = 1

// CommissionAgreementRevisionBasisReference 指名换版所凭的那一份运营决定（登记依据）。同一份决定只换一次版：它是换版的重放键。
type CommissionAgreementRevisionBasisReference struct{ requiredValue }

func NewCommissionAgreementRevisionBasisReference(value string) (CommissionAgreementRevisionBasisReference, error) {
	required, err := newRequiredValue("commission agreement revision basis reference", value)
	return CommissionAgreementRevisionBasisReference{required}, err
}

// CommissionAgreementRevision 是委托上的一次换版：自 appliesFrom 起按 successor 采购，快照与法人取自核过的那一版。
type CommissionAgreementRevision struct {
	number       int
	successor    SupplierAgreementVersion
	snapshot     AgreementSnapshotReference
	legalEntity  ProcurementLegalEntityReference
	appliesFrom  time.Time
	basis        CommissionAgreementRevisionBasisReference
	decidedBy    RegistrantReference
	registeredAt time.Time
}

// Number 是这一版在委托上的号：提交那一版是 1，换版从 2 起。
func (revision CommissionAgreementRevision) Number() int { return revision.number }

// Successor 是这一版指名的后继协议版本。
func (revision CommissionAgreementRevision) Successor() SupplierAgreementVersion {
	return revision.successor
}

func (revision CommissionAgreementRevision) Snapshot() AgreementSnapshotReference {
	return revision.snapshot
}

func (revision CommissionAgreementRevision) LegalEntity() ProcurementLegalEntityReference {
	return revision.legalEntity
}

// AppliesFrom 是这一版起用的业务时刻：业务时间不早于它的事实按这一版判。
func (revision CommissionAgreementRevision) AppliesFrom() time.Time { return revision.appliesFrom }

func (revision CommissionAgreementRevision) Basis() CommissionAgreementRevisionBasisReference {
	return revision.basis
}

// DecidedBy 是做出这一次换版决定的操作者。它是出处，不是换版的内容：同一份决定换人重报仍是这一版。
func (revision CommissionAgreementRevision) DecidedBy() RegistrantReference {
	return revision.decidedBy
}

// RegisteredAt 是换版落册的时刻（登记时刻，不是业务时间）。
func (revision CommissionAgreementRevision) RegisteredAt() time.Time { return revision.registeredAt }

// SameDecisionAs 答本版与一份同依据的换版声明是不是同一个决定：后继与起用时刻都相同。决定人不比——他是出处。
func (revision CommissionAgreementRevision) SameDecisionAs(spec CommissionAgreementRevisionSpec) bool {
	return revision.successor == spec.Successor && sameOnRegistry(revision.appliesFrom, spec.AppliesFrom)
}

// CommissionAgreementRevisionSpec 是登记方声明的一次换版。
type CommissionAgreementRevisionSpec struct {
	Successor   SupplierAgreementVersion
	AppliesFrom time.Time
	Basis       CommissionAgreementRevisionBasisReference
	DecidedBy   RegistrantReference
}

// CommissionAgreementRevisionDraft 是过了声明那一半的换版：后继还没核过，也就还没有快照与法人。与委托草稿同一个次序——声明不合法的
// 请求不该花一次跨上下文读，快照与法人只能由 Revise 从核对结果里来。
type CommissionAgreementRevisionDraft struct {
	commission TransportCommission
	spec       CommissionAgreementRevisionSpec
	number     int
}

// DraftAgreementRevision 判一次换版声明的声明那一半：各格成立，起用时刻不早于委托提交，后继与委托指名的协议是同一个对象。
//
// 「委托指名的协议对象」取提交时记下的那一版；提交时没记下（迁移 TF 0038 之前）时取第一次换版的后继——那一次换版就是这份委托
// 第一次记下版本两格。
func (commission TransportCommission) DraftAgreementRevision(spec CommissionAgreementRevisionSpec) (CommissionAgreementRevisionDraft, error) {
	if !commission.commission.valid() || !spec.Successor.valid() || !spec.Basis.valid() || !spec.DecidedBy.valid() ||
		AtRegistryPrecision(spec.AppliesFrom).IsZero() {
		return CommissionAgreementRevisionDraft{}, ErrInvalidCommissionAgreementRevision
	}
	if AtRegistryPrecision(spec.AppliesFrom).Before(AtRegistryPrecision(commission.submittedAt)) {
		return CommissionAgreementRevisionDraft{}, ErrRevisionAppliesBeforeSubmission
	}
	if object, named := commission.agreementObject(); named && object != spec.Successor.Object() {
		return CommissionAgreementRevisionDraft{}, ErrRevisionAgreementObjectMismatch
	}
	return CommissionAgreementRevisionDraft{
		commission: commission,
		spec:       spec,
		number:     submittedAgreementRevision + len(commission.revisions) + 1,
	}, nil
}

func (commission TransportCommission) agreementObject() (string, bool) {
	if commission.agreementVersion.valid() {
		return commission.agreementVersion.Object(), true
	}
	if len(commission.revisions) > 0 {
		return commission.revisions[0].successor.Object(), true
	}
	return "", false
}

// Successor 是待核的后继版本。
func (draft CommissionAgreementRevisionDraft) Successor() SupplierAgreementVersion {
	return draft.spec.Successor
}

// AppliesFrom 是起用时刻，也是核后继「在那一刻支持采购」所用的那一刻。
func (draft CommissionAgreementRevisionDraft) AppliesFrom() time.Time {
	return draft.spec.AppliesFrom.UTC()
}

// Revise 以核过的快照与采购责任法人落下这一版，交回带上它的委托与这一版本身。草稿不变，同一份草稿可以再交一次。
func (draft CommissionAgreementRevisionDraft) Revise(
	snapshot AgreementSnapshotReference,
	legalEntity ProcurementLegalEntityReference,
	registeredAt time.Time,
) (TransportCommission, CommissionAgreementRevision, error) {
	if !snapshot.valid() || !legalEntity.valid() || AtRegistryPrecision(registeredAt).IsZero() || draft.number <= submittedAgreementRevision {
		return TransportCommission{}, CommissionAgreementRevision{}, ErrInvalidCommissionAgreementRevision
	}
	revision := CommissionAgreementRevision{
		number:       draft.number,
		successor:    draft.spec.Successor,
		snapshot:     snapshot,
		legalEntity:  legalEntity,
		appliesFrom:  draft.spec.AppliesFrom.UTC(),
		basis:        draft.spec.Basis,
		decidedBy:    draft.spec.DecidedBy,
		registeredAt: registeredAt.UTC(),
	}
	revised := draft.commission
	revised.members = slices.Clone(draft.commission.members)
	revised.revisions = append(slices.Clone(draft.commission.revisions), revision)
	return revised, revision, nil
}

// AgreementRevisions 交回委托上的各次换版，按号从小到大；提交那一版不在其中。
func (commission TransportCommission) AgreementRevisions() []CommissionAgreementRevision {
	return slices.Clone(commission.revisions)
}

// BoundAgreementSnapshot 答委托有没有按这一版协议快照采购过：提交那一版，或任一次换版的后继。按委托形成的事实带的是它业务时间所凭的
// 那一版（AgreementTermsAt），换过版之后可以是其中任一版，后一次换版又可以把前一次覆盖的那一段改掉——所以认的是全集，不是此刻那一版，
// 也不是按事实的业务时间重算的那一版。
func (commission TransportCommission) BoundAgreementSnapshot(snapshot AgreementSnapshotReference) bool {
	if commission.agreement == snapshot {
		return true
	}
	for _, revision := range commission.revisions {
		if revision.snapshot == snapshot {
			return true
		}
	}
	return false
}

// AgreementRevisionBy 按依据找一次换版：同一份决定只换一次版。
func (commission TransportCommission) AgreementRevisionBy(basis CommissionAgreementRevisionBasisReference) (CommissionAgreementRevision, bool) {
	for _, revision := range commission.revisions {
		if revision.basis == basis {
			return revision, true
		}
	}
	return CommissionAgreementRevision{}, false
}

// CommissionAgreementTerms 是委托在一个业务时刻按哪一版协议采购：版本号、协议版本（提交那一版可能没记下）、快照与采购责任法人。
type CommissionAgreementTerms struct {
	revision    int
	version     SupplierAgreementVersion
	snapshot    AgreementSnapshotReference
	legalEntity ProcurementLegalEntityReference
}

// Revision 是作答的那一版在委托上的号：1 是提交那一版。
func (terms CommissionAgreementTerms) Revision() int { return terms.revision }

// Version 是作答的那一版协议；提交那一版在迁移 TF 0038 之前没记下，答 false。
func (terms CommissionAgreementTerms) Version() (SupplierAgreementVersion, bool) {
	return terms.version, terms.version.valid()
}

func (terms CommissionAgreementTerms) Snapshot() AgreementSnapshotReference { return terms.snapshot }

func (terms CommissionAgreementTerms) LegalEntity() ProcurementLegalEntityReference {
	return terms.legalEntity
}

// AgreementTermsAt 答业务时刻 at 的事实按哪一版协议判（ADR-0231 决定四）：起用时刻不晚于 at 的各次换版里**最后登记**的那一版；
// 没有这样的换版就是提交那一版。
//
// 按登记先后取而不按起用时刻取：后一次换版说的是「自它的起用时刻起改按它」，起用时刻比前一次早，就连同前一次覆盖的那一段一并改掉
// ——运营人员发现前一次换错了，再换一次就能纠正，不必去撤前一次。按事实的业务时间取而不按此刻取：事实发生那一刻委托按哪一版采购，
// 不因事后换版而改变；那一刻之前起用的换版才管得着它。
func (commission TransportCommission) AgreementTermsAt(at time.Time) CommissionAgreementTerms {
	for index := len(commission.revisions) - 1; index >= 0; index-- {
		revision := commission.revisions[index]
		if !afterOnRegistry(revision.appliesFrom, at) {
			return CommissionAgreementTerms{
				revision:    revision.number,
				version:     revision.successor,
				snapshot:    revision.snapshot,
				legalEntity: revision.legalEntity,
			}
		}
	}
	return CommissionAgreementTerms{
		revision:    submittedAgreementRevision,
		version:     commission.agreementVersion,
		snapshot:    commission.agreement,
		legalEntity: commission.legalEntity,
	}
}

// RehydrateCommissionAgreementRevisionSpec 是换版行在库里的样子。
type RehydrateCommissionAgreementRevisionSpec struct {
	Number       int
	Successor    SupplierAgreementVersion
	Snapshot     AgreementSnapshotReference
	LegalEntity  ProcurementLegalEntityReference
	AppliesFrom  time.Time
	Basis        CommissionAgreementRevisionBasisReference
	DecidedBy    RegistrantReference
	RegisteredAt time.Time
}

// RehydrateCommissionAgreementRevision 验一行换版各格成立；它在委托上排第几、起用时刻与对象对不对得上委托，由
// RehydrateTransportCommission 连同整串一起验。
func RehydrateCommissionAgreementRevision(spec RehydrateCommissionAgreementRevisionSpec) (CommissionAgreementRevision, error) {
	if spec.Number <= submittedAgreementRevision || !spec.Successor.valid() || !spec.Snapshot.valid() || !spec.LegalEntity.valid() ||
		!spec.Basis.valid() || !spec.DecidedBy.valid() || spec.AppliesFrom.IsZero() || spec.RegisteredAt.IsZero() {
		return CommissionAgreementRevision{}, ErrInvalidRehydratedCommission
	}
	return CommissionAgreementRevision{
		number:       spec.Number,
		successor:    spec.Successor,
		snapshot:     spec.Snapshot,
		legalEntity:  spec.LegalEntity,
		appliesFrom:  spec.AppliesFrom.UTC(),
		basis:        spec.Basis,
		decidedBy:    spec.DecidedBy,
		registeredAt: spec.RegisteredAt.UTC(),
	}, nil
}

// validRevisionChain 验一串换版能挂在这份委托上：号从 2 起连续，依据两两不同，起用时刻不早于提交，后继都是委托指名的那个协议对象。
func validRevisionChain(commission TransportCommission, revisions []CommissionAgreementRevision) bool {
	seen := make(map[CommissionAgreementRevisionBasisReference]struct{}, len(revisions))
	object, named := "", false
	if commission.agreementVersion.valid() {
		object, named = commission.agreementVersion.Object(), true
	}
	for index, revision := range revisions {
		if revision.number != submittedAgreementRevision+index+1 {
			return false
		}
		if _, duplicate := seen[revision.basis]; duplicate {
			return false
		}
		seen[revision.basis] = struct{}{}
		if AtRegistryPrecision(revision.appliesFrom).Before(AtRegistryPrecision(commission.submittedAt)) {
			return false
		}
		if !named {
			object, named = revision.successor.Object(), true
		}
		if revision.successor.Object() != object {
			return false
		}
	}
	return true
}

package domain

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrInvalidGoodsSubmission 是提交途径、来源、键与依据引用凑不成一个自洽的提交：货主口要有来源而不带依据引用，代录口要有依据引用而没有来源。
	ErrInvalidGoodsSubmission = errors.New("warehouse fulfillment: invalid goods submission")
	// ErrInvalidGoodsRevision 是一笔商品修订缺了身份、提交、登记责任方或登记时刻。
	ErrInvalidGoodsRevision = errors.New("warehouse fulfillment: invalid goods revision")
)

// BasisRevision 是一次商品修订所基于的修订号。首次登记不带，零值就是没带：「没带」与「带了 0」是两回事，后者是一个不是当前修订的基准。
type BasisRevision struct {
	number int
	given  bool
}

func NewBasisRevision(number int) BasisRevision {
	return BasisRevision{number: number, given: true}
}

func (basis BasisRevision) Given() bool {
	return basis.given
}

// Number 是基准修订号；没带时是 0，调用方须先看 Given，不可把它当成「基于修订 0」。
func (basis BasisRevision) Number() int {
	return basis.number
}

// GoodsSubmission 是一笔商品修订的提交：途径、来源与键（判重空间是租户加这三样），代录另带依据引用。货主口的来源是客户渠道来源、键是来源请求键；
// 代录口没有来源、键是决定标识。
type GoodsSubmission struct {
	path     SubmissionPath
	source   SubmissionSource
	key      RequestKey
	evidence EvidenceReference
}

// NewOwnerSubmission 构造货主经客户渠道的提交：来源与键都要有，不带依据引用。
func NewOwnerSubmission(source SubmissionSource, key RequestKey) (GoodsSubmission, error) {
	if !source.valid() || !key.valid() {
		return GoodsSubmission{}, ErrInvalidGoodsSubmission
	}
	return GoodsSubmission{path: SubmissionByOwner, source: source, key: key}, nil
}

// NewOnBehalfSubmission 构造运营代录的提交：决定标识与依据引用都要有，没有来源。
func NewOnBehalfSubmission(key RequestKey, evidence EvidenceReference) (GoodsSubmission, error) {
	if !key.valid() || !evidence.valid() {
		return GoodsSubmission{}, ErrInvalidGoodsSubmission
	}
	return GoodsSubmission{path: SubmissionOnBehalf, key: key, evidence: evidence}, nil
}

func (submission GoodsSubmission) Path() SubmissionPath {
	return submission.path
}

// Source 交回货主口的来源；代录口没有来源，第二个返回值为假。
func (submission GoodsSubmission) Source() (SubmissionSource, bool) {
	return submission.source, submission.source.valid()
}

func (submission GoodsSubmission) Key() RequestKey {
	return submission.key
}

// EvidenceReference 交回代录的依据引用；货主口没有，第二个返回值为假。
func (submission GoodsSubmission) EvidenceReference() (EvidenceReference, bool) {
	return submission.evidence, submission.evidence.valid()
}

// valid 回答这个提交是不是经构造器得来的：两个构造器之外的零值与拼凑都不是。
func (submission GoodsSubmission) valid() bool {
	switch submission.path {
	case SubmissionByOwner:
		return submission.source.valid() && submission.key.valid() && !submission.evidence.valid()
	case SubmissionOnBehalf:
		return !submission.source.valid() && submission.key.valid() && submission.evidence.valid()
	default:
		return false
	}
}

// SameSlotAs 回答两个提交是不是占同一个判重位置：途径、来源与键都相同。依据引用不属于位置，是要比的内容（见 GoodsRequest）。
func (submission GoodsSubmission) SameSlotAs(other GoodsSubmission) bool {
	return submission.path == other.path && submission.source == other.source && submission.key == other.key
}

// GoodsRequest 是一次商品修订请求里判重要比的那几样：货主、商品编码、基准修订、完整资料、依据引用。同一个键下的两次提交，这几样都相同就是重放，
// 有一样不同就是来源冲突。登记责任方与登记时刻不比：重放可以换了操作者，也总在更晚的时刻。
type GoodsRequest struct {
	owner    OwnerAccountID
	code     GoodsCode
	basis    BasisRevision
	content  GoodsContent
	evidence EvidenceReference
}

func NewGoodsRequest(
	owner OwnerAccountID,
	code GoodsCode,
	basis BasisRevision,
	content GoodsContent,
	submission GoodsSubmission,
) GoodsRequest {
	return GoodsRequest{owner: owner, code: code, basis: basis, content: content, evidence: submission.evidence}
}

// SameAs 回答两次请求是不是同一份内容。
func (request GoodsRequest) SameAs(other GoodsRequest) bool {
	return request.owner == other.owner &&
		request.code == other.code &&
		request.basis == other.basis &&
		request.evidence == other.evidence &&
		request.content.SameAs(other.content)
}

// GoodsRevision 是商品册上的一笔修订：身份（租户，货主，商品编码）、修订号、完整资料，另记提交（途径、来源与键、代录的依据引用）、登记责任方与登记时刻。
// 修订从 1 起连续，只追加；货主口与代录口写的是同一条序列（CONTEXT「主数据登记」）。
type GoodsRevision struct {
	tenant       TenantID
	owner        OwnerAccountID
	code         GoodsCode
	revision     int
	content      GoodsContent
	submission   GoodsSubmission
	registrant   Registrant
	registeredAt time.Time
}

// NewGoodsRevision 判一笔修订的各部分是不是齐的。登记时刻收成库里存得下的精度（微秒）并统一到 UTC：不收的话，登记口回显的时刻与读回的差在
// 纳秒上。
func NewGoodsRevision(
	tenant TenantID,
	owner OwnerAccountID,
	code GoodsCode,
	revision int,
	content GoodsContent,
	submission GoodsSubmission,
	registrant Registrant,
	registeredAt time.Time,
) (GoodsRevision, error) {
	if revision < 1 {
		return GoodsRevision{}, fmt.Errorf("%w: %d", ErrInvalidRevisionNumber, revision)
	}
	if missing := content.MissingCells(); len(missing) > 0 {
		return GoodsRevision{}, &GoodsContentFieldError{Cell: missing[0]}
	}
	if !tenant.valid() || !owner.valid() || !code.valid() || !submission.valid() || !registrant.valid() || registeredAt.IsZero() {
		return GoodsRevision{}, ErrInvalidGoodsRevision
	}
	return GoodsRevision{
		tenant:       tenant,
		owner:        owner,
		code:         code,
		revision:     revision,
		content:      content,
		submission:   submission,
		registrant:   registrant,
		registeredAt: registeredAt.UTC().Truncate(time.Microsecond),
	}, nil
}

func (revision GoodsRevision) Tenant() TenantID {
	return revision.tenant
}

func (revision GoodsRevision) Owner() OwnerAccountID {
	return revision.owner
}

func (revision GoodsRevision) Code() GoodsCode {
	return revision.code
}

func (revision GoodsRevision) Revision() int {
	return revision.revision
}

func (revision GoodsRevision) Content() GoodsContent {
	return revision.content
}

func (revision GoodsRevision) Submission() GoodsSubmission {
	return revision.submission
}

func (revision GoodsRevision) Registrant() Registrant {
	return revision.registrant
}

func (revision GoodsRevision) RegisteredAt() time.Time {
	return revision.registeredAt
}

// Basis 是这一笔当初所基于的修订：修订 1 没有基准，其后每一笔基于它前一笔——落册要求基准就是当时的当前修订，所以不必另存。
func (revision GoodsRevision) Basis() BasisRevision {
	if revision.revision <= 1 {
		return BasisRevision{}
	}
	return NewBasisRevision(revision.revision - 1)
}

// Request 是这一笔当初的请求里判重要比的那几样。
func (revision GoodsRevision) Request() GoodsRequest {
	return NewGoodsRequest(revision.owner, revision.code, revision.Basis(), revision.content, revision.submission)
}

// IdentifierClaims 列出这一笔占用的码，次序同 GoodsContent.IdentifierClaims。
func (revision GoodsRevision) IdentifierClaims() []IdentifierClaim {
	return revision.content.IdentifierClaims(revision.code)
}

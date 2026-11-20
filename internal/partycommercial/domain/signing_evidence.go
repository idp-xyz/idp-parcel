package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
)

// 签署证据挂在客户合同或供应商协议的某一个版本旁，不进版本正文，也不改内容摘要。
// 签署件本身留在受限证据库：这里只收文件名、SHA-256 与库索引。来源本票只收线下签署。
// 登错了再追加一条并指名被更正的那一条，旧的一条留着。

var (
	ErrInvalidSigningEvidence            = errors.New("party commercial: invalid signing evidence")
	ErrSigningEvidenceCorrectsSelf       = errors.New("party commercial: a signing evidence cannot correct itself")
	ErrSigningEvidenceCrossVersion       = errors.New("party commercial: a signing evidence correction cannot cross versions")
	ErrSigningEvidenceAlreadyCorrected   = errors.New("party commercial: that signing evidence has already been corrected")
	ErrSigningEvidenceCorrectsUnknown    = errors.New("party commercial: the corrected signing evidence is not in the chain")
	ErrInvalidSigningEvidenceRequirement = errors.New("party commercial: invalid signing evidence requirement")
	// ErrSigningEvidenceRequired 是发布前提：租户登记了这一类要有签署证据，而这一版还没有。
	ErrSigningEvidenceRequired = errors.New("party commercial: signing evidence is required before this version can be published")
)

// SigningEvidenceSource 是签署证据来源的封闭集。本票只有线下签署；电子签章回执以后另立，不在这里占一格实现。
type SigningEvidenceSource uint8

const (
	SigningEvidenceSourceInvalid SigningEvidenceSource = iota
	SigningEvidenceOfflineSigned
)

func (source SigningEvidenceSource) String() string {
	switch source {
	case SigningEvidenceOfflineSigned:
		return "OFFLINE_SIGNED"
	default:
		return ""
	}
}

func SigningEvidenceSourceNamed(name string) (SigningEvidenceSource, bool) {
	if name == SigningEvidenceOfflineSigned.String() {
		return SigningEvidenceOfflineSigned, true
	}
	return SigningEvidenceSourceInvalid, false
}

// SigningEvidenceID 是登记方给出的证据标识。产品不签发：同一标识再交，由登记册按内容判重放或冲突。
type SigningEvidenceID struct{ requiredValue }

func NewSigningEvidenceID(value string) (SigningEvidenceID, error) {
	required, err := newRequiredValue("signing evidence ID", value)
	if err != nil {
		return SigningEvidenceID{}, fmt.Errorf("%w: %s", ErrInvalidSigningEvidence, err.Error())
	}
	return SigningEvidenceID{required}, nil
}

// SigningFile 是受限证据库里那一份签署件的外部引用：文件名、SHA-256、库索引。文件字节不进产品库。
type SigningFile struct {
	name         string
	sha256       string
	libraryIndex string
}

func NewSigningFile(name, digest, libraryIndex string) (SigningFile, error) {
	name = strings.TrimSpace(name)
	digest = strings.TrimSpace(digest)
	libraryIndex = strings.TrimSpace(libraryIndex)
	if name == "" || strings.ContainsAny(name, `/\`) || libraryIndex == "" || !sha256Hex(digest) {
		return SigningFile{}, fmt.Errorf("%w: the signing file needs a path-free name, a lowercase SHA-256 and a library index", ErrInvalidSigningEvidence)
	}
	return SigningFile{name: name, sha256: digest, libraryIndex: libraryIndex}, nil
}

func (file SigningFile) Name() string         { return file.name }
func (file SigningFile) SHA256() string       { return file.sha256 }
func (file SigningFile) LibraryIndex() string { return file.libraryIndex }

func (file SigningFile) valid() bool {
	return file.name != "" && sha256Hex(file.sha256) && file.libraryIndex != "" && !strings.ContainsAny(file.name, `/\`)
}

func sha256Hex(digest string) bool {
	if len(digest) != 64 {
		return false
	}
	for _, r := range digest {
		if !unicode.Is(unicode.ASCII_Hex_Digit, r) || unicode.IsUpper(r) {
			return false
		}
	}
	return true
}

// SigningEvidence 是挂在（租户、类别、对象、版本）旁的一条追加事实。
type SigningEvidence struct {
	id           SigningEvidenceID
	tenant       TenantID
	kind         CommercialObjectKind
	objectID     CommercialObjectID
	version      CommercialVersionLabel
	signedOn     time.Time
	counterparty string
	ourSide      string
	file         SigningFile
	source       SigningEvidenceSource
	corrects     SigningEvidenceID
	hasCorrects  bool
	recordedBy   OperatorSubjectReference
	recordedAt   time.Time
}

// NewSigningEvidence 建立一条证据。corrects 为零值表示这不是更正。双方签署人可缺，空串即缺席。
// signedOn 必须是 UTC 的日历日：带了钟点就不是签署日期。
func NewSigningEvidence(
	id SigningEvidenceID,
	tenant TenantID,
	kind CommercialObjectKind,
	objectID CommercialObjectID,
	version CommercialVersionLabel,
	signedOn time.Time,
	counterparty, ourSide string,
	file SigningFile,
	source SigningEvidenceSource,
	corrects SigningEvidenceID,
	recordedBy OperatorSubjectReference,
	recordedAt time.Time,
) (SigningEvidence, error) {
	evidence := SigningEvidence{
		id: id, tenant: tenant, kind: kind, objectID: objectID, version: version,
		signedOn: signedOn.UTC(), counterparty: strings.TrimSpace(counterparty), ourSide: strings.TrimSpace(ourSide),
		file: file, source: source, corrects: corrects, hasCorrects: corrects.valid(),
		recordedBy: recordedBy, recordedAt: recordedAt.UTC(),
	}
	if !evidence.valid() {
		return SigningEvidence{}, ErrInvalidSigningEvidence
	}
	if evidence.hasCorrects && evidence.corrects.String() == evidence.id.String() {
		return SigningEvidence{}, ErrSigningEvidenceCorrectsSelf
	}
	return evidence, nil
}

func (evidence SigningEvidence) valid() bool {
	return evidence.id.valid() && evidence.tenant.valid() && signingEvidenceKind(evidence.kind) &&
		evidence.objectID.valid() && evidence.version.valid() && civilUTCDate(evidence.signedOn) &&
		evidence.file.valid() && evidence.source == SigningEvidenceOfflineSigned &&
		evidence.recordedBy.valid() && !evidence.recordedAt.IsZero()
}

func signingEvidenceKind(kind CommercialObjectKind) bool {
	return kind == CustomerContractObject || kind == SupplierAgreementObject
}

func civilUTCDate(at time.Time) bool {
	if at.IsZero() {
		return false
	}
	utc := at.UTC()
	return utc.Hour() == 0 && utc.Minute() == 0 && utc.Second() == 0 && utc.Nanosecond() == 0
}

func (evidence SigningEvidence) ID() SigningEvidenceID                { return evidence.id }
func (evidence SigningEvidence) Tenant() TenantID                     { return evidence.tenant }
func (evidence SigningEvidence) Kind() CommercialObjectKind           { return evidence.kind }
func (evidence SigningEvidence) ObjectID() CommercialObjectID         { return evidence.objectID }
func (evidence SigningEvidence) Version() CommercialVersionLabel      { return evidence.version }
func (evidence SigningEvidence) SignedOn() time.Time                  { return evidence.signedOn }
func (evidence SigningEvidence) CounterpartySignatory() string        { return evidence.counterparty }
func (evidence SigningEvidence) OurSignatory() string                 { return evidence.ourSide }
func (evidence SigningEvidence) File() SigningFile                    { return evidence.file }
func (evidence SigningEvidence) Source() SigningEvidenceSource        { return evidence.source }
func (evidence SigningEvidence) RecordedBy() OperatorSubjectReference { return evidence.recordedBy }
func (evidence SigningEvidence) RecordedAt() time.Time                { return evidence.recordedAt }

func (evidence SigningEvidence) Corrects() (SigningEvidenceID, bool) {
	return evidence.corrects, evidence.hasCorrects
}

// SameContent 比的是这条事实的全部内容，含登记人与登记时刻：同标识再交时，差任何一格都是另一份内容。
func (evidence SigningEvidence) SameContent(other SigningEvidence) bool {
	corrects, has := evidence.Corrects()
	otherCorrects, otherHas := other.Corrects()
	return evidence.id.String() == other.id.String() &&
		evidence.tenant.String() == other.tenant.String() &&
		evidence.kind == other.kind &&
		evidence.objectID.String() == other.objectID.String() &&
		evidence.version.String() == other.version.String() &&
		evidence.signedOn.Equal(other.signedOn) &&
		evidence.counterparty == other.counterparty &&
		evidence.ourSide == other.ourSide &&
		evidence.file == other.file &&
		evidence.source == other.source &&
		has == otherHas && corrects.String() == otherCorrects.String() &&
		evidence.recordedBy.String() == other.recordedBy.String() &&
		evidence.recordedAt.Equal(other.recordedAt)
}

// AcceptCorrection 把 next 接到已有的证据链上。被更正的那一条必须在同一版本上，且还没有被别的一条更正过。
// history 里可以有别的版本：跨版本的那一条会在这里被拒，而不是被当成「找不到」。
func (evidence SigningEvidence) AcceptCorrection(history []SigningEvidence) error {
	corrects, ok := evidence.Corrects()
	if !ok || !evidence.valid() {
		return ErrInvalidSigningEvidence
	}
	if corrects.String() == evidence.id.String() {
		return ErrSigningEvidenceCorrectsSelf
	}
	var prior SigningEvidence
	found := false
	already := false
	for _, item := range history {
		if item.Tenant().String() != evidence.tenant.String() {
			continue
		}
		if item.ID().String() == corrects.String() {
			prior = item
			found = true
		}
		if other, isCorrection := item.Corrects(); isCorrection && other.String() == corrects.String() && item.ID().String() != evidence.id.String() {
			already = true
		}
	}
	if !found {
		return ErrSigningEvidenceCorrectsUnknown
	}
	if prior.Kind() != evidence.kind || prior.ObjectID().String() != evidence.objectID.String() || prior.Version().String() != evidence.version.String() {
		return ErrSigningEvidenceCrossVersion
	}
	if already {
		return ErrSigningEvidenceAlreadyCorrected
	}
	return nil
}

// SigningEvidenceRequirement 是租户要不要把签署证据当作发布前提。两个布尔分开：没登记这一行，与登记了但两个都不要，不是一回事。
type SigningEvidenceRequirement struct {
	tenant                    TenantID
	requiresCustomerContract  bool
	requiresSupplierAgreement bool
}

func NewSigningEvidenceRequirement(tenant TenantID, requiresCustomerContract, requiresSupplierAgreement bool) (SigningEvidenceRequirement, error) {
	rule := SigningEvidenceRequirement{
		tenant: tenant, requiresCustomerContract: requiresCustomerContract, requiresSupplierAgreement: requiresSupplierAgreement,
	}
	if !rule.valid() {
		return SigningEvidenceRequirement{}, ErrInvalidSigningEvidenceRequirement
	}
	return rule, nil
}

func (rule SigningEvidenceRequirement) valid() bool { return rule.tenant.valid() }

func (rule SigningEvidenceRequirement) Tenant() TenantID { return rule.tenant }
func (rule SigningEvidenceRequirement) RequiresCustomerContract() bool {
	return rule.requiresCustomerContract
}
func (rule SigningEvidenceRequirement) RequiresSupplierAgreement() bool {
	return rule.requiresSupplierAgreement
}

// Requires 只对能挂签署证据的两类有意义；其余类别这道门不存在。
func (rule SigningEvidenceRequirement) Requires(kind CommercialObjectKind) bool {
	switch kind {
	case CustomerContractObject:
		return rule.requiresCustomerContract
	case SupplierAgreementObject:
		return rule.requiresSupplierAgreement
	default:
		return false
	}
}

func (rule SigningEvidenceRequirement) SameContent(other SigningEvidenceRequirement) bool {
	return rule.tenant.String() == other.tenant.String() &&
		rule.requiresCustomerContract == other.requiresCustomerContract &&
		rule.requiresSupplierAgreement == other.requiresSupplierAgreement
}

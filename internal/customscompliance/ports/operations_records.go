package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// 关务运营查阅的四本记录读面（演示票 demo-2026-10-30/01）：案件与申报单元、提交版本与发送事实、外部监管结果
// 与放行结果、动作准入判断册。它们转述各自册里**已经存在的事实**，不触发判断、决定或披露，也不折叠任何结论——
// 技术回执、监管接收、业务受理、放行是分层保存的外部结果，读面逐层原样上列，任何一层都不推导下一层。
//
// 行类型用纯字符串与时间，不重建领域对象：这些册由各自的写口经领域构造入库，读面只做转述；
// 重建一遍只会让「绕过写口改过的坏数据」在查阅面上抛错而不是被看见，而查阅面本来就不该替写口把关。

// CaseParcelRecord 是案件关联的一个包裹。
type CaseParcelRecord struct {
	Parcel    string
	Customer  string
	SourceRef string
}

// CaseRoleRecord 是案件角色快照里的一项。
type CaseRoleRecord struct {
	Role      string
	Party     string
	Authority string
}

// DeclarationUnitRecord 是案件下的一个申报单元。
type DeclarationUnitRecord struct {
	Unit      string
	Procedure string
	Members   []string
	FormedAt  time.Time
	// ReplacesUnit 空即不替代任何单元。
	ReplacesUnit string
}

// CaseClosureRecord 是案件的关闭记录；ClosedAt 为零表示案件未关闭（本结构不会被构造）。
type CaseClosureRecord struct {
	DecidedBy   string
	CutoffAt    time.Time
	VerifiedAt  time.Time
	ClosedAt    time.Time
	ItemCount   int
	ReopenCount int
}

// CustomsCaseRecord 是一个关务案件连同其申报单元与关闭记录（如有）。
type CustomsCaseRecord struct {
	Case          string
	Jurisdiction  string
	Direction     string
	Procedure     string
	Obligation    string
	EstablishedAt time.Time
	Parcels       []CaseParcelRecord
	Roles         []CaseRoleRecord
	Units         []DeclarationUnitRecord
	Closure       *CaseClosureRecord
}

// CustomsCaseRecordRead 按租户上列案件。case 非空时只答该案件（单份读回）。
type CustomsCaseRecordRead interface {
	ListCustomsCases(
		ctx context.Context,
		tenant domain.TenantID,
		caseID string,
		limit int,
	) ([]CustomsCaseRecord, error)
}

// TechnicalReceiptRecord 是一份技术回执。
type TechnicalReceiptRecord struct {
	Source     string
	Outcome    string
	OccurredAt time.Time
	ReceivedAt time.Time
}

// SendFactRecord 是越过发送边界后记下的发送事实。
type SendFactRecord struct {
	SentAt         time.Time
	Outcome        string
	ChannelMessage string
	ResponseRef    string
}

// SubmissionAttemptRecord 是一次提交尝试连同发送事实与技术回执。SentAt 为零即尚未越过发送边界（已登记待发送）。
type SubmissionAttemptRecord struct {
	Sequence   int
	Target     string
	Result     string
	SafeResend string
	SentAt     time.Time
	SendFact   *SendFactRecord
	Receipts   []TechnicalReceiptRecord
}

// SubmissionVersionRecord 是一个提交版本连同全部尝试。
type SubmissionVersionRecord struct {
	Version        string
	Unit           string
	Procedure      string
	ContentDigest  string
	Members        []string
	Dossier        string
	Roles          string
	ReadinessBasis string
	Authority      string
	FixedAt        time.Time
	IsCurrent      bool
	CorrectedFrom  string
	Attempts       []SubmissionAttemptRecord
}

// SubmissionRecordRead 按租户上列提交版本；version 非空时只答该版本。
type SubmissionRecordRead interface {
	ListSubmissionVersions(
		ctx context.Context,
		tenant domain.TenantID,
		version string,
		limit int,
	) ([]SubmissionVersionRecord, error)
}

// ExternalResultReleaseRecord 是放行层结果带出的放行事实。
type ExternalResultReleaseRecord struct {
	Kind      string
	Authority string
	Scope     string
	Condition string
}

// ExternalResultViewRecord 是一份已接收的外部监管结果。Unattributable 为真即归属不上任何提交版本：
// 留存原始语义，不猜版本、不补造层次。
type ExternalResultViewRecord struct {
	Source            string
	Layer             string
	Role              string
	RawSemantics      string
	Rule              string
	SubmissionVersion string
	ClaimedVersion    string
	Scope             string
	Unattributable    bool
	LayerConflict     bool
	CorrectsSource    string
	CorrectionInForce bool
	OccurredAt        time.Time
	ReceivedAt        time.Time
	RecordedAt        time.Time
	Release           *ExternalResultReleaseRecord
}

// ExternalResultViewRead 按租户上列外部监管结果；version 非空时只答归属于该提交版本的。
type ExternalResultViewRead interface {
	ListExternalResults(
		ctx context.Context,
		tenant domain.TenantID,
		version string,
		limit int,
	) ([]ExternalResultViewRecord, error)
}

// AdmissionBasisRecord 是一项答复的依据。
type AdmissionBasisRecord struct {
	Cause     string
	Scope     string
	Procedure string
	Sources   []string
}

// AdmissionFindingRecord 是对一个受守护动作的逐动作答复。
type AdmissionFindingRecord struct {
	Action string
	Answer string
	Bases  []AdmissionBasisRecord
}

// AdmissionZoneRecord 是判断时读到的节点区域读数。
type AdmissionZoneRecord struct {
	Node       string
	Zone       string
	Registered bool
}

// ActionAdmissionJudgementRecord 是判断册里的一次判断。
type ActionAdmissionJudgementRecord struct {
	Judgement  string
	Object     string
	Intent     string
	Node       string
	OnwardNode string
	ActionAt   time.Time
	CrossZone  string
	Answer     string
	Zones      []AdmissionZoneRecord
	Findings   []AdmissionFindingRecord
	JudgedAt   time.Time
}

// ActionAdmissionJudgementRead 按租户上列判断册；object 非空时只答该对象（包裹或申报单元）的判断，最近的在前。
type ActionAdmissionJudgementRead interface {
	ListActionAdmissionJudgements(
		ctx context.Context,
		tenant domain.TenantID,
		object string,
		limit int,
	) ([]ActionAdmissionJudgementRecord, error)
}

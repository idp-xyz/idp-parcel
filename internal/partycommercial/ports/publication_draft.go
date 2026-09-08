package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 待批准发布载体与审批职责规则的端口（ADR-0126 Decision 三）。单立一个文件而不进 ports.go：
// 载体不是商业版本，任何解析读不到它，与 PublicationRegistry 那一族的只增登记不同源——它是本上下文
// 唯一一处就地更新的行（`待批准`期间的修订替换行、批准与发布推进状态），理由写在 ADR-0126 Decision 三：
// 权威记录是发布后的 commercial_version，那一册照旧只插不改。

// PublicationDraftSubmitOutcome 是录入落点的封闭代数（ADR-0031 的纪律：重放与修订都不是 error）。
type PublicationDraftSubmitOutcome uint8

const (
	PublicationDraftSubmitOutcomeInvalid PublicationDraftSubmitOutcome = iota
	// PublicationDraftSaved：新的一行。
	PublicationDraftSaved
	// PublicationDraftReplayed：同版本同内容再录，行一字不动——不论载体此刻在哪一格。
	PublicationDraftReplayed
	// PublicationDraftRevised：`待批准`期间换了内容，行被替换（正文、摘要、录入者与时刻随之更新）。
	PublicationDraftRevised
	// PublicationDraftContentFixed：载体已`已批准`或`已发布`，内容固定；换内容要另起版本号。
	PublicationDraftContentFixed
)

func (outcome PublicationDraftSubmitOutcome) String() string {
	switch outcome {
	case PublicationDraftSaved:
		return "SAVED"
	case PublicationDraftReplayed:
		return "REPLAYED"
	case PublicationDraftRevised:
		return "REVISED"
	case PublicationDraftContentFixed:
		return "CONTENT_FIXED"
	default:
		return ""
	}
}

// PublicationDraftAdvanceOutcome 是状态推进（批准 / 发布）落点的封闭代数。推进带条件：库上那一行必须仍是
// 推进所基于的那份（前一格状态、同一摘要），否则答`已被替换`而不是盖过去——两个操作者对同一载体先后动手时，
// 后到的那个要重读再来。
type PublicationDraftAdvanceOutcome uint8

const (
	PublicationDraftAdvanceOutcomeInvalid PublicationDraftAdvanceOutcome = iota
	PublicationDraftAdvanced
	PublicationDraftAdvanceNotFound
	PublicationDraftAdvanceSuperseded
)

func (outcome PublicationDraftAdvanceOutcome) String() string {
	switch outcome {
	case PublicationDraftAdvanced:
		return "ADVANCED"
	case PublicationDraftAdvanceNotFound:
		return "NOT_FOUND"
	case PublicationDraftAdvanceSuperseded:
		return "SUPERSEDED"
	default:
		return ""
	}
}

// PublicationDraftRegistry 是载体的存取面：录入（一版一行）、按版本身份点读、推进状态。
//
// 读回的载体经 domain.RehydratePublicationDraft 重建，快照折回再算一遍与列上摘要比；写口按框架合同无事务
// 即拒（RequireExecutor），录入 / 批准 / 发布三口各自一笔事务，发布那一笔与受控发布用例同事务。
type PublicationDraftRegistry interface {
	SubmitDraft(ctx context.Context, draft domain.PublicationDraft) (PublicationDraftSubmitOutcome, error)
	LoadDraft(
		ctx context.Context,
		tenant domain.TenantID,
		kind domain.CommercialObjectKind,
		objectID domain.CommercialObjectID,
		version domain.CommercialVersionLabel,
	) (domain.PublicationDraft, bool, error)
	// AdvanceDraft 把一份已在领域推进过状态的载体写回：`已批准`写回时库上必须是`待批准`，`已发布`写回时
	// 库上必须是`已批准`，且摘要相同。
	AdvanceDraft(ctx context.Context, draft domain.PublicationDraft) (PublicationDraftAdvanceOutcome, error)
}

// ApprovalDutyRuleSaveOutcome 是审批职责规则登记落点的封闭代数，判据同各正文册（ADR-0031）。
type ApprovalDutyRuleSaveOutcome uint8

const (
	ApprovalDutyRuleSaveOutcomeInvalid ApprovalDutyRuleSaveOutcome = iota
	ApprovalDutyRuleSaved
	ApprovalDutyRuleAlreadyRegistered
	ApprovalDutyRuleContentConflict
)

func (outcome ApprovalDutyRuleSaveOutcome) String() string {
	switch outcome {
	case ApprovalDutyRuleSaved:
		return "SAVED"
	case ApprovalDutyRuleAlreadyRegistered:
		return "ALREADY_REGISTERED"
	case ApprovalDutyRuleContentConflict:
		return "CONTENT_CONFLICT"
	default:
		return ""
	}
}

// ApprovalDutyRuleView 按租户读审批职责规则（PAR-COM-18）。found=false 就是未登记——批准门据此答`未配置`、
// 不放行（ADR-0126 Decision 三；分界句同 ADR-0052：读一个空登记册并如实答未配置不是默认实现）。
type ApprovalDutyRuleView interface {
	LoadApprovalDutyRule(ctx context.Context, tenant domain.TenantID) (domain.ApprovalDutyRule, bool, error)
}

// ApprovalDutyRuleRegistry 在读口之上加登记写口。规则是租户治理参数（实例半边）：今天没有治理写面调它，
// 写口只给装配与测试用；登记面归治理写面那一族按 ADR-0085 决定四另裁（ADR-0126 Decision 五）。一租户一条、
// 撞键不覆盖：同内容是重放，异内容是冲突——改规则今天没有入口，等那一族的裁决。
type ApprovalDutyRuleRegistry interface {
	ApprovalDutyRuleView
	SaveApprovalDutyRule(ctx context.Context, rule domain.ApprovalDutyRule) (ApprovalDutyRuleSaveOutcome, error)
}

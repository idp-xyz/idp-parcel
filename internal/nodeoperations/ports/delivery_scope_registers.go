package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件是可验证交付范围各本册与号码形状门的端口（ADR-0212 决定二、三；票 ps-external-mark-relations/08）：送站预报册、送站
// 交接单册、预报有效时窗册。各本册都只增：写口撞键时不覆盖，回显册上那一行，内容是不是同一份由调用方比——重放与冲突怎么答，归
// 各自的命令口（预报口、交接单命令口、时窗登记），册不替它们定。

// DeliveryScopeAppendOutcome 是这几本册写口的封闭答案。
type DeliveryScopeAppendOutcome uint8

const (
	DeliveryScopeAppendInvalid DeliveryScopeAppendOutcome = iota
	// DeliveryScopeAppended：新落一行，回显的就是它。
	DeliveryScopeAppended
	// DeliveryScopeAlreadyRecorded：撞上册上已有的一行（同一来源请求键、同一事实身份、同一结论键，或时窗区间重叠），回显那一行，什么
	// 也没落。
	DeliveryScopeAlreadyRecorded
	// DeliveryScopeTargetNotFound：指名的预报或交接单在本租户查不到，或撤回方不是预报的客户账户、结论的客户账户不在声称之列。几种
	// 同答，不泄露别的客户账户的东西在不在（UC-NO-006「权限、隔离与审计」）。
	DeliveryScopeTargetNotFound
	// DeliveryScopeNoteClosed：向已关闭的交接单加成员。关闭冻结成员；同一事实在关闭前已落的，照旧答已在册。
	DeliveryScopeNoteClosed
)

func (outcome DeliveryScopeAppendOutcome) String() string {
	switch outcome {
	case DeliveryScopeAppended:
		return "APPENDED"
	case DeliveryScopeAlreadyRecorded:
		return "ALREADY_RECORDED"
	case DeliveryScopeTargetNotFound:
		return "TARGET_NOT_FOUND"
	case DeliveryScopeNoteClosed:
		return "NOTE_CLOSED"
	default:
		return ""
	}
}

// DeliveryPreAdviceRegister 是送站预报册。写口要求环境事务。
type DeliveryPreAdviceRegister interface {
	// RecordDeliveryPreAdvice 落一份预报。同一（租户，来源，来源请求键）已在册时答已在册，回显那一份——它带着当初签发的标识与内容
	// 摘要，调用方据摘要分重放与来源冲突。
	RecordDeliveryPreAdvice(ctx context.Context, preAdvice domain.DeliveryPreAdvice) (domain.DeliveryPreAdvice, DeliveryScopeAppendOutcome, error)
	// RecordDeliveryPreAdviceWithdrawal 落一份撤回。预报不在本租户、或不属撤回方的客户账户，答查无此预报；同一来源请求键已在册时
	// 回显那一份撤回，否则这份预报已撤回时回显它的撤回（撤回是终态）。
	RecordDeliveryPreAdviceWithdrawal(ctx context.Context, withdrawal domain.DeliveryPreAdviceWithdrawal) (domain.DeliveryPreAdviceWithdrawal, DeliveryScopeAppendOutcome, error)
	// FindDeliveryPreAdvice 读一份预报此刻的证据状态。found=false 即本租户没有这份预报。
	FindDeliveryPreAdvice(ctx context.Context, tenant domain.TenantID, id domain.DeliveryPreAdviceID) (domain.DeliveryPreAdviceState, bool, error)
}

// DeliveryHandoverNoteRegister 是送站交接单册：交接单、成员、关闭与绑定结论。写口要求环境事务。
type DeliveryHandoverNoteRegister interface {
	// OpenDeliveryHandoverNote 落一张交接单；同一标识已在册时回显册上那一张。
	OpenDeliveryHandoverNote(ctx context.Context, note domain.DeliveryHandoverNote) (domain.DeliveryHandoverNote, DeliveryScopeAppendOutcome, error)
	// AppendDeliveryHandoverMember 落一个成员。同一（租户，形成方式，事实身份）已在册时回显那一个；交接单不在册答查无此单，已关闭答
	// 已关闭。关闭与加成员在同一张交接单上串行：关闭之后不会再落进成员。
	AppendDeliveryHandoverMember(ctx context.Context, member domain.DeliveryHandoverMember) (domain.DeliveryHandoverMember, DeliveryScopeAppendOutcome, error)
	// CloseDeliveryHandoverNote 落一份关闭；已关闭时回显第一份关闭。
	CloseDeliveryHandoverNote(ctx context.Context, closure domain.DeliveryHandoverClosure) (domain.DeliveryHandoverClosure, DeliveryScopeAppendOutcome, error)
	// RecordDeliveryHandoverBinding 落一份绑定结论。（租户，交接单，客户账户）已有结论时回显那一份，不覆盖；交接单不在册、或客户账户
	// 不在声称之列，答查无此单。
	RecordDeliveryHandoverBinding(ctx context.Context, binding domain.DeliveryHandoverBinding) (domain.DeliveryHandoverBinding, DeliveryScopeAppendOutcome, error)
	// FindDeliveryHandoverNote 读一张交接单此刻的证据状态（交接单、关闭、各结论）。found=false 即本租户没有这张交接单。
	FindDeliveryHandoverNote(ctx context.Context, tenant domain.TenantID, id domain.DeliveryHandoverNoteID) (domain.DeliveryHandoverNoteState, bool, error)
	// ListDeliveryHandoverMembers 按落册先后列一张交接单的成员。
	ListDeliveryHandoverMembers(ctx context.Context, tenant domain.TenantID, id domain.DeliveryHandoverNoteID) ([]domain.DeliveryHandoverMember, error)
	// FindLastAssignedHandoverNote 交回一件作业实物最后归入的交接单（ADR-0212 决定三「一件实物的依据取它最后归入的交接单」）：只看
	// 归入，不看收寄带来的成员；「最后」按归入的业务时间认，同一时刻按落册先后。found=false 即没归入过。
	FindLastAssignedHandoverNote(ctx context.Context, tenant domain.TenantID, unit domain.HandlingUnitID) (domain.DeliveryHandoverNoteID, bool, error)
}

// PreAdviceWindowRegister 是预报有效时窗册（PAR-NET-18）。写口要求环境事务。
type PreAdviceWindowRegister interface {
	// RegisterPreAdviceWindow 落一行时窗。同租户生效区间与在册行重叠时什么也不落，回显撞上的那一行（不止一行时取起点最早的）——
	// 原样重登撞上的就是它自己。重叠由库面排他约束拒，并发两笔各自查不到对方也只进一笔。
	RegisterPreAdviceWindow(ctx context.Context, window domain.PreAdviceValidityWindow) (domain.PreAdviceValidityWindow, DeliveryScopeAppendOutcome, error)
	// FindPreAdviceWindowAt 读租户在时刻 at 生效的那一行。found=false 即那一刻没有生效行：推导据此答未配置，不拿别的行或默认时长
	// 顶替。
	FindPreAdviceWindowAt(ctx context.Context, tenant domain.TenantID, at time.Time) (domain.PreAdviceValidityWindow, bool, error)
}

// ErrPreAdvisedNumberNotAdmitted 说明一个号码进不了预报：全空白、带 NUL，或超过 PS 入册门的字节界。它是确定的，同一个号再交多少次
// 都一样，提交口据此答坏载荷。
var ErrPreAdvisedNumberNotAdmitted = errors.New("node operations: pre-advised parcel number not admitted")

// PreAdvisedNumberGate 是预报号码的形状门。界与 PS 入册门同一道（ADR-0212 决定二；票 ps-external-mark-relations/06 裁定一）：预报
// 列的号要与 PS 关联册里的号逐字比，册里存不下的号预报里列了也永远比不上。那一界归 PS 的领域，跨上下文不得直接导入
// （internal/architecture 的边界门禁），所以照防腐层的做法立成端口，实现落在 adapters/parcelshipment，界值只在 PS 一处。
type PreAdvisedNumberGate interface {
	AdmitPreAdvisedNumber(value string) (domain.PreAdvisedParcelNumber, error)
}

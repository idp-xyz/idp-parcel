package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件是收寄后服务处置决定要读、要记与要交出的口（ADR-0336）：越过取消边界的包裹，客户的停止请求停在待处置（CancellationRecord 的
// RecordDispositionPending），有权主体对它或对一件包裹自己发起，形成一份只追加的决定；决定向各对象所有者提出范围化请求，由信封交出。

// PostIntakeDispositionRequestOrigin 说明一份待回应的请求为什么待回应。
//
//   - 越界待处置：取消请求到达提交边界时有效网络收寄已在册，取消不成立，记为待处置。
//   - 取消被认定不成立：取消曾经成立并答过`包裹已取消`，后来被取消先后冲突的解开决定认定依据不成立（ADR-0295）——边界事实成立、提出取消时包裹其实已越界。
//     客户要停的意思没有变，只是这份取消不是有效取消，所以它同样等一份收寄后服务处置决定来回应。
type PostIntakeDispositionRequestOrigin uint8

const (
	PostIntakeDispositionRequestOriginInvalid PostIntakeDispositionRequestOrigin = iota
	PostIntakeRequestBoundaryCrossed
	PostIntakeRequestCancellationNotUpheld
)

func (origin PostIntakeDispositionRequestOrigin) String() string {
	switch origin {
	case PostIntakeRequestBoundaryCrossed:
		return "BOUNDARY_CROSSED_AT_REQUEST"
	case PostIntakeRequestCancellationNotUpheld:
		return "CANCELLATION_NOT_UPHELD"
	default:
		return ""
	}
}

// PostIntakeDispositionRequest 是一份等收寄后服务处置决定回应的客户停止请求。请求方、授权、原因和请求的业务时间在待处置记录里随记录保全；这些列在
// 先于保全落下的待处置记录上为空，读方不得把空当成「没有请求方」，只能当成当时没有保全。
type PostIntakeDispositionRequest struct {
	Key          CancellationRequestKey
	Origin       PostIntakeDispositionRequestOrigin
	IntakeBefore domain.SourceResultVersion
	Cancellation domain.ParcelCancellationID
	Requester    domain.CancellationRequesterReference
	Authority    domain.CancellationAuthorityReference
	Reason       domain.CancellationReasonReference
	RequestedAt  time.Time
	RecordedAt   time.Time
	// CurrentVersion 是这件包裹决定链当前的版本，没有决定时为零：决定人要取代它时命令里带的就是这个数。
	CurrentVersion int
}

// UnansweredPostIntakeDispositionRequests 是一个租户还没有被回应的请求：最早的在前，至多 limit 行；Count 是全部行数，大于 len(Requests) 即清单被页大小截断。
type UnansweredPostIntakeDispositionRequests struct {
	Requests []PostIntakeDispositionRequest
	Count    int
}

// PostIntakeDispositionRequests 读等回应的请求。租户条件进每条语句，否定结果不区分「没有」与「属于另一个租户」。
//
// 清单排除两类：已被某份决定回应的；包裹此刻已有当前终局的——终局之后不再有收寄后服务处置可做（新需求建立关联委托），留在清单里只会堆成永远清不掉的行。
type PostIntakeDispositionRequests interface {
	// FindPostIntakeDispositionRequest 读指名的那份请求；它不是待处置记录、也不是被认定不成立的取消时答 false。
	FindPostIntakeDispositionRequest(
		ctx context.Context,
		key CancellationRequestKey,
	) (PostIntakeDispositionRequest, bool, error)
	ListUnansweredPostIntakeDispositionRequests(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) (UnansweredPostIntakeDispositionRequests, error)
}

// PostIntakeDispositionAppendOutcome 是追加一份决定的封闭结果。`版本已被占`说明决定链上这个版本先被别的写入占了——锁内不该发生，它是锁之外写入的最后一道
// 兜底，调用方按「前一份已变」处理。
type PostIntakeDispositionAppendOutcome uint8

const (
	PostIntakeDispositionAppendOutcomeInvalid PostIntakeDispositionAppendOutcome = iota
	PostIntakeDispositionAppended
	PostIntakeDispositionVersionTaken
)

// PostIntakeDispositions 是收寄后服务处置决定的留痕：每个包裹一条线性的决定链，只追加，当前决定是链尾。
//
// 写口要环境事务：决定留痕与交出信封同笔落地，留痕落了信封没入队，所有者就永远收不到请求。读口按（租户，包裹）切，租户条件进每条语句。
type PostIntakeDispositions interface {
	// FindCurrentPostIntakeDisposition 读包裹决定链的链尾。
	FindCurrentPostIntakeDisposition(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) (domain.PostIntakeDisposition, bool, error)
	// FindPostIntakeDispositionAt 读链上指名的一版。
	FindPostIntakeDispositionAt(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
		version int,
	) (domain.PostIntakeDisposition, bool, error)
	// FindPostIntakeDispositionAnswering 读回应了指名请求的那份决定。
	FindPostIntakeDispositionAnswering(
		ctx context.Context,
		key CancellationRequestKey,
	) (domain.PostIntakeDisposition, bool, error)
	// ListPostIntakeDispositionChain 读整条链，版本小的在前。
	ListPostIntakeDispositionChain(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) ([]domain.PostIntakeDisposition, error)
	AppendPostIntakeDisposition(
		ctx context.Context,
		disposition domain.PostIntakeDisposition,
	) (PostIntakeDispositionAppendOutcome, error)
}

// PostIntakeDispositionHandoffIntent 把一份刚形成的决定交给各对象所有者：携带决定本身，信封载荷（去向、范围、对各所有者的请求、与原旅程的关系）都由它派生。
type PostIntakeDispositionHandoffIntent struct {
	Disposition domain.PostIntakeDisposition
}

// PostIntakeDispositionHandoff 把收寄后服务处置决定写入 Outbox。信封 ID 由（租户，包裹，决定版本）认领，一个版本至多一封。它必须与决定留痕同一事务交出
// （ADR-0094 决定四：触发与格同笔落地），任一步失败全部回滚。
type PostIntakeDispositionHandoff interface {
	HandOffPostIntakeDisposition(ctx context.Context, intent PostIntakeDispositionHandoffIntent) error
}

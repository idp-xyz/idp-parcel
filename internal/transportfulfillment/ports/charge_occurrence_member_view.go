package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 发生项成员的窄只读口（pp-seams/01）。另开一份文件不并进 `charge_occurrence.go`：那份文件是
// 登记册那一侧的形，这里是给持引用方读成员用的第二份契约，两份契约的实现者相同、说的话不同。

// ChargeOccurrenceMembers 是一个发生项版本被读走的成员切面：成员载运对象清单、业务时点与
// 主要业务范围（都是 CONTEXT「运输收费发生项」固定下来的，其余不在此交）。
//
// Members 就是登记时的 `CarriedObjectReference` 原样切片，**不带种类**：TF 不铸正式包裹身份
// 与集运单元（`CarriedObjectReference` 头注），按引用前缀或形状猜「这是包裹还是集运单元」
// 等于替 parcel-shipment / node-operations 解释它们的标识空间；成员是集运单元时也**不展开**
// 到成员包裹——集运成员关系归 node-operations，展开归持引用方的消费侧。
type ChargeOccurrenceMembers struct {
	Members    []domain.CarriedObjectReference
	OccurredAt time.Time
	Scope      domain.OccurrenceScopeReference
}

// ChargeOccurrenceMemberView 按发生项键答那一版的成员切面，供持发生项引用的上下文读成员用。
//
// 与 `ChargeOccurrenceRegistry` 分开而不是让调用方拿 `FindByKey`：那个口带首登方法，依赖它等于
// 声明自己可能写发生项（`FailedAttemptSource` 头注同一条理由——契约窄一格，说的话就准一格）；
// 它交的还是整条 `ChargeOccurrenceRecord`，而读成员的一方不该看见协议、数量与修订。
// `ChargeOccurrences` 适配器同时满足两口，不需要第二个实现。
//
// 键精确到有效性版本：键不存在答 found=false，不答「最近一版」或「当前版本」——
// `ChargeOccurrenceKey` 头注已拒绝任何「哪个是当前」的第二口径，本口不另立一套。
//
// Members 的顺序是契约：成员按引用字面升序，与 `FindByKey` 读回的成员同序。持引用方要把成员
// 清单拿去进计价输入快照的指纹，序不定则同一发生项两次读出两个指纹；两口同序是为了同一行
// 从哪一口读出来都是同一份清单，实现者不得各排各的。
type ChargeOccurrenceMemberView interface {
	LoadMembers(ctx context.Context, key ChargeOccurrenceKey) (ChargeOccurrenceMembers, bool, error)
}

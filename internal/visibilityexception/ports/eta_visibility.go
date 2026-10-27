package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// ETACurrentRead 读内部预测各条来源线的当前版，是 ETA 客户可见性判断取候选 ETA 的读面。预测线的键是
// （租户+包裹+里程碑+来源）：承运商提供的与运营企业派生的预测各是一条线，互不覆盖。只读：判断引用候选版本，不改预测。
type ETACurrentRead interface {
	// FindCurrent 读一条来源线的当前版。
	FindCurrent(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.TrackedParcelReference,
		milestone domain.MilestoneReference,
		source domain.ETASourceKind,
	) (domain.ETAPrediction, bool, error)
	// ListCurrent 一次读出（租户+包裹+里程碑）下每条来源线的当前版，没有预测时交回空表。两路来源并存时
	// 它们来自同一时点的一份快照，判断据此选候选（domain.CandidateETAFrom），不会读到一路新一路旧的拼接。
	ListCurrent(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.TrackedParcelReference,
		milestone domain.MilestoneReference,
	) ([]domain.ETAPrediction, error)
}

// ETAVisibilityRuleView 回答「这个租户的这个货主客户账户在判断时点适用哪一条 ETA 对客规则」
// （`PAR-VIS-03` 的对客质量门槛与 `PAR-COM-07` 的合同与客户服务规则，CONTEXT 把这两样都划为
// 租户取值）。本上下文只消费答复：没有登记即交回「未配置」，同一时点不止一条适用即交回「冲突」，
// 两者都是如实的答复，不是错误，更不能按某个默认值补成可见。依赖调不通才作为错误返回，由应用层
// 形成未决——读不动是未决，缺失与冲突是确定的业务答案（ADR-0029）。
//
// 适用版本按判断时点选，不按查询当下：规则换版只作用于生效后的新判断。租户在签名上看得见
// （ADR-0003）：客户账户引用只在租户内唯一。
type ETAVisibilityRuleView interface {
	AssessETAVisibilityRule(
		ctx context.Context,
		tenant domain.TenantID,
		customer domain.CustomerAccountReference,
		at time.Time,
	) (domain.ETAVisibilityRuleBasis, error)
}

// ETAVisibilityJudgementRead 读一个客户账户在一件包裹上的当前 ETA 客户可见性判断。客户视图
// 派生读它回答 ETA 一维；账户在键上，跨账户的判断互不可见。
type ETAVisibilityJudgementRead interface {
	FindCurrent(
		ctx context.Context,
		tenant domain.TenantID,
		customer domain.CustomerAccountReference,
		parcel domain.TrackedParcelReference,
	) (domain.ETAVisibilityJudgement, bool, error)
}

// ETAVisibilityJudgementStore 保存 ETA 客户可见性判断版本。键含租户与客户账户——租户是最高数据
// 隔离边界（ADR-0003），同一件包裹在不同客户账户名下各有各的判断。
//
// 版本只增不改写：替代追加新版本并回指前一版，被替代的判断连同依据一并留存、按版本读得回
// （UC-VE-003 规则「原判断不覆盖」）；当前版由显式标记指名。一个账户在一件包裹上同时只有
// 一条判断线，里程碑是判断的属性不是键的一维——客户视图只有一维 ETA，规则指名哪个里程碑
// 的 ETA 是客户要的那一个。
//
// Save 走事务（RequireExecutor），首版只在尚无当前版时成立，替代只在当前版恰是它所指的前一版
// 时成立；零行命中交回 ErrCurrentJudgementChanged，输家重读再走，不覆盖赢家。
//
// 锁序：投影行（CurrentProjectionView.LockCurrent）→ 判断 → 客户视图。判断与视图派生在同一
// 事务里先后写，投影锁在最前，与投影派生、客户归属补派生两路取锁方向一致。
type ETAVisibilityJudgementStore interface {
	ETAVisibilityJudgementRead
	FindVersion(
		ctx context.Context,
		tenant domain.TenantID,
		customer domain.CustomerAccountReference,
		parcel domain.TrackedParcelReference,
		version domain.ETAVisibilityJudgementVersionID,
	) (domain.ETAVisibilityJudgement, bool, error)
	Save(ctx context.Context, tenant domain.TenantID, judgement domain.ETAVisibilityJudgement) error
}

// ETAVisibilityJudgementIdentityFactory 签发判断版本标识。与预测、视图身份工厂分开，理由相同：
// 不同用例触发，合并会让一个编排依赖它根本不签发的身份。
type ETAVisibilityJudgementIdentityFactory interface {
	NextETAVisibilityJudgementVersionID(ctx context.Context) (domain.ETAVisibilityJudgementVersionID, error)
}

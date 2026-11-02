package application

import (
	"context"
	"errors"
	"fmt"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// CaseNumberingRedriveInterval 是一件撞号的命中再试一次之前等多久（产品策略）。
//
// 撞号要等人在「设置 → 单据编号」换格式，或周期换了、旧号让出——都是分钟以上的事。每再试一次是事务内一次取号：最多从序列
// 取跳号上限那么多个值（在保存点里，撞了就退回）加同样多次「是否已用」的索引查，几十毫秒，并短暂占着该租户该周期的序列行；
// 五分钟一回，换完格式的人最多等五分钟就看到案件建起来，没换的时候也不会在同一段旧号上反复空撞、跟正常开立抢那一行锁。
//
// 重驱停在依赖上（或出错）的那几件同样推后这一个间隔（Defer）：撞号命中已经等过了，晚一个间隔建案不改变什么；不推后，
// 读不动的那几件到期时刻不变，每拍都排在页首，页一满后面真能建成的就永远轮不到。
const CaseNumberingRedriveInterval = 5 * time.Minute

// ErrCaseNumberingRuleCollision 标出「案件单据号仍撞号」的那一类重驱结果：派发进程据它把这一类单独说给看日志的人——要人去
// 换格式，不是依赖坏了，也不是数据矛盾。
var ErrCaseNumberingRuleCollision = errors.New("visibility exception: exception case numbering rule collision")

// ErrCaseNumberingRedriveUndecided 表示重驱那一次分诊停在依赖上（规则视图、发作期库、标识签发），或建成了而分诊意图没交出去：
// 调用方回滚这一笔，册上那一行原样留着，下一拍再来。
var ErrCaseNumberingRedriveUndecided = errors.New("visibility exception: exception case numbering redrive is undecided")

// CaseNumberingRedriveOutcome 是重驱一件撞号命中的答案。
type CaseNumberingRedriveOutcome uint8

const (
	CaseNumberingRedriveOutcomeInvalid CaseNumberingRedriveOutcome = iota
	// CaseNumberingRedriven：这一次分诊走完了（建了案、或这回分诊不建案、或这一原因已经有了活跃发作期），册上那一行销掉。
	CaseNumberingRedriven
	// CaseNumberingStillColliding：又撞了，册上那一行推后 CaseNumberingRedriveInterval。
	CaseNumberingStillColliding
	// CaseNumberingRedriveSkipped：没抢到这一行——另一个派发进程正在重驱它，或它已被销掉、推后。什么都不做，不算失败。
	CaseNumberingRedriveSkipped
)

func (outcome CaseNumberingRedriveOutcome) String() string {
	switch outcome {
	case CaseNumberingRedriven:
		return "REDRIVEN"
	case CaseNumberingStillColliding:
		return "STILL_COLLIDING"
	case CaseNumberingRedriveSkipped:
		return "SKIPPED"
	default:
		return ""
	}
}

// CaseNumberingRedrive 把案件撞号册上到期的命中按原样再交一次分诊（CONTEXT「异常案件」：撞号的那次命中留待规则册换格式后
// 再分诊）。分诊规则按租户绑定，所以交分诊的那一只（SignalRaiser）由调用方按行上的租户给；本类型只管到期、推后与销账。
type CaseNumberingRedrive struct {
	queue ports.CaseNumberingCollisionQueue
	clock ports.Clock
}

func NewCaseNumberingRedrive(queue ports.CaseNumberingCollisionQueue, clock ports.Clock) (*CaseNumberingRedrive, error) {
	if queue == nil || clock == nil {
		return nil, errors.New("visibility exception: case numbering redrive needs a queue and a clock")
	}
	return &CaseNumberingRedrive{queue: queue, clock: clock}, nil
}

// Due 交回此刻到期的撞号命中，最多 limit 件。
func (redrive *CaseNumberingRedrive) Due(ctx context.Context, limit int) ([]ports.CaseNumberingCollision, error) {
	return redrive.queue.DueCaseNumberingCollisions(ctx, redrive.clock.Now(), limit)
}

// Redrive 在调用方的事务里把一件撞号命中按原样再交一次分诊。先抢册上那一行（锁不到、已不在、不再到期就答跳过）：取页与
// 分诊之间隔着别的派发进程与正常命中，按取页时那一份直接分诊会让同一原因立两件案。命令照抢到的那一行重建——首次撞号的
// 那次命中，判断时间仍是当初那一刻：发作期没落过，这一回就是它的第一次分诊；分诊规则若已改成不建案，就按新规则走——
// 「是否建立案件重新经过分诊规则」。
func (redrive *CaseNumberingRedrive) Redrive(
	ctx context.Context,
	raiser SignalRaiser,
	due ports.CaseNumberingCollision,
) (CaseNumberingRedriveOutcome, error) {
	if raiser == nil {
		return CaseNumberingRedriveOutcomeInvalid, errors.New("visibility exception: case numbering redrive needs a signal raiser")
	}
	collision, claimed, err := redrive.queue.ClaimDueCaseNumberingCollision(ctx, due.Key(), redrive.clock.Now())
	if err != nil {
		return CaseNumberingRedriveOutcomeInvalid, err
	}
	if !claimed {
		return CaseNumberingRedriveSkipped, nil
	}
	hit := collision.Hit
	result, err := raiser.Handle(ctx, RaiseSignalCommand{
		TenantID:   collision.Tenant,
		Kind:       hit.Kind,
		Parcel:     hit.Parcel,
		Cause:      hit.Cause,
		Rule:       hit.Rule,
		Confidence: hit.Confidence,
		Basis:      hit.Basis,
		HitAt:      hit.At,
	})
	if err != nil {
		return CaseNumberingRedriveOutcomeInvalid, err
	}
	key := collision.Key()
	switch result.Outcome() {
	case CaseNumberingCollided:
		// 编排已把这一次覆盖记进册里（并让它到期）；推后，免得下一拍就在同一段旧号上再撞一回。
		if err := redrive.queue.PostponeCaseNumberingCollision(ctx, key, redrive.clock.Now().Add(CaseNumberingRedriveInterval)); err != nil {
			return CaseNumberingRedriveOutcomeInvalid, err
		}
		return CaseNumberingStillColliding, nil
	case SignalEpisodeOpened, SignalEpisodeReopened, SignalHitRecorded:
		if result.HandoffReference() != "" {
			return CaseNumberingRedriveOutcomeInvalid, fmt.Errorf("%w: triage handoff pending %s", ErrCaseNumberingRedriveUndecided, result.HandoffReference())
		}
		if err := redrive.queue.ResolveCaseNumberingCollision(ctx, key); err != nil {
			return CaseNumberingRedriveOutcomeInvalid, err
		}
		return CaseNumberingRedriven, nil
	case RaiseSignalUndecided:
		return CaseNumberingRedriveOutcomeInvalid, fmt.Errorf("%w: %s", ErrCaseNumberingRedriveUndecided, result.UndecidedReason())
	default:
		// 未受理说明册上那一行重建不出一次完整的命中——那是写坏了，不是业务答案；封闭集合外同样上抛。
		return CaseNumberingRedriveOutcomeInvalid, fmt.Errorf("visibility exception: case numbering redrive got %q", result.Outcome())
	}
}

// Defer 把一件这一拍没重驱成的命中（停在依赖上或出错，事务已回滚）推后一个间隔，在调用方另开的事务里调。行不在了不算错。
func (redrive *CaseNumberingRedrive) Defer(ctx context.Context, key ports.CaseNumberingCollisionKey) error {
	return redrive.queue.PostponeCaseNumberingCollision(ctx, key, redrive.clock.Now().Add(CaseNumberingRedriveInterval))
}

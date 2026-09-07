package postgres

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 本文件是终局规则声明父行上那一格面单有效期（0026，ADR-0119）在持久化面的两个方向：列 → 领域、
// 领域 → 列。两列同在同缺由库上 CHECK 钉住，这里读回时仍逐格核——半缺的行进不来，读到就是库与
// 领域分叉，报错不吸收。

// microsecondsPerDay 是 PostgreSQL interval 里「天」段到微秒的换算。interval 的月份段不换算：月不是
// 固定时长，批文口（ISO-8601 子集禁年月周）写不出它，读到即坏数据。
const microsecondsPerDay = 24 * 60 * 60 * 1_000_000

// labelValidityFromColumns 把父行两列折回领域声明。anchor 为 NULL 即「没有这一格」，第二个返回值为 false。
func labelValidityFromColumns(anchor *string, duration pgtype.Interval) (domain.LabelValidityDeclaration, bool, error) {
	none := domain.LabelValidityDeclaration{}
	if anchor == nil && !duration.Valid {
		return none, false, nil
	}
	if anchor == nil || !duration.Valid {
		return none, false, fmt.Errorf("label validity columns are half-present; the database and the domain have diverged")
	}
	kind, err := validityAnchorKindFrom(*anchor)
	if err != nil {
		return none, false, err
	}
	if duration.Months != 0 {
		return none, false, fmt.Errorf("label validity duration carries a month segment, which is not a fixed duration")
	}
	total := time.Duration(int64(duration.Days)*microsecondsPerDay+duration.Microseconds) * time.Microsecond
	declaration, err := domain.NewLabelValidityDeclaration(kind, total)
	if err != nil {
		return none, false, err
	}
	return declaration, true, nil
}

// labelValidityColumns 把领域声明拆成两列；没有这一格时两列都是 NULL（interval 以 Valid=false 编码为 NULL）。
// 时长只写微秒段：领域的 time.Duration 没有「天」这一层，写回天段会让同一个时长有两种列写法、比对失配。
func labelValidityColumns(content domain.FinalRuleContent) (*string, pgtype.Interval) {
	validity, declared := content.Validity()
	if !declared {
		return nil, pgtype.Interval{}
	}
	anchor := validity.Anchor().String()
	return &anchor, pgtype.Interval{Microseconds: validity.Duration().Microseconds(), Valid: true}
}

// sameLabelValidity 按「没有 / 有且相等」比对既有列与新声明，供 SaveFinalRule 的重放 / 冲突判读。
func sameLabelValidity(existingAnchor *string, existingDuration pgtype.Interval, content domain.FinalRuleContent) (bool, error) {
	existing, present, err := labelValidityFromColumns(existingAnchor, existingDuration)
	if err != nil {
		return false, err
	}
	incoming, declared := content.Validity()
	if present != declared {
		return false, nil
	}
	return !present || existing == incoming, nil
}

func validityAnchorKindFrom(raw string) (domain.ValidityAnchorKind, error) {
	for _, kind := range []domain.ValidityAnchorKind{domain.ChannelResultObservedAnchor} {
		if kind.String() == raw {
			return kind, nil
		}
	}
	return domain.ValidityAnchorKindInvalid, fmt.Errorf("unknown validity anchor kind %q", raw)
}

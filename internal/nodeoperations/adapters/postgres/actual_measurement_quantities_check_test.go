package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

const (
	// quantityConstraint 是库里守称重量方四个数量列的那条 CHECK。
	quantityConstraint = "actual_measurement_quantities_positive_decimal"
	// presentConstraint 是更早就在的非空约束：空串与只有空白的串它也拒得掉。
	presentConstraint = "actual_measurement_weight_and_dimensions_present"

	quantityRowSQL = `INSERT INTO node_operations.actual_measurement
		(tenant_id, source_id, content_digest, object_kind, object_id, node_ref, location_ref, basis_ref,
		 weight_value, weight_unit, length_value, width_value, height_value, dimension_unit,
		 performed_by, evidence, occurred_at, recorded_at)
		VALUES ($1, $2, 'syn-digest', 'HANDLING_UNIT', 'SYN-HU-1', 'SYN-NODE-1', 'SYN-BENCH-1', 'SYN-BASIS-1',
		        $3, 'KG', $4, $5, $6, 'CM', 'SYN-SCALE-1', 'WEIGH/1', now(), now())`
)

// 本文件对真实 PostgreSQL 证称重量方的数量约束与领域构造器是同一个口径。夹具全为合成（S 级）。

// quantityColumns 是一条测量的四个数量，按列名排开：毛重、长、宽、高。
type quantityColumns struct{ weight, length, width, height string }

// legalQuantities 是四格都合法的一组数量，用例只替换其中被测的那一格。
var legalQuantities = quantityColumns{weight: "2.10", length: "20", width: "15", height: "10"}

// insertQuantities 绕开应用往登记册直接写一行，数量由用例给。事实行要先在来源身份登记表占位才写得进，占位照规矩先做；
// 被约束拒的写入占位会留下，所以每次写入各用一个来源身份。
func insertQuantities(ctx context.Context, pool *pgxpool.Pool, sourceID string, quantities quantityColumns) error {
	if _, err := pool.Exec(ctx, claimSourceIdentitySQL, sourceIdentityTenant, sourceID, "MEASUREMENT"); err != nil {
		return fmt.Errorf("占来源身份 %s：%w", sourceID, err)
	}
	_, err := pool.Exec(ctx, quantityRowSQL, sourceIdentityTenant, sourceID,
		quantities.weight, quantities.length, quantities.width, quantities.height)
	return err
}

// wantCheckViolationOf 要求写入被指名的 CHECK 之一拒掉：SQLSTATE 23514 且约束名在 constraints 里。只看「报了错」不够，
// 别的约束或类型错误也会报错；认 SQLSTATE 与约束名，才是这条约束在拒。
func wantCheckViolationOf(t *testing.T, what string, err error, constraints ...string) {
	t.Helper()
	var refusal *pgconn.PgError
	if !errors.As(err, &refusal) {
		t.Fatalf("%s：库没有拒（err = %v），want 违反 CHECK %v", what, err, constraints)
	}
	if refusal.Code != "23514" {
		t.Fatalf("%s：被拒了但不是 CHECK 违反：%s %s（%s），want 23514 %v", what, refusal.Code, refusal.ConstraintName, refusal.Message, constraints)
	}
	for _, constraint := range constraints {
		if refusal.ConstraintName == constraint {
			return
		}
	}
	t.Fatalf("%s：被拒了但不是想要的那道约束：%s，want %v", what, refusal.ConstraintName, constraints)
}

// Covers: 库里的数量约束与领域构造器是同一个口径。每个存下的文本以 domain.NewGrossWeight 的判断为准，测试里不再另写一遍解析：
//   - 构造器拒 → 把这串写进 weight_value，库必须拒（23514）；
//   - 构造器收且 Value() 与原文相同 → 写入必须成功；
//   - 构造器收但 Value() 与原文不同（去了首尾空白）→ 原文写入必须被库拒：应用存的是 Value()，库里不会有带空白的那一串。
//
// 空串与只有空白的串两道约束都拒得掉：更早就在的非空约束与数量约束。PostgreSQL 按约束名序检查 CHECK，实测先拒的是数量约束，
// 但先后取决于两个约束名的排序，不是这里要钉住的口径，所以这几格只断言 23514，约束名两者之一都认。
// 文本含带换行的串（尾部换行、中间换行）、全角数字与 Unicode 空白：PostgreSQL 的 $ 默认只匹配串尾，[0-9] 是码位区间，
// 它们与构造器的判断是否一致由这里实测。
func TestTheQuantityConstraintAgreesWithTheDomainConstructor(t *testing.T) {
	pool := pgtest.Pool(t)
	ctx := t.Context()

	for index, stored := range []struct {
		text string
		// blank 的格两道约束都拒得掉，约束名两者之一都认。
		blank bool
	}{
		{text: "1"},
		{text: "0.1"},
		{text: "0.10"},
		{text: "01"},
		{text: "10.50"},
		{text: "2.10"},
		{text: "2.100"},
		{text: "0"},
		{text: "0.0"},
		{text: "00"},
		{text: "0.00"},
		{text: "2.1x"},
		{text: "+2.10"},
		{text: "-1"},
		{text: "1."},
		{text: ".5"},
		{text: "1.2.3"},
		{text: "1e2"},
		{text: " 1"},
		{text: "1 "},
		{text: "1\n"},
		{text: "1\n2"},
		{text: "\n1"},
		{text: "1\r\n"},
		{text: "\t1"},
		{text: "1\t"},
		{text: "\u00a01"},
		{text: "1\u2003"},
		{text: "\uff11"},
		{text: "", blank: true},
		{text: " ", blank: true},
		{text: " \t\n", blank: true},
	} {
		t.Run(fmt.Sprintf("%02d %q", index, stored.text), func(t *testing.T) {
			sourceID := fmt.Sprintf("SYN-QTY/%02d", index)
			gross, constructorErr := domain.NewGrossWeight(stored.text, "KG")
			err := insertQuantities(ctx, pool, sourceID, quantityColumns{
				weight: stored.text, length: legalQuantities.length, width: legalQuantities.width, height: legalQuantities.height,
			})

			constraints := []string{quantityConstraint}
			if stored.blank {
				constraints = append(constraints, presentConstraint)
				if constructorErr == nil {
					t.Fatalf("%q 被当成空白格，构造器却收下了它", stored.text)
				}
			}
			switch {
			case constructorErr != nil:
				wantCheckViolationOf(t, fmt.Sprintf("构造器拒的 %q", stored.text), err, constraints...)
			case gross.Value() == stored.text:
				if err != nil {
					t.Fatalf("构造器收下、存下的就是原文的 %q，库却拒了：%v", stored.text, err)
				}
			default:
				wantCheckViolationOf(t, fmt.Sprintf("构造器收下但会去掉空白成 %q 的原文 %q", gross.Value(), stored.text), err, constraints...)
			}
		})
	}
}

// Covers: 四个数量列都在约束里，不止毛重：长、宽、高各写一次不合口径的值（其余三列合法），库都拒。每一列各写两种：整串不是十进制数、
// 是十进制数却没有 1–9（零），分别压着约束的两半。不合口径与否仍以领域构造器为准，不在这里另写判断。
func TestEveryQuantityColumnIsUnderTheQuantityConstraint(t *testing.T) {
	pool := pgtest.Pool(t)
	ctx := t.Context()

	columns := []struct {
		name  string
		place func(quantities quantityColumns, value string) quantityColumns
	}{
		{"毛重", func(q quantityColumns, value string) quantityColumns { q.weight = value; return q }},
		{"长", func(q quantityColumns, value string) quantityColumns { q.length = value; return q }},
		{"宽", func(q quantityColumns, value string) quantityColumns { q.width = value; return q }},
		{"高", func(q quantityColumns, value string) quantityColumns { q.height = value; return q }},
	}
	index := 0
	for _, column := range columns {
		for _, bad := range []string{"2.1x", "0"} {
			index++
			t.Run(fmt.Sprintf("%s %q", column.name, bad), func(t *testing.T) {
				quantities := column.place(legalQuantities, bad)
				// 构造器是不合口径的判据：这一组数量它要拒，夹具才算选对了。
				_, weightErr := domain.NewGrossWeight(quantities.weight, "KG")
				_, dimensionsErr := domain.NewOuterDimensions(quantities.length, quantities.width, quantities.height, "CM")
				if errors.Join(weightErr, dimensionsErr) == nil {
					t.Fatalf("夹具有误：构造器收下了 %+v", quantities)
				}
				err := insertQuantities(ctx, pool, fmt.Sprintf("SYN-COL/%02d", index), quantities)
				wantCheckViolationOf(t, fmt.Sprintf("%s写成 %q", column.name, bad), err, quantityConstraint)
			})
		}
	}

	t.Run("四格都合法照收", func(t *testing.T) {
		if err := insertQuantities(ctx, pool, "SYN-COL/legal", legalQuantities); err != nil {
			t.Fatalf("四格都合法的那一行应当写得进：%v", err)
		}
	})
}

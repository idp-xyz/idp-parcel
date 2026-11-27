package postgres_test

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

func (register operatorRegister) saveName(
	t *testing.T, tenant string, subject accessidentity.OperatorSubject, name string, by accessidentity.OperatorSubject,
) accessidentity.OperatorNameOutcome {
	t.Helper()
	var outcome accessidentity.OperatorNameOutcome
	if err := register.transactor.WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var saveErr error
		outcome, saveErr = register.registry.SaveOperatorName(txCtx, tenant, subject, name, by.Issuer()+"#"+by.Subject())
		return saveErr
	}); err != nil {
		t.Fatalf("登姓名事务：%v", err)
	}
	return outcome
}

func (register operatorRegister) names(t *testing.T, tenant string) map[string]string {
	t.Helper()
	rows, err := register.registry.ListOperatorNames(t.Context(), tenant)
	if err != nil {
		t.Fatalf("读姓名：%v", err)
	}
	names := map[string]string{}
	for _, row := range rows {
		names[row.Subject.Subject()] = row.Name
	}
	return names
}

// 姓名可以登、可以改、可以删；只落在主体所绑的那个租户里，别的租户既登不上也读不到。
func TestOperatorNameIsRecordedReplacedAndClearedWithinItsTenant(t *testing.T) {
	register := newOperatorRegister(t)
	administrator := subjectOf(t, syntheticIssuer, "SYN-OPERATOR-01")
	picker := subjectOf(t, syntheticIssuer, "SYN-OPERATOR-02")
	register.bind(t, bindingOf(t, administrator, tenantOne, "SYN-OPERATOR-BASIS-01"))
	register.bind(t, bindingOf(t, picker, tenantOne, "SYN-OPERATOR-BASIS-02"))

	if got := register.names(t, tenantOne); len(got) != 0 {
		t.Fatalf("没登过姓名时读到 %v", got)
	}
	if got := register.saveName(t, tenantOne, picker, "合成甲", administrator); got != accessidentity.OperatorNameRecorded {
		t.Fatalf("登姓名 = %s", got)
	}
	if got := register.saveName(t, tenantOne, picker, "合成乙", administrator); got != accessidentity.OperatorNameRecorded {
		t.Fatalf("改姓名 = %s", got)
	}
	if got := register.names(t, tenantOne); len(got) != 1 || got["SYN-OPERATOR-02"] != "合成乙" {
		t.Fatalf("改过之后读到 %v，要只有改后的那一个", got)
	}
	recorded := func() (by string, at time.Time) {
		t.Helper()
		if err := register.pool.QueryRow(t.Context(),
			`SELECT recorded_by, recorded_at FROM access_identity.operator_name WHERE issuer = $1 AND subject = $2`,
			syntheticIssuer, "SYN-OPERATOR-02").Scan(&by, &at); err != nil {
			t.Fatal(err)
		}
		return by, at
	}
	firstBy, firstAt := recorded()
	if firstBy != syntheticIssuer+"#SYN-OPERATOR-01" {
		t.Fatalf("登的人 = %s", firstBy)
	}
	// 同一个姓名再登一遍是重放：答复照旧是登上了，但那一行不动——谁登的、什么时候登的留着头一次的。
	if got := register.saveName(t, tenantOne, picker, "合成乙", picker); got != accessidentity.OperatorNameRecorded {
		t.Fatalf("重放 = %s", got)
	}
	if by, at := recorded(); by != firstBy || !at.Equal(firstAt) {
		t.Fatalf("重放之后登的人 %s、时刻 %s，要仍是 %s、%s", by, at, firstBy, firstAt)
	}

	// 别的租户：给这个主体登不上，也读不到它的姓名；不存在的主体同答。
	if got := register.saveName(t, tenantTwo, picker, "合成丙", administrator); got != accessidentity.OperatorNameOperatorNotFound {
		t.Fatalf("另一租户登姓名 = %s", got)
	}
	if got := register.saveName(t, tenantOne, subjectOf(t, syntheticIssuer, "SYN-OPERATOR-GHOST"), "合成丙", administrator); got != accessidentity.OperatorNameOperatorNotFound {
		t.Fatalf("给不在册的主体登姓名 = %s", got)
	}
	if got := register.names(t, tenantTwo); len(got) != 0 {
		t.Fatalf("另一租户读到 %v", got)
	}
	if got := register.names(t, tenantOne); got["SYN-OPERATOR-02"] != "合成乙" {
		t.Fatalf("被拒的登记改动了姓名：%v", got)
	}

	if got := register.saveName(t, tenantOne, picker, "", administrator); got != accessidentity.OperatorNameCleared {
		t.Fatalf("删姓名 = %s", got)
	}
	if got := register.saveName(t, tenantOne, picker, "", administrator); got != accessidentity.OperatorNameCleared {
		t.Fatalf("本来就没有时删姓名 = %s", got)
	}
	if got := register.names(t, tenantOne); len(got) != 0 {
		t.Fatalf("删掉之后读到 %v", got)
	}
}

// 操作者册的查阅行带姓名，q 也在姓名上找；没登姓名的操作者照常上列、姓名一格为空，连上姓名不多出行、总数不变。
func TestOperatorCatalogueCarriesAndSearchesTheName(t *testing.T) {
	register := newOperatorRegister(t)
	pages := accessidentity.OperatorCataloguePages
	named := seedOperator(t, register, syntheticIssuer, pageSubject(1), "SYN-BINDING-ALPHA", pageAt(1))
	seedOperator(t, register, syntheticIssuer, pageSubject(2), "SYN-BINDING-BETA", pageAt(2))
	register.saveName(t, tenantOne, named, "合成甲", named)

	all := operatorPage(t, register, 10, pageQuery(t, pages, url.Values{}))
	assertOperatorPage(t, "全册", all, []int{1, 2}, 2)
	if all.Rows[0].Name() != "合成甲" || all.Rows[1].Name() != "" {
		t.Fatalf("姓名 = %q、%q，要第一位有、第二位空", all.Rows[0].Name(), all.Rows[1].Name())
	}

	found := operatorPage(t, register, 10, pageQuery(t, pages, url.Values{"q": {"成甲"}}))
	assertOperatorPage(t, "按姓名找", found, []int{1}, 1)
	// 没登姓名的操作者照常按主体标识找得到：姓名那一格是 NULL，不把整行的匹配带成 NULL。
	assertOperatorPage(t, "按主体找没登姓名的", operatorPage(t, register, 10, pageQuery(t, pages, url.Values{"q": {"p02"}})), []int{2}, 1)
}

// Covers: PBC-08——登姓名只经框架事务，拿不到事务句柄时答 ErrTransactionRequired。
func TestOperatorNameWriteRefusesToRunOutsideATransaction(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, syntheticIssuer, "SYN-OPERATOR-01")
	if _, err := register.registry.SaveOperatorName(t.Context(), tenantOne, subject, "合成甲", "SYN-RECORDER"); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务登姓名：err = %v, want ErrTransactionRequired", err)
	}
}

// 表自己守得住：空白姓名、超长姓名、指向别的租户的行都落不进去。
func TestOperatorNameTableGuardsItsInvariants(t *testing.T) {
	register := newOperatorRegister(t)
	subject := subjectOf(t, syntheticIssuer, "SYN-OPERATOR-01")
	register.bind(t, bindingOf(t, subject, tenantOne, "SYN-OPERATOR-BASIS-01"))

	const insert = `INSERT INTO access_identity.operator_name (issuer, subject, tenant_id, name, recorded_by) VALUES ($1, $2, $3, $4, $5)`
	expectSQLState(t, register.pool, "空白姓名", checkViolation, insert, syntheticIssuer, "SYN-OPERATOR-01", tenantOne, "  ", "SYN-X")
	expectSQLState(t, register.pool, "超长姓名", checkViolation, insert, syntheticIssuer, "SYN-OPERATOR-01", tenantOne,
		longName(accessidentity.MaxOperatorNameLength+1), "SYN-X")
	expectSQLState(t, register.pool, "别的租户", foreignKeyViolation, insert, syntheticIssuer, "SYN-OPERATOR-01", tenantTwo, "合成甲", "SYN-X")
}

func longName(length int) string {
	runes := make([]rune, length)
	for index := range runes {
		runes[index] = '甲'
	}
	return string(runes)
}

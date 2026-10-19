package domain_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

var judgedAt = time.Date(2026, 8, 12, 16, 0, 0, 0, time.UTC)

func judgmentSpec(t *testing.T, mode domain.JudgmentMode) domain.ComplianceJudgmentSpec {
	t.Helper()
	return domain.ComplianceJudgmentSpec{
		Topic:      mustValue(t, domain.NewComplianceTopicReference, "COMMODITY_CLASSIFICATION"),
		Scope:      mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		Rule:       mustValue(t, domain.NewComplianceRuleVersionReference, "hs-rules/v7"),
		Facts:      "declared description + node observation",
		Mode:       mode,
		Role:       mustValue(t, domain.NewRoleSnapshotReference, "roles/classifier-v1"),
		Conclusion: "HS 8517.62",
		JudgedAt:   judgedAt,
	}
}

// Covers: CC CONTEXT「自动与人工合规判断都必须保存适用规则版本、事实依据、决定方式和责任角色。人工判断不能删除自动判断，后续自动计算也不能覆盖已经形成的人工判断历史」——四件缺一立不起（自动与人工一视同仁）；替版指回前版方式与规则、
// 原判断不可变；跨事项替版不是同一条判断线。
func TestJudgmentsKeepTheirFourPartsAndNeverDeleteEachOther(t *testing.T) {
	automatic, err := domain.FormComplianceJudgment(judgmentSpec(t, domain.AutomaticJudgment))
	if err != nil {
		t.Fatalf("form automatic judgment: %v", err)
	}

	missingRole := judgmentSpec(t, domain.AutomaticJudgment)
	missingRole.Role = domain.RoleSnapshotReference{}
	if _, err := domain.FormComplianceJudgment(missingRole); !errors.Is(err, domain.ErrInvalidComplianceJudgment) {
		t.Fatalf("err = %v; 自动判断也不能少责任角色", err)
	}

	manualSpec := judgmentSpec(t, domain.ManualJudgment)
	manualSpec.Conclusion = "HS 8517.79"
	manualSpec.JudgedAt = judgedAt.Add(time.Hour)
	manual, err := automatic.Supersede(manualSpec)
	if err != nil {
		t.Fatalf("supersede with manual: %v", err)
	}
	priorMode, priorRule, present := manual.PriorJudgment()
	if !present || priorMode != domain.AutomaticJudgment || priorRule.String() != "hs-rules/v7" {
		t.Fatalf("prior = %s/%s present = %v; 人工替自动必须指回前版", priorMode, priorRule, present)
	}
	if automatic.Conclusion() != "HS 8517.62" {
		t.Fatal("原自动判断被删了")
	}

	foreign := judgmentSpec(t, domain.AutomaticJudgment)
	foreign.Topic = mustValue(t, domain.NewComplianceTopicReference, "ORIGIN")
	foreign.JudgedAt = judgedAt.Add(2 * time.Hour)
	if _, err := manual.Supersede(foreign); !errors.Is(err, domain.ErrInvalidComplianceJudgment) {
		t.Fatalf("err = %v; 跨事项的替版不是同一条判断线", err)
	}
}

// Covers: CC CONTEXT「监管凭证」语言「凭证具有独立身份、不可变版本、适用辖区、商品、程序、线路、有效期以及适用次数或额度；附件文件只是其证据，不能代替凭证身份和适用性判断」——七件缺一立不起（有效期倒置拒、负额度矛盾拒）；适用性按程序×持有人×时点
// 三维判，任一不符独立哨兵拒。
func TestCredentialApplicabilityIsAThreeWayJudgment(t *testing.T) {
	credential, err := domain.RegisterCredential(
		mustValue(t, domain.NewCredentialID, "permit-1"),
		mustValue(t, domain.NewRegulatoryAuthorityReference, "CUSTOMS/US-CBP"),
		mustValue(t, domain.NewCredentialHolderReference, "entity-cn-1"),
		mustValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		judgedAt,
		judgedAt.Add(365*24*time.Hour),
		0,
	)
	if err != nil {
		t.Fatalf("register credential: %v", err)
	}

	if err := credential.JudgeApplicability(
		mustValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		mustValue(t, domain.NewCredentialHolderReference, "entity-cn-1"),
		judgedAt.Add(30*24*time.Hour),
	); err != nil {
		t.Fatalf("judge applicability: %v", err)
	}

	if err := credential.JudgeApplicability(
		mustValue(t, domain.NewCustomsProcedureReference, "US-EXPORT/EEI"),
		mustValue(t, domain.NewCredentialHolderReference, "entity-cn-1"),
		judgedAt.Add(30*24*time.Hour),
	); !errors.Is(err, domain.ErrCredentialNotApplicable) {
		t.Fatalf("err = %v; 别的程序用上了这份凭证", err)
	}
	if err := credential.JudgeApplicability(
		mustValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		mustValue(t, domain.NewCredentialHolderReference, "entity-other"),
		judgedAt.Add(30*24*time.Hour),
	); !errors.Is(err, domain.ErrCredentialNotApplicable) {
		t.Fatalf("err = %v; 别的持有人用上了这份凭证", err)
	}
	if err := credential.JudgeApplicability(
		mustValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		mustValue(t, domain.NewCredentialHolderReference, "entity-cn-1"),
		judgedAt.Add(400*24*time.Hour),
	); !errors.Is(err, domain.ErrCredentialNotApplicable) {
		t.Fatalf("err = %v; 过期凭证还适用", err)
	}

	if _, err := domain.RegisterCredential(
		mustValue(t, domain.NewCredentialID, "permit-2"),
		mustValue(t, domain.NewRegulatoryAuthorityReference, "CUSTOMS/US-CBP"),
		mustValue(t, domain.NewCredentialHolderReference, "entity-cn-1"),
		mustValue(t, domain.NewCustomsProcedureReference, "US-IMPORT/TYPE-86"),
		judgedAt.Add(time.Hour),
		judgedAt,
		0,
	); !errors.Is(err, domain.ErrInvalidCredential) {
		t.Fatalf("err = %v; 有效期倒置被收下了", err)
	}
}

// Covers: 凭证登记按登记册存得下的值判（AtRegistryPrecision 与额度上限）——按纳秒放行的那几格入册后要么撞上库面的
// 有效期严格先后约束，要么读回成零时刻、整张凭证重建不出来；额度超出额度列 integer 的上限存不进去。
func TestCredentialRegistrationJudgesTheValuesTheRegistryCanStore(t *testing.T) {
	register := func(from, to time.Time, uses int) error {
		_, err := domain.RegisterCredential(
			mustValue(t, domain.NewCredentialID, "SYN-CRED-TFA11"),
			mustValue(t, domain.NewRegulatoryAuthorityReference, "SYN-AUTHORITY-TFA11"),
			mustValue(t, domain.NewCredentialHolderReference, "SYN-HOLDER-TFA11"),
			mustValue(t, domain.NewCustomsProcedureReference, "SYN-PROC-TFA11"),
			from, to, uses)
		return err
	}
	type input struct {
		from, to time.Time
		uses     int
	}
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	year := base.AddDate(1, 0, 0)
	for name, given := range map[string]input{
		"有效期两端落在同一微秒":   {base.Add(100 * time.Nanosecond), base.Add(900 * time.Nanosecond), 3},
		"起点在零时刻之后不足一微秒": {time.Time{}.Add(100 * time.Nanosecond), year, 3},
		"额度超出额度列的上限":    {base, year, math.MaxInt32 + 1},
	} {
		t.Run(name, func(t *testing.T) {
			if err := register(given.from, given.to, given.uses); !errors.Is(err, domain.ErrInvalidCredential) {
				t.Fatalf("err = %v，want ErrInvalidCredential——这一份永远存不进登记册", err)
			}
		})
	}
	for name, given := range map[string]input{
		"整秒":          {base, year, 3},
		"微秒":          {base.Add(123456 * time.Microsecond), year, 3},
		"两端跨一微秒":      {base.Add(100 * time.Nanosecond), base.Add(1100 * time.Nanosecond), 3},
		"起点恰在零时刻后一微秒": {time.Time{}.Add(time.Microsecond), year, 3},
		"额度恰在上限":      {base, year, math.MaxInt32},
		"额度未提供":       {base, year, 0},
	} {
		t.Run(name, func(t *testing.T) {
			if err := register(given.from, given.to, given.uses); err != nil {
				t.Fatalf("err = %v，want 照常登记", err)
			}
		})
	}
}

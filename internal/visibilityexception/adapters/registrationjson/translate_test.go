package registrationjson_test

import (
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

// 本文件证翻译层的两条纪律：字段照登记者的原话落到命令上（零默认、零改写），
// 不是这份登记快照形状的输入在翻译处就拒（未知字段、集合外取值、矛盾声明、构造门）。
//
// 走外部测试包：本包下沉后由受控 CLI 与在线登记口两侧消费，能证的只该是导出面——
// 从包内测私有件会让「导出面够用」这件事无人担保。

func TestMilestoneMappingTranslatesEveryField(t *testing.T) {
	raw := []byte(`{
		"tenantId": "SYN-TEN-VE15",
		"version": "SYN-MAP-V1",
		"approvedBy": "SYN-approver-1",
		"effectiveFrom": "2026-09-01T00:00:00Z",
		"entries": [
			{"source": "PARCEL_SHIPMENT", "factKind": "SYN_KIND_DELIVERED", "milestone": "SYN-MILESTONE-DELIVERED"}
		]
	}`)
	command, err := registrationjson.MilestoneMappingFromJSON(raw)
	if err != nil {
		t.Fatalf("翻译：%v", err)
	}
	if command.TenantID.String() != "SYN-TEN-VE15" {
		t.Fatalf("租户 = %q", command.TenantID.String())
	}
	if command.Header.Version != "SYN-MAP-V1" || command.Header.ApprovedBy != "SYN-approver-1" {
		t.Fatalf("抬头 = %+v", command.Header)
	}
	wantFrom := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if !command.Header.EffectiveFrom.Equal(wantFrom) || command.Header.HasEffectiveTo {
		t.Fatalf("生效区间 = %+v，要 [%s, 未闭)", command.Header, wantFrom)
	}
	if len(command.Entries) != 1 ||
		command.Entries[0].Source != domain.SourceParcelShipment ||
		command.Entries[0].Kind.String() != "SYN_KIND_DELIVERED" ||
		command.Entries[0].Milestone.String() != "SYN-MILESTONE-DELIVERED" {
		t.Fatalf("条目 = %+v", command.Entries)
	}
}

func TestVersionHeaderClosedRangeTranslates(t *testing.T) {
	raw := []byte(`{
		"tenantId": "SYN-TEN-VE15",
		"version": "SYN-MAP-V0",
		"approvedBy": "SYN-approver-1",
		"effectiveFrom": "2026-01-01T00:00:00Z",
		"effectiveTo": "2026-09-01T00:00:00Z",
		"entries": [
			{"source": "NODE_OPERATIONS", "factKind": "SYN_KIND_SCAN", "milestone": "SYN-MILESTONE-SCANNED"}
		]
	}`)
	command, err := registrationjson.MilestoneMappingFromJSON(raw)
	if err != nil {
		t.Fatalf("翻译：%v", err)
	}
	wantTo := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if !command.Header.HasEffectiveTo || !command.Header.EffectiveTo.Equal(wantTo) {
		t.Fatalf("闭区间没译出来：%+v", command.Header)
	}
}

func TestTriageRulesTranslateWithClosedOutcome(t *testing.T) {
	raw := []byte(`{
		"tenantId": "SYN-TEN-VE15",
		"version": "SYN-TRI-V1",
		"approvedBy": "SYN-approver-1",
		"effectiveFrom": "2026-09-01T00:00:00Z",
		"entries": [
			{"kind": "SYN-SIGNAL-STALL", "confidence": "SYN-CONF-HIGH", "outcome": "AUTO_ESTABLISH", "team": "SYN-TEAM-EXC"},
			{"kind": "SYN-SIGNAL-STALL", "confidence": "SYN-CONF-LOW", "outcome": "MANUAL_REVIEW"}
		]
	}`)
	command, err := registrationjson.TriageRulesFromJSON(raw)
	if err != nil {
		t.Fatalf("翻译：%v", err)
	}
	if len(command.Entries) != 2 || command.Entries[0].Outcome != domain.AutoEstablishCase {
		t.Fatalf("条目 = %+v", command.Entries)
	}
	// team 只在场时译成引用；缺席留零值——成对与否归用例判，这里不代填也不拒。
	if command.Entries[0].Team.String() != "SYN-TEAM-EXC" || command.Entries[1].Team.String() != "" {
		t.Fatalf("团队维没原样译出：%+v", command.Entries)
	}
}

func TestNotificationPolicyTranslatesDuration(t *testing.T) {
	raw := []byte(`{
		"tenantId": "SYN-TEN-VE15",
		"policy": "SYN-DISC-POLICY-1",
		"channel": "SYN-CHANNEL-EMAIL",
		"deadlineAfter": "72h",
		"obligation": "SYN-OBLIGATION-1",
		"approvedBy": "SYN-approver-1"
	}`)
	command, err := registrationjson.NotificationPolicyFromJSON(raw)
	if err != nil {
		t.Fatalf("翻译：%v", err)
	}
	if command.DeadlineAfter != 72*time.Hour {
		t.Fatalf("时限 = %s，要 72h", command.DeadlineAfter)
	}
	if command.Policy.String() != "SYN-DISC-POLICY-1" || command.ApprovedBy != "SYN-approver-1" {
		t.Fatalf("命令 = %+v", command)
	}
}

func TestClaimAuthorizationDistinguishesEmptyListFromMissingField(t *testing.T) {
	// 空名单是显式声明（不授权任何人），翻译放行、语义判断归用例。
	explicit := []byte(`{
		"tenantId": "SYN-TEN-VE15",
		"version": "SYN-AUTH-V1",
		"approvedBy": "SYN-approver-1",
		"customer": "SYN-CUSTOMER-1",
		"applicants": []
	}`)
	command, err := registrationjson.ClaimAuthorizationFromJSON(explicit)
	if err != nil {
		t.Fatalf("显式空名单被拒：%v", err)
	}
	if len(command.Applicants) != 0 {
		t.Fatalf("名单 = %+v，要空", command.Applicants)
	}

	// 缺字段是漏填，不当显式空名单收——两者的恢复动作不同。
	missing := []byte(`{
		"tenantId": "SYN-TEN-VE15",
		"version": "SYN-AUTH-V1",
		"approvedBy": "SYN-approver-1",
		"customer": "SYN-CUSTOMER-1"
	}`)
	if _, err := registrationjson.ClaimAuthorizationFromJSON(missing); err == nil {
		t.Fatalf("缺名单字段被当成显式空名单收下了")
	}
}

func TestDisclosurePolicyTranslatesAllThreeDimensionStates(t *testing.T) {
	raw := []byte(`{
		"tenantId": "SYN-TEN-VE15",
		"version": "SYN-DISC-V1",
		"approvedBy": "SYN-approver-1",
		"effectiveFrom": "2026-09-01T00:00:00Z",
		"entries": [{
			"customer": "SYN-CUSTOMER-1",
			"milestones": {"state": "SHOWN", "content": "SYN-CONTENT-MILESTONES"},
			"eta": {"state": "PENDING_CONFIRMATION"},
			"final": {"state": "NOT_DISCLOSED"},
			"note": {"state": "SHOWN", "content": "SYN-CONTENT-NOTE"}
		}]
	}`)
	command, err := registrationjson.DisclosurePolicyFromJSON(raw)
	if err != nil {
		t.Fatalf("翻译：%v", err)
	}
	entry := command.Entries[0]
	if entry.Milestones.State() != domain.DimensionShown {
		t.Fatalf("milestones 态 = %v", entry.Milestones.State())
	}
	if content, ok := entry.Milestones.Content(); !ok || content.String() != "SYN-CONTENT-MILESTONES" {
		t.Fatalf("milestones 内容 = %v（ok=%v）", content, ok)
	}
	if entry.ETA.State() != domain.DimensionPendingConfirmation ||
		entry.Final.State() != domain.DimensionNotDisclosed {
		t.Fatalf("eta/final 态 = %v/%v", entry.ETA.State(), entry.Final.State())
	}
}

// TestExceptionDisclosureRulesTranslateEveryField 证异常披露规则（0023）快照逐字段落到
// 命令上：条目字段名照读面 /visibility-catalogues?kind=EXCEPTION_DISCLOSURE_RULE 落下的原词
// （customer / signalKind / confidence / disclosable / autoRelease / content），登记方对着读签
// 写快照不必换词。content 缺席留零值——内容随披露的成对判据归用例（ENTRY_INCOMPLETE），
// 这里不代填也不拒。
func TestExceptionDisclosureRulesTranslateEveryField(t *testing.T) {
	raw := []byte(`{
		"tenantId": "SYN-TEN-VE15",
		"version": "SYN-EDR-V1",
		"approvedBy": "SYN-approver-1",
		"effectiveFrom": "2026-09-01T00:00:00Z",
		"entries": [
			{"customer": "SYN-CUSTOMER-1", "signalKind": "SYN-SIGNAL-STALL", "confidence": "SYN-CONF-HIGH",
			 "disclosable": true, "autoRelease": true, "content": "SYN-CONTENT-STALL"},
			{"customer": "SYN-CUSTOMER-1", "signalKind": "SYN-SIGNAL-STALL", "confidence": "SYN-CONF-LOW",
			 "disclosable": false, "autoRelease": false}
		]
	}`)
	command, err := registrationjson.ExceptionDisclosureRulesFromJSON(raw)
	if err != nil {
		t.Fatalf("翻译：%v", err)
	}
	if command.TenantID.String() != "SYN-TEN-VE15" ||
		command.Header.Version != "SYN-EDR-V1" ||
		command.Header.ApprovedBy != "SYN-approver-1" ||
		command.Header.HasEffectiveTo {
		t.Fatalf("抬头 = %+v（租户 %s）", command.Header, command.TenantID)
	}
	if len(command.Entries) != 2 {
		t.Fatalf("条目数 = %d，要 2", len(command.Entries))
	}
	shown := command.Entries[0]
	if shown.Customer.String() != "SYN-CUSTOMER-1" ||
		shown.Kind.String() != "SYN-SIGNAL-STALL" ||
		shown.Confidence.String() != "SYN-CONF-HIGH" ||
		!shown.Disclosable || !shown.AutoRelease ||
		shown.Content.String() != "SYN-CONTENT-STALL" {
		t.Fatalf("披露条目 = %+v", shown)
	}
	withheld := command.Entries[1]
	if withheld.Disclosable || withheld.AutoRelease || withheld.Content.String() != "" {
		t.Fatalf("不披露条目 = %+v，content 应留零值", withheld)
	}
}

// TestConflictSignalRuleTranslatesEveryField 证冲突信号规则（0025）快照逐字段落到命令上。
// 字段名同样照读面原词（signalKind / version / confidence / approvedBy）；version 在这册里
// 就是识别规则版本引用——它没有目录版本抬头，一租户一条。
func TestConflictSignalRuleTranslatesEveryField(t *testing.T) {
	raw := []byte(`{
		"tenantId": "SYN-TEN-VE15",
		"signalKind": "SYN-SIGNAL-FACT-CONFLICT",
		"version": "SYN-RULE-V1",
		"confidence": "SYN-CONF-MEDIUM",
		"approvedBy": "SYN-approver-1"
	}`)
	command, err := registrationjson.ConflictSignalRuleFromJSON(raw)
	if err != nil {
		t.Fatalf("翻译：%v", err)
	}
	if command.TenantID.String() != "SYN-TEN-VE15" ||
		command.Kind.String() != "SYN-SIGNAL-FACT-CONFLICT" ||
		command.Rule.String() != "SYN-RULE-V1" ||
		command.Confidence.String() != "SYN-CONF-MEDIUM" ||
		command.ApprovedBy != "SYN-approver-1" {
		t.Fatalf("命令 = %+v", command)
	}
}

// TestTranslationRejectsWhatIsNotThisEntranceShape 逐格证翻译处的拒绝：未知字段、
// 集合外取值、矛盾声明、构造门缺件。错误信息不逐字断言——它是给操作员看的话，
// 断言拒绝本身与大致指向即可。
func TestTranslationRejectsWhatIsNotThisEntranceShape(t *testing.T) {
	cases := []struct {
		name      string
		translate func([]byte) error
		raw       string
		hint      string
	}{
		{
			name:      "未知字段拒收——打错字段名不得静默变成没给",
			translate: func(raw []byte) error { _, err := registrationjson.MilestoneMappingFromJSON(raw); return err },
			raw: `{"tenantId":"SYN-TEN","version":"v1","approvedBy":"a","effectiveFrom":"2026-09-01T00:00:00Z",
				"entries":[],"unknownField":"x"}`,
			hint: "unknownField",
		},
		{
			name:      "通道技术身份不可由输入写入——osUser 是①轨，不在任何输入形状里",
			translate: func(raw []byte) error { _, err := registrationjson.MilestoneMappingFromJSON(raw); return err },
			raw: `{"tenantId":"SYN-TEN","version":"v1","approvedBy":"a","effectiveFrom":"2026-09-01T00:00:00Z",
				"entries":[],"osUser":"forged-operator"}`,
			hint: "osUser",
		},
		{
			name:      "源上下文集合外取值",
			translate: func(raw []byte) error { _, err := registrationjson.MilestoneMappingFromJSON(raw); return err },
			raw: `{"tenantId":"SYN-TEN","version":"v1","approvedBy":"a","effectiveFrom":"2026-09-01T00:00:00Z",
				"entries":[{"source":"SOMEWHERE_ELSE","factKind":"k","milestone":"m"}]}`,
			hint: "SOMEWHERE_ELSE",
		},
		{
			name:      "分诊走向集合外取值",
			translate: func(raw []byte) error { _, err := registrationjson.TriageRulesFromJSON(raw); return err },
			raw: `{"tenantId":"SYN-TEN","version":"v1","approvedBy":"a","effectiveFrom":"2026-09-01T00:00:00Z",
				"entries":[{"kind":"k","confidence":"c","outcome":"ESCALATE"}]}`,
			hint: "ESCALATE",
		},
		{
			name:      "时限不是时长字面",
			translate: func(raw []byte) error { _, err := registrationjson.NotificationPolicyFromJSON(raw); return err },
			raw: `{"tenantId":"SYN-TEN","policy":"p","channel":"c","deadlineAfter":"three days",
				"obligation":"o","approvedBy":"a"}`,
			hint: "时长",
		},
		{
			name:      "展示维缺内容来处",
			translate: func(raw []byte) error { _, err := registrationjson.DisclosurePolicyFromJSON(raw); return err },
			raw: `{"tenantId":"SYN-TEN","version":"v1","approvedBy":"a","effectiveFrom":"2026-09-01T00:00:00Z",
				"entries":[{"customer":"c","milestones":{"state":"SHOWN"},
				"eta":{"state":"PENDING_CONFIRMATION"},"final":{"state":"NOT_DISCLOSED"},
				"note":{"state":"NOT_DISCLOSED"}}]}`,
			hint: "内容来处",
		},
		{
			name:      "待确认维带内容是矛盾声明",
			translate: func(raw []byte) error { _, err := registrationjson.DisclosurePolicyFromJSON(raw); return err },
			raw: `{"tenantId":"SYN-TEN","version":"v1","approvedBy":"a","effectiveFrom":"2026-09-01T00:00:00Z",
				"entries":[{"customer":"c","milestones":{"state":"PENDING_CONFIRMATION","content":"leftover"},
				"eta":{"state":"PENDING_CONFIRMATION"},"final":{"state":"NOT_DISCLOSED"},
				"note":{"state":"NOT_DISCLOSED"}}]}`,
			hint: "矛盾",
		},
		{
			name:      "维状态集合外取值",
			translate: func(raw []byte) error { _, err := registrationjson.DisclosurePolicyFromJSON(raw); return err },
			raw: `{"tenantId":"SYN-TEN","version":"v1","approvedBy":"a","effectiveFrom":"2026-09-01T00:00:00Z",
				"entries":[{"customer":"c","milestones":{"state":"MAYBE"},
				"eta":{"state":"PENDING_CONFIRMATION"},"final":{"state":"NOT_DISCLOSED"},
				"note":{"state":"NOT_DISCLOSED"}}]}`,
			hint: "MAYBE",
		},
		{
			name:      "租户构造门缺件",
			translate: func(raw []byte) error { _, err := registrationjson.ClaimEligibilityFromJSON(raw); return err },
			raw:       `{"tenantId":"  ","version":"v1","approvedBy":"a","contract":"c","coveredKinds":["k"]}`,
			hint:      "tenant",
		},
		{
			name:      "异常披露规则条目字段名照读面原词——kind 不是 signalKind",
			translate: func(raw []byte) error { _, err := registrationjson.ExceptionDisclosureRulesFromJSON(raw); return err },
			raw: `{"tenantId":"SYN-TEN","version":"v1","approvedBy":"a","effectiveFrom":"2026-09-01T00:00:00Z",
				"entries":[{"customer":"c","kind":"k","confidence":"x","disclosable":false,"autoRelease":false}]}`,
			hint: "kind",
		},
		{
			name:      "冲突信号规则的识别规则版本构造门缺件",
			translate: func(raw []byte) error { _, err := registrationjson.ConflictSignalRuleFromJSON(raw); return err },
			raw:       `{"tenantId":"SYN-TEN","signalKind":"k","version":" ","confidence":"c","approvedBy":"a"}`,
			hint:      "signal rule version",
		},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			err := testCase.translate([]byte(testCase.raw))
			if err == nil {
				t.Fatalf("该拒的输入被收下了")
			}
			if !strings.Contains(err.Error(), testCase.hint) {
				t.Fatalf("拒绝没指向要改的东西：%v（要含 %q）", err, testCase.hint)
			}
		})
	}
}

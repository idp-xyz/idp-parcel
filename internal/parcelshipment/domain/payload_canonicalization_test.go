package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// payloadSpec 造一份五类俱全的规范化输入：成员两名（其一带测量画像）、寄收件范围与
// 服务要求条目、显式声明的 requestEffectiveAt；基础版本按首次提交缺席，要补的测试
// 自己设。成员与条目刻意乱序给入——排序是规范化的事，不是调用方的义务。
func payloadSpec(t *testing.T) domain.SubmissionPayloadSpec {
	t.Helper()
	return domain.SubmissionPayloadSpec{
		RequestReference: mustValue(t, domain.NewShipmentRequestID, "shipment-1"),
		EffectiveAt:      time.Date(2026, 8, 7, 0, 0, 0, 0, time.UTC),
		DeclaredParcelIDs: []domain.DeclaredParcelID{
			mustValue(t, domain.NewDeclaredParcelID, "parcel-2"),
			mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
		},
		Profiles: []domain.DeclaredParcelProfile{profileOf(t, "parcel-1", "1.50")},
		Scope: []domain.CanonicalContentEntry{
			contentEntry(t, "sender_region", "north"),
			contentEntry(t, "recipient_country", "DE"),
		},
		Service: []domain.CanonicalContentEntry{
			contentEntry(t, "service_product", "express"),
		},
	}
}

func contentEntry(t *testing.T, name, value string) domain.CanonicalContentEntry {
	t.Helper()
	entry, err := domain.NewCanonicalContentEntry(name, value)
	if err != nil {
		t.Fatalf("new canonical content entry %q: %v", name, err)
	}
	return entry
}

func canonicalize(t *testing.T, spec domain.SubmissionPayloadSpec) domain.PayloadDigest {
	t.Helper()
	digest, err := domain.CanonicalizeSubmissionPayload(spec)
	if err != nil {
		t.Fatalf("canonicalize submission payload: %v", err)
	}
	return digest
}

// Covers: 票 14 / ADR-0014——规范化形状带显式版本且版本进摘要输入；同一规范化业务内容
// 两次产出同一 PayloadDigest，摘要自带版本标识（不带版本的摘要视为不完整）。
func TestCanonicalizeSubmissionPayloadIsStableAndCarriesItsVersion(t *testing.T) {
	first := canonicalize(t, payloadSpec(t))
	second := canonicalize(t, payloadSpec(t))

	if first != second {
		t.Fatalf("digest = %q vs %q; 同一规范化内容两次算出了不同摘要", first, second)
	}
	// 版本字面量钉在测试里，不经包内出口取：本测试证的是「摘要携带产生它的规范化版本」
	// （ADR-0014），版本一换这里就该红——那是引入 PSC-2 的那笔工作要看见的信号，不是要
	// 绕开的耦合。包外读版本的出口今天零消费者，已删。
	prefix := "PSC-1:"
	if !strings.HasPrefix(first.String(), prefix) {
		t.Fatalf("digest = %q, want prefix %q; 摘要没有携带产生它的规范化版本", first, prefix)
	}
	if first.String() == prefix {
		t.Fatalf("digest = %q; 版本后面没有任何内容散列", first)
	}
}

// Covers: 规范化的「规范」二字——成员是集合、条目是集合：给入顺序、逐字重复的条目与
// 条目名两侧的空白都不是内容，重排/重复/加空白后摘要不变。渠道翻译的这些不稳定若进了
// 摘要，同一份请求的重试会被误判成冲突。
func TestCanonicalizeSubmissionPayloadNormalizesOrderDuplicatesAndNameSpacing(t *testing.T) {
	base := canonicalize(t, payloadSpec(t))

	reshuffled := payloadSpec(t)
	reshuffled.DeclaredParcelIDs = []domain.DeclaredParcelID{
		mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
		mustValue(t, domain.NewDeclaredParcelID, "parcel-2"),
	}
	reshuffled.Scope = []domain.CanonicalContentEntry{
		contentEntry(t, "recipient_country", "DE"),
		contentEntry(t, "  sender_region  ", "north"),
		contentEntry(t, "recipient_country", "DE"),
	}
	if got := canonicalize(t, reshuffled); got != base {
		t.Fatalf("digest = %q, want %q; 顺序、重复条目或条目名空白改变了摘要", got, base)
	}
}

// Covers: CONTEXT 摘要定义的覆盖面——成员、范围、基础版本、服务要求以及客户委托参考
// 各自都是内容：任一处变化都必须换出另一个摘要，否则同一来源身份下的冲突会被误判成重放。
func TestCanonicalizeSubmissionPayloadCoversEveryContentSection(t *testing.T) {
	base := canonicalize(t, payloadSpec(t))

	mutations := map[string]func(*domain.SubmissionPayloadSpec, *testing.T){
		"少一名成员": func(spec *domain.SubmissionPayloadSpec, t *testing.T) {
			spec.DeclaredParcelIDs = spec.DeclaredParcelIDs[:1]
			spec.Profiles = nil
		},
		"成员测量变化": func(spec *domain.SubmissionPayloadSpec, t *testing.T) {
			spec.Profiles = []domain.DeclaredParcelProfile{profileOf(t, "parcel-1", "2.00")}
		},
		"成员测量画像撤掉": func(spec *domain.SubmissionPayloadSpec, t *testing.T) {
			spec.Profiles = nil
		},
		"范围条目改值": func(spec *domain.SubmissionPayloadSpec, t *testing.T) {
			spec.Scope = []domain.CanonicalContentEntry{
				contentEntry(t, "sender_region", "north"),
				contentEntry(t, "recipient_country", "FR"),
			}
		},
		"服务要求多一条": func(spec *domain.SubmissionPayloadSpec, t *testing.T) {
			spec.Service = append(spec.Service, contentEntry(t, "signature", "required"))
		},
		"服务条目显式空值不同于条目缺席": func(spec *domain.SubmissionPayloadSpec, t *testing.T) {
			spec.Service = append(spec.Service, contentEntry(t, "signature", ""))
		},
		"声明基础版本": func(spec *domain.SubmissionPayloadSpec, t *testing.T) {
			spec.BasisVersion = mustValue(t, domain.NewSubmissionVersionID, "version-1")
		},
		"换一个客户委托参考": func(spec *domain.SubmissionPayloadSpec, t *testing.T) {
			spec.RequestReference = mustValue(t, domain.NewShipmentRequestID, "shipment-2")
		},
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			mutated := payloadSpec(t)
			mutate(&mutated, t)
			if got := canonicalize(t, mutated); got == base {
				t.Fatalf("digest 未变（%q）；这一类内容变化被摘要吞掉了", got)
			}
		})
	}
}

// Covers: CONTEXT「requestEffectiveAt 的值以及缺失或显式存在的状态必须进入 PayloadDigest」
// （S02-AT-03 同款语义）：缺失与显式声明分成两个摘要，值不同再分；同一时刻换时区表示
// 不是第二种内容。
func TestCanonicalizeSubmissionPayloadSeparatesEffectiveAtValueAndPresence(t *testing.T) {
	declared := canonicalize(t, payloadSpec(t))

	absent := payloadSpec(t)
	absent.EffectiveAt = time.Time{}
	if got := canonicalize(t, absent); got == declared {
		t.Fatalf("digest = %q; 缺失与显式声明的 requestEffectiveAt 折成了同一摘要", got)
	}

	later := payloadSpec(t)
	later.EffectiveAt = time.Date(2026, 8, 8, 0, 0, 0, 0, time.UTC)
	if got := canonicalize(t, later); got == declared {
		t.Fatalf("digest = %q; 不同的 requestEffectiveAt 值折成了同一摘要", got)
	}

	rezoned := payloadSpec(t)
	rezoned.EffectiveAt = time.Date(2026, 8, 7, 8, 0, 0, 0, time.FixedZone("UTC+8", 8*60*60))
	if got := canonicalize(t, rezoned); got != declared {
		t.Fatalf("digest = %q, want %q; 同一时刻换时区表示被当成了另一种内容", got, declared)
	}
}

// Covers: 摘要函数的失败边界。成员规则与提交管线同款裁决（成员至少一名、不重复；画像
// 指着集合外成员与一员两张即拒）——摘要若收下委托管线会拒的输入，重放分类会先于受理
// 答「已有结果」。零值参考与零值条目只能绕过构造函数硬造，一律拒。
func TestCanonicalizeSubmissionPayloadRefusesMalformedContent(t *testing.T) {
	tests := map[string]struct {
		mutate func(*domain.SubmissionPayloadSpec, *testing.T)
		want   error
	}{
		"客户委托参考缺席": {
			mutate: func(spec *domain.SubmissionPayloadSpec, t *testing.T) {
				spec.RequestReference = domain.ShipmentRequestID{}
			},
			want: domain.ErrInvalidSubmissionPayload,
		},
		"没有任何成员": {
			mutate: func(spec *domain.SubmissionPayloadSpec, t *testing.T) {
				spec.DeclaredParcelIDs = nil
				spec.Profiles = nil
			},
			want: domain.ErrNoDeclaredParcels,
		},
		"成员重复": {
			mutate: func(spec *domain.SubmissionPayloadSpec, t *testing.T) {
				spec.DeclaredParcelIDs = append(
					spec.DeclaredParcelIDs,
					mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
				)
			},
			want: domain.ErrDuplicateDeclaredParcel,
		},
		"零值成员": {
			mutate: func(spec *domain.SubmissionPayloadSpec, t *testing.T) {
				spec.DeclaredParcelIDs = append(spec.DeclaredParcelIDs, domain.DeclaredParcelID{})
			},
			want: domain.ErrInvalidSubmissionPayload,
		},
		"画像指着集合外成员": {
			mutate: func(spec *domain.SubmissionPayloadSpec, t *testing.T) {
				spec.Profiles = []domain.DeclaredParcelProfile{profileOf(t, "parcel-9", "1")}
			},
			want: domain.ErrInvalidDeclaredMeasurement,
		},
		"一员两张画像": {
			mutate: func(spec *domain.SubmissionPayloadSpec, t *testing.T) {
				spec.Profiles = []domain.DeclaredParcelProfile{
					profileOf(t, "parcel-1", "1"),
					profileOf(t, "parcel-1", "2"),
				}
			},
			want: domain.ErrInvalidDeclaredMeasurement,
		},
		"零值范围条目": {
			mutate: func(spec *domain.SubmissionPayloadSpec, t *testing.T) {
				spec.Scope = append(spec.Scope, domain.CanonicalContentEntry{})
			},
			want: domain.ErrInvalidSubmissionPayload,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			spec := payloadSpec(t)
			test.mutate(&spec, t)
			if _, err := domain.CanonicalizeSubmissionPayload(spec); !errors.Is(err, test.want) {
				t.Fatalf("err = %v, want %v", err, test.want)
			}
		})
	}

	if _, err := domain.NewCanonicalContentEntry("   ", "value"); !errors.Is(err, domain.ErrBlankValue) {
		t.Fatalf("err = %v; 没有名字的条目被立起来了", err)
	}
}

// Covers: CONTEXT「（摘要）用于同一来源身份下的重放与冲突分类」与 S02-W01「信封时间
// 变化仍是重放」（S02-AT-02/03 同款语义）——真摘要接进既有分类缝：同内容异信封时间
// 判重放，内容变化判冲突。此前这条语义只被手造摘要串演过，本测试让它第一次踩在真的
// 规范化产出上。
func TestCanonicalizedDigestsDriveReplayAndConflictClassification(t *testing.T) {
	identity := sourceIdentity(t, "tenant-1", "customer-1", "api", "request-1")
	preserved, err := domain.NewSourceSubmissionFingerprint(
		identity,
		canonicalize(t, payloadSpec(t)),
		time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 5, 10, 0, 1, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("new preserved fingerprint: %v", err)
	}

	// 同一份规范化内容重试：occurredAt/receivedAt 都变了，摘要不变，仍是重放。
	retried, err := domain.NewSourceSubmissionFingerprint(
		identity,
		canonicalize(t, payloadSpec(t)),
		time.Date(2026, 8, 6, 9, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 6, 9, 0, 2, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("new retried fingerprint: %v", err)
	}
	classification, err := domain.ClassifySourceSubmission(preserved, retried)
	if err != nil {
		t.Fatalf("classify retried submission: %v", err)
	}
	if classification != domain.SourceReplay {
		t.Fatalf("classification = %v, want replay; 信封时间变化被升格成了冲突", classification)
	}

	// 同一来源身份改了一个声明字段：摘要必须换，分类必须是冲突。
	amended := payloadSpec(t)
	amended.Scope = []domain.CanonicalContentEntry{
		contentEntry(t, "sender_region", "north"),
		contentEntry(t, "recipient_country", "FR"),
	}
	conflicting, err := domain.NewSourceSubmissionFingerprint(
		identity,
		canonicalize(t, amended),
		time.Date(2026, 8, 5, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 5, 10, 0, 1, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("new conflicting fingerprint: %v", err)
	}
	classification, err = domain.ClassifySourceSubmission(preserved, conflicting)
	if err != nil {
		t.Fatalf("classify conflicting submission: %v", err)
	}
	if classification != domain.SourceConflict {
		t.Fatalf("classification = %v, want conflict; 内容变化没有换出新摘要", classification)
	}
}

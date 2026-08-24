package identity_test

import (
	"context"
	"strings"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/identity"
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
)

// mintFunc 把七个返回类型各异的签发方法收成同一形状，好让下面两条对全部七个都跑一遍。
// 各 ID 类型的 String() 由领域侧的 requiredValue 提升而来。
type mintFunc func(context.Context) (string, error)

func allMinters(t *testing.T, options ...platformidentity.Option) map[string]mintFunc {
	t.Helper()

	submissions, err := adapter.NewSubmissionIdentities(options...)
	if err != nil {
		t.Fatalf("构造建单期签发器：%v", err)
	}
	decisions, err := adapter.NewAcceptanceDecisions(options...)
	if err != nil {
		t.Fatalf("构造决定签发器：%v", err)
	}
	commitments, err := adapter.NewCommitmentVersions(options...)
	if err != nil {
		t.Fatalf("构造承诺签发器：%v", err)
	}
	finals, err := adapter.NewFinalOutcomeVersions(options...)
	if err != nil {
		t.Fatalf("构造终局签发器：%v", err)
	}
	sourceData, err := adapter.NewSourceDataVersions(options...)
	if err != nil {
		t.Fatalf("构造资料版本签发器：%v", err)
	}
	cancellations, err := adapter.NewParcelCancellations(options...)
	if err != nil {
		t.Fatalf("构造取消签发器：%v", err)
	}

	return map[string]mintFunc{
		"SUBV": func(ctx context.Context) (string, error) {
			minted, err := submissions.NextSubmissionVersionID(ctx)
			return minted.String(), err
		},
		"ADTK": func(ctx context.Context) (string, error) {
			minted, err := submissions.NextAcceptanceDecisionTaskID(ctx)
			return minted.String(), err
		},
		"ADEC": func(ctx context.Context) (string, error) {
			minted, err := decisions.NextAcceptanceDecisionID(ctx)
			return minted.String(), err
		},
		"CMTV": func(ctx context.Context) (string, error) {
			minted, err := commitments.NextCommitmentVersionID(ctx)
			return minted.String(), err
		},
		"FINV": func(ctx context.Context) (string, error) {
			minted, err := finals.NextFinalOutcomeVersionID(ctx)
			return minted.String(), err
		},
		"SDV": func(ctx context.Context) (string, error) {
			minted, err := sourceData.NextSourceDataVersionID(ctx)
			return minted.String(), err
		},
		"PCXL": func(ctx context.Context) (string, error) {
			minted, err := cancellations.NextParcelCancellationID(ctx)
			return minted.String(), err
		},
	}
}

// TestEachIdentityCarriesItsOwnPrefix 钉住七个前缀互不相同。
//
// 它防的是一类抄改错误：SubmissionIdentities 内部有两个签发器，复用同一个就会让提交版本
// 与判断任务共用一个前缀。那样编译得过、测得过身份不重，却让日志里两类标识分不开——而
// 前缀存在的全部理由就是分开它们。
func TestEachIdentityCarriesItsOwnPrefix(t *testing.T) {
	minters := allMinters(t)
	seen := make(map[string]string, len(minters))

	for prefix, mint := range minters {
		minted, err := mint(t.Context())
		if err != nil {
			t.Fatalf("%s 签发：%v", prefix, err)
		}
		got, _, found := strings.Cut(minted, "-")
		if !found {
			t.Fatalf("%s 的标识没有分隔符：%q", prefix, minted)
		}
		if got != prefix {
			t.Fatalf("前缀 = %q，want %q（标识 %q）", got, prefix, minted)
		}
		if earlier, clash := seen[got]; clash {
			t.Fatalf("%s 与 %s 共用前缀 %q", prefix, earlier, got)
		}
		seen[got] = prefix
	}
}

func TestTwoMintsOfTheSameIdentityDiffer(t *testing.T) {
	for prefix, mint := range allMinters(t) {
		first, err := mint(t.Context())
		if err != nil {
			t.Fatalf("%s 首次签发：%v", prefix, err)
		}
		second, err := mint(t.Context())
		if err != nil {
			t.Fatalf("%s 再次签发：%v", prefix, err)
		}
		if first == second {
			t.Fatalf("%s 两次签发同值：%q", prefix, first)
		}
	}
}

// TestAFailedMintIsReportedNotSubstituted 证熵源出问题时七个签发方法一律如实报错，
// 不交回一个凑出来的标识。可注入熵源的全部意义就在这一支——真实熵源逼不出它。
func TestAFailedMintIsReportedNotSubstituted(t *testing.T) {
	// 只给 4 字节，签发要读满 16。
	starved := platformidentity.WithEntropy(strings.NewReader("abcd"))

	for prefix, mint := range allMinters(t, starved) {
		minted, err := mint(t.Context())
		if err == nil {
			t.Fatalf("%s 在熵源读不满时仍签出了 %q", prefix, minted)
		}
		if minted != "" {
			t.Fatalf("%s 报错时仍交回了标识：%q", prefix, minted)
		}
	}
}

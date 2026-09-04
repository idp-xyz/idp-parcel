package domain_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证抓取所得公布记录的形状（ADR-0099 决定六、ADR-0092 决定一）：内容摘要在字节还在
// 手上那一刻由构造器自己算，调用方给不出一个与字节不符的摘要；抓取时刻与来源地址必备；
// 取值凭证由摘要、时刻、地址拼成，本体存放定位符可缺——凭证等级不依赖本体在不在。

var (
	fetchMoment  = time.Date(2026, 9, 4, 1, 2, 3, 0, time.UTC)
	publishedDay = time.Date(2026, 9, 4, 0, 0, 0, 0, time.UTC)
)

func publishedRecord(t *testing.T, body string) domain.PublishedRecord {
	t.Helper()
	record, err := domain.NewPublishedRecord([]byte(body), fetchMoment, "file:rates/usd-cny/2026-09-04.json", publishedDay)
	if err != nil {
		t.Fatalf("构造公布记录：%v", err)
	}
	return record
}

// TestPublishedRecordDigestsTheBytesItHolds 证摘要是对原文字节算的 SHA-256，字节不同摘要不同；
// 交出去的字节是副本，改副本不动记录。
func TestPublishedRecordDigestsTheBytesItHolds(t *testing.T) {
	record := publishedRecord(t, `{"value":"7.1234"}`)

	sum := sha256.Sum256([]byte(`{"value":"7.1234"}`))
	if record.ContentDigest() != "sha256:"+hex.EncodeToString(sum[:]) {
		t.Fatalf("摘要 = %q，不是原文字节的 SHA-256", record.ContentDigest())
	}
	if other := publishedRecord(t, `{"value":"7.1235"}`); other.ContentDigest() == record.ContentDigest() {
		t.Fatal("不同字节算出了同一摘要")
	}
	if !record.FetchedAt().Equal(fetchMoment) || record.SourceLocator() != "file:rates/usd-cny/2026-09-04.json" || !record.PublishedOn().Equal(publishedDay) {
		t.Fatalf("记录三件走样：%v %q %v", record.FetchedAt(), record.SourceLocator(), record.PublishedOn())
	}

	body := record.Body()
	body[0] = 'X'
	if string(record.Body()) != `{"value":"7.1234"}` {
		t.Fatal("改动交出的字节副本改动了记录本身")
	}
}

// TestPublishedRecordRefusesWhatCannotBeEvidence 证四件缺一即拒：空字节没有可摘要的内容；缺
// 抓取时刻或地址的记录事后无从复核；缺公布日期的记录放不到时间线上。
func TestPublishedRecordRefusesWhatCannotBeEvidence(t *testing.T) {
	cases := map[string]func() (domain.PublishedRecord, error){
		"空字节": func() (domain.PublishedRecord, error) {
			return domain.NewPublishedRecord(nil, fetchMoment, "file:x", publishedDay)
		},
		"零抓取时刻": func() (domain.PublishedRecord, error) {
			return domain.NewPublishedRecord([]byte("x"), time.Time{}, "file:x", publishedDay)
		},
		"空地址": func() (domain.PublishedRecord, error) {
			return domain.NewPublishedRecord([]byte("x"), fetchMoment, " ", publishedDay)
		},
		"零公布日期": func() (domain.PublishedRecord, error) {
			return domain.NewPublishedRecord([]byte("x"), fetchMoment, "file:x", time.Time{})
		},
	}
	for label, construct := range cases {
		if _, err := construct(); !errors.Is(err, domain.ErrInvalidPublishedRecord) {
			t.Fatalf("%s 被接受了：err=%v", label, err)
		}
	}
}

// TestEvidenceReferenceCarriesDigestMomentAndAddress 证取值凭证的形状：`sha256:… @ 抓取时刻 ← 地址`；
// 本体另有存放处时追加定位符，未配置存放（定位符为空）或原地引用（定位符即地址）时不追加——
// 摘要照旧在，凭证不因本体在不在而降级。
func TestEvidenceReferenceCarriesDigestMomentAndAddress(t *testing.T) {
	record := publishedRecord(t, `{"value":"7.1234"}`)
	want := record.ContentDigest() + " @ 2026-09-04T01:02:03Z ← file:rates/usd-cny/2026-09-04.json"

	if got := record.EvidenceReference(""); got != want {
		t.Fatalf("未配置存放的凭证 = %q，想要 %q", got, want)
	}
	if got := record.EvidenceReference("file:rates/usd-cny/2026-09-04.json"); got != want {
		t.Fatalf("原地引用的凭证 = %q，想要 %q", got, want)
	}
	if got := record.EvidenceReference("s3://bucket/usd-cny/2026-09-04.json"); got != want+" → s3://bucket/usd-cny/2026-09-04.json" {
		t.Fatalf("另有存放处的凭证 = %q", got)
	}
}

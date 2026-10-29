package domain_test

import (
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// Covers: `AT-PP-045`，ADR-0349 决定五——页面表单提交的价卡没有文件：来源标识名由散列推出（form: 加散列前 12 位），认「来自页面表单」
// 只看名称是否恰是自己散列推出的那一个——一个碰巧叫 form:xxxx 的上传文件名对不上自己字节的散列，不会被当成表单。
func TestAFormSourceFileIdentityIsNamedAfterItsOwnDigest(t *testing.T) {
	digest := strings.Repeat("ab", 32)
	source, err := domain.NewFormSourceFileIdentity(digest)
	if err != nil || source.Name() != "form:abababababab" || source.SHA256() != digest || !source.FromForm() {
		t.Fatalf("source = %+v err=%v", source, err)
	}
	for name, bad := range map[string]string{"太短": "abc", "太长": digest + "0", "大写": strings.ToUpper(digest), "空": ""} {
		if _, err := domain.NewFormSourceFileIdentity(bad); err == nil {
			t.Errorf("%s 的散列不该立得成表单身份", name)
		}
	}

	file, err := domain.NewSourceFileIdentity("card.xlsx", digest)
	if err != nil || file.FromForm() {
		t.Fatalf("上传文件不是表单：%+v err=%v", file, err)
	}
	spoof, err := domain.NewSourceFileIdentity("form:abababababab", strings.Repeat("cd", 32))
	if err != nil || spoof.FromForm() {
		t.Fatalf("名叫 form: 却对不上自己散列的上传文件不是表单：%+v err=%v", spoof, err)
	}
	if (domain.SourceFileIdentity{}).FromForm() {
		t.Fatal("零值不是表单")
	}
}

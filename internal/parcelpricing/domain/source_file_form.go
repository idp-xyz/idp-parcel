package domain

// 页面表单提交的价卡没有文件：源文件身份的名称是 "form:" 加规范化工作簿 SHA-256 的前 12 位十六进制，散列是规范化字节的完整
// SHA-256（规范化怎么做归导入适配器，这里只认结果）。名称因此由散列推出：认「来自页面表单」只看名称是否恰是散列推出的
// 那一个，一个碰巧叫 form:xxxx 的上传文件名对不上自己字节的散列，不会被当成表单。
const (
	formSourcePrefix       = "form:"
	formSourceDigestLength = 12
)

// NewFormSourceFileIdentity 立页面表单提交的源文件身份。
func NewFormSourceFileIdentity(sha256Hex string) (SourceFileIdentity, error) {
	if len(sha256Hex) != sha256HexLength {
		return SourceFileIdentity{}, ErrInvalidSourceFileIdentity
	}
	return NewSourceFileIdentity(formSourcePrefix+sha256Hex[:formSourceDigestLength], sha256Hex)
}

// FromForm 报出这份身份是不是页面表单提交的：名称恰是自己散列推出的那一个。
func (identity SourceFileIdentity) FromForm() bool {
	return identity.valid() && identity.name == formSourcePrefix+identity.sha256[:formSourceDigestLength]
}

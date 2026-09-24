// parcel-pricing-template 写出价卡导入的空白模板（docs/design/pp-price-card-import-template-and-validation-spec.md
// 第七节）。入库的模板文件只由它生成，同包测试重算一遍逐字节比对：模板与解析器用同一张列定义，
// 手工改过的模板文件会在那里变红，而不是在租户上传时才发现对不上。
//
// 在仓库根执行：go run ./cmd/parcel-pricing-template
package main

import (
	"bytes"
	"flag"
	"log"
	"os"
	"path/filepath"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/pricecardtemplate"
)

// committedPath 是入库模板相对仓库根的位置：管理台静态资源，「导入价卡」签从这里下载。
var committedPath = filepath.Join("apps", "admin-web", "public", "templates", pricecardtemplate.TemplateFileName)

func main() {
	out := flag.String("out", committedPath, "输出路径")
	flag.Parse()
	var buffer bytes.Buffer
	if err := pricecardtemplate.WriteTemplate(&buffer); err != nil {
		log.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(*out), 0o755); err != nil {
		log.Fatal(err)
	}
	if err := os.WriteFile(*out, buffer.Bytes(), 0o644); err != nil {
		log.Fatal(err)
	}
}

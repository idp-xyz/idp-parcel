# 18 目录读口的原始查询串解析收进共用件

Category: enhancement
Status: ready-for-agent
Blocked by: 02–13、15（各册端点进 main 之后再统一）
地盘：`internal/platform/cataloguepage`；各上下文目录端点里取查询串的那一行。
出处：ADR-0144 决定四、七；04 的非作者评审（钉 `c39f772f2`）。

## 为什么

目录端点取查询串有两种口径：parcel-pricing 的 `decodePageQuery` 照 `url.ParseQuery` 解，转义坏掉或带分号的那一段答 400；其余各上下文
照 `request.URL.Query()` 解，那一段被静默丢掉、答出整册。同一个 `?q=%zz`，两边一个拒、一个当没给。查询契约要一处定义（决定七），
不静默忽略坏参数（决定四）——前一种口径合乎这两条，后一种不合。

## 做什么

- 在 `cataloguepage` 给出从原始查询串解码的入口：解不开的段答 `*MalformedQuery`，理由散文点名「查询串里有解不开的一段」，不回显坏值。
- 各上下文目录端点改用这个入口，删掉 parcel-pricing 自己那份 `decodePageQuery` 里的解析。
- 选择器由端点自己读的那几处，读的也是同一份解好的取值，不另解一遍；选择器的取值同样按 `storable.Text` 判，库存不下的答 400（今天 `Decode`
  只把选择器记进摘要、不判，端点又只判非空，装配处的请求目标检查之外 NUL 仍会进 SQL——01、02 的修订轮复核指出）。

## 完成判据

- 共用件单元用例证坏转义与分号答 400、理由不回显坏值；各端点表驱动用例补一问 `?q=%zz` 答 400。
- 全仓再没有目录端点直接用 `URL.Query()` 喂给声明解码。

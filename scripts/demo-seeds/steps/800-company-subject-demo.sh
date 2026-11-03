echo "== 公司主体完整资料演示（party-commercial：上海、香港、美国、深圳及原演示法人，多联系人；证据 S） =="
# 新增主体独立登记；原演示法人仅追加资料修订，不改它已被其他演示流程引用的身份。
"$BIN/parcel-commercial" register-registration-number-types -input "$SEEDS/commercial/register-company-demo-types.json"
"$BIN/parcel-commercial" register-parties -input "$SEEDS/commercial/register-company-demo-parties.json"
"$BIN/parcel-commercial" register-legal-entity-profiles -input "$SEEDS/commercial/register-company-demo-profiles.json"
"$BIN/parcel-commercial" register-legal-entity-profiles -input "$SEEDS/commercial/register-company-demo-social-profiles.json"
# 跨境资料新修订（票 company-subject-ux/05，ADR-0359）：每家在上一版完整内容之上加监管登记号（海关注册编码、BR No.、EIN、
# 纳税人识别号，类型来自 100 步经参考配置采用进目录的资料层类型）、双语抬头与 SYN- 收款账户（上海、香港各两个币种）。
# 排在社交账号那一版之后：资料修订号连续，这一版是第 3 版（原演示法人第 5 版）。
"$BIN/parcel-commercial" register-legal-entity-profiles -input "$SEEDS/commercial/register-company-demo-cross-border-profiles.json"

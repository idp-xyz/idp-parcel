echo "== 公司主体完整资料演示（party-commercial：上海、香港、美国、深圳及原演示法人，多联系人；证据 S） =="
# 新增主体独立登记；原演示法人仅追加资料修订，不改它已被其他演示流程引用的身份。
"$BIN/parcel-commercial" register-registration-number-types -input "$SEEDS/commercial/register-company-demo-types.json"
"$BIN/parcel-commercial" register-parties -input "$SEEDS/commercial/register-company-demo-parties.json"
"$BIN/parcel-commercial" register-legal-entity-profiles -input "$SEEDS/commercial/register-company-demo-profiles.json"
"$BIN/parcel-commercial" register-legal-entity-profiles -input "$SEEDS/commercial/register-company-demo-social-profiles.json"

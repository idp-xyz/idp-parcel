import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { RegistrationAnswerNote, RegistrationPanel } from '../../components/registration';
import {
  commercialRegistrationEndpoints,
  listBusinessParties,
  listRegistrationNumberTypes,
  partyIdentityOutcomeLabels,
  type BusinessPartyListResponseBody,
  type GroupLegalEntityRecord,
  type RegistrationNumberTypeListResponseBody,
} from './api';
import { problemNote, registrationSnapshotHints, registrationTitles } from './presentation';
import { Field, Problems, ReferencePicker, ReferencePickerFor, fieldLabel, useLoaded } from './PublicationFormFields';
import {
  RevisionField,
  WallTimeField,
  businessPartyPickerOptions,
  useRegistrationForm,
} from './party-registration-fields';
import {
  emptyLegalEntityDraft,
  identityCountryOptions,
  identityTypeOptions,
  legalEntityLocalProblems,
  legalEntityPayloadOf,
  suggestedRevision,
  type LegalEntityDraft,
  type LifetimeNumberDraft,
} from './legal-entity-form';

/**
 * 责任法人身份登记的逐字段表单（票 admin-web-group-legal-entities/02，身份层三格随票 legal-entity-profile/04 加入；
 * ADR-0101 决定八自裁：格少、低频、无矩阵，直接逐字段表单，不走「模板导入 → 草稿 → 批准 → 发布」那条为上百格矩阵设计的路）。壳与共用格在
 * party-registration-fields.tsx（与业务参与方页三份表单同一份），这里只摆本册的格。
 *
 * **本组件不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）：参与方在不在册、届时是否已生效、
 * 修订号连不连续、依据是否为空，一律原样送上去让服务端逐格答。本地只拦编码层的两件（修订号编不进正整数、
 * 生效时刻换不成 RFC 3339），纯函数在 legal-entity-form.ts。
 *
 * **不带租户格**：在线 Intake 从认证结果取租户、只从载荷取行内容（register_party_identity.go 包注释）；表单里摆一格
 * 租户就是让操作者自报。今天端点挂 UnconfiguredIntake{} 必答 403，答案由 RegistrationAnswerNote 与 JSON 签同一份呈现。
 *
 * JSON 快照签降为底部折叠区「高级：粘贴登记快照 JSON」——它是受控批量口的在线镜像（ADR-0101 决定一），不是运营
 * 配置员的主路径，但仍要在：CLI 那份形状能原样粘进来核对，是两口锁到同一登记用例的可见证据。
 */
export interface LegalEntityRegistrationFormProps {
  /** 已取回的法人列表（给修订号建议用）；列表没取到（未配置 / 出错 / 加载中）传 null，建议一律为 1。 */
  knownEntities: readonly GroupLegalEntityRecord[] | null;
  /** 登记册答 `REGISTERED` 时回调，页面借它重取列表让新行在「集团与法人」签立刻可见。 */
  onRegistered: () => void;
}

const info = moduleInfoById['group-legal-entities'];
const kind = 'legal-entity';
const endpoint = `POST ${commercialRegistrationEndpoints[kind]}`;

export function LegalEntityRegistrationForm({ knownEntities, onRegistered }: LegalEntityRegistrationFormProps) {
  const form = useRegistrationForm<LegalEntityDraft>({
    kind,
    empty: emptyLegalEntityDraft,
    suggestion: (draft) => suggestedRevision(knownEntities, draft.legalEntityId),
    localProblems: legalEntityLocalProblems,
    payloadOf: legalEntityPayloadOf,
    // 答 `REGISTERED`（PartyRegistryOutcome 里 PartyIdentityRegistered 的线上名）才重取列表。
    landedOutcome: 'REGISTERED',
    onLanded: onRegistered,
  });
  const { draft, patch, problems, locked, timeZone } = form;
  // 国家与号类型的候选读一次：一张表单里国家格与每一行的类型格共用这一份目录答案。
  const typeCatalogue = useLoaded(listRegistrationNumberTypes);

  // 本册的载荷裁空白（legalEntityPayloadOf），在册提示按裁过的标识找，与送出的是同一个对象。
  const known = knownEntities?.find((row) => row.legalEntityId === draft.legalEntityId.trim());

  const setNumberRow = (index: number, change: Partial<LifetimeNumberDraft>) =>
    patch({ lifetimeNumbers: draft.lifetimeNumbers.map((row, at) => (at === index ? { ...row, ...change } : row)) });
  const addNumberRow = () => patch({ lifetimeNumbers: [...draft.lifetimeNumbers, { typeCode: '', number: '' }] });
  const removeNumberRow = (index: number) =>
    patch({ lifetimeNumbers: draft.lifetimeNumbers.filter((_, at) => at !== index) });

  return (
    <div className="flex-1 overflow-auto p-4 flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{registrationTitles[kind]}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <p className="text-xs text-idpxyz-textMuted">
            一笔登记一个修订。首笔修订从 1 起、此后必须连续；更正占下一个修订号翻旧插新，不覆盖。参与方必须已在册且在
            法人生效时点已生效——这些都由服务端按册面判，表单只负责把格编对。提交打到{' '}
            <span className="font-mono">{endpoint}</span>；租户不在表单上，由接入渠道的认证结果填入。
          </p>
          <p className="text-xs text-idpxyz-textMuted">
            注册国家 / 地区与终身注册号随身份登记：首笔两格缺一由服务端拒登，号的类型与格式按国家取自注册号类型目录。
            号变了就是另一个法人；录错时在下一个修订里改正，并填身份更正依据。
          </p>

          <div className="grid grid-cols-2 gap-3">
            <Field label="法人标识 *" path="legalEntities[0].legalEntityId" problems={problems}>
              <Input
                value={draft.legalEntityId}
                readOnly={locked}
                className="font-mono text-[13px]"
                placeholder="如 LE-SHA-01"
                onChange={(event) => patch({ legalEntityId: event.target.value })}
              />
              <span className="block text-[11px] text-idpxyz-textMuted mt-1">
                {known
                  ? `已取回列表里有这一法人，最新修订 r${known.revision}；本次登记是它的新修订。`
                  : '已取回列表里没有这一标识；本次登记是首笔修订（列表可能已陈旧，连续性仍由服务端判）。'}
              </span>
            </Field>

            <RevisionField
              path="legalEntities[0].revision"
              problems={problems}
              {...form.revisionField}
              note="建议值取已取回列表里该法人的最新修订 + 1（不在册为 1）；只是省一次翻册，连续性仍由服务端按册面判。"
            />

            <ReferencePicker<BusinessPartyListResponseBody>
              label="业务参与方身份 *"
              path="legalEntities[0].partyId"
              problems={problems}
              value={draft.partyId}
              locked={locked}
              onChange={(partyId) => patch({ partyId })}
              load={listBusinessParties}
              optionsOf={businessPartyPickerOptions}
              emptyNote="参与方册今天为空；先在「业务参与方」页登记参与方身份，或手填标识由服务端判。"
              readFace="参与方册"
              optionsNote="候选不按状态过滤：法人生效时点上参与方是否已生效由服务端判。"
            />

            <Field label="登记依据 *" path="legalEntities[0].basis" problems={problems}>
              <Input
                value={draft.basis}
                readOnly={locked}
                className="font-mono text-[13px]"
                placeholder="登记依据引用（如董事会决议编号）"
                onChange={(event) => patch({ basis: event.target.value })}
              />
            </Field>

            <WallTimeField
              label="生效自（留空由服务端点名）"
              path="legalEntities[0].effectiveFrom"
              problems={problems}
              value={draft.effectiveFrom}
              locked={locked}
              timeZone={timeZone}
              onChange={(effectiveFrom) => patch({ effectiveFrom })}
            />

            <ReferencePickerFor<RegistrationNumberTypeListResponseBody>
              answer={typeCatalogue}
              label="注册国家 / 地区 *"
              path="legalEntities[0].registrationCountry"
              problems={problems}
              value={draft.registrationCountry}
              locked={locked}
              onChange={(registrationCountry) => patch({ registrationCountry })}
              optionsOf={(body) => identityCountryOptions(body.registrationNumberTypes)}
              emptyNote="注册号类型目录今天没有身份层类型；先登记注册号类型，或手填国家码由服务端判。"
              readFace="注册号类型目录"
              manualPlaceholder="国家 / 地区码（如 CN、SG）"
              optionsNote="候选是目录里登过身份层类型的国家 / 地区；目录里没有该国家的类型时服务端拒登，不以默认格式代替。"
            />

            <Field label="身份更正依据（只在更正修订上填）" path="legalEntities[0].identityCorrectionBasis" problems={problems}>
              <Input
                value={draft.identityCorrectionBasis}
                readOnly={locked}
                className="font-mono text-[13px]"
                placeholder="更正依据引用；留空即不带"
                onChange={(event) => patch({ identityCorrectionBasis: event.target.value })}
              />
              <span className="block text-[11px] text-idpxyz-textMuted mt-1">
                首笔修订不收更正依据；改正国家或号时要不要依据、依据够不够，由服务端判。
              </span>
            </Field>

            <div className="col-span-2">
              <p className={fieldLabel}>终身注册号 *</p>
              <div className="flex flex-col gap-2">
                {draft.lifetimeNumbers.map((row, index) => (
                  <div key={index} className="grid grid-cols-[1fr_1fr_auto] items-start gap-3">
                    <ReferencePickerFor<RegistrationNumberTypeListResponseBody>
                      answer={typeCatalogue}
                      label={`类型（第 ${index + 1} 行）`}
                      path={`legalEntities[0].lifetimeRegistrationNumbers[${index}].typeCode`}
                      problems={problems}
                      value={row.typeCode}
                      locked={locked}
                      onChange={(typeCode) => setNumberRow(index, { typeCode })}
                      optionsOf={(body) => identityTypeOptions(body.registrationNumberTypes, draft.registrationCountry)}
                      emptyNote="目录里这一国家 / 地区没有身份层类型（先选国家）；也可手填类型码由服务端判。"
                      readFace="注册号类型目录"
                      manualPlaceholder="类型码（如 USCC、UEN）"
                    />
                    <Field
                      label={`号（第 ${index + 1} 行）`}
                      path={`legalEntities[0].lifetimeRegistrationNumbers[${index}].number`}
                      problems={problems}
                    >
                      <Input
                        value={row.number}
                        readOnly={locked}
                        className="font-mono text-[13px]"
                        placeholder="注册号原样填"
                        onChange={(event) => setNumberRow(index, { number: event.target.value })}
                      />
                    </Field>
                    <Button variant="ghost" className="mt-5" disabled={locked} onClick={() => removeNumberRow(index)}>
                      移除
                    </Button>
                  </div>
                ))}
              </div>
              <Problems lines={problems['legalEntities[0].lifetimeRegistrationNumbers']} />
              <div className="mt-2 flex items-center gap-3">
                <Button variant="outline" disabled={locked} onClick={addNumberRow}>
                  添加一个终身注册号
                </Button>
                <span className="text-[11px] text-idpxyz-textMuted">
                  一个法人可按类型登多个终身注册号；号的格式与所属层由服务端按目录判。
                </span>
              </div>
            </div>
          </div>

          <div className="flex items-start gap-3">
            <Button onClick={form.send} disabled={locked || !form.canSend}>
              {locked ? '提交中…' : '提交登记'}
            </Button>
            <RegistrationAnswerNote
              state={form.state}
              owner={info.owner}
              outcomeLabels={partyIdentityOutcomeLabels}
              problemNote={problemNote}
            />
          </div>
        </CardContent>
      </Card>

      {/* 受控批量口的在线镜像，折起来放底部：主路径是上面那几格（ADR-0101 决定一原句）。 */}
      <details className="rounded border border-idpxyz-border">
        <summary className="cursor-pointer select-none px-4 py-2 text-[13px] text-idpxyz-textMuted">
          高级：粘贴登记快照 JSON（受控批量口 parcel-commercial register-parties 的在线镜像）
        </summary>
        <RegistrationPanel
          moduleId="group-legal-entities"
          title={registrationTitles[kind]}
          endpoint={endpoint}
          snapshotHint={registrationSnapshotHints[kind]}
          submit={form.submit}
          outcomeLabels={partyIdentityOutcomeLabels}
          problemNote={problemNote}
        />
      </details>
    </div>
  );
}

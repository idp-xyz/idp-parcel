import { useEffect, useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { primaryActionClass } from '../../components/primary-action';
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
import { identityRevisionLabel, problemNote, registrationSnapshotHints, registrationTitles } from './presentation';
import { Field, Problems, ReferencePicker, ReferencePickerFor, fieldLabel, useLoaded } from './PublicationFormFields';
import {
  AdvancedGroup,
  FormGroup,
  RevisionField,
  WallTimeField,
  businessPartyPickerOptions,
  useRegistrationForm,
} from './party-registration-fields';
import { advancedGroupOpen } from './registration-form';
import {
  emptyLegalEntityDraft,
  identityCountryOptions,
  identityTypeOptions,
  legalEntityDraftUnsaved,
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
 *
 * **分组与措辞照读者来**（票 group-legal-entities-uiux/03）：读表单的是运营配置员，所以格按「法人是谁 / 这一笔的依据
 * 与时点 / 高级」分三组，修订号与身份更正依据这两格平时用不上（修订号有建议值，更正依据只在更正那一笔填），收进高级组；
 * 端点、「由服务端判」这类实现交代不进格下，「登记册判定」只在说明里说一次。
 */
export interface LegalEntityRegistrationFormProps {
  /** 已取回的法人列表（给修订号建议用）；列表没取到（未配置 / 出错 / 加载中）传 null，建议一律为 1。 */
  knownEntities: readonly GroupLegalEntityRecord[] | null;
  /** 登记册答 `REGISTERED` 时回调，页面借它重取列表让新行在「集团与法人」签立刻可见。 */
  onRegistered: () => void;
  /** 草稿有没有没提交的输入（legalEntityDraftUnsaved）；页面据此决定离开登记视图前要不要先确认。卸载时报 false。 */
  onUnsavedChange?: (unsaved: boolean) => void;
}

const info = moduleInfoById['group-legal-entities'];
const kind = 'legal-entity';
const endpoint = `POST ${commercialRegistrationEndpoints[kind]}`;
// 收在高级组里的两格；组里有问题时组一律展开，问题不被收起藏住。
const advancedPaths = ['legalEntities[0].revision', 'legalEntities[0].identityCorrectionBasis'];

export function LegalEntityRegistrationForm({
  knownEntities,
  onRegistered,
  onUnsavedChange,
}: LegalEntityRegistrationFormProps) {
  // 最近一次逐字段落册时送出的那份；JSON 镜像那条路落册交 null，与这份草稿无关，不动它。
  const [landedDraft, setLandedDraft] = useState<LegalEntityDraft | null>(null);
  const form = useRegistrationForm<LegalEntityDraft>({
    kind,
    empty: emptyLegalEntityDraft,
    suggestion: (draft) => suggestedRevision(knownEntities, draft.legalEntityId),
    localProblems: legalEntityLocalProblems,
    payloadOf: legalEntityPayloadOf,
    // 答 `REGISTERED`（PartyRegistryOutcome 里 PartyIdentityRegistered 的线上名）才重取列表。
    landedOutcome: 'REGISTERED',
    onLanded: (sent) => {
      if (sent !== null) setLandedDraft(sent);
      onRegistered();
    },
  });
  const { draft, patch, problems, locked, timeZone } = form;
  const unsaved = legalEntityDraftUnsaved(draft, landedDraft);
  useEffect(() => {
    onUnsavedChange?.(unsaved);
  }, [onUnsavedChange, unsaved]);
  useEffect(() => () => onUnsavedChange?.(false), [onUnsavedChange]);

  // 国家与号类型的候选读一次：一张表单里国家格与每一行的类型格共用这一份目录答案。
  const typeCatalogue = useLoaded(listRegistrationNumberTypes);

  // 本册的载荷裁空白（legalEntityPayloadOf），在册提示按裁过的标识找，与送出的是同一个对象。
  const identifier = draft.legalEntityId.trim();
  const known = knownEntities?.find((row) => row.legalEntityId === identifier);
  // 列表没取到时说没取到，不说「不在册」：那时每个标识看起来都像首笔，提示会骗人。
  const identifierNote =
    identifier === ''
      ? '法人的稳定标识，登记后不变。'
      : known !== undefined
        ? `已在册，最新修订 ${identityRevisionLabel(known.revision)}：本次登记形成它的下一修订。`
        : knownEntities === null
          ? '法人列表没取到，提示不了这一标识在不在册。'
          : '已取回的列表里没有这一标识：本次登记是首笔修订。';

  // 高级组：null 即操作者还没点过，按 advancedGroupOpen 的缺省走。
  const [advancedChoice, setAdvancedChoice] = useState<boolean | null>(null);

  const setNumberRow = (index: number, change: Partial<LifetimeNumberDraft>) =>
    patch({ lifetimeNumbers: draft.lifetimeNumbers.map((row, at) => (at === index ? { ...row, ...change } : row)) });
  const addNumberRow = () => patch({ lifetimeNumbers: [...draft.lifetimeNumbers, { typeCode: '', number: '' }] });
  const removeNumberRow = (index: number) =>
    patch({ lifetimeNumbers: draft.lifetimeNumbers.filter((_, at) => at !== index) });

  const advancedOpen = advancedGroupOpen(advancedChoice, problems, advancedPaths, known !== undefined);
  const correctionBasis = draft.identityCorrectionBasis.trim();
  const advancedSummary =
    `身份修订号 ${form.revisionField.value.trim() || '未填'}${form.revisionField.edited ? '（手填）' : '（建议值）'}` +
    ` · 身份更正依据${correctionBasis === '' ? '不带' : ` ${correctionBasis}`}`;

  return (
    <div className="flex-1 overflow-auto p-4 flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{registrationTitles[kind]}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-6">
          <div className="rounded-md border border-idpxyz-border bg-idpxyz-sidebar p-3">
            <p className="text-[13px] font-medium text-idpxyz-text">登记说明</p>
            <ul className="mt-1 list-disc pl-4 text-xs leading-5 text-idpxyz-textMuted">
              <li>一笔登记形成一个身份修订，登记过的不被覆盖。首笔修订从 1 起，此后必须连续。</li>
              <li>录错了就登记同一法人的下一修订；改正注册国家 / 地区或终身注册号时写明身份更正依据。</li>
              <li>注册国家 / 地区与终身注册号不作变更：号变了就是另一个法人，停用旧身份、登记新身份。</li>
              <li>表单只检查格式；参与方是否在册且已生效、修订是否连续，由登记册判定，答复显示在提交按钮旁。</li>
            </ul>
          </div>

          <FormGroup title="法人身份" description="法人是谁：稳定标识、所钉的业务参与方身份、注册国家 / 地区与终身注册号。">
            <Field label="法人标识 *" path="legalEntities[0].legalEntityId" problems={problems}>
              <Input
                value={draft.legalEntityId}
                readOnly={locked}
                className="font-mono text-[13px]"
                placeholder="如 LE-SHA-01"
                onChange={(event) => patch({ legalEntityId: event.target.value })}
              />
              <span className="block text-[11px] text-idpxyz-textMuted mt-1">{identifierNote}</span>
            </Field>

            <ReferencePicker<BusinessPartyListResponseBody>
              label="业务参与方身份 *"
              path="legalEntities[0].partyId"
              problems={problems}
              value={draft.partyId}
              locked={locked}
              onChange={(partyId) => patch({ partyId })}
              load={listBusinessParties}
              optionsOf={businessPartyPickerOptions}
              emptyNote="参与方册还没有身份：先到「业务参与方」页登记参与方身份。"
              readFace="参与方册"
              optionsNote="候选列出参与方册上的全部身份并标出状态；参与方须在法人生效时点已生效。"
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
              emptyNote="注册号类型目录里还没有身份层类型：先登记注册号类型。"
              readFace="注册号类型目录"
              manualPlaceholder="国家 / 地区码（如 CN、SG）"
              optionsNote="候选是注册号类型目录里登过身份层类型的国家 / 地区。"
            />

            <div className="md:col-span-2">
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
                      emptyNote="先选注册国家 / 地区；目录里该国家 / 地区还没有身份层类型时，先登记注册号类型。"
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
                <span className="text-[11px] text-idpxyz-textMuted">一个法人可按类型登记多个终身注册号。</span>
              </div>
            </div>
          </FormGroup>

          <FormGroup title="登记" description="这一笔修订的依据与生效时点。">
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
              label="生效自 *"
              path="legalEntities[0].effectiveFrom"
              problems={problems}
              value={draft.effectiveFrom}
              locked={locked}
              timeZone={timeZone}
              onChange={(effectiveFrom) => patch({ effectiveFrom })}
            />
          </FormGroup>

          <AdvancedGroup open={advancedOpen} onToggle={() => setAdvancedChoice(!advancedOpen)} summary={advancedSummary}>
            <RevisionField
              label="身份修订号 *"
              path="legalEntities[0].revision"
              problems={problems}
              {...form.revisionField}
              note="建议值取该法人最新修订 + 1，不在册为 1。"
            />

            <Field label="身份更正依据" path="legalEntities[0].identityCorrectionBasis" problems={problems}>
              <Input
                value={draft.identityCorrectionBasis}
                readOnly={locked}
                className="font-mono text-[13px]"
                placeholder="更正依据引用；留空即不带"
                onChange={(event) => patch({ identityCorrectionBasis: event.target.value })}
              />
              <span className="block text-[11px] text-idpxyz-textMuted mt-1">
                只在更正修订上填：改正录错的注册国家 / 地区或终身注册号时，写明依据。首笔修订不填。
              </span>
            </Field>
          </AdvancedGroup>

          <div className="flex items-start gap-3 border-t border-idpxyz-border pt-4">
            <Button
              variant="default"
              className={primaryActionClass}
              onClick={form.send}
              disabled={locked || !form.canSend}
            >
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

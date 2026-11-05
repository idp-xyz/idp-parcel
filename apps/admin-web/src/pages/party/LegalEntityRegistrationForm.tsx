import { useEffect, useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { primaryActionClass } from '../../components/primary-action';
import { RegistrationAnswerNote, RegistrationPanel, type RegistrationPanelState } from '../../components/registration';
import {
  commercialRegistrationEndpoints,
  listBusinessParties,
  listRegistrationNumberTypes,
  partyIdentityOutcomeLabels,
  registerCommercial,
  type BusinessPartyListResponseBody,
  type GroupLegalEntityRecord,
  type RegistrationNumberTypeListResponseBody,
} from './api';
import { identityRevisionLabel, problemNote, registrationSnapshotHints, registrationTitles } from './presentation';
import { Field, Problems, ReferencePickerFor, fieldLabel, useLoaded } from './PublicationFormFields';
import {
  AdvancedGroup,
  FormGroup,
  RevisionField,
  WallTimeField,
  businessPartyPickerOptions,
  useRegistrationForm,
} from './party-registration-fields';
import { advancedGroupOpen } from './registration-form';
import { businessPartyPayloadOf } from './business-party-form';
import {
  companionPartyIdOf,
  emptyLegalEntityDraft,
  identityCountryOptions,
  identityTypeOptions,
  legalEntityDraftUnsaved,
  legalEntityLocalProblems,
  legalEntityPayloadOf,
  partyStepLanded,
  runPairedSave,
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
 * **建档一体化**（票 company-subject-ux/02）：往来单位档案是一个二选一——这家公司还没有档案就一并登记（缺省），已有就从
 * 参与方册上选。一并登记时保存串行两步：先打往来单位口、落了再打法人口，次序与停止条件在 runPairedSave；两步各自原样送、
 * 原样示答复，往来单位那一步的答复显在它自己那组里，法人那一步的显在保存钮旁。第一步落了、第二步没过时，那一组锁成
 * 「已登记 ✓」、输入全留着，再保存只发第二步。两步串行是管理台把两次登记串起来，不加后端复合命令（spec「不变的决定」）。
 *
 * **不带租户格**：在线 Intake 从认证结果取租户、只从载荷取行内容（register_party_identity.go 包注释）；表单里摆一格
 * 租户就是让操作者自报。今天端点挂 UnconfiguredIntake{} 必答 403，答案由 RegistrationAnswerNote 与 JSON 签同一份呈现。
 *
 * JSON 快照签降为底部折叠区「高级：导入 JSON」——它是受控批量口的在线镜像（ADR-0101 决定一），不是运营
 * 配置员的主路径，但仍要在：CLI 那份形状能原样粘进来核对，是两口锁到同一登记用例的可见证据。它只镜像法人那一步：
 * 批量口的一项就是一个法人，往来单位在那条路上是另一批。
 *
 * **分组与措辞照读者来**（票 group-legal-entities-uiux/03，用语照 company-subject-ux spec 的统一用语表）：读表单的是运营
 * 配置员，所以格按「公司身份 / 登记 / 高级」分三组，版本号与更正来源单据这两格平时用不上（版本号有系统算的值，更正来源单据
 * 只在更正那一笔填），收进高级组；端点、「由服务端判」这类实现交代不进格下，「登记册判定」只在说明里说一次。
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
const partyInfo = moduleInfoById['business-parties'];
const kind = 'legal-entity';
const endpoint = `POST ${commercialRegistrationEndpoints[kind]}`;
// 收在高级组里的两格；组里有问题时组一律展开，问题不被收起藏住。
const advancedPaths = ['legalEntities[0].revision', 'legalEntities[0].identityCorrectionBasis'];
const numberTypeCatalogueHint = '先到「基础数据 › 注册号类型」配置';

export function LegalEntityRegistrationForm({
  knownEntities,
  onRegistered,
  onUnsavedChange,
}: LegalEntityRegistrationFormProps) {
  // 最近一次逐字段落册时送出的那份；JSON 镜像那条路落册交 null，与这份草稿无关，不动它。
  const [landedDraft, setLandedDraft] = useState<LegalEntityDraft | null>(null);
  // 往来单位那一步的答复，与法人那一步分开示：两步各自原样示答复，一句里混着两口的答案分不清哪步没过。
  const [partyState, setPartyState] = useState<RegistrationPanelState>({ kind: 'idle' });
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
  // 参与方册读一次：选已有档案的下拉与一并登记那组的「编码已在册」提示共用这一份。
  const partyCatalogue = useLoaded(listBusinessParties);
  const knownParties = partyCatalogue?.kind === 'outcome' ? partyCatalogue.body.parties : null;

  // 本册的载荷裁空白（legalEntityPayloadOf），在册提示按裁过的标识找，与送出的是同一个对象。
  const identifier = draft.legalEntityId.trim();
  const known = knownEntities?.find((row) => row.legalEntityId === identifier);
  // 列表没取到时说没取到，不说「不在册」：那时每个标识看起来都像首笔，提示会骗人。
  const identifierNote =
    identifier === ''
      ? '公司主体的编码，登记后不变。'
      : known !== undefined
        ? `已在册（${identityRevisionLabel(known.revision)}）：这次保存会形成新的一版。`
        : knownEntities === null
          ? '公司主体列表没取到，提示不了这个编码在不在册。'
          : '列表里没有这个编码：这是首次登记。';

  // 一并登记那一组：第一步落册后锁成「已登记 ✓」，编码与名称都不再改——改了就不是登记成的那份档案。
  const companionRegistered = draft.companionRegisteredPartyId !== '';
  const companionLocked = locked || companionRegistered;
  const companionPartyId = companionPartyIdOf(draft);
  const companionKnown = knownParties?.find((row) => row.partyId === companionPartyId);
  const companionIdNote = companionRegistered
    ? `已登记为往来单位 ${draft.companionRegisteredPartyId}；这次保存只登记公司主体。`
    : companionPartyId === ''
      ? '留空即与公司主体编码相同；先填上面的编码。'
      : companionKnown !== undefined
        ? `这个编码已登记为「${companionKnown.partyName}」：一并登记只登首笔，要用已有的档案请选下面「已有往来单位档案」。`
        : `将登记为往来单位 ${companionPartyId}；留空即与公司主体编码相同。`;

  // 两步串行（runPairedSave）：壳只管锁与法人那一步的答复，往来单位那一步的答复与落册后钉编码由这里接。
  const save = () =>
    form.sendVia(async (sending, submitDraft) => {
      const result = await runPairedSave(sending, {
        party: async (companion) => {
          setPartyState({ kind: 'submitting' });
          const answer = await registerCommercial('business-party', businessPartyPayloadOf(companion, timeZone));
          setPartyState({ kind: 'answered', answer });
          if (partyStepLanded(answer)) patch({ companionRegisteredPartyId: companion.partyId });
          return answer;
        },
        entity: (pinned) => submitDraft(legalEntityPayloadOf(pinned, timeZone), pinned),
      });
      return result.entity;
    });

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
    `版本号 ${form.revisionField.value.trim() || '未填'}${form.revisionField.edited ? '（手填）' : '（系统算的）'}` +
    ` · 更正来源单据${correctionBasis === '' ? '不带' : ` ${correctionBasis}`}`;

  return (
    <div className="flex-1 overflow-auto p-4 flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{registrationTitles[kind]}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-6">
          <div className="rounded-md border border-idpxyz-border bg-idpxyz-sidebar p-3">
            <p className="type-value font-medium text-idpxyz-text">说明</p>
            <ul className="mt-1 list-disc pl-4 type-body leading-5 text-idpxyz-breadcrumb">
              <li>这里登记的是我们自己开票、签约用的公司。客户、供应商在「往来单位」里登记。</li>
              <li>还没有往来单位档案的公司会在保存时一并登记一份；已有档案的从下拉里选。</li>
              <li>录错了就填同一个编码再保存一次；改正注册地或注册号时在「高级」里写明更正的来源单据。</li>
              <li>注册号换了就是另一家公司：停用旧的、新增一个。</li>
            </ul>
          </div>

          <FormGroup title="公司身份" description="这家公司是谁：编码、注册地、注册号。">
            <Field label="编码 *" path="legalEntities[0].legalEntityId" problems={problems}>
              <Input
                value={draft.legalEntityId}
                readOnly={locked}
                className="font-mono type-value"
                placeholder="如 LE-SHA-01"
                onChange={(event) => patch({ legalEntityId: event.target.value })}
              />
              <span className="block type-body text-idpxyz-breadcrumb mt-1">{identifierNote}</span>
            </Field>

            <ReferencePickerFor<RegistrationNumberTypeListResponseBody>
              answer={typeCatalogue}
              label="注册地（国家 / 地区）*"
              path="legalEntities[0].registrationCountry"
              problems={problems}
              value={draft.registrationCountry}
              locked={locked}
              onChange={(registrationCountry) => patch({ registrationCountry })}
              optionsOf={(body) => identityCountryOptions(body.registrationNumberTypes)}
              emptyNote={`还没有配置任何注册号类型：${numberTypeCatalogueHint}。`}
              readFace="注册号类型目录"
              manualPlaceholder="国家 / 地区码（如 CN、SG）"
              optionsNote={`只能选已配置注册号类型的国家 / 地区；要加别的国家，${numberTypeCatalogueHint}。`}
            />

            {/* 往来单位档案二选一：缺省一并登记，选已有时那组收起、下拉照旧。单选是原生控件，与结算页的做法一样。 */}
            <fieldset className="md:col-span-2 flex flex-col gap-2">
              <legend className={fieldLabel}>往来单位档案 *</legend>
              <label className="flex items-center gap-1.5 type-value text-idpxyz-text">
                <input
                  type="radio"
                  name="legal-entity-party-mode"
                  value="together"
                  checked={draft.partyMode === 'together'}
                  disabled={companionLocked}
                  onChange={() => patch({ partyMode: 'together' })}
                />
                <span>这家公司还没有往来单位档案，一并登记</span>
              </label>
              {draft.partyMode === 'together' ? (
                <div
                  className="ml-5 grid grid-cols-1 gap-3 rounded-md border border-idpxyz-border p-3 md:grid-cols-2"
                  aria-label="一并登记的往来单位"
                >
                  {companionRegistered ? (
                    <p className="md:col-span-2 type-value text-idpxyz-text">
                      已登记 ✓ 往来单位 <span className="font-mono">{draft.companionRegisteredPartyId}</span> 已在册，这次保存只登记公司主体。
                    </p>
                  ) : null}
                  <Field label="公司名称 *" path="businessParties[0].name" problems={problems}>
                    <Input
                      value={draft.partyName}
                      readOnly={companionLocked}
                      className="type-value"
                      placeholder="如 深圳某某跨境电商有限公司"
                      onChange={(event) => patch({ partyName: event.target.value })}
                    />
                  </Field>
                  <Field label="往来单位编码" path="businessParties[0].partyId" problems={problems}>
                    <Input
                      value={companionRegistered ? draft.companionRegisteredPartyId : draft.companionPartyId}
                      readOnly={companionLocked}
                      className="font-mono type-value"
                      placeholder={identifier === '' ? '缺省与公司主体编码相同' : identifier}
                      onChange={(event) => patch({ companionPartyId: event.target.value })}
                    />
                    <span className="block type-body text-idpxyz-breadcrumb mt-1">{companionIdNote}</span>
                  </Field>
                  <p className="md:col-span-2 type-body text-idpxyz-breadcrumb">
                    来源单据与生效时间用下面「登记」里填的那一份：往来单位与公司主体同一时刻生效。
                  </p>
                  {partyState.kind === 'answered' ? (
                    <div className="md:col-span-2 flex items-start gap-2">
                      <span className="shrink-0 type-body leading-4 text-idpxyz-textMuted">往来单位这一步：</span>
                      <RegistrationAnswerNote
                        state={partyState}
                        owner={partyInfo.owner}
                        outcomeLabels={partyIdentityOutcomeLabels}
                        problemNote={problemNote}
                      />
                    </div>
                  ) : null}
                </div>
              ) : null}
              <label className="flex items-center gap-1.5 type-value text-idpxyz-text">
                <input
                  type="radio"
                  name="legal-entity-party-mode"
                  value="existing"
                  checked={draft.partyMode === 'existing'}
                  disabled={companionLocked}
                  onChange={() => patch({ partyMode: 'existing' })}
                />
                <span>已有往来单位档案：选择</span>
              </label>
              {draft.partyMode === 'existing' ? (
                <div className="ml-5">
                  <ReferencePickerFor<BusinessPartyListResponseBody>
                    answer={partyCatalogue}
                    label="往来单位档案"
                    path="legalEntities[0].partyId"
                    problems={problems}
                    value={draft.partyId}
                    locked={locked}
                    onChange={(partyId) => patch({ partyId })}
                    optionsOf={businessPartyPickerOptions}
                    emptyNote="还没有登记任何往来单位：选上面「一并登记」。"
                    readFace="往来单位册"
                    optionsNote="候选列出往来单位册上的全部档案并标出状态。"
                  />
                </div>
              ) : null}
            </fieldset>

            <div className="md:col-span-2">
              <p className={fieldLabel}>注册号 *</p>
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
                      emptyNote={`先选注册地；该国家 / 地区还没有配置注册号类型时，${numberTypeCatalogueHint}。`}
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
                        className="font-mono type-value"
                        placeholder="注册号原样填"
                        onChange={(event) => setNumberRow(index, { number: event.target.value })}
                      />
                    </Field>
                    <Button variant="ghost" className="mt-5 text-idpxyz-breadcrumb" disabled={locked} onClick={() => removeNumberRow(index)}>
                      移除
                    </Button>
                  </div>
                ))}
              </div>
              <Problems lines={problems['legalEntities[0].lifetimeRegistrationNumbers']} />
              <div className="mt-2 flex items-center gap-3">
                <Button className="text-idpxyz-breadcrumb" variant="outline" disabled={locked} onClick={addNumberRow}>
                  添加一个注册号
                </Button>
                <span className="type-body text-idpxyz-breadcrumb">
                  如统一社会信用代码、公司注册编号（CR No.）、EIN；一个公司可按类型登记多个。
                </span>
              </div>
            </div>
          </FormGroup>

          <FormGroup title="登记" description="这次登记依据哪份单据、从什么时候起生效。">
            <Field label="来源单据 *" path="legalEntities[0].basis" problems={problems}>
              <Input
                value={draft.basis}
                readOnly={locked}
                className="font-mono type-value"
                placeholder="董事会决议、营业执照或审批单号"
                onChange={(event) => patch({ basis: event.target.value })}
              />
            </Field>

            <WallTimeField
              label="生效时间 *"
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
              label="版本号"
              path="legalEntities[0].revision"
              problems={problems}
              {...form.revisionField}
              note="系统按已登记的记录自动算好，一般不用改。保存时提示版本号对不上，说明列表已过时，刷新后再试。"
            />

            <Field label="更正来源单据" path="legalEntities[0].identityCorrectionBasis" problems={problems}>
              <Input
                value={draft.identityCorrectionBasis}
                readOnly={locked}
                className="font-mono type-value"
                placeholder="只在改正注册地或注册号时填"
                onChange={(event) => patch({ identityCorrectionBasis: event.target.value })}
              />
              <span className="block type-body text-idpxyz-breadcrumb mt-1">
                改正录错的注册地或注册号时，写明依据哪份单据；首次登记不填。
              </span>
            </Field>
          </AdvancedGroup>

          <div className="flex items-start gap-3 border-t border-idpxyz-border pt-4">
            <Button
              variant="default"
              className={primaryActionClass}
              onClick={save}
              disabled={locked || !form.canSend}
            >
              {locked ? '保存中…' : '保存'}
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

      {/* 受控批量口的在线镜像，折起来放底部：主路径是上面那几格（ADR-0101 决定一原句）。只镜像法人那一步，理由在组件头注。 */}
      <details className="rounded border border-idpxyz-border">
        <summary className="cursor-pointer select-none px-4 py-2 type-value text-idpxyz-breadcrumb">
          高级：导入 JSON（批量迁移用）
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

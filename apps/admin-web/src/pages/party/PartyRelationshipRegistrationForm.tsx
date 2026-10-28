import { useEffect, useId, useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { RegistrationAnswerNote } from '../../components/registration';
import type { ApiResult } from '../catalogue-api';
import {
  partyIdentityOutcomeLabels,
  type BusinessPartyListResponseBody,
  type PartyRelationshipRecord,
} from './api';
import { loadCustomerAccountSuggestions } from './reference-inputs';
import { labelOf, partyRoleLabels, problemNote, registrationTitles } from './presentation';
import { Field, ReferencePickerFor, fieldLabel, selectClass, useLoaded } from './PublicationFormFields';
import { authorizedDeliveringPartyRole } from './authorized-delivery-scope';
import {
  AdvancedGroup,
  RevisionField,
  SnapshotJsonDetails,
  WallTimeField,
  businessPartyPickerOptions,
  customerAccountPickerOptions,
  useRegistrationForm,
} from './party-registration-fields';
import { advancedGroupOpen } from './registration-form';
import {
  deliveryScopeSpellingOf,
  emptyPartyRelationshipDraft,
  partyRelationshipLocalProblems,
  partyRelationshipPayloadOf,
  partyRoleOptions,
  suggestedPartyRelationshipRevision,
  type PartyRelationshipDraft,
} from './party-relationship-form';

/**
 * 「建立合作关系」表单（票 admin-web-group-legal-entities/10 第 2 条）。本页几册里它价值最大：格里有封闭词与从册上选的引用，
 * 粘 JSON 时打错一个词只得到一个说不清的 400。壳在 useRegistrationForm，这里只摆本册的格。
 *
 * 格按人话排：谁、担任什么角色、对谁、覆盖哪些业务、从什么时候到什么时候、依据哪份单据、批准了没有。关系编号仍要人起
 * （服务端不生成），放在最后一格；更新序号由表单算好收在「高级」里。
 *
 * **本组件不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）：双方在不在册、角色与双方是否匹配、
 * 区间是否倒置、修订连不连续，一律送上去让服务端答，答回来的问题码经 field-problem-text.ts 译成人话；本地只拦编码层（修订号与
 * 各时刻），纯函数在 party-relationship-form.ts。**可缺键缺席而不是零值**——结束时间留空即长期有效、不勾「已批准」即待确认，
 * 理由在那个文件头上。
 *
 * 双方用 ReferencePickerFor 从参与方册选：候选显名称 · 编码 · 状态，不按状态过滤，读面不可用退回手填。候选取的是页面持有的
 * 那份参与方列表答案，不另读——页面在登记册答 REGISTERED 后重取，刚登进去的那一家随新答案立刻出现在候选里（票 10 评审 N5）。
 *
 * 选了授权交付方时，适用范围一格换成「客户账户」「网点（可选）」两个输入：这一角色的范围服务端只认一种拼法，由
 * authorized-delivery-scope.ts 拼（票 ps-external-mark-relations/16）。别的角色照旧手填引用。
 */
export interface PartyRelationshipRegistrationFormProps {
  /** 页面已取回的关系列表（给修订号建议用）；没取到传 null，建议一律为 1。 */
  knownRelationships: readonly PartyRelationshipRecord[] | null;
  /** 页面持有的参与方列表答案（给双方候选用）；首取回来之前为 null。 */
  parties: ApiResult<BusinessPartyListResponseBody> | null;
  /** 登记册答 `REGISTERED` 时回调，页面借它重取关系列表。 */
  onRegistered: () => void;
  /** 草稿有没有没提交的输入；页面据此决定离开登记视图前要不要先确认。卸载时报 false。 */
  onUnsavedChange?: (unsaved: boolean) => void;
}

const info = moduleInfoById['business-parties'];
const kind = 'party-relationship';
const roleOptions = partyRoleOptions();
const advancedPaths = ['relationships[0].revision'] as const;

export function PartyRelationshipRegistrationForm({
  knownRelationships,
  parties,
  onRegistered,
  onUnsavedChange,
}: PartyRelationshipRegistrationFormProps) {
  const form = useRegistrationForm<PartyRelationshipDraft>({
    kind,
    empty: emptyPartyRelationshipDraft,
    suggestion: (draft) => suggestedPartyRelationshipRevision(knownRelationships, draft.relationshipId),
    localProblems: partyRelationshipLocalProblems,
    payloadOf: partyRelationshipPayloadOf,
    landedOutcome: 'REGISTERED',
    onLanded: onRegistered,
  });
  const { draft, patch, problems, locked, timeZone, unsaved } = form;
  useEffect(() => {
    onUnsavedChange?.(unsaved);
  }, [onUnsavedChange, unsaved]);
  useEffect(() => () => onUnsavedChange?.(false), [onUnsavedChange]);

  const known = knownRelationships?.find((row) => row.relationshipId === draft.relationshipId);
  const deliveryScope = draft.role === authorizedDeliveringPartyRole ? deliveryScopeSpellingOf(draft) : null;
  const accounts = useLoaded(loadCustomerAccountSuggestions);
  const accountOptions = accounts?.kind === 'outcome' ? customerAccountPickerOptions(accounts.body) : [];
  const accountListId = useId();
  const roleWord = draft.role === '' ? '这个角色' : labelOf(partyRoleLabels, draft.role);
  // 高级组：null 即操作者还没点过，按 advancedGroupOpen 的缺省走（编号已在册时展开——这一笔是更新，让人看见序号）。
  const [advancedChoice, setAdvancedChoice] = useState<boolean | null>(null);
  const advancedOpen = advancedGroupOpen(advancedChoice, problems, advancedPaths, known !== undefined);
  const advancedSummary = form.revisionField.edited
    ? `更新序号改为 ${form.revisionField.value}`
    : known
      ? `这是关系 ${known.relationshipId} 的第 ${form.revisionField.suggestion} 次更新，序号已算好`
      : '首次登记，序号由系统填';

  return (
    <div className="flex-1 overflow-auto p-4 flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{registrationTitles[kind]}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <p className="text-[12px] text-idpxyz-breadcrumb">
            记录谁对谁担任什么角色、从什么时候开始。双方都要先登记为往来单位；没有批准信息的先作为待确认关系，批准后再保存一次。
          </p>

          <div className="grid grid-cols-2 gap-3">
            <ReferencePickerFor<BusinessPartyListResponseBody>
              label="哪一方 *"
              path="relationships[0].holder"
              problems={problems}
              value={draft.holder}
              locked={locked}
              onChange={(holder) => patch({ holder })}
              answer={parties}
              optionsOf={businessPartyPickerOptions}
              emptyNote="还没有登记任何往来单位；先在「新增往来单位」里登记双方，或直接填编码。"
              readFace="往来单位"
              optionsNote={`担任${roleWord}的单位，例如客户、承运商。`}
            />

            <Field label="角色 *" path="relationships[0].role" problems={problems}>
              <select
                className={selectClass}
                value={draft.role}
                disabled={locked}
                onChange={(event) => patch({ role: event.target.value })}
              >
                <option value="">请选择</option>
                {roleOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
              <span className="block text-[12px] text-idpxyz-breadcrumb mt-1">它对对方是什么：客户、供应商、承运商代理……</span>
            </Field>

            <ReferencePickerFor<BusinessPartyListResponseBody>
              label="对哪一方 *"
              path="relationships[0].counterparty"
              problems={problems}
              value={draft.counterparty}
              locked={locked}
              onChange={(counterparty) => patch({ counterparty })}
              answer={parties}
              optionsOf={businessPartyPickerOptions}
              emptyNote="还没有登记任何往来单位；先在「新增往来单位」里登记双方，或直接填编码。"
              readFace="往来单位"
              optionsNote="通常是我们自己的公司主体；授权交付时是客户所属的单位。"
            />

            {deliveryScope ? (
              <Field label="覆盖范围 *" path="relationships[0].scope" problems={problems} as="div">
                <div className="grid grid-cols-2 gap-2">
                  <label className="block">
                    <span className={fieldLabel}>客户账户 *</span>
                    {/* 账户册读口分页：能直接输入、带第一页候选作提示，不做成只列得出第一页的选单（票 party-commercial-uiux/13）。 */}
                    <Input
                      value={draft.scopeCustomerAccount}
                      readOnly={locked}
                      list={accountOptions.length > 0 ? accountListId : undefined}
                      className="font-mono text-[13px]"
                      placeholder="可从提示里选，也可直接输入账户编码"
                      onChange={(event) => patch({ scopeCustomerAccount: event.target.value })}
                    />
                    {accountOptions.length > 0 ? (
                      <datalist id={accountListId}>
                        {accountOptions.map((option) => (
                          <option key={option.value} value={option.value} label={option.label} />
                        ))}
                      </datalist>
                    ) : null}
                  </label>
                  <label className="block">
                    <span className={fieldLabel}>网点（可选）</span>
                    <Input
                      value={draft.scopeNode}
                      readOnly={locked}
                      className="font-mono text-[13px]"
                      placeholder="留空即不限网点"
                      onChange={(event) => patch({ scopeNode: event.target.value })}
                    />
                  </label>
                </div>
                <span className="block text-[12px] text-idpxyz-breadcrumb mt-1">
                  这家单位替哪个客户账户交付，可以再限定到一个网点。
                </span>
              </Field>
            ) : (
              <Field label="覆盖范围 *" path="relationships[0].scope" problems={problems}>
                <Input
                  value={draft.scope}
                  readOnly={locked}
                  className="font-mono text-[13px]"
                  placeholder="如 全部业务，或某条线路、某个客户账户"
                  onChange={(event) => patch({ scope: event.target.value })}
                />
                <span className="block text-[12px] text-idpxyz-breadcrumb mt-1">这段关系管哪些业务；按公司约定的写法填。</span>
              </Field>
            )}

            <WallTimeField
              label="开始时间 *"
              path="relationships[0].effectiveStartsAt"
              problems={problems}
              value={draft.effectiveStartsAt}
              locked={locked}
              timeZone={timeZone}
              onChange={(effectiveStartsAt) => patch({ effectiveStartsAt })}
            />

            <WallTimeField
              label="结束时间"
              path="relationships[0].effectiveEndsAt"
              problems={problems}
              value={draft.effectiveEndsAt}
              locked={locked}
              timeZone={timeZone}
              onChange={(effectiveEndsAt) => patch({ effectiveEndsAt })}
              note="留空即长期有效，直到撤销或到期。"
            />

            <Field label="来源单据 *" path="relationships[0].basis" problems={problems}>
              <Input
                value={draft.basis}
                readOnly={locked}
                className="font-mono text-[13px]"
                placeholder="合同编号、邮件或审批单号"
                onChange={(event) => patch({ basis: event.target.value })}
              />
            </Field>

            <Field label="关系编号 *" path="relationships[0].relationshipId" problems={problems}>
              <Input
                value={draft.relationshipId}
                readOnly={locked}
                className="font-mono text-[13px]"
                placeholder="如 REL-SHIPPER-01-CUSTOMER"
                onChange={(event) => patch({ relationshipId: event.target.value })}
              />
              <span className="block text-[12px] text-idpxyz-breadcrumb mt-1">
                {known
                  ? `这个编号已登记；这次保存会更新那段关系。`
                  : '给这段关系起一个内部编号，登记后不能改。'}
              </span>
            </Field>
          </div>

          {/* 批准是一件事实而不是几格文本：勾选框才是「有没有批准」的声明，不勾时批准那几格不进载荷、残字也不进。 */}
          <div className="flex flex-col gap-3 rounded border border-idpxyz-border p-3">
            <label className="flex items-center gap-2 text-[13px] text-idpxyz-text">
              <input
                type="checkbox"
                checked={draft.approved}
                disabled={locked}
                onChange={(event) => patch({ approved: event.target.checked })}
              />
              已批准
              <span className="text-[12px] text-idpxyz-breadcrumb">不勾则先作为待确认关系，批准后再保存一次</span>
            </label>
            {draft.approved ? (
              <div className="grid grid-cols-2 gap-3">
                <Field label="批准单据 *" path="relationships[0].approval.reference" problems={problems}>
                  <Input
                    value={draft.approvalReference}
                    readOnly={locked}
                    className="font-mono text-[13px]"
                    placeholder="审批单号或批准邮件"
                    onChange={(event) => patch({ approvalReference: event.target.value })}
                  />
                </Field>
                <WallTimeField
                  label="批准时间 *"
                  path="relationships[0].approval.approvedAt"
                  problems={problems}
                  value={draft.approvedAt}
                  locked={locked}
                  timeZone={timeZone}
                  onChange={(approvedAt) => patch({ approvedAt })}
                />
              </div>
            ) : null}
          </div>

          <AdvancedGroup open={advancedOpen} onToggle={() => setAdvancedChoice(!advancedOpen)} summary={advancedSummary}>
            <RevisionField
              label="更新序号"
              path="relationships[0].revision"
              problems={problems}
              {...form.revisionField}
              note="系统按已登记的记录自动算好，一般不用改。保存时提示序号对不上，说明列表已过时，刷新后再试。"
            />
          </AdvancedGroup>

          <div className="flex items-start gap-3">
            <Button
              variant="default"
              className={primaryActionClass}
              onClick={form.send}
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

      <SnapshotJsonDetails kind={kind} submit={form.submit} />
    </div>
  );
}

import { useEffect, useId } from 'react';
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
import { problemNote, registrationTitles } from './presentation';
import { Field, ReferencePickerFor, fieldLabel, selectClass, useLoaded } from './PublicationFormFields';
import { authorizedDeliveringPartyRole } from './authorized-delivery-scope';
import {
  RevisionField,
  SnapshotJsonDetails,
  WallTimeField,
  businessPartyPickerOptions,
  customerAccountPickerOptions,
  useRegistrationForm,
} from './party-registration-fields';
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
 * 参与方关系登记的逐字段表单（票 admin-web-group-legal-entities/10 第 2 条）。本页几册里它价值最大：格里有封闭词与
 * 从册上选的引用，粘 JSON 时打错一个词只得到一个说不清的 400。壳在 useRegistrationForm，这里只摆本册的格。
 *
 * **本组件不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）：双方在不在册、角色与双方是否匹配、
 * 区间是否倒置、修订连不连续，一律送上去让服务端答；本地只拦编码层（修订号与各时刻），纯函数在
 * party-relationship-form.ts。**可缺键缺席而不是零值**——终点留空即开区间、不勾「已批准」即候选关系，理由在那个
 * 文件头上。
 *
 * 双方用 ReferencePickerFor 从参与方册选：候选显名称 · 标识 · 状态，不按状态过滤，读面不可用退回手填。候选取的是
 * 页面持有的那份参与方列表答案，不另读——页面在登记册答 REGISTERED 后重取，刚在「参与方身份」册登进去的那一个随
 * 新答案立刻出现在这里的候选里，两只 Picker 不再各自重读一遍（票 10 评审 N5）。
 *
 * 选了授权交付方时，适用范围一格换成「客户账户」「节点（可选）」两个输入，下面显出将送的那一串：这一角色的范围
 * 服务端只认一种拼法，由 authorized-delivery-scope.ts 拼（票 ps-external-mark-relations/16）。别的角色照旧手填引用。
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

  return (
    <div className="flex-1 overflow-auto p-4 flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{registrationTitles[kind]}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {/* 双方在不在册、角色与双方是否匹配、区间是否合法都由服务端按册面判；租户由操作者身份给出，不在表单上。 */}
          <p className="text-[12px] text-idpxyz-breadcrumb">
            {'关系记录谁对谁持有什么角色、在什么范围、多长时间。一笔登记形成一个修订，首笔修订号从 1 起，之后必须连续。双方都要已登记为业务参与方；角色与双方是否匹配、区间是否合法，由服务端检查。不填批准信息即登记为候选关系，批准另登一笔新修订。'}
          </p>

          <div className="grid grid-cols-2 gap-3">
            <Field label="关系标识 *" path="relationships[0].relationshipId" problems={problems}>
              <Input
                value={draft.relationshipId}
                readOnly={locked}
                className="font-mono text-[13px]"
                placeholder="如 SYN-REL-AGENT-07"
                onChange={(event) => patch({ relationshipId: event.target.value })}
              />
              <span className="block text-[12px] text-idpxyz-breadcrumb mt-1">
                {known
                  ? `已取回关系册里有这一关系，最新修订 r${known.revision}；本次登记是它的新修订。`
                  : '列表里还没有这个关系标识，这一笔是它的首笔修订；如果别人刚登记过，服务端会拒绝。'}
              </span>
            </Field>

            <RevisionField
              path="relationships[0].revision"
              problems={problems}
              {...form.revisionField}
              note="建议值是列表里这段关系最新的修订号 + 1（没有则为 1）；如果期间有人登记过，服务端会拒绝。"
            />

            <ReferencePickerFor<BusinessPartyListResponseBody>
              label="持有方 *"
              path="relationships[0].holder"
              problems={problems}
              value={draft.holder}
              locked={locked}
              onChange={(holder) => patch({ holder })}
              answer={parties}
              optionsOf={businessPartyPickerOptions}
              emptyNote="还没有登记任何业务参与方；先在「参与方身份」里登记双方，或手填标识。"
              readFace="参与方册"
              optionsNote="持有这个角色的一方。候选里也有未生效或已停用的参与方，是否可用由服务端检查。"
            />

            <ReferencePickerFor<BusinessPartyListResponseBody>
              label="相对方 *"
              path="relationships[0].counterparty"
              problems={problems}
              value={draft.counterparty}
              locked={locked}
              onChange={(counterparty) => patch({ counterparty })}
              answer={parties}
              optionsOf={businessPartyPickerOptions}
              emptyNote="还没有登记任何业务参与方；先在「参与方身份」里登记双方，或手填标识。"
              readFace="参与方册"
              optionsNote="被持有该角色的一方。方向由持有方 → 相对方表达，不另设方向格。"
            />

            <Field label="角色 *" path="relationships[0].role" problems={problems}>
              <select
                className={selectClass}
                value={draft.role}
                disabled={locked}
                onChange={(event) => patch({ role: event.target.value })}
              >
                <option value="">未选</option>
                {roleOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
              <span className="block text-[12px] text-idpxyz-breadcrumb mt-1">
                角色与双方是否匹配由服务端检查。
              </span>
            </Field>

            {deliveryScope ? (
              <Field label="适用范围 *" path="relationships[0].scope" problems={problems} as="div">
                <div className="grid grid-cols-2 gap-2">
                  <label className="block">
                    <span className={fieldLabel}>客户账户 *</span>
                    {/* 账户册读口分页：能直接输入、带第一页候选作提示，不做成只列得出第一页的选单（票 party-commercial-uiux/13）。 */}
                    <Input
                      value={draft.scopeCustomerAccount}
                      readOnly={locked}
                      list={accountOptions.length > 0 ? accountListId : undefined}
                      className="font-mono text-[13px]"
                      placeholder="可从提示里选，也可直接输入账户标识"
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
                    <span className={fieldLabel}>节点（可选）</span>
                    <Input
                      value={draft.scopeNode}
                      readOnly={locked}
                      className="font-mono text-[13px]"
                      placeholder="留空即不收窄到节点"
                      onChange={(event) => patch({ scopeNode: event.target.value })}
                    />
                  </label>
                </div>
                <span className="block text-[12px] text-idpxyz-breadcrumb mt-1">
                  持有方是交付方，相对方是这个客户账户所属的参与方；范围收到这个客户账户，可再收到一个节点。
                  {deliveryScope.ok ? (
                    <>
                      {' '}将送 <span className="font-mono break-all">{deliveryScope.scope}</span>
                    </>
                  ) : null}
                </span>
              </Field>
            ) : (
              <Field label="适用范围 *" path="relationships[0].scope" problems={problems}>
                <Input
                  value={draft.scope}
                  readOnly={locked}
                  className="font-mono text-[13px]"
                  placeholder="商业范围引用"
                  onChange={(event) => patch({ scope: event.target.value })}
                />
              </Field>
            )}

            <Field label="依据 *" path="relationships[0].basis" problems={problems}>
              <Input
                value={draft.basis}
                readOnly={locked}
                className="font-mono text-[13px]"
                placeholder="关系依据引用"
                onChange={(event) => patch({ basis: event.target.value })}
              />
            </Field>

            <WallTimeField
              label="生效起点 *"
              path="relationships[0].effectiveStartsAt"
              problems={problems}
              value={draft.effectiveStartsAt}
              locked={locked}
              timeZone={timeZone}
              onChange={(effectiveStartsAt) => patch({ effectiveStartsAt })}
            />

            <WallTimeField
              label="生效终点（留空即开区间）"
              path="relationships[0].effectiveEndsAt"
              problems={problems}
              value={draft.effectiveEndsAt}
              locked={locked}
              timeZone={timeZone}
              onChange={(effectiveEndsAt) => patch({ effectiveEndsAt })}
              note="留空则该键缺席，关系一直适用到被撤销 / 到期 / 替代。"
            />
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
              已批准（带批准事实登记；不勾即登为候选关系，approval 键缺席）
            </label>
            {draft.approved ? (
              <div className="grid grid-cols-2 gap-3">
                <Field label="批准引用 *" path="relationships[0].approval.reference" problems={problems}>
                  <Input
                    value={draft.approvalReference}
                    readOnly={locked}
                    className="font-mono text-[13px]"
                    placeholder="批准依据引用"
                    onChange={(event) => patch({ approvalReference: event.target.value })}
                  />
                </Field>
                <WallTimeField
                  label="批准时刻 *"
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

          <div className="flex items-start gap-3">
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

      <SnapshotJsonDetails kind={kind} submit={form.submit} />
    </div>
  );
}

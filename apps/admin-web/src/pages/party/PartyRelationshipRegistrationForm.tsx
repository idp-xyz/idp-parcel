import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { RegistrationAnswerNote } from '../../components/registration';
import type { ApiResult } from '../catalogue-api';
import {
  commercialRegistrationEndpoints,
  partyIdentityOutcomeLabels,
  type BusinessPartyListResponseBody,
  type PartyRelationshipRecord,
} from './api';
import { problemNote, registrationTitles } from './presentation';
import { Field, ReferencePickerFor, selectClass } from './PublicationFormFields';
import {
  RevisionField,
  SnapshotJsonDetails,
  WallTimeField,
  businessPartyPickerOptions,
  useRegistrationForm,
} from './party-registration-fields';
import {
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
 */
export interface PartyRelationshipRegistrationFormProps {
  /** 页面已取回的关系列表（给修订号建议用）；没取到传 null，建议一律为 1。 */
  knownRelationships: readonly PartyRelationshipRecord[] | null;
  /** 页面持有的参与方列表答案（给双方候选用）；首取回来之前为 null。 */
  parties: ApiResult<BusinessPartyListResponseBody> | null;
  /** 登记册答 `REGISTERED` 时回调，页面借它重取关系列表。 */
  onRegistered: () => void;
}

const info = moduleInfoById['business-parties'];
const kind = 'party-relationship';
const endpoint = `POST ${commercialRegistrationEndpoints[kind]}`;
const roleOptions = partyRoleOptions();

export function PartyRelationshipRegistrationForm({
  knownRelationships,
  parties,
  onRegistered,
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
  const { draft, patch, problems, locked, timeZone } = form;

  const known = knownRelationships?.find((row) => row.relationshipId === draft.relationshipId);

  return (
    <div className="flex-1 overflow-auto p-4 flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{registrationTitles[kind]}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <p className="text-xs text-idpxyz-textMuted">
            关系记谁对谁持有什么角色、在什么范围、多久。一笔登记一个修订，首笔从 1 起、此后连续；双方必须已在参与方册、
            角色是否与双方匹配、区间是否合法，都由服务端按册面判。不带批准事实即登为候选关系，批准另行形成新修订。
            提交打到 <span className="font-mono">{endpoint}</span>；租户不在表单上，由接入渠道的认证结果填入。
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
              <span className="block text-[11px] text-idpxyz-textMuted mt-1">
                {known
                  ? `已取回关系册里有这一关系，最新修订 r${known.revision}；本次登记是它的新修订。`
                  : '已取回关系册里没有这一标识；本次登记是首笔修订（列表可能已陈旧，连续性仍由服务端判）。'}
              </span>
            </Field>

            <RevisionField
              path="relationships[0].revision"
              problems={problems}
              {...form.revisionField}
              note="建议值取已取回关系册里该关系的最新修订 + 1（不在册为 1）；只是省一次翻册，连续性仍由服务端按册面判。"
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
              emptyNote="参与方册今天为空；先在「参与方身份」册登记双方，或手填标识由服务端判。"
              readFace="参与方册"
              optionsNote="持有该角色的一方。候选不按状态过滤：双方届时是否已生效由服务端判。"
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
              emptyNote="参与方册今天为空；先在「参与方身份」册登记双方，或手填标识由服务端判。"
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
              <span className="block text-[11px] text-idpxyz-textMuted mt-1">
                封闭集；未选照送空串，由服务端点名。角色与双方是否匹配不在这里判。
              </span>
            </Field>

            <Field label="适用范围 *" path="relationships[0].scope" problems={problems}>
              <Input
                value={draft.scope}
                readOnly={locked}
                className="font-mono text-[13px]"
                placeholder="商业范围引用"
                onChange={(event) => patch({ scope: event.target.value })}
              />
            </Field>

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

      <SnapshotJsonDetails kind={kind} submit={form.submit} />
    </div>
  );
}

import { useEffect } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import { RegistrationAnswerNote } from '../../components/registration';
import { partyIdentityOutcomeLabels, type BusinessPartyRecord } from './api';
import { problemNote, registrationTitles } from './presentation';
import { Field } from './PublicationFormFields';
import { RevisionField, SnapshotJsonDetails, WallTimeField, useRegistrationForm } from './party-registration-fields';
import {
  businessPartyLocalProblems,
  businessPartyPayloadOf,
  emptyBusinessPartyDraft,
  suggestedBusinessPartyRevision,
  type BusinessPartyDraft,
} from './business-party-form';

/**
 * 业务参与方身份登记的逐字段表单（票 admin-web-group-legal-entities/10 第 1 条；ADR-0101 决定八自裁：格少、低频、
 * 无矩阵，直接逐字段表单，做法照票 02 的法人表单）。壳在 useRegistrationForm，这里只摆本册的格。
 *
 * **本组件不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）：标识与名称是否为空、修订号连不
 * 连续，一律原样送上去让服务端逐格答；本地只拦编码层（修订号编不进正整数、生效时刻换不成 RFC 3339），纯函数在
 * business-party-form.ts。**不带租户格**、**不裁首尾空白**，理由在那个文件头上。
 */
export interface BusinessPartyRegistrationFormProps {
  /** 页面已取回的参与方列表（给修订号建议用）；列表没取到传 null，建议一律为 1。 */
  knownParties: readonly BusinessPartyRecord[] | null;
  /** 登记册答 `REGISTERED` 时回调，页面借它重取列表让新行在「参与方身份」册立刻可见。 */
  onRegistered: () => void;
  /** 草稿有没有没提交的输入；页面据此决定离开登记视图前要不要先确认。卸载时报 false。 */
  onUnsavedChange?: (unsaved: boolean) => void;
}

const info = moduleInfoById['business-parties'];
const kind = 'business-party';

export function BusinessPartyRegistrationForm({
  knownParties,
  onRegistered,
  onUnsavedChange,
}: BusinessPartyRegistrationFormProps) {
  const form = useRegistrationForm<BusinessPartyDraft>({
    kind,
    empty: emptyBusinessPartyDraft,
    suggestion: (draft) => suggestedBusinessPartyRevision(knownParties, draft.partyId),
    localProblems: businessPartyLocalProblems,
    payloadOf: businessPartyPayloadOf,
    // 答 `REGISTERED`（PartyRegistryOutcome 里 PartyIdentityRegistered 的线上名）才重取列表。
    landedOutcome: 'REGISTERED',
    onLanded: onRegistered,
  });
  const { draft, patch, problems, locked, timeZone, unsaved } = form;
  useEffect(() => {
    onUnsavedChange?.(unsaved);
  }, [onUnsavedChange, unsaved]);
  useEffect(() => () => onUnsavedChange?.(false), [onUnsavedChange]);

  // 按原串找而不裁空白：载荷也不裁，" X" 送上去就是另一个身份，这里若裁了就会把它说成「已在册」。
  const known = knownParties?.find((row) => row.partyId === draft.partyId);

  return (
    <div className="flex-1 overflow-auto p-4 flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{registrationTitles[kind]}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          {/* 修订是否连续由服务端按册面判，表单只把格编对；租户由操作者身份给出，不在表单上。各格首尾空白原样送出——
              不替人合并两个只差空格的标识，所以要把这一点说给人听。 */}
          <p className="text-[12px] text-idpxyz-breadcrumb">
            {'一笔登记形成一个修订。首笔修订号从 1 起，之后必须连续；要更正，用下一个修订号再登一笔，原来的不会被覆盖。标识的首尾空格会原样提交：多一个空格就是另一个参与方。'}
          </p>

          <div className="grid grid-cols-2 gap-3">
            <Field label="参与方标识 *" path="businessParties[0].partyId" problems={problems}>
              <Input
                value={draft.partyId}
                readOnly={locked}
                className="font-mono text-[13px]"
                placeholder="如 SYN-PARTY-AGENT-07"
                onChange={(event) => patch({ partyId: event.target.value })}
              />
              <span className="block text-[12px] text-idpxyz-breadcrumb mt-1">
                {known
                  ? `已取回列表里有这一参与方，最新修订 r${known.revision}；本次登记是它的新修订。`
                  : '列表里还没有这个标识，这一笔是它的首笔修订；如果别人刚登记过，服务端会拒绝。'}
              </span>
            </Field>

            <RevisionField
              path="businessParties[0].revision"
              problems={problems}
              {...form.revisionField}
              note="建议值是列表里这个参与方最新的修订号 + 1（没有则为 1）；如果期间有人登记过，服务端会拒绝。"
            />

            <Field label="名称 *" path="businessParties[0].name" problems={problems}>
              <Input
                value={draft.name}
                readOnly={locked}
                className="text-[13px]"
                placeholder="参与方名称"
                onChange={(event) => patch({ name: event.target.value })}
              />
            </Field>

            <Field label="登记依据 *" path="businessParties[0].basis" problems={problems}>
              <Input
                value={draft.basis}
                readOnly={locked}
                className="font-mono text-[13px]"
                placeholder="登记依据引用"
                onChange={(event) => patch({ basis: event.target.value })}
              />
            </Field>

            <WallTimeField
              label="生效自 *"
              path="businessParties[0].effectiveFrom"
              problems={problems}
              value={draft.effectiveFrom}
              locked={locked}
              timeZone={timeZone}
              onChange={(effectiveFrom) => patch({ effectiveFrom })}
            />
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

import { useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import {
  RegistrationAnswerNote,
  RegistrationPanel,
  type RegistrationPanelState,
} from '../../components/registration';
import { currentDisplayTimeZone } from '../moment';
import {
  commercialRegistrationEndpoints,
  listBusinessParties,
  partyIdentityOutcomeLabels,
  registerCommercial,
  type BusinessPartyListResponseBody,
  type GroupLegalEntityRecord,
} from './api';
import {
  identityStatusLabels,
  labelOf,
  problemNote,
  registrationSnapshotHints,
  registrationTitles,
} from './presentation';
import { Field, ReferencePicker, type PickerOption } from './PublicationFormFields';
import {
  emptyLegalEntityDraft,
  legalEntityLocalProblems,
  legalEntityPayloadOf,
  suggestedRevision,
  type LegalEntityDraft,
} from './legal-entity-form';

/**
 * 责任法人身份登记的逐字段表单（票 admin-web-group-legal-entities/02；ADR-0101 决定八自裁：五格、低频、无矩阵，
 * 直接逐字段表单，不走「模板导入 → 草稿 → 批准 → 发布」那条为上百格矩阵设计的路）。
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
const endpoint = `POST ${commercialRegistrationEndpoints['legal-entity']}`;

// 候选显名称 + 标识 + 状态、不按状态过滤——表单不裁，届时是否已生效由服务端判；状态摆出来只是让人看。
function partyOptionsOf(body: BusinessPartyListResponseBody): PickerOption[] {
  return body.parties.map((party) => ({
    value: party.partyId,
    label: `${party.partyName} · ${party.partyId} · ${labelOf(identityStatusLabels, party.status)}`,
  }));
}

export function LegalEntityRegistrationForm({ knownEntities, onRegistered }: LegalEntityRegistrationFormProps) {
  const [draft, setDraft] = useState<LegalEntityDraft>(emptyLegalEntityDraft());
  // 修订号建议随法人标识变；操作者一改过就不再跟着建议走，直到点「用建议值」复位。
  const [revisionEdited, setRevisionEdited] = useState(false);
  const [state, setState] = useState<RegistrationPanelState>({ kind: 'idle' });

  const timeZone = currentDisplayTimeZone();
  const suggestion = suggestedRevision(knownEntities, draft.legalEntityId);
  const effectiveDraft: LegalEntityDraft = revisionEdited ? draft : { ...draft, revision: String(suggestion) };
  const problems = legalEntityLocalProblems(effectiveDraft, timeZone);
  const locked = state.kind === 'submitting';
  const patch = (change: Partial<LegalEntityDraft>) => setDraft((current) => ({ ...current, ...change }));

  // 两条路（逐字段 / JSON 镜像）共用同一次提交：答 `REGISTERED`（PartyRegistryOutcome 里 PartyIdentityRegistered 的
  // 线上名）才重取列表；重放、冲突、未受理都没写进去，重取只会让人以为写进去了。
  const submit = (snapshot: unknown) =>
    registerCommercial('legal-entity', snapshot).then((answer) => {
      if (answer.kind === 'outcome' && answer.body.outcome === 'REGISTERED') onRegistered();
      return answer;
    });

  const send = () => {
    if (Object.keys(problems).length > 0) return;
    setState({ kind: 'submitting' });
    void submit(legalEntityPayloadOf(effectiveDraft, timeZone)).then((answer) => setState({ kind: 'answered', answer }));
  };

  const known = knownEntities?.find((row) => row.legalEntityId === draft.legalEntityId.trim());

  return (
    <div className="flex-1 overflow-auto p-4 flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{registrationTitles['legal-entity']}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <p className="text-xs text-idpxyz-textMuted">
            一笔登记一个修订。首笔修订从 1 起、此后必须连续；更正占下一个修订号翻旧插新，不覆盖。参与方必须已在册且在
            法人生效时点已生效——这些都由服务端按册面判，表单只负责把格编对。提交打到{' '}
            <span className="font-mono">{endpoint}</span>；租户不在表单上，由接入渠道的认证结果填入。
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

            <Field label="修订号 *" path="legalEntities[0].revision" problems={problems}>
              <div className="flex items-center gap-2">
                <Input
                  value={effectiveDraft.revision}
                  readOnly={locked}
                  inputMode="numeric"
                  className="font-mono text-[13px]"
                  onChange={(event) => {
                    setRevisionEdited(true);
                    patch({ revision: event.target.value });
                  }}
                />
                {revisionEdited ? (
                  <Button variant="outline" size="sm" disabled={locked} onClick={() => setRevisionEdited(false)}>
                    用建议值 {suggestion}
                  </Button>
                ) : null}
              </div>
              <span className="block text-[11px] text-idpxyz-textMuted mt-1">
                建议值取已取回列表里该法人的最新修订 + 1（不在册为 1）；只是省一次翻册，连续性仍由服务端按册面判。
              </span>
            </Field>

            <ReferencePicker<BusinessPartyListResponseBody>
              label="业务参与方身份 *"
              path="legalEntities[0].partyId"
              problems={problems}
              value={draft.partyId}
              locked={locked}
              onChange={(partyId) => patch({ partyId })}
              load={listBusinessParties}
              optionsOf={partyOptionsOf}
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

            <Field label="生效自（留空由服务端点名）" path="legalEntities[0].effectiveFrom" problems={problems}>
              <Input
                type="datetime-local"
                step={1}
                value={draft.effectiveFrom}
                readOnly={locked}
                className="font-mono text-[13px]"
                onChange={(event) => patch({ effectiveFrom: event.target.value })}
              />
              <span className="block text-[11px] text-idpxyz-textMuted mt-1">
                按操作者本地时刻（{timeZone}）填，送出时换成 RFC 3339 UTC。
              </span>
            </Field>
          </div>

          <div className="flex items-start gap-3">
            <Button onClick={send} disabled={locked || Object.keys(problems).length > 0}>
              {locked ? '提交中…' : '提交登记'}
            </Button>
            <RegistrationAnswerNote
              state={state}
              owner={info.owner}
              outcomeLabels={partyIdentityOutcomeLabels}
              problemNote={problemNote}
            />
          </div>
        </CardContent>
      </Card>

      {/* 受控批量口的在线镜像，折起来放底部：主路径是上面那五格（ADR-0101 决定一原句）。 */}
      <details className="rounded border border-idpxyz-border">
        <summary className="cursor-pointer select-none px-4 py-2 text-[13px] text-idpxyz-textMuted">
          高级：粘贴登记快照 JSON（受控批量口 parcel-commercial register-parties 的在线镜像）
        </summary>
        <RegistrationPanel
          moduleId="group-legal-entities"
          title={registrationTitles['legal-entity']}
          endpoint={endpoint}
          snapshotHint={registrationSnapshotHints['legal-entity']}
          submit={submit}
          outcomeLabels={partyIdentityOutcomeLabels}
          problemNote={problemNote}
        />
      </details>
    </div>
  );
}

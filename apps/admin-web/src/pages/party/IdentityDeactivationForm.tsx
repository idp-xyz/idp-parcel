import { useEffect, useState } from 'react';
import { Button, Card, CardContent, CardHeader, CardTitle, Input } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { RegistrationAnswerNote } from '../../components/registration';
import type { ApiResult } from '../catalogue-api';
import {
  commercialRegistrationEndpoints,
  listBusinessParties,
  listCustomerAccounts,
  listGroupLegalEntities,
  partyIdentityOutcomeLabels,
  type BusinessPartyListResponseBody,
  type CustomerAccountListResponseBody,
  type GroupLegalEntityListResponseBody,
} from './api';
import { problemNote, registrationTitles, type IdentityKind } from './presentation';
import { Field, ReferencePickerFor, selectClass, type PickerOption } from './PublicationFormFields';
import {
  RevisionField,
  SnapshotJsonDetails,
  WallTimeField,
  businessPartyPickerOptions,
  customerAccountPickerOptions,
  legalEntityPickerOptions,
  useRegistrationForm,
} from './party-registration-fields';
import {
  emptyIdentityDeactivationDraft,
  identityDeactivationLocalProblems,
  identityDeactivationPayloadOf,
  identityKindOptions,
  identityTargetOf,
  isIdentityKind,
  suggestedDeactivationRevision,
  type IdentityDeactivationDraft,
  type RevisionedIdentity,
} from './identity-deactivation-form';

/**
 * 身份停用的逐字段表单（票 admin-web-group-legal-entities/10 第 3 条）。停用口一个命令带种类，法人与客户账户的停用
 * 也走本签（页面上登记签那段头注说明为什么它摆在本页）；结果分别显示在集团与法人页、客户与合同页的
 * 「客户账户」签，本页读得见的是参与方身份那一册。壳在 useRegistrationForm，这里只摆本册的格。
 *
 * **本组件不算摘要、不裁任何门、不判领域规则**（伞票 admin-write-faces/07 硬句）：身份在不在册、已否停用、修订是否
 * 错位，一律送上去让服务端答；本地只拦编码层（修订号、停用时刻），纯函数在 identity-deactivation-form.ts。
 *
 * 标识按种类从对应的册上选，候选与修订建议取的是**同一份答案**（票 13 第 5 条）：参与方册用页面持有的那份，不另读；
 * 法人册与客户账户册在选中那一种类时才读、读一次，Picker 收这份答案（ReferencePickerFor）而不自己再读——此前一挂
 * 即读两册、Picker 又各自读一遍，同一册在一张表单里被读两三遍。种类 → 读口 / 候选 / 读面名 / 投影合成一张按
 * IdentityKind 键的表（第 2 条），三分支 JSX 与 switch 收掉；词表多一格，表编不过。
 */
export interface IdentityDeactivationFormProps {
  /** 页面持有的参与方列表答案（种类为业务参与方时给候选与修订号建议用）；首取回来之前为 null。 */
  parties: ApiResult<BusinessPartyListResponseBody> | null;
  /** 登记册答 `DEACTIVATED` 时回调，页面借它重取参与方列表让「已停用」+ 时点 + 依据立刻可见。 */
  onDeactivated: () => void;
}

const info = moduleInfoById['business-parties'];
const kind = 'identity-deactivation';
const endpoint = `POST ${commercialRegistrationEndpoints[kind]}`;
const kindOptions = identityKindOptions();

const pickerNotes = {
  emptyNote: '对应册今天为空；先登记身份，或手填标识由服务端判。',
  optionsNote: '候选不按状态过滤：已停用的再停一次由服务端答，表单不拦。',
} as const;

/**
 * 一种身份对应的那一册：读口、候选转写、读面名、投成「标识 + 修订」。答案体的形状只有本行知道，表外一律按
 * unknown 传——三册体形各异，表要键在同一个 IdentityKind 上就得在这里擦掉。
 */
interface KindRegister {
  readFace: string;
  load(): Promise<ApiResult<unknown>>;
  optionsOf(body: unknown): PickerOption[];
  targetsOf(body: unknown): readonly RevisionedIdentity[];
}

function kindRegister<Body>(entry: {
  readFace: string;
  load(): Promise<ApiResult<Body>>;
  optionsOf(body: Body): PickerOption[];
  targetsOf(body: Body): readonly RevisionedIdentity[];
}): KindRegister {
  return entry;
}

/** 参与方册那一行的 load 不会被调到：页面已持有那份答案传进来（props.parties），再读一次就是第 5 条要收的那次。 */
const kindRegisters: Record<IdentityKind, KindRegister> = {
  BUSINESS_PARTY: kindRegister<BusinessPartyListResponseBody>({
    readFace: '参与方册',
    load: listBusinessParties,
    optionsOf: businessPartyPickerOptions,
    targetsOf: (body) => body.parties.map(identityTargetOf.BUSINESS_PARTY),
  }),
  LEGAL_ENTITY: kindRegister<GroupLegalEntityListResponseBody>({
    readFace: '法人册',
    load: listGroupLegalEntities,
    optionsOf: legalEntityPickerOptions,
    targetsOf: (body) => body.entities.map(identityTargetOf.LEGAL_ENTITY),
  }),
  CUSTOMER_ACCOUNT: kindRegister<CustomerAccountListResponseBody>({
    readFace: '客户账户册',
    load: listCustomerAccounts,
    optionsOf: customerAccountPickerOptions,
    targetsOf: (body) => body.accounts.map(identityTargetOf.CUSTOMER_ACCOUNT),
  }),
};

export function IdentityDeactivationForm({ parties, onDeactivated }: IdentityDeactivationFormProps) {
  // 当前选中种类那一册的答案（参与方册除外——它从页面来）。换种类即换册，上一册的答案不留：与「换种类连标识一起清」
  // 同一条纪律，上一册的候选留在下拉里会被送到另一册去查。
  const [loaded, setLoaded] = useState<ApiResult<unknown> | null>(null);
  const answerFor = (selected: IdentityKind): ApiResult<unknown> | null =>
    selected === 'BUSINESS_PARTY' ? parties : loaded;
  const targetsFor = (selected: string): readonly RevisionedIdentity[] | null => {
    if (!isIdentityKind(selected)) return null;
    const answer = answerFor(selected);
    return answer?.kind === 'outcome' ? kindRegisters[selected].targetsOf(answer.body) : null;
  };

  const form = useRegistrationForm<IdentityDeactivationDraft>({
    kind,
    empty: emptyIdentityDeactivationDraft,
    suggestion: (draft) => suggestedDeactivationRevision(targetsFor(draft.kind), draft.id),
    localProblems: identityDeactivationLocalProblems,
    payloadOf: identityDeactivationPayloadOf,
    // 答 `DEACTIVATED`（PartyRegistryOutcome 里 PartyIdentityDeactivated 的线上名）才重取；`册上没有这一个身份`与
    // 修订错位都没写进去。JSON 镜像那条路上分不出种类（快照是未译的 JSON），一律重取参与方列表：多取一次是一个 GET，
    // 为分种类去解一份 unknown 不值。
    landedOutcome: 'DEACTIVATED',
    onLanded: () => onDeactivated(),
  });
  const { draft, patch, problems, locked, timeZone } = form;
  const selectedKind = isIdentityKind(draft.kind) ? draft.kind : null;

  useEffect(() => {
    // 参与方册不在这里读：页面持有的那份答案就是它。其余两册在选中时读一次；未回的旧请求按 cancelled 丢。
    setLoaded(null);
    if (selectedKind === null || selectedKind === 'BUSINESS_PARTY') return;
    let cancelled = false;
    void kindRegisters[selectedKind].load().then((next) => {
      if (!cancelled) setLoaded(next);
    });
    return () => {
      cancelled = true;
    };
  }, [selectedKind]);

  const known = targetsFor(draft.kind)?.find((row) => row.id === draft.id);
  const pickerProps = {
    label: '身份标识 *',
    path: 'deactivations[0].id',
    problems,
    value: draft.id,
    locked,
    onChange: (id: string) => patch({ id }),
    ...pickerNotes,
  };

  return (
    <div className="flex-1 overflow-auto p-4 flex flex-col gap-4">
      <Card>
        <CardHeader>
          <CardTitle>{registrationTitles[kind]}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4">
          <p className="text-xs text-idpxyz-textMuted">
            停用形成修订链上新的一笔，原修订不被改写；自停用时点起该身份不再支持新的商业决定。修订号是你声明自己看到的
            册面（册上最新 + 1），错位说明册面已被并发推进或意图已陈旧，由服务端拒而不是替你猜。关系不在这里停：关系的
            终止走撤销 / 到期 / 替代。提交打到 <span className="font-mono">{endpoint}</span>；租户不在表单上。
          </p>

          <div className="grid grid-cols-2 gap-3">
            <Field label="身份种类 *" path="deactivations[0].kind" problems={problems}>
              <select
                className={selectClass}
                value={draft.kind}
                disabled={locked}
                // 换种类连标识一起清：上一册的标识留在格里，会被送到另一册去查。
                onChange={(event) => patch({ kind: event.target.value, id: '' })}
              >
                <option value="">未选</option>
                {kindOptions.map((option) => (
                  <option key={option.value} value={option.value}>
                    {option.label}
                  </option>
                ))}
              </select>
              <span className="block text-[11px] text-idpxyz-textMuted mt-1">
                法人与客户账户的停用也走本签；结果分别显示在集团与法人页、客户与合同页的「客户账户」签。
              </span>
            </Field>

            {selectedKind !== null ? (
              // key 带种类：换种类即换 Picker，上一册的候选与「不在读面上」那一项不留在下拉里。
              <ReferencePickerFor<unknown>
                key={selectedKind}
                {...pickerProps}
                answer={answerFor(selectedKind)}
                optionsOf={kindRegisters[selectedKind].optionsOf}
                readFace={kindRegisters[selectedKind].readFace}
              />
            ) : (
              <Field label="身份标识 *" path="deactivations[0].id" problems={problems}>
                <Input
                  value={draft.id}
                  readOnly={locked}
                  className="font-mono text-[13px]"
                  placeholder="先选身份种类，再从对应册上选"
                  onChange={(event) => patch({ id: event.target.value })}
                />
              </Field>
            )}

            <RevisionField
              path="deactivations[0].revision"
              problems={problems}
              {...form.revisionField}
              note={
                known
                  ? `对应册里该身份最新修订 r${known.revision}，建议停用落点 r${known.revision + 1}；册面可能已陈旧，错位由服务端判。`
                  : '对应册里没有这一标识（或册没取到），建议为 1；停用一个不在册的身份会被服务端答「册上没有这一个身份」。'
              }
            />

            <Field label="停用依据 *" path="deactivations[0].basis" problems={problems}>
              <Input
                value={draft.basis}
                readOnly={locked}
                className="font-mono text-[13px]"
                placeholder="停用依据引用"
                onChange={(event) => patch({ basis: event.target.value })}
              />
            </Field>

            <WallTimeField
              label="停用时点 *"
              path="deactivations[0].at"
              problems={problems}
              value={draft.at}
              locked={locked}
              timeZone={timeZone}
              onChange={(at) => patch({ at })}
            />
          </div>

          <div className="flex items-start gap-3">
            <Button onClick={form.send} disabled={locked || !form.canSend}>
              {locked ? '提交中…' : '提交停用'}
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

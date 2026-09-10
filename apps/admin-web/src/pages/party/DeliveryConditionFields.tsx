import { Input } from '@idpxyz/ui-primitives';
import type { PublicationFormContext } from './PublicationDraftFlow';
import { Field, Problems, selectClass } from './PublicationFormFields';
import type { DeliveryConditionDraft, DeliveryConditionLayer } from './delivery-condition-section';

/**
 * 交付条件一节的字段组（票 admin-write-faces/25；ADR-0133 决定四），产品版本表单与客户合同表单各挂一节、共用本组件：
 * 产品层三格（方式集合 / 收件范围规则引用 / 交付证明规则引用），合同层多 tightens 两格（所收紧的服务产品版本）。纯逻辑与
 * 路径表在 delivery-condition-section.ts；本组件只摆，改这里的 path 要同步改那边的 deliveryConditionRenderedPaths。
 *
 * **一节可缺、不预开、不预选、不给候选**：一格都没填即这一版不声明交付条件，整节不进载荷；方式是开放引用（PAR-NET-09），
 * 多行文本一行一项、不内置任何一种方式的下拉——给一份词表就是替租户封闭一个租户自己的集合（pc-gaps/11 裁开放引用）。
 * **表单不裁任何门**：零方式、同方式两行、合同层缺 tightens、「只能收紧」一律由服务端答；「只能收紧」在服务端写口核，
 * 预览与批准那两步看不出放宽，发布那一步才拒——组件在提示句里把这一点说给操作者。
 */
export function DeliveryConditionFields({
  root,
  layer,
  draft,
  form,
  onPatch,
}: {
  /** 这一节在载荷里的根（serviceProduct.deliveryConditions / customerContract.deliveryConditions）。 */
  root: string;
  layer: DeliveryConditionLayer;
  draft: DeliveryConditionDraft;
  form: PublicationFormContext;
  onPatch: (change: Partial<DeliveryConditionDraft>) => void;
}) {
  // 服务端对方式某一项的话落在 methods[i]；方式在这里是一块多行文本，逐项的话汇到文本框下、带上项号（照客户服务规则表单的材料清单）。
  const methodLines = Object.entries(form.problems)
    .filter(([path]) => path.startsWith(`${root}.methods[`))
    .flatMap(([path, lines]) => lines.map((line) => `${path.slice(root.length + 1)}：${line}`));

  return (
    <section className="flex flex-col gap-2">
      <h3 className="text-[13px] font-medium text-idpxyz-text">
        交付条件（可缺）{layer === 'contract' ? '——合同层只能在所收紧的产品层之内收紧' : '——产品层'}
      </h3>
      <p className="text-[11px] text-idpxyz-textMuted">
        一格都不填就是这一版不声明交付条件；填了任一格整节都送上去。三格都是引用串、手填：交付方式一行一项（词表属实例登记，
        表单不给候选），收件范围规则与交付证明规则各填一条引用。零方式、同方式两行由服务端答。
        {layer === 'contract'
          ? '合同层必须指名所收紧的服务产品版本（对象标识 + 版本号，手填——壳上的产品引用没有版本号）；方式是否真在那一版产品层之内由服务端在发布那一步核，预览与批准看不出放宽。'
          : ''}
      </p>
      <Problems lines={form.problems[root]} />
      {layer === 'contract' ? (
        <div className="grid grid-cols-2 gap-3">
          <Field
            label="所收紧的服务产品：对象标识 *"
            path={`${root}.tightens.objectId`}
            alsoPaths={[`${root}.tightens`]}
            problems={form.problems}
          >
            <Input
              value={draft.tightensObjectId}
              readOnly={form.locked}
              className="font-mono text-[13px]"
              placeholder="服务产品的对象标识"
              onChange={(event) => onPatch({ tightensObjectId: event.target.value })}
            />
          </Field>
          <Field label="所收紧的服务产品：版本号 *" path={`${root}.tightens.version`} problems={form.problems}>
            <Input
              value={draft.tightensVersion}
              readOnly={form.locked}
              className="font-mono text-[13px]"
              placeholder="那一版产品层声明所在的版本号"
              onChange={(event) => onPatch({ tightensVersion: event.target.value })}
            />
          </Field>
        </div>
      ) : null}
      <Field label="允许的交付方式 *（一行一项）" path={`${root}.methods[i]`} problems={form.problems} as="div">
        <textarea
          className={`${selectClass} font-mono min-h-[72px]`}
          value={draft.methodsText}
          disabled={form.locked}
          placeholder={'交付方式引用串，一行一项；空行不算项'}
          onChange={(event) => onPatch({ methodsText: event.target.value })}
        />
        {methodLines.map((line) => (
          <span key={line} className="block text-[11px] text-idpxyz-danger mt-1">
            {line}
          </span>
        ))}
      </Field>
      <div className="grid grid-cols-2 gap-3">
        <Field label="收件范围规则引用 *" path={`${root}.recipientScopeRule`} problems={form.problems}>
          <Input
            value={draft.recipientScopeRule}
            readOnly={form.locked}
            className="font-mono text-[13px]"
            placeholder="谁可以收件的规则引用"
            onChange={(event) => onPatch({ recipientScopeRule: event.target.value })}
          />
        </Field>
        <Field label="交付证明规则引用 *" path={`${root}.proofOfDeliveryRule`} problems={form.problems}>
          <Input
            value={draft.proofOfDeliveryRule}
            readOnly={form.locked}
            className="font-mono text-[13px]"
            placeholder="凭什么算有效交付的规则引用"
            onChange={(event) => onPatch({ proofOfDeliveryRule: event.target.value })}
          />
        </Field>
      </div>
    </section>
  );
}

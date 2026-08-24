import { UnconfiguredState } from '../../components/states';
import { Section } from './controls';

// UC-PS-006 接受后取消入口:取消已接受委托中的明确包裹,或协调收寄后服务处置。
//
// 后端应用编排已在 internal/parcelshipment/application 就位(封闭结果集见
// CancelParcelOutcome),但 adapters/http 尚无该用例的端点形状,接入契约属
// PAR-INT-03/07 待登记。本页因此只立入口与边界说明,动作区如实呈现「未配置」:
// 不造请求形状、不发请求——端点形状落地后照 withdraw 页模式对形状实现
// (api.ts 收编端点、presentation 收编结果词表、表单只收客户可声明部分)。
//
// 边界文案的出处:parcel-shipment CONTEXT.md「包裹取消决定」「收寄后服务处置决定」
// 与 UC-PS-006 决定矩阵。措辞守住该用例的「本用例不执行」:不把客户消息自动解释
// 为取消,不提供把已收寄包裹改回「已取消」的口子。
export function CancelParcelPage() {
  return (
    <div className="flex-1 overflow-y-auto">
      <div className="max-w-[720px] mx-auto px-6 py-6 space-y-4">
        <header>
          <h1 className="text-[18px] font-bold text-idpxyz-textBright">
            取消包裹或协调收寄后服务处置
          </h1>
          <p className="text-[12px] leading-5 text-idpxyz-textMuted mt-1">
            对已接受委托中的明确包裹逐件裁决(UC-PS-006):尚未越过适用取消边界时形成
            包裹取消决定;已越过边界时明确拒绝回退取消,转收寄后服务处置。仍为
            「已提交」的整份委托撤回走「决定前撤回」,不在本页。
          </p>
        </header>

        <Section title="入口边界">
          <ul className="list-disc pl-5 space-y-1.5 text-[12px] leading-5 text-idpxyz-textMuted">
            <li>
              取消权按包裹判断,批量请求允许部分成功;一个包裹的结果不回滚其他包裹
              已合法形成的决定。
            </li>
            <li>
              网络服务包裹只允许在有效网络收寄结果形成前取消;有效网络收寄已先行成立时
              不回退为「已取消」,只能形成收寄后服务处置决定、保持未决或明确拒绝。
            </li>
            <li>
              取消保留包裹身份、接受基线、请求、授权与既有交易或作业历史;已有面单交易时,
              包裹取消不能代替渠道作废或渠道退款结果。
            </li>
            <li>
              客户消息、异常信号、拒收、无路由或运输停止本身不自动生成取消、退运或
              服务终止;处置决定必须由有权主体明确形成。
            </li>
          </ul>
        </Section>

        {/* 动作区占位:端点形状落地后,这一格换成对形状的取消请求表单与结果面板。 */}
        <div className="rounded border border-idpxyz-border py-10 flex justify-center">
          <UnconfiguredState
            title="接受后取消端点尚未建立"
            description="该用例的接入端点形状与接入契约(PAR-INT-03/07)待登记,本入口暂不受理请求:页面不虚构请求字段,待端点形状落地后按真实形状接线。"
          />
        </div>
      </div>
    </div>
  );
}

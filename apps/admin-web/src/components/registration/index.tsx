// 登记写面的共享表单区（ADR-0085）。命名与 props 是跨页契约：各登记册页的「登记」签
// 按名导入，改名或收紧必填都会拆到消费方（纪律同 components/states）。
export {
  RegistrationPanel,
  type RegistrationPanelProps,
  type RegistrationResponseBody,
} from './RegistrationPanel';
export {
  MultiRegistrationPanel,
  type MultiRegistrationPanelProps,
  type RegistrationTarget,
} from './MultiRegistrationPanel';

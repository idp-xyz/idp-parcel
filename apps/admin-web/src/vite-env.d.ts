/// <reference types="vite/client" />

interface ImportMetaEnv {
  /** 管理台呈现时区（IANA 名）；未配置按浏览器时区。 */
  readonly VITE_DISPLAY_TIME_ZONE?: string;
}

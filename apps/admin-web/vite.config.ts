import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';

// 开发代理到 parcel-api（cmd/parcel-api 默认监听 :8080，后端用
// IDP_PARCEL_HTTP_ADDR 改监听、前端用 PARCEL_API_TARGET 改目标）。
// 路径约定：前端一律带 /api 前缀发起（装配侧 configureShipmentRequestApi
// 传 basePrefix='/api'），代理剥掉前缀转发——parcel-api 的路由本身不带
// /api（如 POST /shipment-requests），前缀只属前端侧的转发约定，
// 这样单条代理规则即可覆盖全部端点，不必逐路径枚举。
const apiTarget = process.env.PARCEL_API_TARGET || 'http://localhost:8080';

export default defineConfig({
  plugins: [react()],
  build: {
    rollupOptions: {
      output: {
        // 按变更频率拆 vendor 块：react 与组件库几乎不随业务页变，拆开后
        // 业务改动只失效 app 块的缓存，也让单块体积回到告警线内。
        // 页面本身不做路由级懒加载——重量在依赖不在页面，拆页面只添加载闪烁。
        manualChunks(id: string) {
          if (!id.includes('node_modules')) return undefined;
          if (/[\\/]node_modules[\\/](react|react-dom|scheduler)[\\/]/.test(id)) {
            return 'react-vendor';
          }
          if (id.includes('@idpxyz')) return 'ui-kit';
          return 'vendor';
        },
      },
    },
  },
  server: {
    proxy: {
      '/api': {
        target: apiTarget,
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/api/, ''),
      },
    },
  },
});

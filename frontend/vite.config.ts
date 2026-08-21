import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'path';

const proxyTarget = process.env.VITE_PROXY_TARGET || 'http://127.0.0.1:8080';

export default defineConfig({
  plugins: [react()],
  envDir: path.resolve(__dirname, '..'),
  server: {
    host: '0.0.0.0',
    port: 5173,
    proxy: {
      '/api': {
        target: proxyTarget,
        changeOrigin: true,
      },
      '/webhooks': {
        target: proxyTarget,
        changeOrigin: true,
      },
      '/internal': {
        target: proxyTarget,
        changeOrigin: true,
      },
    },
  },
});

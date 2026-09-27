import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

const bff = 'http://127.0.0.1:8400';
const apiProxy = {
  '/advisor-radar/v1': {
    target: bff,
    changeOrigin: true,
    rewrite: (path: string) => path.replace(/^\/advisor-radar/, ''),
  },
};

export default defineConfig({
  base: '/advisor-radar/',
  plugins: [react()],
  server: {
    host: '0.0.0.0',
    port: 3400,
    strictPort: true,
    fs: { allow: ['..'] },
    proxy: apiProxy,
  },
  preview: {
    host: '0.0.0.0',
    port: 3400,
    strictPort: true,
    proxy: apiProxy,
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.{ts,tsx}'],
    setupFiles: ['src/test/setup.ts'],
  },
});

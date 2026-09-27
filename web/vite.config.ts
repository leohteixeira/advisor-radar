import react from '@vitejs/plugin-react';
import { defineConfig } from 'vitest/config';

const bff = 'http://127.0.0.1:8400';

export default defineConfig({
  plugins: [react()],
  server: {
    host: '0.0.0.0',
    port: 3400,
    strictPort: true,
    fs: { allow: ['..'] },
    proxy: {
      '/v1': { target: bff, changeOrigin: true },
    },
  },
  preview: {
    host: '0.0.0.0',
    port: 3400,
    strictPort: true,
    proxy: {
      '/v1': { target: bff, changeOrigin: true },
    },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.{ts,tsx}'],
    setupFiles: ['src/test/setup.ts'],
  },
});

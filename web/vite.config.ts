import { defineConfig } from 'vite'
import { svelte } from '@sveltejs/vite-plugin-svelte'

// Сборка кладётся туда, откуда её вшивает Go (internal/webui/dist).
// В dev-режиме API проксируется на локальный `questaround serve`.
export default defineConfig({
  plugins: [svelte()],
  build: {
    outDir: '../internal/webui/dist/app',
    emptyOutDir: true,
    assetsDir: 'assets',
    target: 'es2022',
  },
  server: {
    proxy: {
      '/api': { target: 'http://localhost:8080', changeOrigin: false },
      '/admin/v1': { target: 'http://localhost:8080', changeOrigin: false },
    },
  },
})

import { defineConfig } from 'vitest/config'

// Standalone vitest config (when this file exists, vitest ignores
// vite.config.ts): the tests are pure TypeScript against the local backend —
// storage is injected, so no DOM environment and no svelte plugin are needed.
export default defineConfig({
  test: {
    include: ['src/**/*.test.ts'],
    environment: 'node',
  },
})

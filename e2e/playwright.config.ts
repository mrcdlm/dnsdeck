import { defineConfig, devices } from '@playwright/test'

// Browser-Tests gegen einen echten dnsdeck-Server und die nachgebaute
// Cloudflare-API (cmd/cfmock). Beide werden von Playwright gestartet.
// Alle Tests teilen sich einen Server → nacheinander ausführen.
export default defineConfig({
  testDir: './tests',
  fullyParallel: false,
  workers: 1,
  timeout: 60_000,
  expect: { timeout: 7_000 },
  retries: process.env.CI ? 1 : 0,
  reporter: process.env.CI ? [['list'], ['html', { open: 'never' }]] : 'list',
  use: {
    baseURL: 'http://localhost:18080',
    ...devices['Desktop Chrome'],
    viewport: { width: 1280, height: 900 },
    permissions: ['clipboard-read', 'clipboard-write'],
    trace: 'retain-on-failure',
    screenshot: 'only-on-failure',
  },
  webServer: [
    {
      command:
        'go run ../cmd/cfmock -addr :8787 -token dev -zones example.com,example.org ' +
        '-account dev-account -tunnels home:healthy,nas:degraded,backup:down',
      url: 'http://localhost:8787/_records',
      reuseExistingServer: false,
      timeout: 120_000,
    },
    {
      command: 'sh ./start-server.sh',
      url: 'http://localhost:18080/healthz',
      reuseExistingServer: false,
      timeout: 300_000,
      stdout: 'pipe',
    },
  ],
})

import { test, expect } from '@playwright/test';

test('opens node logs dialog and renders live plus history sections', async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem('auth_token', btoa('admin:admin'));
    window.__originalFetch = window.fetch.bind(window);
    window.fetch = async (url, options) => {
      if (typeof url === 'string' && url.includes('/logs/stream')) {
        const encoder = new TextEncoder();
        return new Response(new ReadableStream({
          start(controller) {
            window.__pushStreamChunk = (chunk) => controller.enqueue(encoder.encode(chunk));
          }
        }), {
          status: 200,
          headers: { 'Content-Type': 'text/event-stream' }
        });
      }
      return window.__originalFetch(url, options);
    };
  });

  await page.route('**/api/v1/nodes', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        data: [{
          id: 'node-1',
          display_name: 'Tokyo',
          hostname: 'tokyo',
          ip: '1.1.1.1',
          online_state: 'online',
          version: 'v2',
          max_groups: 50,
          last_heartbeat_at: '2026-04-01T00:00:00Z'
        }],
      }),
    });
  });

  await page.route('**/api/v1/node-enrollments', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ data: [] }),
    });
  });

  await page.goto('/nodes');
  await page.getByTestId('open-node-logs-node-1').click();
  await page.getByTestId('start-node-logs').click();

  await page.evaluate(() => {
    window.__pushStreamChunk('event: status\ndata: {"type":"status","content":"live_connected","timestamp":"2026-04-01T12:00:00Z"}\n\n');
    window.__pushStreamChunk('event: history\ndata: {"type":"history","content":"old line","timestamp":"2026-04-01T11:59:00Z"}\n\n');
    window.__pushStreamChunk('event: live\ndata: {"type":"live","content":"new line","timestamp":"2026-04-01T12:00:01Z"}\n\n');
  });

  await expect(page.getByTestId('node-logs-status')).toContainText('实时流已连接');
  await expect(page.getByTestId('node-logs-history')).toContainText('old line');
  await expect(page.getByTestId('node-logs-live')).toContainText('new line');
});

import { test, expect } from '@playwright/test';

test('browser viewer appends VNC password and autoconnect params to iframe url', async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem('auth_token', btoa('admin:admin'));
  });

  await page.route('**/api/v1/browsers', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        data: [{
          id: 'browser-1',
          display_name: 'Browser 1',
          node_id: 'node-1',
          status: 'running',
          vnc_port: 1025,
        }],
      }),
    });
  });

  await page.route('**/api/v1/nodes', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: [] }) });
  });

  await page.route('**/api/v1/proxies', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: [] }) });
  });

  await page.route('**/api/v1/accounts', async route => {
    await route.fulfill({ status: 200, contentType: 'application/json', body: JSON.stringify({ data: [] }) });
  });

  await page.route('**/api/v1/browsers/browser-1/vnc', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        vnc_url: '/vnc/192.227.229.63/1025/?path=vnc%2F192.227.229.63%2F1025%2Fwebsockify',
        vnc_password: 'changeme',
      }),
    });
  });

  await page.goto('/browsers');
  await page.getByRole('button', { name: '查看' }).click();

  const iframe = page.locator('iframe');
  await expect(iframe).toHaveAttribute('src', /password=changeme/);
  await expect(iframe).toHaveAttribute('src', /autoconnect=1/);
  await expect(iframe).toHaveAttribute('src', /path=vnc%2F192\.227\.229\.63%2F1025%2Fwebsockify/);
});

import { test, expect } from '@playwright/test';

test('creates a node enrollment and shows the install command', async ({ page }) => {
  await page.addInitScript(() => {
    localStorage.setItem('auth_token', btoa('admin:admin'));
  });

  await page.route('**/api/v1/nodes', async route => {
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ data: [] }),
    });
  });

  await page.route('**/api/v1/node-enrollments', async route => {
    if (route.request().method() === 'GET') {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ data: [] }),
      });
      return;
    }

    const payload = JSON.parse(route.request().postData() || '{}');
    if (payload.display_name !== 'tokyo-01') {
      throw new Error(`unexpected payload: ${JSON.stringify(payload)}`);
    }

    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({
        data: {
          id: 'enroll-1',
          expires_at: '2026-04-02T12:00:00Z',
          install_command: "curl -fsSL 'https://manager.example.com/api/v1/node-enrollments/enroll-1/install.sh?token=plain-token' | sudo bash",
        },
      }),
    });
  });

  await page.goto('/nodes');
  await page.getByTestId('open-node-enrollment').click();
  await page.locator('[data-testid="node-enrollment-display-name"] input').fill('tokyo-01');
  await page.locator('[data-testid="node-enrollment-http"] input').fill('https://manager.example.com');
  await page.locator('[data-testid="node-enrollment-grpc"] input').fill('manager.example.com:50051');
  await page.getByTestId('submit-node-enrollment').click();

  await expect(page.getByTestId('node-enrollment-command')).toContainText('/api/v1/node-enrollments/enroll-1/install.sh?token=plain-token');
  await expect(page.getByText('2026-04-02')).toBeVisible();
});

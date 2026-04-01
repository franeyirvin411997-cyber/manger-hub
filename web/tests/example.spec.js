import { test, expect } from '@playwright/test';

test('has title', async ({ page }) => {
  await page.goto('/');
  await expect(page).toHaveTitle(/web/);
  await page.screenshot({ path: 'login.png', fullPage: true });
});

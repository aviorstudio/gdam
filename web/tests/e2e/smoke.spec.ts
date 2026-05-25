import { expect, test } from '@playwright/test';

test('homepage loads against local Supabase', async ({ page }) => {
  await page.goto('/');

  await expect(page.getByRole('link', { name: 'GDAM' })).toBeVisible();
  await expect(page.getByText('Godot Addon Manager')).toBeVisible();
  await expect(page.getByRole('heading', { name: 'All addons' })).toBeVisible();
});

test('seeded user can sign in', async ({ page }) => {
  await page.goto('/signin');

  await page.getByLabel('Email').fill('dev@gdam.local');
  await page.getByLabel('Password').fill('password123');
  await page.getByRole('button', { name: 'Sign in' }).click();

  await expect(page).toHaveURL('/');
  await expect(page.getByRole('link', { name: 'Account' })).toHaveAttribute('href', '/@dev');
});

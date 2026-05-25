import { expect, test, type Page } from '@playwright/test';

const signInAsDev = async (page: Page) => {
  await page.goto('/signin');
  await page.getByLabel('Email').fill('dev@gdam.local');
  await page.getByLabel('Password').fill('password123');
  await page.getByRole('button', { name: 'Sign in' }).click();

  await expect(page).toHaveURL('/');
  await expect(page.getByRole('link', { name: 'Account' })).toHaveAttribute('href', '/@dev');
};

const publishAddon = async (
  page: Page,
  opts: { name: string; editorPlugin: boolean }
) => {
  await page.goto('/@dev?create=1');

  await expect(page.getByRole('heading', { name: '@dev' })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Create an addon' })).toBeVisible();

  await page.getByLabel('Addon name').fill(opts.name);
  await page.getByLabel('Repository').fill('https://github.com/aviorstudio/gdam-test-addon');
  await page.getByLabel('Version').fill('0.1.0');
  await page.getByLabel('Sha').fill('df63bd560ea9d97ea8e277fd0fc46a07a5fc38fc');

  const editorPlugin = page.getByLabel('Editor plugin');
  if (opts.editorPlugin) {
    await editorPlugin.check();
  } else {
    await editorPlugin.uncheck();
  }

  await page.getByRole('button', { name: 'Create addon' }).click();
  await expect(page).toHaveURL(`/@dev/${opts.name}`);
};

test('homepage loads against local Supabase', async ({ page }) => {
  await page.goto('/');

  await expect(page.getByRole('link', { name: 'GDAM', exact: true })).toBeVisible();
  await expect(page.getByText('Godot Addon Manager')).toBeVisible();
  await expect(page.getByRole('heading', { name: 'All addons' })).toBeVisible();
});

test('seeded user can sign in', async ({ page }) => {
  await signInAsDev(page);
});

test('signed-in user can publish runtime and editor addons', async ({ page }) => {
  await signInAsDev(page);

  const suffix = Date.now().toString(36);
  const runtimeAddon = `runtime-${suffix}`;
  const editorAddon = `editor-${suffix}`;

  await publishAddon(page, { name: runtimeAddon, editorPlugin: false });
  await expect(page.getByText('Runtime addon')).toBeVisible();
  await expect(page.getByRole('link', { name: '0.1.0' })).toBeVisible();

  await publishAddon(page, { name: editorAddon, editorPlugin: true });
  await expect(page.getByText('Editor plugin')).toBeVisible();
  await expect(page.getByRole('link', { name: '0.1.0' })).toBeVisible();
});

test('signed-in user can publish a new release', async ({ page }) => {
  await signInAsDev(page);

  const addon = `release-${Date.now().toString(36)}`;
  await publishAddon(page, { name: addon, editorPlugin: true });

  await page.getByRole('link', { name: 'Create release' }).click();
  await expect(page.getByRole('heading', { name: 'Create a release' })).toBeVisible();
  await page.getByLabel('Version').fill('0.2.0');
  await page.getByLabel('Sha').fill('df63bd560ea9d97ea8e277fd0fc46a07a5fc38fc');
  await page.getByRole('button', { name: 'Create release' }).click();

  await expect(page).toHaveURL(`/@dev/${addon}`);
  await expect(page.getByRole('link', { name: '0.2.0' })).toBeVisible();
});

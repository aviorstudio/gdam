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
  opts: { name: string; editorPlugin: boolean; owner?: string }
) => {
  const owner = opts.owner ?? 'dev';
  await page.goto(`/@${owner}?create=1`);

  await expect(page.getByRole('heading', { name: `@${owner}` })).toBeVisible();
  await expect(page.getByRole('heading', { name: 'Create an addon' })).toBeVisible();

  await page.getByLabel('Addon name').fill(opts.name);
  await page.getByLabel('Repository').fill('https://github.com/aviorstudio/gdam-test-addon');
  await page.getByLabel('Version').fill('0.1.0');
  await page.getByLabel('Release tag (optional)').fill('v0.1.0');

  const editorPlugin = page.getByLabel('Editor plugin');
  if (opts.editorPlugin) {
    await editorPlugin.check();
  } else {
    await editorPlugin.uncheck();
  }

  await page.getByRole('button', { name: 'Create addon' }).click();
  await expect(page).toHaveURL(`/@${owner}/${opts.name}`);
};

const fillCreateAddonForm = async (page: Page, name: string) => {
  await page.getByLabel('Addon name').fill(name);
  await page.getByLabel('Repository').fill('https://github.com/aviorstudio/gdam-test-addon');
  await page.getByLabel('Version').fill('0.1.0');
  await page.getByLabel('Release tag (optional)').fill('v0.1.0');
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
  await page.getByLabel('Release tag (optional)').fill('v0.2.0');
  await page.getByRole('button', { name: 'Create release' }).click();

  await expect(page).toHaveURL(`/@dev/${addon}`);
  await expect(page.getByRole('link', { name: '0.2.0' })).toBeVisible();
});

test('signed-in user can create an org and publish under it', async ({ page }) => {
  await signInAsDev(page);

  const org = `org-${Date.now().toString(36)}`;
  await page.goto('/@dev/settings?create_org=1');
  await expect(page.getByRole('heading', { name: 'Create an org' })).toBeVisible();
  await page.getByLabel('Org username').fill(org);
  await page.getByLabel('Org name').fill(`Org ${org}`);
  await page.getByRole('button', { name: 'Create org' }).click();
  await expect(page).toHaveURL(`/@${org}`);

  const addon = `org-addon-${Date.now().toString(36)}`;
  await publishAddon(page, { owner: org, name: addon, editorPlugin: true });
  await expect(page.getByText('Editor plugin')).toBeVisible();
});

test('unauthenticated users cannot open publish dialogs', async ({ page }) => {
  await page.goto('/@dev?create=1');
  await expect(page).toHaveURL('/signin');
});

test('create addon form rejects invalid input and duplicates', async ({ page }) => {
  await signInAsDev(page);

  const addon = `negative-${Date.now().toString(36)}`;
  await page.goto('/@dev?create=1');
  await fillCreateAddonForm(page, addon);
  await page.getByLabel('Version').fill('1.0');
  await page.getByRole('button', { name: 'Create addon' }).click();
  await expect(page.getByText('Version must be in MAJOR.MINOR.PATCH format.')).toBeVisible();

  await page.getByLabel('Version').fill('0.1.0');
  await page.getByLabel('Repository').fill('https://example.com/owner/repo');
  await page.getByRole('button', { name: 'Create addon' }).click();
  await expect(page.getByText('Only GitHub repositories are supported')).toBeVisible();

  await publishAddon(page, { name: addon, editorPlugin: false });
  await page.goto('/@dev?create=1');
  await fillCreateAddonForm(page, addon);
  await page.getByRole('button', { name: 'Create addon' }).click();
  await expect(page.getByText('duplicate key value violates unique constraint')).toBeVisible();
});

test('create release form rejects invalid versions and duplicates', async ({ page }) => {
  await signInAsDev(page);

  const addon = `release-negative-${Date.now().toString(36)}`;
  await publishAddon(page, { name: addon, editorPlugin: true });

  await page.goto(`/@dev/${addon}?create=1`);
  await page.getByLabel('Version').fill('0.2');
  await page.getByLabel('Release tag (optional)').fill('v0.1.0');
  await page.getByRole('button', { name: 'Create release' }).click();
  await expect(page.getByText('Version must be in MAJOR.MINOR.PATCH format.')).toBeVisible();

  await page.getByLabel('Version').fill('0.1.0');
  await page.getByRole('button', { name: 'Create release' }).click();
  await expect(page.getByText('duplicate key value violates unique constraint')).toBeVisible();
});

import { expect, test, type Page } from '@playwright/test';

const signInAsDev = async (page: Page) => {
  await page.goto('/signin');
  await page.getByLabel('Email').fill('test@gdam.dev');
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
  await page.goto(`/create/@${owner}`);

  await expect(page.getByRole('heading', { name: 'Create addon' })).toBeVisible();

  await page.getByLabel('Addon name').fill(opts.name);
  await page.getByLabel('Repository').fill('https://github.com/aviorstudio/gdam-test-addon');
  await page.getByLabel('Publish release').check();
  await page.getByLabel('Version').fill('0.1.0');
  await page.getByLabel('Release tag').fill('v0.1.0');

  const editorPlugin = page.getByLabel('Editor enabled');
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
  await page.getByLabel('Publish release').check();
  await page.getByLabel('Version').fill('0.1.0');
  await page.getByLabel('Release tag').fill('v0.1.0');
};

test('homepage loads against local Supabase', async ({ page }) => {
  await page.goto('/');

  await expect(page.getByRole('link', { name: 'GDAM', exact: true })).toBeVisible();
  await expect(page.getByText('Godot Addon Manager')).toBeVisible();
  await expect(page.getByRole('link', { name: 'Docs' })).toBeVisible();
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
  await expect(page.getByRole('link', { name: '0.1.0' })).toBeVisible();

  await publishAddon(page, { name: editorAddon, editorPlugin: true });
  await expect(page.getByRole('link', { name: '0.1.0' })).toBeVisible();
});

test('signed-in user can create addon without release', async ({ page }) => {
  await signInAsDev(page);

  const addon = `empty-${Date.now().toString(36)}`;
  await page.goto('/create/@dev');
  await page.getByLabel('Addon name').fill(addon);
  await page.getByLabel('Repository').fill('https://github.com/aviorstudio/gdam-test-addon');
  await expect(page.getByLabel('Publish release')).not.toBeChecked();
  await expect(page.getByLabel('Version')).toBeHidden();
  await expect(page.getByLabel('Release tag')).toBeHidden();
  await expect(page.getByLabel('Asset name')).toBeHidden();
  await page.getByRole('button', { name: 'Create addon' }).click();

  await expect(page).toHaveURL(`/@dev/${addon}`);
  await expect(page.getByText('No versions yet')).toBeVisible();
});

test('signed-in user can publish a new release', async ({ page }) => {
  await signInAsDev(page);

  const addon = `release-${Date.now().toString(36)}`;
  await publishAddon(page, { name: addon, editorPlugin: true });

  await page.getByRole('link', { name: 'Publish release' }).click();
  await expect(page).toHaveURL(`/publish/@dev/${addon}`);
  await expect(page.getByRole('heading', { name: 'Publish release' })).toBeVisible();
  await page.getByLabel('Version').fill('0.2.0');
  await page.getByLabel('Release tag').fill('v0.2.0');
  await page.getByRole('button', { name: 'Publish release' }).click();

  await expect(page).toHaveURL(`/@dev/${addon}`);
  await expect(page.getByRole('link', { name: '0.2.0' })).toBeVisible();
});

test('signed-in user can delete releases and addons they own', async ({ page }) => {
  await signInAsDev(page);

  const addon = `delete-${Date.now().toString(36)}`;
  await publishAddon(page, { name: addon, editorPlugin: false });

  await page.getByRole('link', { name: 'Publish release' }).click();
  await page.getByLabel('Version').fill('0.2.0');
  await page.getByLabel('Release tag').fill('v0.2.0');
  await page.getByRole('button', { name: 'Publish release' }).click();
  await expect(page.getByRole('link', { name: '0.2.0' })).toBeVisible();

  page.once('dialog', async (dialog) => dialog.accept());
  await page.getByRole('button', { name: 'Delete', exact: true }).first().click();
  await expect(page).toHaveURL(`/@dev/${addon}`);
  await expect(page.getByRole('link', { name: '0.2.0' })).toHaveCount(0);
  await expect(page.getByRole('link', { name: '0.1.0' })).toBeVisible();

  await page.getByRole('link', { name: 'Settings' }).click();
  await expect(page).toHaveURL(`/settings/@dev/${addon}`);
  page.once('dialog', async (dialog) => dialog.accept());
  await page.getByRole('button', { name: 'Delete addon' }).click();
  await expect(page).toHaveURL('/@dev');
  await expect(page.getByRole('link', { name: addon })).toHaveCount(0);
});

test('signed-in user can create an org and publish under it', async ({ page }) => {
  await signInAsDev(page);

  const org = `org-${Date.now().toString(36)}`;
  await page.goto('/orgs/create');
  await expect(page.getByRole('heading', { name: 'Create org' })).toBeVisible();
  await page.getByLabel('Org username').fill(org);
  await page.getByLabel('Link').fill('https://example.com/org');
  await page.getByLabel('Bio').fill('Shared addon publishing');
  await page.getByRole('button', { name: 'Create org' }).click();
  await expect(page).toHaveURL(`/@${org}`);
  await expect(page.getByText('Shared addon publishing')).toBeVisible();
  await expect(page.getByRole('link', { name: 'example.com/org' })).toHaveAttribute('href', 'https://example.com/org');

  const addon = `org-addon-${Date.now().toString(36)}`;
  await publishAddon(page, { owner: org, name: addon, editorPlugin: true });
  await expect(page.getByRole('link', { name: '0.1.0' })).toBeVisible();
});

test('signed-in user can set profile link and bio', async ({ page }) => {
  await signInAsDev(page);

  const bio = `Building addons ${Date.now().toString(36)}`;
  await page.goto('/settings/@dev');
  await page.getByLabel('Link').fill('https://example.com/dev');
  await page.getByLabel('Bio').fill(bio);
  await page.getByRole('button', { name: 'Save changes' }).click();
  await expect(page).toHaveURL('/settings/@dev');

  await page.goto('/@dev');
  await expect(page.getByText(bio)).toBeVisible();
  await expect(page.getByRole('link', { name: 'example.com/dev' })).toHaveAttribute('href', 'https://example.com/dev');
});

test('unauthenticated users cannot open publish pages', async ({ page }) => {
  await page.goto('/create/@dev');
  await expect(page).toHaveURL('/signin');
});

test('create addon form rejects invalid input and duplicates', async ({ page }) => {
  await signInAsDev(page);

  const addon = `negative-${Date.now().toString(36)}`;
  await page.goto('/create/@dev');
  await fillCreateAddonForm(page, addon);
  await page.getByLabel('Version').fill('1.0');
  await page.getByRole('button', { name: 'Create addon' }).click();
  await expect(page.getByText('Version must be in MAJOR.MINOR.PATCH format.')).toBeVisible();

  await page.getByLabel('Version').fill('0.1.0');
  await page.getByLabel('Repository').fill('https://example.com/owner/repo');
  await page.getByRole('button', { name: 'Create addon' }).click();
  await expect(page.getByText('Only GitHub repositories are supported')).toBeVisible();

  await publishAddon(page, { name: addon, editorPlugin: false });
  await page.goto('/create/@dev');
  await fillCreateAddonForm(page, addon);
  await page.getByRole('button', { name: 'Create addon' }).click();
  await expect(page.getByText('duplicate key value violates unique constraint')).toBeVisible();
});

test('publish release form rejects invalid versions and duplicates', async ({ page }) => {
  await signInAsDev(page);

  const addon = `release-negative-${Date.now().toString(36)}`;
  await publishAddon(page, { name: addon, editorPlugin: true });

  await page.goto(`/publish/@dev/${addon}`);
  await page.getByLabel('Version').fill('0.2');
  await page.getByLabel('Release tag').fill('v0.1.0');
  await page.getByRole('button', { name: 'Publish release' }).click();
  await expect(page.getByText('Version must be in MAJOR.MINOR.PATCH format.')).toBeVisible();

  await page.getByLabel('Version').fill('0.1.0');
  await page.getByRole('button', { name: 'Publish release' }).click();
  await expect(page.getByText('duplicate key value violates unique constraint')).toBeVisible();
});

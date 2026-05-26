import { expect, test, type Page } from '@playwright/test';
import { createClient } from '@supabase/supabase-js';

const SUPABASE_URL = process.env.SUPABASE_URL ?? 'http://127.0.0.1:54421';
const SUPABASE_PUBLISHABLE_KEY = process.env.SUPABASE_PUBLISHABLE_KEY ?? 'sb_publishable_ACJWlzQHlZjBrEguHvfOxg_3BJgxAaH';
const TEST_PASSWORD = 'password123';

const signIn = async (page: Page, email: string, password = TEST_PASSWORD, accountHref = '/@dev') => {
  await page.goto('/signin');
  await page.getByLabel('Email').fill(email);
  await page.getByLabel('Password').fill(password);
  await page.getByRole('button', { name: 'Sign in' }).click();

  await expect(page).toHaveURL('/');
  await expect(page.getByRole('link', { name: 'Account' })).toHaveAttribute('href', accountHref);
};

const signInAsDev = async (page: Page) => {
  await signIn(page, 'test@gdam.dev');
};

const disableNativeValidation = async (page: Page) => {
  await page.locator('form').first().evaluate((form) => {
    (form as HTMLFormElement).noValidate = true;
  });
};

const createLocalUser = async (username: string) => {
  const email = `${username}@gdam.dev`;
  const supabase = createClient(SUPABASE_URL, SUPABASE_PUBLISHABLE_KEY);
  let authResult = await supabase.auth.signUp({ email, password: TEST_PASSWORD });
  if (!authResult.data.session) {
    authResult = await supabase.auth.signInWithPassword({ email, password: TEST_PASSWORD });
  }

  const session = authResult.data.session;
  const userId = session?.user?.id ?? '';
  if (authResult.error || !session || !userId) {
    throw new Error(authResult.error?.message || `Unable to create ${email}`);
  }

  await supabase.auth.setSession({ access_token: session.access_token, refresh_token: session.refresh_token });
  const profileResult = await supabase.from('profiles').upsert({ id: userId });
  if (profileResult.error) throw new Error(profileResult.error.message);

  const usernameResult = await supabase.from('usernames').insert({ name: username, user_id: userId, org_id: null });
  if (usernameResult.error && usernameResult.error.code !== '23505') {
    throw new Error(usernameResult.error.message);
  }

  return { email, password: TEST_PASSWORD, username };
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

test('org admin can delete an org and cascade its addons', async ({ page }) => {
  await signInAsDev(page);

  const suffix = Date.now().toString(36);
  const org = `delete-org-${suffix}`;
  const addon = `delete-org-addon-${suffix}`;

  await page.goto('/orgs/create');
  await page.getByLabel('Org username').fill(org);
  await page.getByRole('button', { name: 'Create org' }).click();
  await expect(page).toHaveURL(`/@${org}`);

  await publishAddon(page, { owner: org, name: addon, editorPlugin: true });
  await expect(page.getByRole('link', { name: '0.1.0' })).toBeVisible();

  await page.goto(`/settings/@${org}`);
  page.once('dialog', async (dialog) => dialog.accept());
  await page.getByRole('button', { name: 'Delete org' }).click();
  await expect(page).toHaveURL('/orgs');
  await expect(page.getByRole('link', { name: `@${org}` })).toHaveCount(0);

  await page.goto(`/@${org}`);
  await expect(page.getByRole('heading', { name: 'Not found' })).toBeVisible();

  await page.goto(`/@${org}/${addon}`);
  await expect(page.getByRole('heading', { name: 'Not found' })).toBeVisible();
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

test('signed-in user can create and revoke a copy-once scoped secret key', async ({ page }) => {
  await signInAsDev(page);

  const org = `key-org-${Date.now().toString(36)}`;
  await page.goto('/orgs/create');
  await page.getByLabel('Org username').fill(org);
  await page.getByRole('button', { name: 'Create org' }).click();
  await expect(page).toHaveURL(`/@${org}`);

  const keyName = `Release workflow ${Date.now().toString(36)}`;
  await page.goto('/settings/@dev');
  await page.getByLabel('Key name').fill(keyName);
  await expect(page.getByLabel('@dev')).toBeChecked();
  await page.getByLabel(`@${org}`).check();
  await page.getByRole('button', { name: 'Create secret key' }).click();
  const secretKeyDialog = page.getByRole('dialog', { name: 'Copy your secret key' });
  await expect(secretKeyDialog).toBeVisible();
  await expect(page.getByText('This secret key will never be shown again.')).toBeVisible();
  await expect(secretKeyDialog.getByRole('textbox', { name: 'Secret key' })).toHaveValue(/^gdam_sk_/);
  await expect(secretKeyDialog.getByRole('button', { name: 'Copy' })).toBeVisible();
  await expect(page.getByText(keyName)).toBeVisible();
  await expect(page.getByText(new RegExp(`Scopes: .*@dev.*@${org}`))).toBeVisible();
  const secretKeyRow = page.locator('.flex.items-start').filter({ hasText: keyName });
  await expect(secretKeyRow.getByText(/\d{1,2}\/\d{1,2}\/\d{4}/)).toBeVisible();

  await page.goto('/settings/@dev');
  await expect(page.getByRole('dialog', { name: 'Copy your secret key' })).toHaveCount(0);
  await expect(page.getByText(keyName)).toBeVisible();
  await expect(page.getByText(new RegExp(`Scopes: .*@dev.*@${org}`))).toBeVisible();

  page.once('dialog', async (dialog) => dialog.accept());
  await page.locator('.flex.items-start').filter({ hasText: keyName }).getByRole('button', { name: 'Revoke' }).click();
  await expect(page.getByText('Secret key revoked.')).toBeVisible();
  await expect(page.getByText(keyName)).toHaveCount(0);
});

test('org creation validates every field and preserves valid input', async ({ page }) => {
  await signInAsDev(page);

  await page.goto('/orgs/create');
  await disableNativeValidation(page);
  await page.getByRole('button', { name: 'Create org' }).click();
  await expect(page.getByText('Organization username is required.')).toBeVisible();

  await page.getByLabel('Org username').fill('settings');
  await page.getByRole('button', { name: 'Create org' }).click();
  await expect(page.getByText('That username is reserved.')).toBeVisible();

  await page.getByLabel('Org username').fill('dev');
  await page.getByRole('button', { name: 'Create org' }).click();
  await expect(page.getByText('That username is already taken.')).toBeVisible();

  const org = `validate-org-${Date.now().toString(36)}`;
  await page.getByLabel('Org username').fill(org);
  await page.getByLabel('Link').fill('ftp://example.com/org');
  await page.getByLabel('Bio').fill('Validation org bio');
  await page.getByRole('button', { name: 'Create org' }).click();
  await expect(page.getByText('Link must start with http:// or https://.')).toBeVisible();
  await expect(page.getByLabel('Org username')).toHaveValue(org);
  await expect(page.getByLabel('Bio')).toHaveValue('Validation org bio');

  await page.getByLabel('Link').fill('https://example.com/validation-org');
  await page.getByRole('button', { name: 'Create org' }).click();
  await expect(page).toHaveURL(`/@${org}`);
  await expect(page.getByText('Validation org bio')).toBeVisible();
  await expect(page.getByRole('link', { name: 'example.com/validation-org' })).toHaveAttribute('href', 'https://example.com/validation-org');
});

test('profile and org settings validate username, link, bio, and redirects', async ({ page }) => {
  await signInAsDev(page);

  await page.goto('/settings/@dev');
  await disableNativeValidation(page);
  await page.getByLabel('Username').fill('settings');
  await page.getByRole('button', { name: 'Save changes' }).click();
  await expect(page.getByText('That username is reserved.')).toBeVisible();

  await page.getByLabel('Username').fill('dev');
  await page.getByLabel('Link').fill('notaurl');
  await disableNativeValidation(page);
  await page.getByRole('button', { name: 'Save changes' }).click();
  await expect(page.getByText('Link must be a valid URL.')).toBeVisible();

  const org = `settings-org-${Date.now().toString(36)}`;
  await page.goto('/orgs/create');
  await page.getByLabel('Org username').fill(org);
  await page.getByRole('button', { name: 'Create org' }).click();
  await expect(page).toHaveURL(`/@${org}`);

  await page.goto(`/settings/@${org}`);
  await page.getByLabel('Username').fill('dev');
  await page.getByRole('button', { name: 'Save changes' }).click();
  await expect(page.getByText('That username is already taken.')).toBeVisible();

  const renamedOrg = `${org}-renamed`;
  await page.getByLabel('Username').fill(renamedOrg);
  await page.getByLabel('Link').fill('https://example.com/renamed-org');
  await page.getByLabel('Bio').fill('Renamed org bio');
  await page.getByRole('button', { name: 'Save changes' }).click();
  await expect(page).toHaveURL(`/settings/@${renamedOrg}`);
  await page.goto(`/@${renamedOrg}`);
  await expect(page.getByText('Renamed org bio')).toBeVisible();
  await expect(page.getByRole('link', { name: 'example.com/renamed-org' })).toHaveAttribute('href', 'https://example.com/renamed-org');
});

test('unauthenticated users cannot open publish pages', async ({ page }) => {
  await page.goto('/create/@dev');
  await expect(page).toHaveURL('/signin');
});

test('unauthenticated users cannot open protected pages', async ({ page }) => {
  await page.goto('/orgs');
  await expect(page).toHaveURL('/');

  await page.goto('/orgs/create');
  await expect(page).toHaveURL('/signin');

  await page.goto('/settings/@dev');
  await expect(page).toHaveURL('/@dev');

  await page.goto('/publish/@dev/missing-addon');
  await expect(page).toHaveURL('/signin');
});

test('signed-in non-owners cannot manage another profile or org', async ({ page }) => {
  await signInAsDev(page);

  const suffix = Date.now().toString(36);
  const org = `private-org-${suffix}`;
  const addon = `private-addon-${suffix}`;
  await page.goto('/orgs/create');
  await page.getByLabel('Org username').fill(org);
  await page.getByRole('button', { name: 'Create org' }).click();
  await expect(page).toHaveURL(`/@${org}`);
  await publishAddon(page, { owner: org, name: addon, editorPlugin: true });

  const outsider = await createLocalUser(`outsider-${suffix}`);
  await page.goto('/api/auth/signout');
  await signIn(page, outsider.email, outsider.password, `/@${outsider.username}`);

  await page.goto('/settings/@dev');
  await expect(page).toHaveURL('/@dev');
  await expect(page.getByRole('heading', { name: 'Settings' })).toHaveCount(0);

  await page.goto(`/settings/@${org}`);
  await expect(page).toHaveURL(`/@${org}`);
  await expect(page.getByRole('link', { name: 'Settings' })).toHaveCount(0);
  await expect(page.getByRole('button', { name: 'Delete org' })).toHaveCount(0);

  await page.goto(`/settings/@${org}/${addon}`);
  await expect(page).toHaveURL(`/@${org}/${addon}`);
  await expect(page.getByRole('button', { name: 'Delete addon' })).toHaveCount(0);

  await page.goto(`/create/@${org}`);
  await expect(page.getByText('You do not have permission to publish addons for this account.')).toBeVisible();

  await page.goto(`/publish/@${org}/${addon}`);
  await expect(page.getByText('You do not have permission to publish releases for this addon.')).toBeVisible();
});

test('create addon form rejects invalid input and duplicates', async ({ page }) => {
  await signInAsDev(page);

  const addon = `negative-${Date.now().toString(36)}`;
  await page.goto('/create/@dev');
  await disableNativeValidation(page);
  await page.getByRole('button', { name: 'Create addon' }).click();
  await expect(page.getByText('Addon name and repository are required.')).toBeVisible();

  await fillCreateAddonForm(page, addon);
  await page.getByLabel('Release tag').fill('');
  await page.getByRole('button', { name: 'Create addon' }).click();
  await expect(page.getByText('Version and release tag are required to publish a release.')).toBeVisible();

  await page.getByLabel('Release tag').fill('v0.1.0');
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
  await disableNativeValidation(page);
  await page.getByRole('button', { name: 'Publish release' }).click();
  await expect(page.getByText('Version and release tag are required.')).toBeVisible();

  await page.getByLabel('Version').fill('0.2');
  await page.getByLabel('Release tag').fill('v0.1.0');
  await page.getByRole('button', { name: 'Publish release' }).click();
  await expect(page.getByText('Version must be in MAJOR.MINOR.PATCH format.')).toBeVisible();

  await page.getByLabel('Version').fill('0.1.0');
  await page.getByRole('button', { name: 'Publish release' }).click();
  await expect(page.getByText('duplicate key value violates unique constraint')).toBeVisible();
});

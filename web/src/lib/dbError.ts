type DbErrorLike = {
  code?: string | null;
  details?: string | null;
  hint?: string | null;
  message?: string | null;
};

const UNIQUE_VIOLATION = '23505';

const includesConstraint = (error: DbErrorLike, constraints: string[]) => {
  const text = `${error.message ?? ''} ${error.details ?? ''} ${error.hint ?? ''}`;
  return constraints.some((constraint) => text.includes(constraint));
};

export const formatAddonInsertError = (error: DbErrorLike, ownerSlug: string, addonName: string) => {
  if (
    error.code === UNIQUE_VIOLATION &&
    includesConstraint(error, ['addons_profile_name_key', 'addons_org_name_key'])
  ) {
    return `Addon @${ownerSlug}/${addonName} already exists. Choose a different addon name or manage the existing addon.`;
  }

  return error.message ?? 'Unable to create addon.';
};

export const formatReleaseInsertError = (error: DbErrorLike) => {
  if (
    error.code === UNIQUE_VIOLATION &&
    includesConstraint(error, ['releases_addon_id_major_minor_patch_key', 'releases_addon_tag_key'])
  ) {
    return 'That release version or tag already exists for this addon.';
  }

  return error.message ?? 'Unable to publish release.';
};

import type { TFunction } from "i18next";

import i18n, { localeCode } from "@/lib/i18n";

/** Exact backend ErrorResponse.error English string → apiErrors.* slug. */
export const API_ERROR_SLUGS: Record<string, string> = {
  "access control unavailable": "accessControlUnavailable",
  "access denied": "accessDenied",
  "account disabled": "accountDisabled",
  "action already running": "actionAlreadyRunning",
  "admin access required": "adminAccessRequired",
  "admin users have implicit full access": "adminUsersHaveImplicitFullAccess",
  "at least one permission is required": "atLeastOnePermissionIsRequired",
  "audio language list exceeds maximum of 3": "audioLanguageListExceedsMaximumOf3",
  "auth unavailable": "authUnavailable",
  "authentication required": "authenticationRequired",
  "browse failed": "browseFailed",
  "cancel failed": "cancelFailed",
  "cannot delete last admin": "cannotDeleteLastAdmin",
  "cannot modify last admin": "cannotModifyLastAdmin",
  "catalog failed": "catalogFailed",
  "catalog unavailable": "catalogUnavailable",
  "change email failed": "changeEmailFailed",
  "change password failed": "changePasswordFailed",
  "check your email to accept the invite": "checkYourEmailToAcceptTheInvite",
  "confirmation resend unavailable": "confirmationResendUnavailable",
  "create invite failed": "createInviteFailed",
  "create user failed": "createUserFailed",
  "delete failed": "deleteFailed",
  "delete library failed": "deleteLibraryFailed",
  "delete user failed": "deleteUserFailed",
  "dlna settings unavailable": "dlnaSettingsUnavailable",
  "dlna tv userId is required when enabled": "dlnaTvUserIdIsRequiredWhenEnabled",
  "dlna userId must be a tv role account": "dlnaUserIdMustBeATvRoleAccount",
  "email is already used": "emailIsAlreadyUsed",
  "email is required": "emailIsRequired",
  "failed to encode thumbnail": "failedToEncodeThumbnail",
  "failed to load poster": "failedToLoadPoster",
  "failed to load subtitle": "failedToLoadSubtitle",
  "failed to read upload": "failedToReadUpload",
  "failed to store subtitle": "failedToStoreSubtitle",
  "favorite unavailable": "favoriteUnavailable",
  "favorite update failed": "favoriteUpdateFailed",
  "file is not a supported image": "fileIsNotASupportedImage",
  "file is required": "fileIsRequired",
  "folder already belongs to a library": "folderAlreadyBelongsToALibrary",
  "folder not found": "folderNotFound",
  forbidden: "forbidden",
  "home shelves unavailable": "homeShelvesUnavailable",
  "invalid current password": "invalidCurrentPassword",
  "invalid downmixAlgorithm value": "invalidDownmixAlgorithmValue",
  "invalid downmixBoost value": "invalidDownmixBoostValue",
  "invalid email or password": "invalidEmailOrPassword",
  "invalid hwAccel value": "invalidHwAccelValue",
  "invalid invite token": "invalidInviteToken",
  "invalid library root": "invalidLibraryRoot",
  "invalid library type": "invalidLibraryType",
  "invalid password": "invalidPassword",
  "invalid path": "invalidPath",
  "invalid request body": "invalidRequestBody",
  "invalid retentionDays": "invalidRetentionDays",
  "invalid role": "invalidRole",
  "invalid season": "invalidSeason",
  "invalid status filter": "invalidStatusFilter",
  "invalid subtitle file": "invalidSubtitleFile",
  "invalid token": "invalidToken",
  "invalid toneMappingAlgorithm value": "invalidToneMappingAlgorithmValue",
  "invalid two factor code": "invalidTwoFactorCode",
  "lang must be ISO 639-1 alpha-2": "langMustBeISO6391Alpha2",
  "library name is required": "libraryNameIsRequired",
  "library not found": "libraryNotFound",
  "library patch requires name and/or type": "libraryPatchRequiresNameAndOrType",
  "library root not found": "libraryRootNotFound",
  "library slug already exists": "librarySlugAlreadyExists",
  "library sync unavailable": "librarySyncUnavailable",
  "list home shelf failed": "listHomeShelfFailed",
  "list home statistics failed": "listHomeStatisticsFailed",
  "list invites failed": "listInvitesFailed",
  "list libraries failed": "listLibrariesFailed",
  "list runs failed": "listRunsFailed",
  "list sessions failed": "listSessionsFailed",
  "list trash failed": "listTrashFailed",
  "list users failed": "listUsersFailed",
  "load dlna settings failed": "loadDlnaSettingsFailed",
  "load grants failed": "loadGrantsFailed",
  "load library failed": "loadLibraryFailed",
  "load library maintenance failed": "loadLibraryMaintenanceFailed",
  "load maintenance failed": "loadMaintenanceFailed",
  "load network settings failed": "loadNetworkSettingsFailed",
  "load playback preferences failed": "loadPlaybackPreferencesFailed",
  "load provider settings failed": "loadProviderSettingsFailed",
  "load settings failed": "loadSettingsFailed",
  "load transcode settings failed": "loadTranscodeSettingsFailed",
  "load trash settings failed": "loadTrashSettingsFailed",
  "login failed": "loginFailed",
  "maintenance unavailable": "maintenanceUnavailable",
  "media unavailable": "mediaUnavailable",
  "media volume is not writable": "mediaVolumeIsNotWritable",
  "metadata fetch failed": "metadataFetchFailed",
  "metadata patch failed": "metadataPatchFailed",
  "metadata request failed": "metadataRequestFailed",
  "metadata scan already running": "metadataScanAlreadyRunning",
  "metadata scan failed to start": "metadataScanFailedToStart",
  "metadata unavailable": "metadataUnavailable",
  "network settings unavailable": "networkSettingsUnavailable",
  "new password must differ from the current password":
    "newPasswordMustDifferFromTheCurrentPassword",
  "not found": "notFound",
  "path is not a supported video file": "pathIsNotASupportedVideoFile",
  "playback metadata unavailable": "playbackMetadataUnavailable",
  "provider poster not available": "providerPosterNotAvailable",
  "provider subtitle not available": "providerSubtitleNotAvailable",
  "providers only apply to film and series libraries": "providersOnlyApplyToFilmAndSeriesLibraries",
  "providers unavailable": "providersUnavailable",
  "read folder failed": "readFolderFailed",
  "request failed": "requestFailed",
  "resend confirmation failed": "resendConfirmationFailed",
  "resend invite failed": "resendInviteFailed",
  "reset two factor failed": "resetTwoFactorFailed",
  "restore destination exists": "restoreDestinationExists",
  "revoke invite failed": "revokeInviteFailed",
  "revoke session failed": "revokeSessionFailed",
  "revoke sessions failed": "revokeSessionsFailed",
  "run failed to start": "runFailedToStart",
  "run is not running": "runIsNotRunning",
  "run not found": "runNotFound",
  "save dlna settings failed": "saveDlnaSettingsFailed",
  "save library schedules failed": "saveLibrarySchedulesFailed",
  "save maintenance schedules failed": "saveMaintenanceSchedulesFailed",
  "save network settings failed": "saveNetworkSettingsFailed",
  "save playback preferences failed": "savePlaybackPreferencesFailed",
  "save transcode settings failed": "saveTranscodeSettingsFailed",
  "save trash settings failed": "saveTrashSettingsFailed",
  "search failed": "searchFailed",
  "search unavailable": "searchUnavailable",
  "subtitle too large": "subtitleTooLarge",
  "sync libraries failed": "syncLibrariesFailed",
  "token already used": "tokenAlreadyUsed",
  "token expired": "tokenExpired",
  "transcode failed": "transcodeFailed",
  "transcode in progress": "transcodeInProgress",
  "transcode settings unavailable": "transcodeSettingsUnavailable",
  "trash item not found": "trashItemNotFound",
  "trash operation failed": "trashOperationFailed",
  "trash unavailable": "trashUnavailable",
  "tv accounts cannot be invited": "tvAccountsCannotBeInvited",
  "tv accounts cannot use web login": "tvAccountsCannotUseWebLogin",
  "tv user not found": "tvUserNotFound",
  "two factor already enabled": "twoFactorAlreadyEnabled",
  "two factor not configured": "twoFactorNotConfigured",
  "two factor required": "twoFactorRequired",
  "unknown library": "unknownLibrary",
  "update library failed": "updateLibraryFailed",
  "update library roots failed": "updateLibraryRootsFailed",
  "update provider settings failed": "updateProviderSettingsFailed",
  "update settings failed": "updateSettingsFailed",
  "update user failed": "updateUserFailed",
  "user already exists": "userAlreadyExists",
  "user must be disabled before delete": "userMustBeDisabledBeforeDelete",
  "user not found": "userNotFound",
  "user subtitle not found": "userSubtitleNotFound",
  "user subtitles unavailable": "userSubtitlesUnavailable",
  "watch state unavailable": "watchStateUnavailable",
  "watch update failed": "watchUpdateFailed",
};

/** Stable BE English strings used for control-flow checks (not for display). */
export const ApiErrorMessage = {
  invalidCurrentPassword: "invalid current password",
  twoFactorRequired: "two factor required",
  invalidTwoFactorCode: "invalid two factor code",
  emailAlreadyUsed: "email is already used",
  deleteFailed: "delete failed",
} as const;

export function extractApiErrorMessage(error: unknown): string | undefined {
  if (!error || typeof error !== "object") {
    return undefined;
  }

  if ("error" in error && typeof error.error === "string") {
    return error.error;
  }

  return undefined;
}

function rawApiErrorText(error: unknown): string | undefined {
  if (typeof error === "string") {
    return error;
  }
  if (error instanceof Error) {
    return error.message;
  }
  return extractApiErrorMessage(error);
}

export function isApiErrorMessage(error: unknown, english: string): boolean {
  return rawApiErrorText(error)?.trim() === english;
}

export function translateApiError(
  error: unknown,
  t: TFunction,
  opts?: { status?: number; fallbackKey?: string },
): string {
  const trimmed = rawApiErrorText(error)?.trim();
  if (trimmed) {
    const slug = API_ERROR_SLUGS[trimmed];
    if (slug) {
      return t(`apiErrors.${slug}`);
    }
    if (localeCode(i18n.resolvedLanguage ?? i18n.language) === "en") {
      return trimmed;
    }
  }

  if (opts?.fallbackKey) {
    return t(opts.fallbackKey);
  }
  if (opts?.status !== undefined) {
    const statusKey = `apiErrors.status${opts.status}`;
    const translated = t(statusKey);
    if (translated !== statusKey) {
      return translated;
    }
  }
  return t("common.error");
}

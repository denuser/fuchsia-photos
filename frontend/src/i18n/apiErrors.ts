// The API writes its errors in English. The interface shows its own text for the errors it knows,
// matched by the server's message, and the server's message for the rest. Keep the patterns in step
// with the backend's messages: a changed message falls back to English rather than failing.
import { t } from '.'

type Known = [RegExp, (match: RegExpMatchArray) => string]

const known: Known[] = [
  [/^wrong login or password$/, () => t('apiErrors.wrongLogin')],
  [/^the current password is wrong$/, () => t('apiErrors.wrongPassword')],
  [/^a password must be (\d+) to (\d+) characters$/, (m) => t('apiErrors.passwordLength', { min: m[1], max: m[2] })],
  [/^a login must be 2 to 64 characters/, () => t('apiErrors.loginFormat')],
  [/^this login is taken$/, () => t('apiErrors.loginTaken')],
  [/^you cannot change your own role or disable yourself$/, () => t('apiErrors.ownRole')],
  [/^a name is 1 to (\d+) characters/, (m) => t('apiErrors.nameLength', { max: m[1] })],
  [/^you already have a tag with this name$/, () => t('apiErrors.tagTaken')],
  [/^this folder is already linked to an album$/, () => t('apiErrors.folderLinked')],
  [/^a user can link at most (\d+) folders$/, (m) => t('apiErrors.folderLimit', { max: m[1] })],
  [/^a user can have at most (\d+) app passwords$/, (m) => t('apiErrors.appPasswordLimit', { max: m[1] })],
  [/^the file is too large$/, () => t('apiErrors.fileTooLarge')],
  [/^only JPEG, PNG, WebP and HEIC photos and MP4 and MOV videos are supported$/, () => t('apiErrors.unsupportedFormat')],
  [/^unsupported file format$/, () => t('apiErrors.unsupportedFormat')],
  [/^a bulk action takes 1 to (\d+) photos$/, (m) => t('apiErrors.bulkLimit', { max: m[1] })],
  [/^search by description is unavailable/, () => t('apiErrors.searchUnavailable')],
  [/^the query must have 1 to (\d+) characters$/, (m) => t('apiErrors.queryLength', { max: m[1] })],
  [/^another scan is running$/, () => t('apiErrors.scanRunning')],
  [/^this scan is not running$/, () => t('apiErrors.scanNotRunning')],
  [/^no such folder in the import folder$/, () => t('apiErrors.noFolder')],
  [/^no such active user$/, () => t('apiErrors.noOwner')],
  [/^album not found$/, () => t('apiErrors.albumNotFound')],
  [/^not found$/, () => t('apiErrors.notFound')],
  [/^administrators only$/, () => t('apiErrors.adminsOnly')],
  [/^internal error$/, () => t('apiErrors.internal')],
]

// The text to show for an API error, or the fallback when the request failed without one.
export function apiError(failure: { error: string } | string | undefined, fallback?: string): string {
  const message = typeof failure === 'string' ? failure : failure?.error
  if (!message) {
    return fallback ?? t('apiErrors.unknown')
  }
  for (const [pattern, text] of known) {
    const match = message.match(pattern)
    if (match) {
      return text(match)
    }
  }
  return message
}

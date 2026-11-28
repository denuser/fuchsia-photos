import createClient from 'openapi-fetch'

import type { components, paths } from './schema'

export type Activity = components['schemas']['Activity']
export type Album = components['schemas']['Album']
export type AppPassword = components['schemas']['AppPassword']
export type WebDAVAlbum = components['schemas']['WebDAVAlbum']
export type Camera = components['schemas']['Camera']
export type JobError = components['schemas']['JobError']
export type JobKindStats = components['schemas']['JobKindStats']
export type LibraryFolder = components['schemas']['LibraryFolder']
export type LibraryFile = components['schemas']['LibraryFile']
export type ImportKind = components['schemas']['ImportKind']
export type ImportAlbumTrash = components['schemas']['ImportAlbumTrash']
export type ImportProblem = components['schemas']['ImportProblem']
export type ImportRun = components['schemas']['ImportRun']
export type Photo = components['schemas']['Photo']
export type Identity = components['schemas']['Identity']
export type SignInOptions = components['schemas']['SignInOptions']
export type TwoFactorStatus = components['schemas']['TwoFactorStatus']
export type AlbumLink = components['schemas']['AlbumLink']
export type PublicAlbum = components['schemas']['PublicAlbum']
export type PublicPhoto = components['schemas']['PublicPhoto']
export type SignInSettings = components['schemas']['SignInSettings']
export type Language = components['schemas']['Language']
export type GridSize = components['schemas']['GridSize']
export type Location = components['schemas']['Location']
export type MediaKind = components['schemas']['MediaKind']
export type MemoryYear = components['schemas']['MemoryYear']
export type NameCount = components['schemas']['NameCount']
export type MetadataSettings = components['schemas']['MetadataSettings']
export type MLStatus = components['schemas']['MLStatus']
export type FaceScans = components['schemas']['FaceScans']
export type PhotoFaceScan = components['schemas']['PhotoFaceScan']
export type PhotoEvent = components['schemas']['PhotoEvent']
export type PreviewSettings = components['schemas']['PreviewSettings']
export type VideoSettings = components['schemas']['VideoSettings']
export type QueueStats = components['schemas']['QueueStats']
export type Role = components['schemas']['Role']
export type Tag = components['schemas']['Tag']
export type TagRef = components['schemas']['TagRef']
export type UploadResult = components['schemas']['UploadResult']
export type User = components['schemas']['User']
export type UserRef = components['schemas']['UserRef']
export type Visibility = components['schemas']['Visibility']
export type ViewerField = components['schemas']['ViewerField']

export const API_BASE = '/api/v1'

export const api = createClient<paths>({ baseUrl: API_BASE })

let onUnauthorized = () => {}

// Called when the session ends while the app is open.
export function setUnauthorizedHandler(handler: () => void) {
  onUnauthorized = handler
}

export function notifyUnauthorized() {
  onUnauthorized()
}

api.use({
  onResponse({ request, response }) {
    if (response.status === 401 && !request.url.endsWith('/login') && !request.url.endsWith('/me')) {
      onUnauthorized()
    }
  },
})

// Previews are cached for a long time. The version changes when they are made
// again, so that the browser loads the new ones.
export function previewUrl(id: number, kind: 'grid' | 'viewer', version = 0) {
  const url = `${API_BASE}/photos/${id}/previews/${kind}`
  return version ? `${url}?v=${version}` : url
}

export function originalUrl(id: number) {
  return `${API_BASE}/photos/${id}/original`
}

// The original of a video, sent for playing rather than as a download.
export function videoUrl(id: number) {
  return `${API_BASE}/photos/${id}/video`
}

// The video of a motion photo or a Live Photo. Cached like a preview, so it
// carries the version.
export function motionUrl(id: number, version = 0) {
  const url = `${API_BASE}/photos/${id}/motion`
  return version ? `${url}?v=${version}` : url
}

// The H.264 copy of a video, or of the motion of a photo, for browsers that
// cannot play the original, such as HEVC in some of them.
export function copyUrl(id: number) {
  return `${API_BASE}/photos/${id}/copy`
}

export function isVideo(photo: Pick<Photo, 'mime'>) {
  return photo.mime.startsWith('video/')
}

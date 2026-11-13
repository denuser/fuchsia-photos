import createClient from 'openapi-fetch'

import type { components, paths } from './schema'

export type Album = components['schemas']['Album']
export type AppPassword = components['schemas']['AppPassword']
export type WebDAVAlbum = components['schemas']['WebDAVAlbum']
export type Camera = components['schemas']['Camera']
export type JobError = components['schemas']['JobError']
export type JobKindStats = components['schemas']['JobKindStats']
export type ImportProblem = components['schemas']['ImportProblem']
export type ImportRun = components['schemas']['ImportRun']
export type Photo = components['schemas']['Photo']
export type Language = components['schemas']['Language']
export type Location = components['schemas']['Location']
export type MediaKind = components['schemas']['MediaKind']
export type NameCount = components['schemas']['NameCount']
export type MetadataSettings = components['schemas']['MetadataSettings']
export type MLStatus = components['schemas']['MLStatus']
export type PhotoEvent = components['schemas']['PhotoEvent']
export type PreviewSettings = components['schemas']['PreviewSettings']
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

export function isVideo(photo: Pick<Photo, 'mime'>) {
  return photo.mime.startsWith('video/')
}

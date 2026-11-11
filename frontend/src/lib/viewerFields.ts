import type { ViewerField } from '../api/client'

// The photo details the viewer can show, in its order. Their names are under viewerFields in the messages.
export const viewerFields: ViewerField[] = [
  'camera',
  'lens',
  'focalLength',
  'aperture',
  'exposureTime',
  'iso',
  'exposureCompensation',
  'flash',
  'location',
]

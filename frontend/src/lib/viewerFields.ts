import type { ViewerField } from '../api/client'

// The photo details the viewer can show, in its order, with their names in the profile.
export const viewerFields: { value: ViewerField; title: string }[] = [
  { value: 'camera', title: 'Camera' },
  { value: 'lens', title: 'Lens' },
  { value: 'focalLength', title: 'Focal length' },
  { value: 'aperture', title: 'Aperture' },
  { value: 'exposureTime', title: 'Shutter speed' },
  { value: 'iso', title: 'ISO' },
  { value: 'exposureCompensation', title: 'Exposure compensation' },
  { value: 'flash', title: 'Flash' },
  { value: 'location', title: 'Location' },
]

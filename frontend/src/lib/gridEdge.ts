import { ref } from 'vue'

import { api } from '../api/client'

// The default grid preview size, used until the real one loads.
const DEFAULT_GRID_EDGE = 400

const gridEdge = ref(DEFAULT_GRID_EDGE)
let requested = false

// The longest side of a grid preview in pixels, which sets the size of square
// tiles. It is asked for once, the first time it is needed.
export function useGridEdge() {
  if (!requested) {
    requested = true
    void api.GET('/settings/grid').then(({ data }) => {
      if (data) {
        gridEdge.value = data.gridEdge
      } else {
        requested = false
      }
    })
  }
  return gridEdge
}

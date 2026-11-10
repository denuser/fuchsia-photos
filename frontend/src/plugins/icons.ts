import { h, type FunctionalComponent } from 'vue'
import type { IconAliases, IconProps, IconSet } from 'vuetify'

// Icons from Font Awesome Free (https://fontawesome.com, icons under CC BY 4.0),
// the set PatternFly uses. Only the SVG files named in the patterns below reach
// the bundle. Templates name an icon "fa-<name>" for the solid style and
// "fa-regular-<name>" for the regular one, as in icon="fa-trash-can".
const files = import.meta.glob<string>(
  [
    '/node_modules/@fortawesome/fontawesome-free/svgs/solid/{angle-down,angle-left,angle-right,angle-up,arrow-down,arrow-rotate-left,arrow-up,arrows-up-down,backward-step,bars,book-open,caret-down,caret-right,check,circle-check,circle-exclamation,circle-half-stroke,circle-info,circle-minus,circle-user,circle-xmark,clone,cloud-arrow-up,copy,download,ellipsis-vertical,file-import,filter,forward-step,gear,image,images,key,link,link-slash,magnifying-glass,minus,moon,paperclip,pen,plus,right-from-bracket,rotate,share-nodes,square-check,square-minus,star,star-half,sun,tag,tags,trash-can,trash-can-arrow-up,triangle-exclamation,upload,user,user-gear,user-plus,user-shield,users,xmark}.svg',
    '/node_modules/@fortawesome/fontawesome-free/svgs/regular/{calendar-days,circle,circle-dot,clock,square,star}.svg',
  ],
  { query: '?raw', import: 'default', eager: true },
)

interface Glyph {
  viewBox: string
  paths: string[]
}

const glyphs = new Map<string, Glyph>()
for (const [path, svg] of Object.entries(files)) {
  const [, style, name] = /\/(solid|regular)\/([a-z0-9-]+)\.svg$/.exec(path)!
  glyphs.set(style === 'solid' ? `fa-${name}` : `fa-regular-${name}`, {
    viewBox: /viewBox="([^"]+)"/.exec(svg)![1]!,
    paths: [...svg.matchAll(/\sd="([^"]+)"/g)].map((match) => match[1]!),
  })
}

// Renders a glyph the way Vuetify's own SVG icons are drawn, with the viewBox
// of the Font Awesome file, since its icons are not all square.
const FontAwesomeIcon: FunctionalComponent<IconProps> = (props, { attrs }) => {
  const glyph = glyphs.get(String(props.icon))
  if (!glyph && import.meta.env.DEV) console.warn(`Unknown icon ${String(props.icon)}`)
  return h(props.tag, { ...attrs, style: null }, [
    glyph &&
      h(
        'svg',
        { class: 'v-icon__svg', xmlns: 'http://www.w3.org/2000/svg', viewBox: glyph.viewBox, role: 'img', 'aria-hidden': 'true' },
        glyph.paths.map((d) => h('path', { d })),
      ),
  ])
}

export const fontAwesome: IconSet = { component: FontAwesomeIcon }

// Vuetify's own icons, after its Font Awesome set (vuetify/iconsets/fa), with
// the icons PatternFly uses for alerts, calendars and pagination. Aliases for
// components the application does not use, such as media players, keyboard
// hints and ratings, are left out.
export const aliases: Partial<IconAliases> = {
  collapse: 'fa-angle-up',
  complete: 'fa-check',
  cancel: 'fa-circle-xmark',
  close: 'fa-xmark',
  delete: 'fa-circle-xmark',
  clear: 'fa-circle-xmark',
  success: 'fa-circle-check',
  info: 'fa-circle-info',
  warning: 'fa-triangle-exclamation',
  error: 'fa-circle-exclamation',
  prev: 'fa-angle-left',
  next: 'fa-angle-right',
  checkboxOn: 'fa-square-check',
  checkboxOff: 'fa-regular-square',
  checkboxIndeterminate: 'fa-square-minus',
  sortAsc: 'fa-arrow-up',
  sortDesc: 'fa-arrow-down',
  expand: 'fa-angle-down',
  menu: 'fa-bars',
  subgroup: 'fa-caret-down',
  dropdown: 'fa-caret-down',
  radioOn: 'fa-regular-circle-dot',
  radioOff: 'fa-regular-circle',
  edit: 'fa-pen',
  ratingEmpty: 'fa-regular-star',
  ratingFull: 'fa-star',
  ratingHalf: 'fa-star-half',
  loading: 'fa-rotate',
  first: 'fa-backward-step',
  last: 'fa-forward-step',
  unfold: 'fa-arrows-up-down',
  file: 'fa-paperclip',
  plus: 'fa-plus',
  minus: 'fa-minus',
  calendar: 'fa-regular-calendar-days',
  clock: 'fa-regular-clock',
  treeviewCollapse: 'fa-caret-down',
  treeviewExpand: 'fa-caret-right',
  tableGroupExpand: 'fa-angle-right',
  tableGroupCollapse: 'fa-angle-down',
  upload: 'fa-cloud-arrow-up',
  search: 'fa-magnifying-glass',
}

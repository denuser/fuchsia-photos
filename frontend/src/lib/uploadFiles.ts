// The files of folders the user chose or dropped. The server tells a file by
// its content; inside a folder the browser goes by the name, so that the
// folder's other files, such as .DS_Store, .aae or Takeout JSON, are skipped
// rather than refused one by one. Files chosen or dropped one by one all go to
// the server, which decides.

const EXTENSIONS = ['jpg', 'jpeg', 'jpe', 'jfif', 'png', 'webp', 'heic', 'heif', 'hif', 'mp4', 'm4v', 'mov', 'qt']
const KNOWN = new Set(EXTENSIONS)

// The types ingest accepts, for the file picker.
export const ACCEPT = ['image/jpeg', 'image/png', 'image/webp', 'image/heic', 'image/heif', 'video/mp4', 'video/quicktime', ...EXTENSIONS.map((e) => `.${e}`)].join(',')

export interface Chosen {
  files: File[]
  // The paths of the files left out, inside the chosen or dropped folder.
  skipped: string[]
}

function uploadable(name: string) {
  const dot = name.lastIndexOf('.')
  return dot > 0 && KNOWN.has(name.slice(dot + 1).toLowerCase())
}

// Hidden files and folders are the system's, such as .DS_Store, .Trashes or
// the ._IMG_1.jpg that macOS writes next to a photo on volumes without its own
// metadata. They are left out without being reported as skipped, which would
// only list noise next to each photo.
function hidden(path: string) {
  return path.split('/').some((part) => part.startsWith('.'))
}

export function sift(files: File[]): Chosen {
  const visible = files.filter((f) => !hidden(f.webkitRelativePath || f.name))
  return {
    files: visible.filter((f) => uploadable(f.name)),
    skipped: visible.filter((f) => !uploadable(f.name)).map((f) => f.webkitRelativePath || f.name),
  }
}

// Whether the browser lets the user choose a folder. Phones show the input
// but pick files, or nothing, so the choice is hidden there.
export function canPickFolders() {
  const input = document.createElement('input')
  const phone = /iPhone|iPad|iPod|Android/.test(navigator.userAgent) || (navigator.platform === 'MacIntel' && navigator.maxTouchPoints > 1)
  return 'webkitdirectory' in input && !phone
}

// The files of a drop, with the files inside dropped folders. The entries
// must be taken before the drop handler returns or awaits anything, as the
// browser empties the list afterwards; this function does so before its
// first await. A dropped folder can also appear in dataTransfer.files as an
// empty file, so that list is used only when the browser gives no entries.
export function droppedFiles(data: DataTransfer): Promise<Chosen> {
  const entries = Array.from(data.items)
    .filter((item) => item.kind === 'file')
    .map((item) => item.webkitGetAsEntry())
    .filter((entry): entry is FileSystemEntry => entry !== null)
  if (entries.length === 0) {
    return Promise.resolve({ files: Array.from(data.files), skipped: [] })
  }
  return collect(entries)
}

async function collect(entries: FileSystemEntry[]): Promise<Chosen> {
  const files: File[] = []
  const skipped: string[] = []
  // Entries found inside a folder, as opposed to dropped themselves.
  const nested = new Set<FileSystemEntry>()
  const pending = [...entries]
  while (pending.length > 0) {
    const entry = pending.shift()!
    if (nested.has(entry) && hidden(entry.name)) {
      continue
    }
    if (entry.isDirectory) {
      // A folder the browser cannot read is left out, rather than the whole drop.
      const children = await readAll(entry as FileSystemDirectoryEntry).catch(() => [])
      children.forEach((child) => nested.add(child))
      pending.push(...children)
    } else if (nested.has(entry) && !uploadable(entry.name)) {
      skipped.push(entry.fullPath.replace(/^\//, ''))
    } else {
      try {
        files.push(await new Promise<File>((resolve, reject) => (entry as FileSystemFileEntry).file(resolve, reject)))
      } catch {
        skipped.push(entry.fullPath.replace(/^\//, ''))
      }
    }
  }
  return { files, skipped }
}

// readEntries gives at most 100 entries a call, and an empty list at the end.
async function readAll(folder: FileSystemDirectoryEntry): Promise<FileSystemEntry[]> {
  const reader = folder.createReader()
  const all: FileSystemEntry[] = []
  for (;;) {
    const batch = await new Promise<FileSystemEntry[]>((resolve, reject) => reader.readEntries(resolve, reject))
    if (batch.length === 0) {
      return all
    }
    all.push(...batch)
  }
}

// The main path through the application in a real browser. It runs against the
// stack that `make e2e` starts. Steps build on each other, so the first failure ends the run.
import { mkdirSync } from 'node:fs'
import { dirname, join } from 'node:path'
import { fileURLToPath } from 'node:url'

import { chromium } from 'playwright-core'

const BASE = process.env.E2E_BASE_URL ?? 'http://127.0.0.1:18080'
const PASSWORD = process.env.E2E_PASSWORD
const here = dirname(fileURLToPath(import.meta.url))
const FIXTURES = join(here, '../../backend/internal/media/testdata')
const RESULTS = join(here, 'results')
const PHOTOS = ['landscape.jpg', 'landscape.png', 'landscape.webp', 'landscape.heic', 'rotated.jpg'].map((f) => join(FIXTURES, f))

mkdirSync(RESULTS, { recursive: true })

const browser = await chromium.launch({ channel: 'chrome', headless: !process.env.E2E_HEADED })
// The date fields take dates the way the browser's language writes them, so the language is fixed.
const page = await browser.newPage({ viewport: { width: 1280, height: 800 }, locale: 'en-US' })

const problems = []
let loggedIn = false
page.on('pageerror', (error) => problems.push(`page error: ${error.message}`))
page.on('response', (response) => {
  // Before logging in, the application asks who the user is and gets a 401.
  if (response.status() >= 400 && loggedIn) {
    problems.push(`${response.status()} ${response.request().method()} ${response.url()}`)
  }
})

const tile = (n) => page.getByTestId('tile').nth(n)
const button = (name) => page.getByRole('button', { name, exact: true })
const dialog = () => page.getByRole('dialog')

function waitFor(what, predicate, arg) {
  return page.waitForFunction(predicate, arg, { timeout: 30_000 }).catch(() => {
    throw new Error(`timed out waiting for ${what}`)
  })
}

const tiles = (n) =>
  waitFor(`${n} tiles`, (k) => document.querySelectorAll('[data-testid=tile]').length === k, n)

const previews = (n) =>
  waitFor(`${n} previews`, (k) => document.querySelectorAll('[data-testid=tile] img').length === k, n)

// Does something and waits for the request it must cause. Notices do not prove
// this: one from the previous step may still be on the screen.
async function expectRequest(path, action) {
  await Promise.all([
    page.waitForResponse((r) => r.request().method() !== 'GET' && new URL(r.url()).pathname.endsWith(path) && r.ok(), { timeout: 30_000 }),
    action(),
  ])
}

// Types a day, given as YYYY-MM-DD, into a date field the way en-US writes it.
async function typeDate(label, day) {
  const [year, month, date] = day.split('-')
  const field = page.getByLabel(label, { exact: true })
  await field.fill(`${month}/${date}/${year}`)
  await field.press('Enter')
}

async function select(n) {
  await tile(n).hover()
  await page.getByTestId('tile-check').nth(n).click()
}

async function addTag(name) {
  await button('Tags').click()
  await page.getByLabel('Add tags').fill(name)
  await page.keyboard.press('Enter')
  await expectRequest('/photos/tags', () => button('Save').click())
  await button('Save').waitFor({ state: 'hidden' })
}

// A dialog ignores keys during its opening animation, so this keeps trying.
async function closeWithEscape(content) {
  await content.waitFor()
  for (let attempt = 0; attempt < 20; attempt++) {
    await page.keyboard.press('Escape')
    const closed = await content.waitFor({ state: 'hidden', timeout: 250 }).then(() => true, () => false)
    if (closed) {
      return
    }
  }
  throw new Error('Escape did not close the dialog')
}

async function shareWithGuest() {
  await page.getByLabel('Share with').click()
  await page.getByRole('option', { name: 'guest' }).click()
  // The list of logins covers the dialog's lower part until a click elsewhere closes it.
  await dialog().getByText(/^Share this/).click()
  await expectRequest('/sharing', () => button('Save').click())
  await button('Save').waitFor({ state: 'hidden' })
}

async function moveToTrash() {
  await expectRequest('/photos/trash', () => button('Move to trash').click())
}

async function apiTotal(path) {
  const response = await page.request.get(`${BASE}/api/v1/${path}`)
  return (await response.json()).items.length
}

const steps = {
  'log in': async () => {
    await page.goto(BASE)
    await page.getByLabel('Login').fill('admin')
    await page.getByLabel('Password').fill(PASSWORD)
    await page.keyboard.press('Enter')
    await button('Upload').waitFor()
    loggedIn = true
  },
  'upload photos and get previews': async () => {
    await page.locator('input[type=file]').setInputFiles(PHOTOS)
    await previews(5)
    // The timeline groups photos by month, after reading the dates from EXIF.
    await page.getByTestId('month').first().waitFor()
  },
  'the timeline scrolls in a short window': async () => {
    await page.setViewportSize({ width: 1280, height: 360 })
    try {
      await tile(0).hover()
      await page.mouse.wheel(0, 400)
      await waitFor('the grid to scroll', () => document.querySelector('[data-testid=grid-scroller]').scrollTop > 0)
      await page.mouse.wheel(0, -400)
    } finally {
      await page.setViewportSize({ width: 1280, height: 800 })
    }
  },
  'select a range': async () => {
    await select(0)
    await page.getByTestId('tile-check').nth(2).click({ modifiers: ['Shift'] })
    await page.getByText('3 selected').waitFor()
  },
  'add the selection to a new album': async () => {
    await button('Add to album').click()
    await page.getByLabel('New album').fill('Trip')
    await expectRequest('/photos', () => button('Create').click())
    await page.getByText('3 selected').waitFor({ state: 'hidden' })
  },
  'tag photos': async () => {
    await select(0)
    await select(1)
    await addTag('Sea')
    await select(4)
    await addTag('City')
  },
  'filter by one tag from the tags page': async () => {
    await page.goto(`${BASE}/tags`)
    await page.getByRole('link', { name: 'Sea', exact: true }).click()
    await tiles(2)
  },
  'a second tag widens the search': async () => {
    await page.getByRole('combobox', { name: 'Any of these tags' }).click()
    await page.getByRole('option', { name: 'City' }).click()
    await page.keyboard.press('Escape')
    await tiles(3)
    // The chosen tags show as chips under the toolbar, and removing one narrows the search again.
    await button('Remove City').click()
    await tiles(2)
  },
  'filter by date': async () => {
    await typeDate('From', '2000-01-01')
    await typeDate('To', '2000-01-02')
    await page.getByText('No photos match these filters').waitFor()
    // The empty state offers the same way out as the chips above it.
    await page.getByTestId('empty-state').getByRole('button', { name: 'Clear all filters' }).click()
    await tiles(5)
  },
  'filter by camera': async () => {
    await page.getByRole('combobox', { name: 'Any of these cameras' }).click()
    await page.getByRole('option', { name: /Canon EOS R5/ }).click()
    await page.keyboard.press('Escape')
    await tiles(1)
    // The other filters leave the chosen camera no photos, and it stays in its list to be removed.
    await typeDate('From', '2000-01-01')
    await typeDate('To', '2000-01-02')
    await page.getByRole('combobox', { name: 'Any of these cameras' }).click()
    await page.getByRole('option', { name: /Canon EOS R5.*0 photos/ }).waitFor()
    await page.keyboard.press('Escape')
    await page.getByTestId('applied-filters').getByRole('button', { name: 'Clear all filters' }).click()
    await tiles(5)
  },
  'without the ML service, search says so and similar photos open from the viewer': async () => {
    await page.goto(`${BASE}/search`)
    await page.getByText('Search by description is unavailable').waitFor()
    if (await page.getByLabel('Search by description').isEnabled()) {
      throw new Error('the search field is enabled without the ML service')
    }
    await page.goto(BASE)
    await page.getByTestId('tile-open').first().click()
    await page.getByTestId('viewer').waitFor()
    await button('Similar photos').click()
    await page.waitForURL(/\/search\?similar=\d+$/)
    await page.getByText('No similar photos yet').waitFor()
  },
  'open the album and choose a cover': async () => {
    await page.goto(`${BASE}/albums`)
    await page.getByText('Trip').click()
    await tiles(3)
    await select(2)
    await expectRequest('/cover', () => button('Set as cover').click())
  },
  'remove a photo from the album': async () => {
    await select(0)
    await expectRequest('/photos/remove', () => button('Remove from album').click())
    await tiles(2)
  },
  'share an album and a photo with a second user': async () => {
    const created = await page.request.post(`${BASE}/api/v1/users`, { data: { login: 'guest', password: PASSWORD, role: 'user' } })
    if (!created.ok()) {
      throw new Error(`creating the second user answered ${created.status()}`)
    }
    await button('Album actions').click()
    await page.locator('.v-list-item', { hasText: /^Share$/ }).click()
    await shareWithGuest()

    await page.goto(BASE)
    await previews(5)
    await select(4)
    await button('Share').click()
    await shareWithGuest()
    await page.getByText('1 selected').waitFor({ state: 'hidden' })
  },
  'the second user sees them, read-only': async () => {
    const context = await browser.newContext({ viewport: { width: 1280, height: 800 }, locale: 'en-US' })
    const guest = await context.newPage()
    guest.on('pageerror', (error) => problems.push(`second user's page error: ${error.message}`))
    await guest.goto(BASE)
    await guest.getByLabel('Login').fill('guest')
    await guest.getByLabel('Password').fill(PASSWORD)
    await guest.keyboard.press('Enter')
    await guest.getByRole('button', { name: 'Upload', exact: true }).waitFor()

    const guestTiles = (n) =>
      guest.waitForFunction((k) => document.querySelectorAll('[data-testid=tile] img').length === k, n, { timeout: 30_000 }).catch(() => {
        throw new Error(`timed out waiting for the second user to see ${n} photos`)
      })

    // The timeline shows what was shared next to the user's own photos, which the guest has none of.
    await guestTiles(3)
    if ((await guest.getByTestId('tile-check').count()) > 0) {
      throw new Error('another user\'s photos can be selected in the timeline')
    }
    await guest.getByRole('button', { name: 'Mine', exact: true }).click()
    await guest.getByText('No photos yet').waitFor()
    await guest.getByRole('button', { name: 'All', exact: true }).click()
    await guestTiles(3)

    await guest.goto(`${BASE}/shared`)
    await guest.getByText('Trip').click()
    await guestTiles(2)
    if ((await guest.getByTestId('tile-check').count()) > 0) {
      throw new Error('another user\'s photos can be selected')
    }

    await guest.goto(`${BASE}/shared?tab=photos`)
    await guestTiles(1)
    await guest.getByLabel('Include photos from shared albums').click()
    await guestTiles(3)

    await guest.getByTestId('tile-open').first().click()
    await guest.getByTestId('viewer').waitFor()
    if ((await guest.getByRole('button', { name: 'Move to trash', exact: true }).count()) > 0) {
      throw new Error('another user\'s photo offers to be moved to the trash')
    }
    await context.close()
  },
  'an administrator sees the photos of all users': async () => {
    await page.goto(`${BASE}/all`)
    await previews(5)
  },
  'trash a photo from the viewer and stay in the viewer': async () => {
    await page.goto(BASE)
    await previews(5)
    await page.getByTestId('tile-open').first().click()
    const viewer = page.getByTestId('viewer')
    await viewer.waitFor()

    // A swipe to the left shows the next photo, one to the right the previous.
    const swipe = (dx) =>
      page.getByTestId('viewer-image').evaluate((image, dx) => {
        const at = (type, x) =>
          image.dispatchEvent(new PointerEvent(type, { bubbles: true, pointerId: 7, pointerType: 'touch', isPrimary: true, clientX: x, clientY: 400 }))
        at('pointerdown', 640)
        at('pointermove', 640 + dx / 2)
        at('pointerup', 640 + dx)
      }, dx)
    const first = await viewer.getAttribute('aria-label')
    await swipe(-200)
    await waitFor('the next photo', (name) => document.querySelector('[data-testid=viewer]')?.ariaLabel !== name, first)
    await swipe(200)
    await waitFor('the first photo again', (name) => document.querySelector('[data-testid=viewer]')?.ariaLabel === name, first)

    // Escape closes a dialog over the viewer, not the viewer itself.
    await button('Tags').click()
    await closeWithEscape(page.getByLabel('Add tags'))
    if (!(await viewer.isVisible())) {
      throw new Error('Escape closed the viewer behind the dialog')
    }

    await moveToTrash()
    await viewer.waitFor()
    await page.keyboard.press('Escape')
    await tiles(4)
  },
  'restore from the trash': async () => {
    await page.goto(`${BASE}/trash`)
    await tiles(1)
    await select(0)
    await expectRequest('/photos/restore', () => button('Restore').click())
    await tiles(0)
  },
  'uploading a trashed photo restores it': async () => {
    await page.goto(BASE)
    await previews(5)
    await select(0)
    await moveToTrash()
    await tiles(4)
    await page.locator('input[type=file]').setInputFiles(PHOTOS)
    await page.getByText('1 restored from the trash').waitFor({ timeout: 30_000 })
    await tiles(5)
  },
  'delete a photo permanently': async () => {
    await select(0)
    await moveToTrash()
    await page.goto(`${BASE}/trash`)
    await tiles(1)
    await select(0)
    await button('Delete').click()
    await expectRequest('/photos/purge', () => dialog().getByRole('button', { name: 'Delete' }).click())
    await page.getByText('The trash is empty').first().waitFor()
  },
  'empty the trash': async () => {
    await page.goto(BASE)
    await previews(4)
    await select(0)
    await moveToTrash()
    await page.goto(`${BASE}/trash`)
    await tiles(1)
    await button('Empty trash').click()
    await expectRequest('/photos/trash/empty', () => dialog().getByRole('button', { name: 'Empty trash' }).click())
    await page.getByText('The trash is empty').first().waitFor()
  },
  'an administrator sees the job queue and changes the preview settings': async () => {
    await page.goto(`${BASE}/admin`)
    // The uploads so far were processed, and the queue streams in over SSE.
    await page.getByTestId('queue').getByRole('cell', { name: 'Photo processing', exact: true }).waitFor()
    await page.getByLabel('Grid preview size').fill('300')
    await expectRequest('/settings/previews', () => button('Save').click())
    await page.reload()
    await waitFor('the saved size', () => document.querySelector('[data-testid=grid-edge] input')?.value === '300')
  },
  'an administrator imports a folder for the second user, twice': async () => {
    const scanFolder = async (runs, column, count) => {
      await expectRequest('/imports', () => button('Start scan').click())
      // The newest run comes first, once the list has reloaded.
      await waitFor(`${runs} scans`, (k) => document.querySelectorAll('[data-testid=import-run]').length === k, runs)
      const run = page.getByTestId('import-run').first()
      await run.getByText('Finished').waitFor({ timeout: 60_000 })
      const got = await run.getByTestId(column).textContent()
      if (got.trim() !== String(count)) {
        throw new Error(`${column} = ${got}, want ${count}`)
      }
    }
    await page.goto(`${BASE}/import`)
    await page.getByLabel('Owner').click()
    await page.getByRole('option', { name: 'guest' }).click()
    // The fixtures hold two photos and three videos more than the upload uses.
    await scanFolder(1, 'added', PHOTOS.length + 5)
    await scanFolder(2, 'duplicates', PHOTOS.length + 5)
  },
  'an administrator shows where photos were taken': async () => {
    const location = page.getByTestId('viewer').getByRole('link', { name: '59.93000, 30.33000' })
    const openRotated = async () => {
      await page.goto(`${BASE}/all`)
      await page.getByLabel('rotated.jpg', { exact: true }).first().click()
      await page.getByTestId('viewer').waitFor()
    }
    await openRotated()
    if ((await location.count()) > 0) {
      throw new Error('the location is shown before an administrator allows it')
    }
    await page.goto(`${BASE}/admin`)
    await page.getByLabel('Show where photos were taken').check()
    await expectRequest('/settings/metadata', () => button('Save').click())
    await openRotated()
    await location.waitFor()
  },
  'zoom a photo to its own resolution, then choose originals for zooming and details in the profile': async () => {
    const image = page.getByTestId('viewer-image')
    const source = (want) =>
      waitFor(`the ${want} in the viewer`, (w) => document.querySelector('[data-testid=viewer-image]')?.dataset.source === w, want)
    const open = async (name) => {
      await page.goto(`${BASE}/all`)
      await page.getByLabel(name, { exact: true }).first().click()
      await image.waitFor()
    }

    // Without the profile setting, zooming enlarges the preview, no further
    // than the preview's own resolution, and never asks for the original.
    // large.jpg is 2400 × 1800, more than the screen and the viewer preview.
    const originals = []
    const countOriginals = (request) => request.url().endsWith('/original') && originals.push(request.url())
    page.on('request', countOriginals)
    await open('large.jpg')
    await source('preview')
    await image.click()
    await waitFor('the zoom', () => getComputedStyle(document.querySelector('[data-testid=viewer-image]')).transform !== 'none')
    await page.waitForTimeout(500)
    page.off('request', countOriginals)
    if (originals.length > 0) {
      throw new Error(`the viewer loaded originals the user did not ask for: ${originals.join(', ')}`)
    }
    await source('preview')
    await page.keyboard.press('Escape')
    await waitFor('the zoom to end', () => /^(none|matrix\(1, 0, 0, 1, 0, 0\))$/.test(getComputedStyle(document.querySelector('[data-testid=viewer-image]')).transform))
    await page.getByTestId('viewer').waitFor()

    // On a phone a tap and a pinch keep the point of the photo under the finger
    // in place, and a pinch scales with the distance between the fingers.
    await page.setViewportSize({ width: 375, height: 740 })
    try {
      await open('large.jpg')
      const settled = () =>
        waitFor('the zoom to settle', () => document.querySelector('[data-testid=viewer-image]').getAnimations().length === 0)
      const box = () => image.evaluate((i) => i.getBoundingClientRect().toJSON())
      const check = (what, before, after, point, scale) => {
        const share = (b) => [(point.x - b.x) / b.width, (point.y - b.y) / b.height]
        const [a, b] = [share(before), share(after)]
        if (Math.abs(a[0] - b[0]) > 0.01 || Math.abs(a[1] - b[1]) > 0.01 || Math.abs(after.width / before.width - scale) > 0.05) {
          throw new Error(`${what}: ${JSON.stringify({ before, after, point })}`)
        }
      }

      let before = await box()
      const previewWidth = await image.evaluate((i) => i.naturalWidth)
      if (previewWidth >= 2400) {
        throw new Error(`the viewer preview of large.jpg is ${previewWidth} px wide, as wide as the original`)
      }
      const tap = { x: before.x + before.width * 0.2, y: before.y + before.height * 0.3 }
      await page.mouse.click(tap.x, tap.y)
      await waitFor('the zoom', () => document.querySelector('[data-testid=viewer-image]').style.transform !== '')
      await settled()
      check('a tap zooms at the tapped point to the preview resolution', before, await box(), tap, previewWidth / before.width)
      await page.keyboard.press('Escape')
      await waitFor('the zoom to end', () => document.querySelector('[data-testid=viewer-image]').style.transform === '')
      await settled()

      before = await box()
      const pinch = { x: 120, y: 370 }
      await image.evaluate((image, { x, y }) => {
        const at = (type, id, dx) =>
          image.dispatchEvent(new PointerEvent(type, { bubbles: true, pointerId: id, pointerType: 'touch', clientX: x + dx, clientY: y }))
        at('pointerdown', 1, -50)
        at('pointerdown', 2, 50)
        for (const d of [60, 80, 100]) {
          at('pointermove', 1, -d)
          at('pointermove', 2, d)
        }
        at('pointerup', 1, -100)
        at('pointerup', 2, 100)
      }, pinch)
      check('a pinch zooms between the fingers', before, await box(), pinch, 2)
    } finally {
      await page.setViewportSize({ width: 1280, height: 800 })
    }

    await page.goto(`${BASE}/profile`)
    await expectRequest('/me', () => page.getByLabel('Load originals when zooming in').click())
    await expectRequest('/me', () => page.getByLabel('Location', { exact: true }).click())
    // The setting loads the original only for zooming.
    await open('large.jpg')
    await page.waitForTimeout(500)
    await source('preview')
    await image.click()
    await source('original')
    await waitFor('the zoom to the original resolution', () =>
      Math.abs(document.querySelector('[data-testid=viewer-image]').getBoundingClientRect().width - 2400) < 2)
    await open('rotated.jpg')
    await source('preview')
    if ((await page.getByTestId('viewer').getByRole('link', { name: '59.93000, 30.33000' }).count()) > 0) {
      throw new Error('the viewer shows the location the user hid')
    }
  },
  'on a phone the menu opens over the page': async () => {
    await page.setViewportSize({ width: 375, height: 740 })
    try {
      await page.goto(BASE)
      await previews(3)
      // A closed menu stays in the page, moved off the left edge of the screen.
      const albums = page.getByRole('link', { name: 'Albums' })
      const menu = (shown) =>
        waitFor(shown ? 'the menu' : 'the menu to hide', (want) => {
          const box = [...document.querySelectorAll('a')].find((a) => a.ariaLabel === 'Albums')?.getBoundingClientRect()
          return box && box.right > 0 === want
        }, shown)
      await menu(false)
      await button('Expand menu').click()
      await menu(true)
      await albums.click()
      await page.waitForURL(`${BASE}/albums`)
      await menu(false)
    } finally {
      await page.setViewportSize({ width: 1280, height: 800 })
    }
  },
  'square previews from the profile take the grid preview size': async () => {
    await page.goto(`${BASE}/profile`)
    await expectRequest('/me', () => page.getByLabel('Show square previews').click())
    await page.goto(BASE)
    await previews(3)
    // The administrator set the grid preview size to 300 pixels.
    await waitFor('square tiles no larger than the preview', () => {
      const boxes = [...document.querySelectorAll('[data-testid=tile]')].map((t) => t.getBoundingClientRect())
      const side = 300 / window.devicePixelRatio
      return boxes.length > 0 && boxes.every((b) => Math.abs(b.width - b.height) < 1 && b.width <= side && b.width > side / 2)
    })
  },
  'the profile switches the interface to Russian, kept after a reload, and back': async () => {
    await page.goto(`${BASE}/profile`)
    // A select's input sits under the field's own box, which takes the click.
    await page.getByLabel('Interface language').locator('..').click()
    await expectRequest('/me', () => page.getByRole('option', { name: 'Русский' }).click())
    await page.getByText('Язык интерфейса').waitFor()
    await page.goto(`${BASE}/albums`)
    await page.getByRole('heading', { name: 'Альбомы', level: 1 }).waitFor()
    await waitFor('the page language is Russian', () => document.documentElement.lang === 'ru')

    await page.goto(`${BASE}/profile`)
    // A select's input sits under the field's own box, which takes the click.
    await page.getByLabel('Язык интерфейса').locator('..').click()
    await expectRequest('/me', () => page.getByRole('option', { name: 'Автоматически' }).click())
    // The browser is set to en-US, so Automatic means English.
    await page.getByText('Interface language').waitFor()
  },
  'a video plays in the viewer, and a motion photo plays its motion': async () => {
    await page.goto(BASE)
    await page.locator('input[type=file]').setInputFiles(['video.mp4', 'motion.jpg'].map((f) => join(FIXTURES, f)))
    await previews(5)
    await page.getByTestId('tile-video').getByText('0:02').waitFor()
    await page.getByTestId('tile-motion').waitFor()
    // Every file here was processed, so none may be marked as failed.
    const failed = await page.getByTestId('tile-failed').count()
    if (failed > 0) {
      throw new Error(`${failed} processed photos are marked as failed`)
    }

    await page.getByLabel('video.mp4', { exact: true }).click()
    const video = page.getByTestId('viewer-video')
    await video.waitFor()
    // The camera details show at the bottom, and must leave the video's controls clear.
    await page.getByTestId('camera').getByText('iPhone 15').waitFor()
    const covered = await video.evaluate((v) => {
      const box = v.getBoundingClientRect()
      return document.elementFromPoint(box.left + box.width / 2, box.bottom - 20) !== v
    })
    if (covered) {
      throw new Error('the bar with the camera details covers the controls of the video')
    }
    await video.evaluate((v) => v.play())
    await waitFor('the video to play', () => document.querySelector('[data-testid=viewer-video]')?.currentTime > 0)
    await page.keyboard.press('Escape')

    await page.getByLabel('motion.jpg', { exact: true }).click()
    await page.getByTestId('viewer-motion-button').click()
    await page.getByTestId('viewer-motion').waitFor()
    // The motion lasts a second, and the photo comes back after it.
    await waitFor('the motion to end', () => !document.querySelector('[data-testid=viewer-motion]'))
    await page.keyboard.press('Escape')

    // The type filter keeps the video alone.
    await page.getByRole('combobox', { name: 'Any of these types' }).click()
    await page.getByRole('option', { name: /Videos.*1 photo/ }).click()
    await page.keyboard.press('Escape')
    await tiles(1)
    await page.getByTestId('applied-filters').getByRole('button', { name: 'Clear all filters' }).click()
    await tiles(5)
  },
  'the server agrees with the screen': async () => {
    const [library, trash] = [await apiTotal('photos'), await apiTotal('photos/trash')]
    if (library !== 5 || trash !== 0) {
      throw new Error(`the library has ${library} photos and the trash ${trash}, want 5 and 0`)
    }
  },
}

let failed = false
for (const [name, run] of Object.entries(steps)) {
  try {
    await run()
    console.log(`ok    ${name}`)
  } catch (error) {
    failed = true
    const shot = join(RESULTS, 'failure.png')
    await page.screenshot({ path: shot }).catch(() => {})
    console.error(`FAIL  ${name}\n      ${String(error.message).split('\n')[0]}\n      Screenshot: ${shot}`)
    break
  }
}

if (problems.length > 0) {
  failed = true
  console.error(`The browser reported problems:\n${[...new Set(problems)].map((p) => `  ${p}`).join('\n')}`)
}

await browser.close()
process.exit(failed ? 1 : 0)

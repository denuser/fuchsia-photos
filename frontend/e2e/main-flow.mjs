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
const page = await browser.newPage({ viewport: { width: 1280, height: 800 } })

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
    await page.getByRole('link', { name: 'Sea' }).click()
    await tiles(2)
  },
  'a second tag widens the search': async () => {
    await page.getByLabel('Any of these tags').click()
    await page.getByRole('option', { name: 'City' }).click()
    await page.keyboard.press('Escape')
    await tiles(3)
  },
  'filter by date': async () => {
    await page.getByLabel('From', { exact: true }).fill('2000-01-01')
    await page.getByLabel('To', { exact: true }).fill('2000-01-02')
    await page.getByText('No photos match these filters').waitFor()
    await button('Clear').click()
    await tiles(5)
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
  'trash a photo from the viewer and stay in the viewer': async () => {
    await page.goto(BASE)
    await previews(5)
    await page.getByTestId('tile-open').first().click()
    const viewer = page.getByTestId('viewer')
    await viewer.waitFor()

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
  'the server agrees with the screen': async () => {
    const [library, trash] = [await apiTotal('photos'), await apiTotal('photos/trash')]
    if (library !== 3 || trash !== 0) {
      throw new Error(`the library has ${library} photos and the trash ${trash}, want 3 and 0`)
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

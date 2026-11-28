<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'
import { api, previewUrl, type Photo } from '../api/client'
import type { components } from '../api/schema'
import AppPageHeader from '../components/AppPageHeader.vue'
import EmptyState from '../components/EmptyState.vue'
import PhotoViewer from '../components/PhotoViewer.vue'
import { t } from '../i18n'

type Item = components['schemas']['PersonItem']
const groups = ref<Item[]>([])
const members = ref<Item[]>([])
const current = ref<Item>()
const viewing = ref<Photo>()
const name = ref('')
const target = ref(0)
const busy = ref(false)
const failed = ref(false)
const more = ref(false)
const photos = computed(() => {
  const result = new Map<number, Item[]>()
  for (const face of members.value) {
    const group = result.get(face.photo)
    if (group) group.push(face)
    else result.set(face.photo, [face])
  }
  return result
})
const label = (item: Item) => item.name || t('people.unnamed') + ' ' + item.id
const choices = computed(() => [{title:t('people.new'),value:0}, ...groups.value.filter(item => item.id !== current.value?.id).map(item => ({title:label(item),value:item.id}))])

async function page(person?: number, after?: number) {
  const result = await api.GET('/people', {params:{query:{person,after}}})
  if (!result.data) throw new Error()
  return result.data
}
async function run(work: () => Promise<void>) {
  busy.value = true
  failed.value = false
  try { await work() } catch { failed.value = true } finally { busy.value = false }
}
async function loadGroups() {
  const rows: Item[] = []
  let batch: Item[]
  do {
    batch = await page(undefined, rows.at(-1)?.id)
    rows.push(...batch)
  } while (batch.length === 100)
  groups.value = rows
}
async function open(item?: Item) {
  const batch = item ? await page(item.id) : []
  current.value = item
  name.value = item?.name || ''
  target.value = 0
  members.value = batch
  more.value = batch.length === 100
}
async function viewPhoto(id: number) {
  await run(async () => {
    const result = await api.GET('/photos/{id}', { params: { path: { id } } })
    if (!result.data) throw new Error()
    viewing.value = result.data
  })
}
async function loadMore() {
  const batch = await page(current.value?.id, members.value.at(-1)?.id)
  members.value.push(...batch)
  more.value = batch.length === 100
}
async function edit(action: 'rename'|'merge'|'move'|'ignore', face?: number) {
  const person = current.value
  if (!person) return
  await run(async () => {
    const result = await api.POST('/people',{body:{action,person:person.id,face,target:target.value,name:name.value}})
    if (!result.response.ok) throw new Error()
    await loadGroups()
    await open(groups.value.find(item => item.id === (action === 'merge' ? target.value : person.id)))
  })
}
onMounted(() => run(loadGroups))
</script>

<template>
  <div class="page">
    <AppPageHeader :title="current ? label(current) : t('people.title')" />
    <div class="page-section">
      <v-btn v-if="current" :disabled="busy" @click="run(() => open())">{{ t('people.title') }}</v-btn>
      <v-progress-linear v-if="busy" indeterminate />
      <v-btn v-if="failed" @click="run(async () => { await loadGroups(); await open(current) })">{{ t('common.retry') }}</v-btn>
      <EmptyState v-if="!busy && !failed && !groups.length" icon="fa-users" :title="t('people.empty')" />
      <template v-if="current">
        <div class="controls">
          <v-text-field v-model="name" :label="t('common.name')" maxlength="64" hide-details />
          <v-btn :disabled="busy" @click="edit('rename')">{{ t('common.save') }}</v-btn>
          <v-select v-model="target" :items="choices" :label="t('people.merge')" hide-details />
          <v-btn :disabled="busy || !target" @click="edit('merge')">{{ t('people.merge') }}</v-btn>
        </div>
        <div class="cards">
          <article v-for="[photo, faces] in photos" :key="photo">
            <button class="open-photo" :disabled="busy" @click="viewPhoto(photo)">
              <img class="scene" :src="previewUrl(photo,'viewer',faces[0]!.version)" :alt="label(current)" loading="lazy">
            </button>
            <div v-for="face in faces" :key="face.id" class="face-actions">
              <img class="portrait" :src="`/api/v1/people/faces/${face.face}/thumbnail`" width="48" height="48" alt="" loading="lazy">
              <v-btn size="small" :disabled="busy" @click="edit('move',face.id)">{{ t('people.move') }}</v-btn>
              <v-btn size="small" :disabled="busy" @click="edit('ignore',face.id)">{{ t('people.ignore') }}</v-btn>
            </div>
          </article>
        </div>
        <v-btn v-if="more" :disabled="busy" @click="run(loadMore)">{{ t('people.more') }}</v-btn>
      </template>
      <div v-else class="cards">
        <button v-for="group in groups" :key="group.id" :disabled="busy" @click="run(() => open(group))">
          <img class="portrait" :src="`/api/v1/people/faces/${group.face}/thumbnail`" width="104" height="104" alt="" loading="lazy">
          <span>{{ label(group) }}</span>
        </button>
      </div>
    </div>
    <PhotoViewer v-if="viewing" :photo="viewing" :photos="[viewing]" :can-tag="false" :can-manage="false"
      :can-find-similar="false" :can-archive="false" :paused="false" @close="viewing = undefined" />
  </div>
</template>

<style scoped>
.controls,.face-actions { display:flex; flex-wrap:wrap; gap:8px; align-items:center; margin-block:12px }
.controls > .v-input { min-width:180px }
.cards { display:grid; grid-template-columns:repeat(auto-fill,minmax(min(240px,100%),1fr)); gap:16px }
.cards > button { display:grid; justify-items:center; gap:8px; padding:12px }
.portrait { border-radius:50%; object-fit:cover }
.open-photo { width:100% }
.scene { width:100%; aspect-ratio:4/3; object-fit:contain }
</style>

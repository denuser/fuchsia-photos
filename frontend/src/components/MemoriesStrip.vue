<script setup lang="ts">
import { computed, onMounted, ref } from 'vue'

import { api, previewUrl, type MemoryYear } from '../api/client'
import { formatDate, t } from '../i18n'
import { today } from '../lib/today'

// A row of cards above the timeline, one per earlier year that has photos from
// this day or the days around it. It stays hidden while there is nothing to
// show, so that it takes no height from the photos.
const years = ref<MemoryYear[]>([])

onMounted(async () => {
  const tz = Intl.DateTimeFormat().resolvedOptions().timeZone || 'UTC'
  const { data } = await api.GET('/photos/memories', { params: { query: { tz, date: today() } } })
  years.value = data?.years ?? []
})

const title = computed(() => (years.value.some((y) => y.exact) ? t('memories.today') : t('memories.theseDays')))

function when(year: MemoryYear) {
  const date = formatDate(`${year.date}T12:00:00`)
  return year.exact ? date : t('memories.around', { date })
}
</script>

<template>
  <section v-if="years.length > 0" class="memories" data-testid="memories-strip" :aria-label="title">
    <div class="head">
      <h2>{{ title }}</h2>
      <router-link to="/memories">{{ t('memories.seeAll') }}</router-link>
    </div>
    <div class="cards">
      <router-link v-for="year in years" :key="year.year" to="/memories" class="card" data-testid="memory-card">
        <img :src="previewUrl(year.photos[0]!.id, 'grid', year.photos[0]!.previewVersion)" alt="" loading="lazy" />
        <span class="caption">
          <span class="ago">{{ t('memories.yearsAgo', { n: year.yearsAgo }, year.yearsAgo) }}</span>
          <span class="date">{{ when(year) }}</span>
        </span>
      </router-link>
    </div>
  </section>
</template>

<style scoped>
.memories {
  flex: none;
  padding: 8px 24px 4px;
}

.head {
  display: flex;
  align-items: baseline;
  justify-content: space-between;
  gap: 16px;
  margin-bottom: 8px;
}

.cards {
  display: flex;
  gap: 8px;
  overflow-x: auto;
  padding-bottom: 8px;
}

.card {
  position: relative;
  flex: none;
  width: 112px;
  height: 148px;
  overflow: hidden;
  border-radius: var(--radius-small, 6px);
  background: rgb(var(--v-theme-surface-variant));
  color: #fff;
  text-decoration: none;
}

.card img {
  display: block;
  width: 100%;
  height: 100%;
  object-fit: cover;
}

.caption {
  position: absolute;
  inset: auto 0 0;
  display: flex;
  flex-direction: column;
  padding: 24px 8px 8px;
  background: linear-gradient(transparent, rgb(0 0 0 / 0.7));
}

.ago {
  font-weight: 600;
}

.date {
  font-size: 0.85em;
}

@media (max-width: 599px) {
  .memories {
    padding-inline: 16px;
  }
}
</style>

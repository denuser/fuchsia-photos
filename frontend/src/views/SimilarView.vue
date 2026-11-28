<script setup lang="ts">
import { computed, onMounted, ref } from "vue";

import { api, previewUrl, type Photo } from "../api/client";
import AppPageHeader from "../components/AppPageHeader.vue";
import ConfirmDialog from "../components/ConfirmDialog.vue";
import EmptyState from "../components/EmptyState.vue";
import { formatBytes, formatDate, formatDateTime, t } from "../i18n";
import { apiError } from "../i18n/apiErrors";
import { useNoticesStore } from "../stores/notices";

type Tier = "duplicate" | "similar";

// keep is the photo the group's radio buttons have chosen. It starts with the first one, the largest.
interface Group {
  photos: Photo[];
  keep: number;
}

const notices = useNoticesStore();

const tab = ref<Tier>("duplicate");
const groups = ref<Record<Tier, Group[]>>({ duplicate: [], similar: [] });
const totals = ref<Record<Tier, number>>({ duplicate: 0, similar: 0 });
const loading = ref(true);
const error = ref("");

const confirming = ref<Group | null>(null);
const confirmOpen = ref(false);
const busy = ref(false);

const shown = computed(() => groups.value[tab.value]);
const hidden = computed(() => totals.value[tab.value] - shown.value.length);

async function load(tier: Tier) {
  const { data, error: failure } = await api.GET("/similar", {
    params: { query: { tier } },
  });
  if (failure || !data) {
    error.value = t("similar.loadFailed");
    return;
  }
  groups.value[tier] = data.groups.map((g) => ({
    photos: g.photos,
    keep: g.photos[0].id,
  }));
  totals.value[tier] = data.total;
}

onMounted(async () => {
  await Promise.all([load("duplicate"), load("similar")]);
  loading.value = false;
});

function drop(group: Group) {
  const list = groups.value[tab.value];
  list.splice(list.indexOf(group), 1);
  totals.value[tab.value] -= 1;
}

function startResolve(group: Group) {
  confirming.value = group;
  confirmOpen.value = true;
}

async function resolve() {
  const group = confirming.value;
  if (!group || busy.value) {
    return;
  }
  busy.value = true;
  const removeIds = group.photos
    .filter((p) => p.id !== group.keep)
    .map((p) => p.id);
  const { error: failure } = await api.POST("/similar/resolve", {
    body: { keepId: group.keep, removeIds },
  });
  busy.value = false;
  if (failure) {
    notices.notify(apiError(failure), "error");
    return;
  }
  drop(group);
  notices.notify(t("similar.resolved", { n: removeIds.length }), "success");
}

async function dismiss(group: Group) {
  if (busy.value) {
    return;
  }
  busy.value = true;
  const { error: failure } = await api.POST("/similar/dismiss", {
    body: { ids: group.photos.map((p) => p.id) },
  });
  busy.value = false;
  if (failure) {
    notices.notify(apiError(failure), "error");
    return;
  }
  drop(group);
  notices.notify(t("similar.dismissed"), "success");
}

const confirmText = computed(() => {
  const n = confirming.value ? confirming.value.photos.length - 1 : 0;
  return t("similar.confirmText", { n }, n);
});

function when(photo: Photo) {
  return photo.takenAt
    ? formatDateTime(photo.takenAt)
    : formatDate(photo.createdAt);
}
</script>

<template>
  <div class="page">
    <AppPageHeader
      :title="t('similar.title')"
      :description="t('similar.description')"
    />

    <v-tabs v-model="tab" class="px-4 px-sm-6">
      <v-tab value="duplicate"
        >{{ t("similar.duplicates") }} ({{ totals.duplicate }})</v-tab
      >
      <v-tab value="similar"
        >{{ t("similar.shots") }} ({{ totals.similar }})</v-tab
      >
    </v-tabs>

    <div class="page-section">
      <p class="text-medium-emphasis">
        {{
          tab === "duplicate"
            ? t("similar.duplicatesHint")
            : t("similar.shotsHint")
        }}
      </p>
      <v-alert
        v-if="error"
        type="error"
        density="compact"
        class="mb-4"
        :title="error"
      />

      <v-progress-linear v-if="loading" indeterminate />
      <EmptyState
        v-else-if="shown.length === 0 && !error"
        icon="fa-clone"
        :title="t('similar.empty')"
        :text="t('similar.emptyText')"
      />

      <section
        v-for="group in shown"
        :key="group.photos[0].id"
        class="group"
        data-testid="similar-group"
      >
        <div class="group-head">
          <h2>
            {{
              t(
                "similar.groupSize",
                { n: group.photos.length },
                group.photos.length,
              )
            }}
          </h2>
          <div class="group-actions">
            <v-btn
              color="primary"
              :disabled="busy"
              @click="startResolve(group)"
              >{{ t("similar.resolve") }}</v-btn
            >
            <v-btn variant="text" :disabled="busy" @click="dismiss(group)">{{
              t("similar.dismiss")
            }}</v-btn>
          </div>
        </div>
        <div class="tiles" role="radiogroup" :aria-label="t('similar.keep')">
          <div
            v-for="(photo, index) in group.photos"
            :key="photo.id"
            class="tile"
          >
            <a
              :href="`/api/v1/photos/${photo.id}/previews/viewer`"
              target="_blank"
              rel="noopener"
              :aria-label="t('similar.open', { name: photo.name })"
            >
              <img
                :src="previewUrl(photo.id, 'grid', photo.previewVersion)"
                :alt="photo.name"
                loading="lazy"
              />
            </a>
            <v-chip v-if="index === 0" size="x-small" class="best">{{
              t("similar.bestHint")
            }}</v-chip>
            <label class="keep">
              <input
                v-model="group.keep"
                type="radio"
                :name="`keep-${group.photos[0].id}`"
                :value="photo.id"
              />
              <svg
                viewBox="0 0 24 24"
                width="22"
                height="22"
                aria-hidden="true"
                shape-rendering="geometricPrecision"
              >
                <circle class="ring" cx="12" cy="12" r="9" />
                <circle class="dot" cx="12" cy="12" r="5" />
              </svg>
              <span>{{ t("similar.keep") }}</span>
            </label>
            <div class="name" :title="photo.name">{{ photo.name }}</div>
            <div class="meta text-medium-emphasis">
              {{
                photo.width && photo.height
                  ? `${photo.width} × ${photo.height}, `
                  : ""
              }}{{ formatBytes(photo.size) }}
            </div>
            <div class="meta text-medium-emphasis">{{ when(photo) }}</div>
          </div>
        </div>
      </section>

      <p v-if="hidden > 0" class="text-medium-emphasis">
        {{ t("similar.shown", { n: shown.length, total: totals[tab] }) }}
      </p>
    </div>

    <ConfirmDialog
      v-model="confirmOpen"
      :title="t('similar.confirmTitle')"
      :text="confirmText"
      :action="t('similar.confirmAction')"
      @confirm="resolve"
    />
  </div>
</template>

<style scoped>
.group {
  margin: 0 0 24px;
  max-width: 1200px;
}

.group-head {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  justify-content: space-between;
  gap: 8px 16px;
  margin-bottom: 8px;
}

.group-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}

.tiles {
  display: flex;
  flex-wrap: wrap;
  gap: 16px;
}

.tile {
  position: relative;
  width: 200px;
}

.tile img {
  display: block;
  width: 200px;
  height: 150px;
  object-fit: cover;
  border-radius: var(--radius-small, 6px);
  background: rgb(var(--v-theme-surface-variant));
}

.keep {
  display: flex;
  align-items: center;
  gap: 6px;
  margin: 6px 0 2px;
  cursor: pointer;
}

.keep input {
  position: absolute;
  opacity: 0;
  pointer-events: none;
}

.keep .ring {
  fill: none;
  stroke: rgb(var(--v-theme-border-control, var(--v-theme-on-surface)));
  stroke-width: 2;
}

.keep .dot {
  fill: rgb(var(--v-theme-primary));
  opacity: 0;
}

.keep input:checked + svg .ring {
  stroke: rgb(var(--v-theme-primary));
}

.keep input:checked + svg .dot {
  opacity: 1;
}

.keep input:focus-visible + svg {
  outline: 2px solid rgb(var(--v-theme-primary));
  outline-offset: 2px;
  border-radius: 50%;
}

.best {
  position: absolute;
  top: 8px;
  left: 8px;
}

.name {
  overflow: hidden;
  font-weight: 600;
  text-overflow: ellipsis;
  white-space: nowrap;
}

.meta {
  font-size: 12px;
}
</style>

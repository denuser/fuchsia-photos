<script setup lang="ts">
import { onMounted, onUnmounted, ref } from "vue";

import { api, type FaceScans } from "../api/client";
import { formatDateTime, t } from "../i18n";
import EmptyState from "./EmptyState.vue";

const log = ref<FaceScans | null>(null);
let timer: ReturnType<typeof setInterval> | undefined;

async function load() {
  const { data } = await api.GET("/ml/faces");
  if (data) {
    log.value = data;
  }
}

onMounted(() => {
  void load();
  timer = setInterval(() => void load(), 5000);
});
onUnmounted(() => clearInterval(timer));
</script>

<template>
  <div data-testid="face-scan-log">
    <p class="text-medium-emphasis">{{ t("faceLog.description") }}</p>
    <template v-if="log">
      <p class="mb-4">
        {{
          t("faceLog.totals", {
            scanned: log.scanned,
            withFaces: log.withFaces,
            failing: log.failing,
          })
        }}
      </p>
      <EmptyState
        v-if="log.scans.length === 0"
        icon="fa-users"
        :title="t('faceLog.none')"
        :text="t('faceLog.noneText')"
      />
      <v-table v-else>
        <thead>
          <tr>
            <th>{{ t("faceLog.time") }}</th>
            <th>{{ t("faceLog.photo") }}</th>
            <th>{{ t("faceLog.result") }}</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="scan in log.scans" :key="scan.photoId">
            <td>{{ formatDateTime(scan.scannedAt) }}</td>
            <td>{{ scan.name || scan.photoId }}</td>
            <td v-if="scan.error" class="text-error">
              {{ t("faceLog.failed", { error: scan.error }) }}
            </td>
            <td v-else>
              {{
                scan.faces > 0
                  ? t("faceLog.found", { n: scan.faces }, scan.faces)
                  : t("faceLog.noFaces")
              }}
            </td>
          </tr>
        </tbody>
      </v-table>
    </template>
  </div>
</template>

<script setup lang="ts">
import AppDialog from './AppDialog.vue'

const open = defineModel<boolean>({ required: true })

defineProps<{ title: string; text: string; action: string }>()
const emit = defineEmits<{ confirm: [] }>()

function confirm() {
  open.value = false
  emit('confirm')
}
</script>

<template>
  <AppDialog v-model="open" :title="title" :action="action" danger @submit="confirm">
    {{ text }}
  </AppDialog>
</template>

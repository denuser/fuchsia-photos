<script setup lang="ts">
const open = defineModel<boolean>({ required: true })

defineProps<{ title: string; text: string; action: string }>()
const emit = defineEmits<{ confirm: [] }>()

function confirm() {
  open.value = false
  emit('confirm')
}
</script>

<template>
  <v-dialog v-model="open" max-width="420">
    <v-card :title="title" :text="text">
      <v-card-actions>
        <v-spacer />
        <v-btn text="Cancel" @click="open = false" />
        <v-btn color="error" :text="action" @click="confirm" />
      </v-card-actions>
    </v-card>
  </v-dialog>
</template>

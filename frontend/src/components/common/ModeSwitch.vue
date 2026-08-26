<!--
- Copyright (c) 2026 HaydenGuo
- Project: file-share-manager
- Gitee: https://gitee.com/ghl1024/file-share-manager
- GitHub: https://github.com/ghl1024/file-share-manager
- CNB: https://cnb.cool/ghl1024/file-share-manager
- GitCode: https://gitcode.com/haydenguo/file-share-manager
- Author: https://hayden.pub
 -->

<template>
  <div class="common-mode-switch" role="tablist">
    <ActionButton
      v-for="option in options"
      :key="option.value"
      :action="option.action || option.value"
      :text="option.label"
      :icon="option.icon"
      :plain="modelValue !== option.value"
      :aria-selected="modelValue === option.value"
      role="tab"
      @click="select(option.value)"
    />
  </div>
</template>

<script setup>
import ActionButton from './ActionButton.vue'

defineProps({
  modelValue: { type: String, required: true },
  options: { type: Array, required: true }
})

const emit = defineEmits(['update:modelValue', 'change'])

function select(value) {
  emit('update:modelValue', value)
  emit('change', value)
}
</script>

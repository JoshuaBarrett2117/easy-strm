<template>
  <div class="resource-aggregation">
    <n-tabs
      type="line"
      animated
      class="resource-tabs"
      default-value="share-transfer"
    >
      <n-tab-pane name="share-transfer" tab="115 分享转存">
        <ResourceTransfer />
      </n-tab-pane>
      <n-tab-pane name="offline-download" tab="115 云下载">
        <OfflineDownload />
      </n-tab-pane>
    </n-tabs>
  </div>
</template>

<script setup>
import { NTabs, NTabPane } from 'naive-ui'
import ResourceTransfer from './resources/ResourceTransfer.vue'
import OfflineDownload from './resources/OfflineDownload.vue'
</script>

<style scoped>
.resource-aggregation {
  min-height: calc(100vh - 240px);
}

.resource-tabs {
  margin: -4px 0 0;
}

.resource-tabs :deep(.n-tabs-nav) {
  margin-bottom: 16px;
}
</style>

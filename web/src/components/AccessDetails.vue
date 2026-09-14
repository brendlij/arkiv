<script setup lang="ts">
import { ref, watch } from "vue";
import { api } from "../api";
const props = defineProps<{
  assetId?: number;
  library?: string;
  folder?: string;
}>();
type Details = {
  owner: string;
  storage: string;
  people: { name: string; access: string }[];
  publicAlbums: string[];
  detailed: boolean;
  note: string;
  canManage: boolean;
};
const details = ref<Details>(),
  error = ref(""),
  loading = ref(false);
let request = 0;
watch(
  () => [props.assetId, props.library, props.folder],
  async () => {
    const current = ++request;
    details.value = undefined;
    error.value = "";
    loading.value = true;
    const params = new URLSearchParams(
      props.assetId
        ? { asset: String(props.assetId) }
        : { library: props.library || "", folder: props.folder || "" },
    );
    try {
      const result = await api<Details>(`/access-details?${params}`);
      if (current === request) details.value = result;
    } catch (e) {
      if (current === request) error.value = (e as Error).message;
    } finally {
      if (current === request) loading.value = false;
    }
  },
  { immediate: true },
);
</script>
<template>
  <section class="access-details" aria-label="Ownership and access">
    <p v-if="loading" role="status">Checking access…</p>
    <p v-if="error" role="alert">Access details unavailable: {{ error }}</p>
    <template v-if="details">
      <div class="ownership-line">
        <strong>Owned by {{ details.owner }}</strong
        ><span>{{
          details.canManage
            ? "You can organize these uploads"
            : "Viewing access"
        }}</span>
      </div>
      <p>{{ details.storage }}</p>
      <details>
        <summary>Who has access?</summary>
        <ul v-if="details.detailed">
          <li v-for="person in details.people" :key="person.name">
            <strong>{{ person.name }}</strong
            ><span>{{ person.access }}</span>
          </li>
        </ul>
        <p v-if="details.publicAlbums.length">
          <strong>Public now through albums:</strong>
          {{ details.publicAlbums.join(", ") }}. Anyone with those links can
          view the included items.
        </p>
        <p v-else-if="details.detailed">
          No currently active public album links include this content.
        </p>
        <p>{{ details.note }}</p>
      </details>
    </template>
  </section>
</template>
<style scoped>
.access-details {
  padding: 16px 18px;
  border: 1px solid var(--line);
  border-radius: 12px;
  background: var(--surface);
  color: var(--text);
  margin: 0 0 24px;
  font-size: 13px;
}
.ownership-line {
  display: flex;
  gap: 8px 20px;
  flex-wrap: wrap;
  justify-content: space-between;
}
.ownership-line span,
.access-details p,
.access-details li span {
  color: var(--muted);
  line-height: 1.6;
}
.access-details p {
  margin: 8px 0;
}
summary {
  cursor: pointer;
  font-weight: 600;
  padding: 6px 0;
}
ul {
  list-style: none;
  padding: 0;
  margin: 8px 0;
}
li {
  display: flex;
  flex-direction: column;
  padding: 8px 0;
  border-bottom: 1px solid var(--line);
}
</style>

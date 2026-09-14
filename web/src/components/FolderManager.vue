<script setup lang="ts">
import { ref } from "vue";
import { api } from "../api";
const props = defineProps<{ path: string }>();
const emit = defineEmits<{ changed: [path?: string] }>();
const dialog = ref<HTMLDialogElement>(),
  action = ref("create"),
  value = ref(""),
  busy = ref(false),
  error = ref("");
type Share = {
  username: string;
  name: string;
  inherited: boolean;
  source: string;
};
const shares = ref<Share[]>([]);
async function refreshShares() {
  shares.value = await api<Share[]>(
    `/me/folders/sharing?${new URLSearchParams({ path: props.path })}`,
  );
}
async function open(mode: string) {
  action.value = mode;
  value.value =
    mode === "move"
      ? props.path
      : mode === "rename"
        ? props.path.split("/").pop() || ""
        : "";
  error.value = "";
  dialog.value?.showModal();
  if (mode === "share")
    try {
      await refreshShares();
    } catch (e) {
      error.value = (e as Error).message;
    }
}
async function save(revoke?: string) {
  if (busy.value) return;
  busy.value = true;
  error.value = "";
  const parent = props.path.includes("/")
    ? props.path.slice(0, props.path.lastIndexOf("/"))
    : "";
  const path =
    action.value === "create"
      ? [props.path, value.value].filter(Boolean).join("/")
      : props.path;
  const target =
    action.value === "rename"
      ? [parent, value.value].filter(Boolean).join("/")
      : value.value;
  if (
    (action.value === "rename" || action.value === "create") &&
    value.value.includes("/")
  ) {
    error.value = "Enter one folder name. Use Move for a different parent.";
    busy.value = false;
    return;
  }
  try {
    const result = await api<{ path: string }>("/me/folders/manage", "POST", {
      action: revoke
        ? "revoke"
        : action.value === "rename"
          ? "move"
          : action.value,
      path,
      target,
      username: revoke || value.value,
    });
    if (action.value === "share") {
      value.value = "";
      await refreshShares();
      emit("changed");
    } else {
      dialog.value?.close();
      emit("changed", result.path);
    }
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <div class="folder-actions">
    <button class="button" @click="open('create')">New folder</button
    ><button v-if="path" class="button" @click="open('rename')">
      Rename folder</button
    ><button v-if="path" class="button" @click="open('move')">
      Move folder</button
    ><button class="button" @click="open('share')">Share folder</button>
  </div>
  <dialog
    ref="dialog"
    class="folder-dialog"
    aria-labelledby="folder-action-title"
    @cancel="busy && $event.preventDefault()"
  >
    <form @submit.prevent="save()">
      <h2 id="folder-action-title">
        {{
          action === "create"
            ? "New folder"
            : action === "rename"
              ? "Rename folder"
              : action === "move"
                ? "Move folder"
                : "Share folder"
        }}
      </h2>
      <p>{{ path || "My uploads" }}</p>
      <label for="folder-action-value">{{
        action === "share"
          ? "Existing username"
          : action === "move"
            ? "New full path inside My uploads"
            : "Folder name"
      }}</label>
      <input
        id="folder-action-value"
        v-model="value"
        :disabled="busy"
        required
        maxlength="240"
        :placeholder="
          action === 'move'
            ? 'Holidays/Summer'
            : action === 'share'
              ? 'Username'
              : 'Summer'
        "
      />
      <p v-if="action === 'move'">
        Includes all nested folders and media. Explicit shares follow the
        folder. Access inherited from the old parent is removed; access from the
        new parent applies.
      </p>
      <p v-if="action === 'share'">
        Shares this folder, its subfolders and future additions. Recipients can
        view, download and add media to their own shared albums. Only you can
        move or trash your originals.
      </p>
      <ul v-if="action === 'share'">
        <li v-for="share in shares" :key="share.username + share.source">
          <span
            >{{ share.name }} ({{ share.username }})<small>{{
              share.inherited
                ? "Inherited from " + (share.source || "all uploads")
                : "Shared directly"
            }}</small></span
          ><button
            v-if="!share.inherited"
            type="button"
            :disabled="busy"
            @click="save(share.username)"
          >
            Remove access
          </button>
        </li>
      </ul>
      <p v-if="action === 'share'">
        Removing a direct share does not remove access through another folder or
        album. Administrators can still view non-trashed media. Public sharing
        is available through albums.
      </p>
      <p v-if="error" role="alert">{{ error }}</p>
      <footer>
        <button
          class="button"
          type="button"
          :disabled="busy"
          @click="dialog?.close()"
        >
          {{ action === "share" ? "Done" : "Cancel" }}</button
        ><button class="button primary" :disabled="busy">
          {{
            busy ? "Saving…" : action === "share" ? "Share with person" : "Save"
          }}
        </button>
      </footer>
    </form>
  </dialog>
</template>
<style scoped>
.folder-actions {
  display: flex;
  flex-wrap: wrap;
  gap: 10px;
  margin: 0 0 20px;
}
.folder-dialog {
  width: calc(100% - 32px);
  max-width: 550px;
  max-height: 85vh;
  overflow: auto;
  border: 1px solid var(--line);
  border-radius: 16px;
  background: var(--surface);
  color: var(--text);
  padding: 26px;
}
.folder-dialog::backdrop {
  background: #152b2866;
}
input {
  width: 100%;
  padding: 12px;
  margin: 8px 0;
}
p,
small {
  color: var(--muted);
  line-height: 1.6;
}
ul {
  list-style: none;
  padding: 0;
}
li {
  display: flex;
  justify-content: space-between;
  gap: 12px;
  align-items: center;
  padding: 12px 0;
  border-bottom: 1px solid var(--line);
}
small {
  display: block;
}
footer {
  display: flex;
  gap: 12px;
  justify-content: flex-end;
  margin-top: 20px;
}
</style>

import { computed, onMounted, onUnmounted, ref, watch } from "vue";
import { api, ApiError } from "../api";
import type { Asset, Album, Folder, Page, Stats, User } from "../types";
export function useGallery() {
  type View =
    | "Maps"
    | "Photos"
    | "Albums"
    | "Shared with me"
    | "Favorites"
    | "Server libraries"
    | "My library"
    | "Trash"
    | "Inbox"
    | "Account"
    | "People"
    | "Processing";
  const currentUser = ref<User>();
  const nav: { name: View; icon: string }[] = [
    { name: "Photos", icon: "photos" },
    { name: "Maps", icon: "map" },
    { name: "Albums", icon: "albums" },
    { name: "Shared with me", icon: "albums" },
    { name: "My library", icon: "folder" },
    { name: "Favorites", icon: "heart" },
    { name: "Server libraries", icon: "folder" },
    { name: "Inbox", icon: "inbox" },
    { name: "Trash", icon: "inbox" },
  ];
  const session = ref(false),
    initializing = ref(true),
    username = ref("admin"),
    password = ref(""),
    loginBusy = ref(false),
    proxy = ref(false);
  const view = ref<View>("Photos"),
    query = ref(""),
    assets = ref<Asset[]>([]),
    loading = ref(false),
    error = ref(""),
    notice = ref(""),
    stats = ref<Stats>();
  const albums = ref<Album[]>([]),
    folders = ref<Folder[]>([]),
    album = ref<Album>(),
    library = ref(""),
    folder = ref(""),
    newAlbum = ref(""),
    creating = ref(false),
    showCreate = ref(false),
    collectionMore = ref(false),
    collectionOffset = ref(0);
  const folderHome = ref<{ view: View; path: string; library: string }>();
  const activeNav = computed(() =>
    view.value === "Server libraries" && library.value && folderHome.value
      ? folderHome.value.view
      : view.value,
  );
  const viewer = ref<number | null>(null),
    nextCursor = ref(""),
    cursors = ref<string[]>([""]),
    page = ref(0);
  const dark = ref(
    localStorage.getItem("gallery-theme") === "dark" ||
      (!localStorage.getItem("gallery-theme") &&
        matchMedia("(prefers-color-scheme: dark)").matches),
  );
  watch(
    dark,
    (value) => {
      document.documentElement.dataset.theme = value ? "dark" : "light";
      localStorage.setItem("gallery-theme", value ? "dark" : "light");
    },
    { immediate: true },
  );
  const title = computed(() =>
    query.value
      ? "Search results"
      : album.value?.name ||
        (view.value === "Server libraries" && library.value
          ? library.value === "arkiv-uploads" &&
            folder.value === `user-${currentUser.value?.id}`
            ? "My uploads"
            : folder.value.split("/").pop() || "Library"
          : view.value),
  );
  const subtitle = computed(() =>
    view.value === "Processing"
      ? "From originals to memories, see what is happening."
      : view.value === "Maps"
        ? "Every memory has a place."
        : view.value === "People"
          ? "A shared home, with room for privacy."
          : view.value === "Account"
            ? "Make yourself at home."
            : view.value === "Shared with me"
              ? "Albums and assigned folders shared with you. Folder access does not mean ownership."
              : view.value === "My library"
                ? "Your uploads, organized into folders. Administrators can view non-trashed media."
                : query.value
                  ? `Matches for “${query.value}”`
                  : view.value === "Photos"
                    ? "Your uploads, shared photos and accessible server libraries."
                    : view.value === "Server libraries"
                      ? library.value === "arkiv-uploads"
                        ? "Uploaded media, organized into folders inside arkiv."
                        : "Browse files on the server. External libraries are read-only; access is assigned by an administrator."
                      : view.value === "Trash"
                        ? "Restore removed items or delete them permanently."
                        : view.value === "Inbox"
                          ? "New arrivals, ready to explore."
                          : view.value === "Favorites"
                            ? "The moments you keep coming back to."
                            : view.value === "Albums"
                              ? "Your stories, brought together."
                              : "Just as you organized them.",
  );
  const showGrid = computed(
    () =>
      !["Account", "People", "Maps", "Processing"].includes(view.value) &&
      (!!query.value ||
        !["Albums", "Shared with me", "My library"].includes(view.value) ||
        !!album.value),
  );
  let timer: ReturnType<typeof setInterval> | undefined;
  let previewTimer: ReturnType<typeof setInterval> | undefined;
  let refreshingPreviews = false;
  async function refreshPreviews() {
    if (!session.value || refreshingPreviews) return;
    const userID = currentUser.value?.id;
    const pending = assets.value.filter(
      (a) => a.previewStatus === "pending" || a.metadataStatus === "pending",
    );
    if (!pending.length) return;
    refreshingPreviews = true;
    try {
      // Update objects in place without resetting scroll, selection or the viewer's order.
      for (const old of pending) {
        const fresh = await api<Asset>(`/assets/${old.id}`);
        if (!session.value || userID !== currentUser.value?.id) break;
        const index = assets.value.findIndex((a) => a.id === fresh.id);
        if (index >= 0) assets.value[index] = fresh;
      }
    } catch (e) {
      if (e instanceof ApiError && e.status === 401) report(e);
    } finally {
      refreshingPreviews = false;
    }
  }
  function uploadsCompleted(count: number, destination = "") {
    query.value = "";
    album.value = undefined;
    viewer.value = null;
    notice.value = `${count} ${count === 1 ? "file" : "files"} uploaded. Previews will appear automatically.`;
    openFolder({
      libraryId: "arkiv-uploads",
      path:
        `user-${currentUser.value?.id}` +
        (destination ? "/" + destination : ""),
      name: "My uploads",
      count,
    });
    void status();
  }
  let searchTimer: ReturnType<typeof setTimeout> | undefined;
  let request = 0;
  function report(e: unknown) {
    if (e instanceof ApiError && e.status === 401) {
      session.value = false;
      viewer.value = null;
      assets.value = [];
    }
    error.value = (e as Error).message;
  }
  async function status() {
    try {
      stats.value = await api<Stats>("/stats");
    } catch (e) {
      report(e);
    }
  }
  const loadingMore = ref(false);
  async function load(reset = true, append = false) {
    const current = ++request;
    loading.value = false;
    loadingMore.value = false;
    if (currentUser.value?.mustChangePassword && !proxy.value) {
      view.value = "Account";
      return;
    }
    if (["Account", "People", "Maps", "Processing"].includes(view.value)) {
      assets.value = [];
      return;
    }
    loading.value = !append;
    loadingMore.value = append;
    error.value = "";
    if (reset) {
      page.value = 0;
      cursors.value = [""];
      collectionOffset.value = 0;
    }
    const parameters = new URLSearchParams({ limit: "120" });
    if (view.value === "Trash") parameters.set("trash", "true");
    if (query.value) parameters.set("q", query.value);
    else {
      if (view.value === "Favorites") parameters.set("favorite", "true");
      if (view.value === "Inbox") parameters.set("inbox", "true");
      if (album.value) parameters.set("album", String(album.value.id));
      if (view.value === "Server libraries" && library.value) {
        parameters.set("library", library.value);
        parameters.set("folder", folder.value);
      }
    }
    if (append) parameters.set("cursor", nextCursor.value);
    else if (cursors.value[page.value])
      parameters.set("cursor", cursors.value[page.value]);
    try {
      const data = showGrid.value
        ? await api<Page>(`/assets?${parameters}`)
        : { items: [], nextCursor: "" };
      if (current !== request) return;
      assets.value = append
        ? [
            ...assets.value,
            ...data.items.filter(
              (item) => !assets.value.some((old) => old.id === item.id),
            ),
          ]
        : data.items;
      nextCursor.value = data.nextCursor;
      if (append) return;
      if (
        ["Albums", "Shared with me"].includes(view.value) &&
        !album.value &&
        !query.value
      ) {
        const data = await api<{ items: Album[]; hasMore: boolean }>(
          `/albums?scope=${view.value === "Shared with me" ? "shared" : "owned"}&offset=${collectionOffset.value}`,
        );
        if (current !== request) return;
        albums.value = data.items;
        collectionMore.value = data.hasMore;
      }
      if (
        ["My library", "Shared with me"].includes(view.value) &&
        !query.value &&
        !album.value
      ) {
        const grants = (await api<{ items: Folder[] }>("/me/folders")).items;
        if (current !== request) return;
        const own = (f: Folder) =>
          f.libraryId === "arkiv-uploads" &&
          (f.path === `user-${currentUser.value?.id}` ||
            f.path.startsWith(`user-${currentUser.value?.id}/`));
        folders.value = grants.filter((f) =>
          view.value === "My library" ? own(f) : !own(f),
        );
        if (view.value === "My library") collectionMore.value = false;
      }
      if (view.value === "Server libraries" && !query.value) {
        const data = await api<{ items: Folder[]; hasMore: boolean }>(
          `/folders?${new URLSearchParams({ library: library.value, parent: folder.value, offset: String(collectionOffset.value) })}`,
        );
        if (current !== request) return;
        folders.value = data.items;
        collectionMore.value = data.hasMore;
      }
    } catch (e) {
      if (current === request) report(e);
    } finally {
      if (current === request) {
        loading.value = false;
        loadingMore.value = false;
      }
    }
  }
  async function login() {
    loginBusy.value = true;
    error.value = "";
    try {
      await api("/auth/login", "POST", {
        username: username.value,
        password: password.value,
      });
      password.value = "";
      await refreshSession();
    } catch (e) {
      report(e);
    } finally {
      loginBusy.value = false;
    }
  }
  function clearSession() {
    session.value = false;
    currentUser.value = undefined;
    albums.value = [];
    folders.value = [];
    album.value = undefined;
    stats.value = undefined;
    view.value = "Photos";
    query.value = "";
    library.value = "";
    folder.value = "";
    ++request;
    assets.value = [];
    viewer.value = null;
  }
  function passwordChanged() {
    clearSession();
    error.value = "";
    notice.value = "Password changed. Sign in with your new password.";
  }
  async function logout() {
    try {
      await api("/auth/logout", "POST");
      clearSession();
    } catch (e) {
      report(e);
    }
  }
  function navigate(target: View) {
    folderHome.value = undefined;
    view.value = target;
    album.value = undefined;
    library.value = "";
    folder.value = "";
    query.value = "";
    viewer.value = null;
    void load();
  }
  function search() {
    if (["Maps", "Account", "People", "Processing"].includes(view.value))
      view.value = "Photos";
    clearTimeout(searchTimer);
    searchTimer = setTimeout(() => void load(), 300);
  }
  function openFolder(item: Folder) {
    if (view.value !== "Server libraries")
      folderHome.value = {
        view:
          view.value === "My library" || view.value === "Shared with me"
            ? view.value
            : "My library",
        path: item.path,
        library: item.libraryId,
      };
    view.value = "Server libraries";
    library.value = item.libraryId;
    folder.value = item.path;
    void load();
  }
  function folderChanged(path?: string) {
    if (path) {
      if (folderHome.value)
        folderHome.value.path = `user-${currentUser.value?.id}`;
      openFolder({
        libraryId: "arkiv-uploads",
        path,
        name: path.split("/").pop() || "My uploads",
        count: 0,
      });
      return;
    }
    void load();
  }
  function parentFolder() {
    if (
      folderHome.value &&
      folderHome.value.path === folder.value &&
      folderHome.value.library === library.value
    ) {
      navigate(folderHome.value.view);
      return;
    }
    if (folder.value)
      folder.value = folder.value.split("/").slice(0, -1).join("/");
    else library.value = "";
    void load();
  }
  async function createAlbum() {
    if (!newAlbum.value.trim() || creating.value) return;
    creating.value = true;
    try {
      album.value = await api<Album>("/albums", "POST", {
        name: newAlbum.value,
      });
      newAlbum.value = "";
      showCreate.value = false;
      await load();
    } catch (e) {
      report(e);
    } finally {
      creating.value = false;
    }
  }
  function favorite(asset: Asset) {
    const index = assets.value.findIndex((a) => a.id === asset.id);
    if (index >= 0) assets.value[index] = asset;
    void status();
  }
  function removed() {
    viewer.value = null;
    void load();
  }
  async function loadMore() {
    if (!nextCursor.value || loading.value || loadingMore.value) return;
    await load(false, true);
  }
  function next() {
    if (!nextCursor.value) return;
    cursors.value[page.value + 1] = nextCursor.value;
    page.value++;
    void load(false);
    window.scrollTo({ top: 0 });
  }
  function previous() {
    if (page.value === 0) return;
    page.value--;
    void load(false);
    window.scrollTo({ top: 0 });
  }
  async function scan() {
    try {
      await api("/scan", "POST");
      notice.value =
        "Scan requested. Refresh the view as new previews become available.";
      await status();
    } catch (e) {
      report(e);
    }
  }
  async function retry() {
    try {
      await api("/retry", "POST");
      notice.value = "Processing queued. Existing previews remain available.";
      await status();
    } catch (e) {
      report(e);
    }
  }
  function moreCollections(delta: number) {
    collectionOffset.value = Math.max(0, collectionOffset.value + delta * 200);
    void load(false);
  }
  onMounted(async () => {
    if (location.pathname.startsWith("/share/")) {
      initializing.value = false;
      return;
    }
    try {
      await refreshSession();
    } catch (e) {
      if (!(e instanceof ApiError && e.status === 401)) report(e);
    } finally {
      initializing.value = false;
    }
    timer = setInterval(() => {
      if (
        session.value &&
        (!currentUser.value?.mustChangePassword || proxy.value)
      )
        void status();
    }, 10000);
    previewTimer = setInterval(() => void refreshPreviews(), 2000);
  });
  onUnmounted(() => {
    clearInterval(timer);
    clearInterval(previewTimer);
    clearTimeout(searchTimer);
  });

  async function refreshSession() {
    const s = await api<User>("/auth/session");
    currentUser.value = s;
    username.value = s.username;
    proxy.value = !!s.proxy;
    session.value = true;
    if (s.mustChangePassword && !s.proxy) {
      view.value = "Account";
      return;
    }
    await Promise.all([load(), status()]);
  }
  return {
    uploadsCompleted,
    activeNav,
    currentUser,
    passwordChanged,
    nav,
    session,
    initializing,
    username,
    password,
    loginBusy,
    proxy,
    view,
    query,
    assets,
    loading,
    error,
    notice,
    stats,
    albums,
    folders,
    album,
    library,
    folder,
    newAlbum,
    creating,
    showCreate,
    collectionMore,
    collectionOffset,
    viewer,
    nextCursor,
    loadingMore,
    loadMore,
    page,
    dark,
    title,
    subtitle,
    showGrid,
    status,
    load,
    login,
    logout,
    navigate,
    search,
    openFolder,
    parentFolder,
    folderChanged,
    createAlbum,
    favorite,
    removed,
    next,
    previous,
    scan,
    retry,
    moreCollections,
  };
}

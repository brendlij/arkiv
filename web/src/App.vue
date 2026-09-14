<script setup lang="ts">
import { ref, watch, onUnmounted } from "vue";
import ProcessingDashboard from "./components/ProcessingDashboard.vue";
import PublicAlbum from "./components/PublicAlbum.vue";
const publicPage = location.pathname.startsWith("/share/");
import { useGallery } from "./composables/useGallery";
import MapBrowser from "./components/MapBrowser.vue";
import PhotoUpload from "./components/PhotoUpload.vue";
import AccountSettings from "./components/AccountSettings.vue";
import UsersAdmin from "./components/UsersAdmin.vue";
import AlbumSharing from "./components/AlbumSharing.vue";
import Logo from "./components/Logo.vue";
import Icon from "./components/Icon.vue";
import FolderManager from "./components/FolderManager.vue";
import AccessDetails from "./components/AccessDetails.vue";
import GalleryGrid from "./components/GalleryGrid.vue";
import AssetViewer from "./components/AssetViewer.vue";
const {
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
  moreCollections,
  uploadsCompleted,
} = useGallery();
const mobileMenu = ref<HTMLDialogElement>();
const moreSentinel = ref<HTMLElement>();
let moreObserver: IntersectionObserver | undefined;
watch(moreSentinel, (el) => {
  moreObserver?.disconnect();
  if (!el) return;
  moreObserver = new IntersectionObserver(
    (entries) => {
      if (
        entries.some((e) => e.isIntersecting) &&
        matchMedia("(max-width: 700px)").matches &&
        !loading.value &&
        !loadingMore.value &&
        !error.value &&
        viewer.value === null &&
        assets.value.length < 600
      )
        void loadMore();
    },
    { rootMargin: "400px" },
  );
  moreObserver.observe(el);
});
watch(view, () => mobileMenu.value?.close());
onUnmounted(() => moreObserver?.disconnect());
</script>
<template>
  <PublicAlbum v-if="publicPage" />
  <div v-else-if="initializing" class="initial-loading" role="status">
    <Logo />Opening arkiv…
  </div>
  <main v-else-if="!session" class="login-page">
    <Logo full class="login-logo" />

    <h1>Welcome home.</h1>
    <p class="login-intro">your photos, at home.</p>
    <form class="login-form" @submit.prevent="login">
      <label
        >Username<input
          v-model="username"
          autocomplete="username"
          required /></label
      ><label
        >Password<input
          v-model="password"
          type="password"
          autocomplete="current-password"
          required
          maxlength="1024"
      /></label>
      <p v-if="notice" role="status">{{ notice }}</p>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <button class="primary" :disabled="loginBusy">
        {{ loginBusy ? "Signing in…" : "Open arkiv" }}<Icon name="right" />
      </button>
    </form>
    <p class="private-note">Privately hosted. Entirely yours.</p>
  </main>
  <div v-else class="app-shell">
    <aside class="sidebar">
      <a
        class="brand"
        href="#"
        @click.prevent="navigate('Photos')"
        aria-label="arkiv — Photos"
        ><Logo compact /><span class="brand-wordmark">arkiv</span></a
      >
      <p class="nav-label">YOUR LIBRARY</p>
      <nav>
        <button
          v-for="item in nav"
          :key="item.name"
          :class="{
            active: activeNav === item.name && !query,
            'mobile-primary': [
              'Photos',
              'Maps',
              'Albums',
              'My library',
            ].includes(item.name),
          }"
          @click="navigate(item.name)"
        >
          <Icon :name="item.icon" /><span>{{ item.name }}</span
          ><small v-if="item.name === 'Favorites' && stats?.favorites">{{
            stats.favorites
          }}</small>
        </button>
        <button
          :class="{ active: view === 'Account' }"
          @click="navigate('Account')"
        >
          <Icon name="info" /><span>My account</span>
        </button>
        <button
          v-if="currentUser?.role === 'admin'"
          :class="{ active: view === 'People' }"
          @click="navigate('People')"
        >
          <Icon name="folder" /><span>People & access</span>
        </button>
        <button
          v-if="currentUser?.role === 'admin'"
          :class="{ active: view === 'Processing' }"
          @click="navigate('Processing')"
        >
          <Icon name="refresh" /><span>Processing</span>
        </button>
        <button
          class="mobile-more"
          aria-label="More sections"
          @click="mobileMenu?.showModal()"
        >
          <Icon name="albums" /><span>More</span>
        </button>
      </nav>
      <div class="sidebar-bottom">
        <div class="library-status">
          <span
            class="status-dot"
            :class="{ working: stats?.scanRunning || stats?.thumbnailQueue }"
          />
          <div>
            <strong>{{
              stats?.scanRunning
                ? "Scanning library"
                : stats?.thumbnailQueue
                  ? "Preparing previews"
                  : stats?.failed
                    ? "Processing needs attention"
                    : stats?.scanError
                      ? "Scan needs attention"
                      : "Library ready"
            }}</strong
            ><small
              >{{ stats?.assets.toLocaleString() || 0 }} originals ·
              {{ stats?.thumbnailQueue || 0 }} queued</small
            >
          </div>
        </div>
        <button
          v-if="currentUser?.role === 'admin'"
          class="text-button"
          @click="scan"
        >
          <Icon name="refresh" />Scan library</button
        ><button
          v-if="currentUser?.role === 'admin'"
          class="text-button"
          @click="navigate('Processing')"
        >
          Processing details ({{ stats?.failed || 0 }} failed)
        </button>
        <div class="sidebar-footer">
          <button
            class="icon-button"
            :aria-label="dark ? 'Use light theme' : 'Use dark theme'"
            @click="dark = !dark"
          >
            <Icon :name="dark ? 'sun' : 'moon'" /></button
          ><span>{{ username }}</span
          ><button
            v-if="!proxy"
            class="icon-button"
            aria-label="Sign out"
            @click="logout"
          >
            <Icon name="logout" />
          </button>
        </div>
      </div>
    </aside>
    <dialog
      ref="mobileMenu"
      class="mobile-menu"
      aria-labelledby="mobile-menu-title"
      @click="$event.target === mobileMenu && mobileMenu?.close()"
    >
      <header>
        <h2 id="mobile-menu-title">Your library</h2>
        <button
          class="icon-button"
          aria-label="Close menu"
          @click="mobileMenu?.close()"
        >
          <Icon name="close" />
        </button>
      </header>
      <nav aria-label="All sections">
        <button
          v-for="item in nav"
          :key="item.name"
          @click="
            navigate(item.name);
            mobileMenu?.close();
          "
        >
          <Icon :name="item.icon" />{{ item.name }}
        </button>
        <button
          @click="
            navigate('Account');
            mobileMenu?.close();
          "
        >
          <Icon name="info" />My account
        </button>
        <button
          v-if="currentUser?.role === 'admin'"
          @click="
            navigate('People');
            mobileMenu?.close();
          "
        >
          People &amp; access
        </button>
        <button
          v-if="currentUser?.role === 'admin'"
          @click="
            navigate('Processing');
            mobileMenu?.close();
          "
        >
          Processing
        </button>
        <button @click="dark = !dark">
          {{ dark ? "Light theme" : "Dark theme" }}
        </button>
        <button
          v-if="!proxy"
          @click="
            logout();
            mobileMenu?.close();
          "
        >
          Sign out
        </button>
      </nav>
    </dialog>
    <div class="main-column">
      <header class="topbar">
        <div class="search-field">
          <Icon name="search" /><input
            v-model="query"
            type="search"
            aria-label="Search your library"
            placeholder="Search photos, places, cameras…"
            maxlength="200"
            @input="search"
          />
        </div>
        <button
          class="icon-button"
          aria-label="Refresh photos"
          @click="
            load();
            status();
          "
        >
          <Icon name="refresh" /></button
        ><button
          class="mobile-theme icon-button"
          aria-label="Toggle theme"
          @click="dark = !dark"
        >
          <Icon :name="dark ? 'sun' : 'moon'" />
        </button>
        <details class="mobile-actions">
          <summary aria-label="Library actions"><Icon name="info" /></summary>
          <div>
            <button v-if="currentUser?.role === 'admin'" @click="scan">
              Scan library</button
            ><button
              v-if="currentUser?.role === 'admin'"
              @click="navigate('Processing')"
            >
              Processing details</button
            ><button v-if="!proxy" @click="logout">Sign out</button>
          </div>
        </details>
      </header>
      <main
        class="gallery-content"
        :class="{ 'maps-page': view === 'Maps' && !query }"
      >
        <div v-if="stats?.scanError" class="notification error" role="alert">
          Last scan failed: {{ stats.scanError }}. Check the library mount and
          scan again.
        </div>
        <div class="page-heading">
          <div>
            <p class="eyebrow">
              {{
                view === "Photos" && !query
                  ? "MOMENTS, COLLECTED"
                  : "YOUR PERSONAL COLLECTION"
              }}
            </p>
            <h1>{{ title }}</h1>
            <p>{{ subtitle }}</p>
          </div>
          <div class="heading-actions">
            <PhotoUpload
              v-if="!['Trash', 'Processing'].includes(view)"
              :folder="
                library === 'arkiv-uploads' &&
                folder.startsWith(`user-${currentUser?.id}/`)
                  ? folder.slice(`user-${currentUser?.id}/`.length)
                  : ''
              "
              @saved="
                status();
                load();
              "
              @completed="uploadsCompleted"
            />
            <button
              v-if="view === 'Albums' && !album && !query"
              class="primary compact"
              @click="showCreate = !showCreate"
            >
              <Icon name="plus" />New album</button
            ><span v-else-if="view === 'Photos' && !query" class="photo-total"
              >{{ stats?.assets.toLocaleString() || "0" }}
              <span>memories</span></span
            >
          </div>
        </div>
        <div v-if="error" class="error notification" role="alert">
          {{ error
          }}<button @click="error = ''" aria-label="Dismiss error">×</button>
        </div>
        <div v-if="notice" class="notification" role="status">
          {{ notice
          }}<button @click="notice = ''" aria-label="Dismiss notification">
            ×
          </button>
        </div>
        <div
          v-if="
            (stats?.thumbnailQueue || stats?.scanRunning) &&
            view !== 'Processing'
          "
          class="processing-banner"
        >
          <span class="status-dot working" />{{
            stats.scanRunning
              ? "Discovering your photos…"
              : `${stats.thumbnailQueue} previews are being prepared`
          }}<button @click="load()">Refresh</button>
        </div>
        <form
          v-if="showCreate && view === 'Albums'"
          class="create-album"
          @submit.prevent="createAlbum"
        >
          <label
            >Album name<input
              v-model="newAlbum"
              required
              maxlength="120"
              placeholder="A summer to remember" /></label
          ><button class="primary" :disabled="creating">Create album</button
          ><button class="button" type="button" @click="showCreate = false">
            Cancel
          </button>
        </form>
        <button
          v-if="album && !query"
          class="breadcrumb"
          @click="
            album = undefined;
            load();
          "
        >
          <Icon name="left" />All albums</button
        ><button
          v-if="view === 'Server libraries' && library && !query"
          class="breadcrumb"
          @click="parentFolder"
        >
          <Icon name="left" />{{
            library === "arkiv-uploads"
              ? activeNav === "My library" &&
                folder === `user-${currentUser?.id}`
                ? "My library"
                : "Upload folders"
              : folder
                ? library + " / " + folder
                : "All libraries"
          }}
        </button>
        <div
          v-if="
            ['Albums', 'Shared with me'].includes(view) &&
            !album &&
            !query &&
            !loading
          "
          class="collections"
        >
          <button
            v-for="item in albums"
            :key="item.id"
            class="collection"
            @click="
              album = item;
              load();
            "
          >
            <div class="collection-art"><Icon name="albums" /></div>
            <h2>{{ item.name }}</h2>
            <p>
              {{ item.count }} memories ·
              {{
                item.ownerId === currentUser?.id
                  ? "Owned by you"
                  : "Owned by " + item.ownerName
              }}
            </p>
            <small class="permission-pill">{{
              item.role === "owner" && item.ownerId !== currentUser?.id
                ? "Admin access"
                : item.role
            }}</small>
          </button>
        </div>
        <FolderManager
          v-if="
            !query &&
            (view === 'My library' ||
              (view === 'Server libraries' &&
                library === 'arkiv-uploads' &&
                (folder === `user-${currentUser?.id}` ||
                  folder.startsWith(`user-${currentUser?.id}/`))))
          "
          :path="
            view === 'My library'
              ? ''
              : folder
                  .slice(`user-${currentUser?.id}`.length)
                  .replace(/^\//, '')
          "
          @changed="folderChanged"
        />
        <AccessDetails
          :key="folder + ':' + loading"
          v-if="view === 'Server libraries' && library && !query && !loading"
          :library="library"
          :folder="folder"
        />
        <p v-if="view === 'Shared with me' && !album && !query">
          Albums share selected items. Assigned folders include their subfolders
          and future additions.
        </p>
        <div
          v-if="
            ['Server libraries', 'My library', 'Shared with me'].includes(
              view,
            ) &&
            !album &&
            !query &&
            !loading
          "
          class="folder-list"
        >
          <button
            v-for="item in folders"
            :key="item.libraryId + item.path"
            @click="openFolder(item)"
          >
            <Icon name="folder" /><span
              >{{ item.name
              }}<small
                >{{ item.count }} memories ·
                {{
                  item.libraryId === "arkiv-uploads" &&
                  (item.path === `user-${currentUser?.id}` ||
                    item.path.startsWith(`user-${currentUser?.id}/`))
                    ? "Owned by you"
                    : item.libraryId === "arkiv-uploads"
                      ? "Uploaded media · check access inside"
                      : "Server-managed · read-only"
                }}</small
              ></span
            ><Icon name="right" />
          </button>
        </div>
        <div
          v-if="
            ((['Albums', 'Shared with me'].includes(view) && !album) ||
              view === 'Server libraries') &&
            !query &&
            (collectionMore || collectionOffset)
          "
          class="pagination"
        >
          <button
            class="button"
            :disabled="!collectionOffset"
            @click="moreCollections(-1)"
          >
            Previous collections</button
          ><button
            class="button"
            :disabled="!collectionMore"
            @click="moreCollections(1)"
          >
            More collections
          </button>
        </div>
        <ProcessingDashboard
          v-if="view === 'Processing' && currentUser?.role === 'admin'"
          @changed="status"
        />
        <MapBrowser v-if="view === 'Maps'" />
        <AccountSettings
          v-if="view === 'Account' && currentUser"
          :user="currentUser"
          @changed="passwordChanged"
        />
        <UsersAdmin
          v-if="view === 'People' && currentUser?.role === 'admin'"
          :current-id="currentUser.id"
        />
        <AlbumSharing
          v-if="album && !query"
          :key="album.id"
          :album="album"
          @changed="
            album = $event;
            load();
          "
          @deleted="
            album = undefined;
            load();
          "
        />
        <GalleryGrid
          v-if="showGrid"
          :assets="assets"
          :loading="loading"
          :trash="view === 'Trash'"
          @changed="
            load();
            status();
          "
          @open="viewer = $event"
        />
        <div
          v-if="
            !['Account', 'People', 'Maps', 'Processing'].includes(view) &&
            !loading &&
            !assets.length &&
            !(
              ['Server libraries', 'My library', 'Shared with me'].includes(
                view,
              ) && folders.length
            ) &&
            !(
              ['Albums', 'Shared with me'].includes(view) &&
              !album &&
              albums.length
            )
          "
          class="empty-state"
        >
          <Logo v-if="view === 'Photos' && !query" full class="login-logo" />
          <div v-else class="empty-icon">
            <Icon
              :name="query ? 'search' : view === 'Albums' ? 'albums' : 'photos'"
            />
          </div>
          <h2>
            {{
              query
                ? "No matching moments"
                : view === "My library"
                  ? "Your library starts here"
                  : view === "Shared with me" && !album
                    ? "Nothing shared with you yet"
                    : view === "Trash"
                      ? "Trash is empty"
                      : view === "Favorites"
                        ? "Your favorites belong here"
                        : view === "Albums"
                          ? "Every collection starts somewhere"
                          : "A little space for your memories"
            }}
          </h2>
          <p>
            {{
              query
                ? "Try a filename, folder, camera, lens, or year."
                : view === "My library"
                  ? "Upload photos or videos to create your personal collection."
                  : view === "Shared with me" && !album
                    ? "Shared albums and folders assigned to you will appear here."
                    : view === "Trash"
                      ? "Items you move to Trash will appear here for recovery."
                      : album
                        ? "Open a photo and use + to add it to this album."
                        : view === "Albums"
                          ? "Create an album, then add photos from the viewer."
                          : view === "Favorites"
                            ? "Tap the heart on a photo to keep it close."
                            : "Photos will appear here once your configured library has been scanned."
            }}
          </p>
          <button
            v-if="view === 'Photos' && !query && currentUser?.role === 'admin'"
            class="button"
            @click="scan"
          >
            Scan library
          </button>
        </div>
        <footer v-if="assets.length" class="pagination" ref="moreSentinel">
          <span role="status"
            >{{ assets.length }} items loaded{{
              !nextCursor ? " · All caught up" : ""
            }}</span
          >
          <button
            v-if="nextCursor"
            class="button"
            :disabled="loading || loadingMore"
            @click="loadMore"
          >
            {{ loadingMore ? "Loading more…" : "Load more" }}
          </button>
        </footer>
      </main>
      <footer class="page-footer">your photos, at home.</footer>
    </div>
  </div>
  <AssetViewer
    v-if="viewer !== null && assets[viewer]"
    :assets="assets"
    :index="viewer"
    :album-id="album?.id"
    :can-edit="album?.role !== 'viewer'"
    @close="viewer = null"
    @navigate="viewer = $event"
    @favorite="favorite"
    @removed="removed"
  />
</template>

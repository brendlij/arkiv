# arkiv

your photos, at home.

The interface uses the user-supplied icon and full SVG wordmark. The icon is used in navigation and the favicon; the full mark appears at sign-in, in the empty photo library and in the README. Editable source assets are in `web/public/brand`. Inline marks inherit the current theme color.

Geist weights 400–700 are bundled locally through @fontsource/geist, with Inter/system fallbacks. No font CDN is contacted. Light background #F4F1E8, surface #FAF9F6 and brand #173F38 pair with dark background #151917 and brand #8FB6A5. Small muted text uses a slightly darker light-theme shade for legibility; the supplied secondary color remains available as a token.

Photo grids use responsive masonry columns with reserved original aspect ratios, lazy previews and 120-item pagination. Interactions use short opacity/transform transitions, with no waits before actions or data requests. Reduced-motion preferences disable decorative motion. The viewer retains keyboard navigation, touch swipe, zoom and fullscreen. Its controls fade after pointer inactivity, return on interaction and remain available during keyboard use or while details/album menus are open.

Existing Maps, folders, access controls and sharing remain available. This identity pass does not add an upload pipeline, photo archiving, PWA installation, or drag-and-drop; these need real workflows before navigation entries or progress UI are introduced. Backend names, configuration keys and database paths remain compatible with existing installations.

Performance target: responsive interaction on ordinary hardware. No 60 FPS certification or low-powered NAS/device benchmark is claimed for this visual pass.

The expanded desktop sidebar now pairs a simplified small-size icon with a lowercase arkiv wordmark for better legibility. This supersedes the earlier icon-only sidebar choice.

# Maps and place browsing

Open **Maps** in the sidebar. The map uses embedded GPS coordinates from indexed originals. Click a photo marker, select a place from **Your places**, or choose **Photos in this area** to open a paginated photo selection. The regular viewer still supports favorites, albums and downloads. Search the place list by city/area, region or country; the main library search also includes derived place names. Photo details show the approximate nearby place.

Existing GPS metadata is backfilled on startup in batches of 200 records. New processing jobs resolve place names as they persist metadata. Originals are never edited. The bundled local lookup works without internet access or an API key. Files without GPS stay in the normal library and are counted separately. A coordinate with no populated place within 75 km remains on the map without a place name.

## Accuracy and privacy

Labels describe the nearest populated place in the bundled GeoNames dataset, followed by its administrative region and country. They may name a neighborhood or district rather than the larger city, and they are not address-level or boundary-based reverse geocoding. Near a border, the nearest place can be across the border. Names and map outlines are approximate and can become outdated. Coordinates are not inferred from filenames or visual content.

Map points, place names, counts, area selections and search results use the same server-side access rules as media. Selected-user album sharing can expose an included photo's existing GPS just as its metadata viewer does. Expired/revoked shares and disabled libraries do not appear. Anonymous public album pages do not expose this map or place metadata.

The default is a detailed OpenStreetMap street map. The **Map style** selector can switch to a bundled Natural Earth world outline with no external map requests; that offline mode provides only coarse land/border context at city zoom. The choice is remembered in this browser. Those requests expose the browser's IP address, Gallery origin and viewed tile area to the tile provider; the photos and place lookup remain local. Select Offline map to stop street tile requests. Tile failures leave the local world outline and photo markers available.

OpenStreetMap tiles are requested directly by the browser with visible attribution and a per-image referrer policy that sends the origin, not a private URL path. Normal HTTP tile caching is preserved. There is no tile proxy, bulk downloading, offline tile pack or prefetch job. Review the [OSMF tile usage policy](https://operations.osmfoundation.org/policies/tiles/) before exposing a heavily used instance; this community service has no availability guarantee. The current street-map provider is fixed to the standard OSM HTTPS endpoint.

## Scale and API

- `GET /api/map?bbox=west,south,east,north&zoom=1..18`: permission-filtered counts and geographic cell groups; at most 1,000 groups, with `hasMore` if capped. Zoom in when capped. An omitted box means the world.
- `GET /api/places?q=Berlin&offset=0`: up to 100 named places with accessible photo counts. `hasMore` supports pagination. Results are populated only by accessible geotagged photos.
- `GET /api/assets?place=GEONAMES_ID`: ordinary paginated assets for a derived place.
- `GET /api/assets?bbox=west,south,east,north`: ordinary paginated assets in a box. West greater than east means a box crossing the date line.

Map and area endpoints reject non-finite/out-of-range coordinates. GPS at 0° latitude/longitude is valid; missing coordinates are distinct. Leaflet uses the usual Web Mercator display, so extreme polar locations have projection limitations. Marker groups summarize photo counts and zoom into smaller groups. Photos load in 120-item pages rather than loading a complete library into the browser.

The local nearest-neighbor index uses a 3D unit-sphere k-d tree to handle distances around the date line. Migration 006 adds place fields/indexes and rebuilds the SQLite FTS index to include place names. Initial migration/backfill can delay the first startup of a large library; subsequent startups skip checked coordinates. Metadata remains derived and can be regenerated with Rebuild previews.

## Data attribution and reproducibility

**GeoNames** — https://www.geonames.org/ — licensed under [Creative Commons Attribution 4.0](https://creativecommons.org/licenses/by/4.0/). Downloaded 2026-09-13 from:

- https://download.geonames.org/export/dump/cities5000.zip
- https://download.geonames.org/export/dump/admin1CodesASCII.txt
- https://download.geonames.org/export/dump/countryInfo.txt

This snapshot includes 69,711 populated-place records. The bundled derivative `internal/places/cities.tsv.gz` retains ID/coordinates and combines place, administrative-region and country labels; alternate names and other source columns are omitted. Data is provided without accuracy or completeness guarantees. The map UI includes GeoNames credit and a license link.

**Natural Earth** — https://www.naturalearthdata.com/ — public-domain 1:110m country outlines. Source: https://raw.githubusercontent.com/nvkelso/natural-earth-vector/master/geojson/ne_110m_admin_0_countries.geojson . Downloaded 2026-09-13; properties were removed and the 177 geometries minified into `web/public/maps/world.geojson`. See https://www.naturalearthdata.com/about/terms-of-use/ .

To reproduce the bundled files, download those four sources into a local directory (name the Natural Earth file `world.geojson`) and run `python scripts/build_map_data.py DIRECTORY` from this repository, then rebuild. Review dataset updates before distributing; a new dataset does not automatically relabel previously checked photos.

Leaflet 1.9.4 is bundled through npm under its BSD-2-Clause license. No external scripts, geocoding service or API credentials are used.

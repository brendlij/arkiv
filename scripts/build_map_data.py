"""Build bundled map data from downloaded public datasets; never reads photo files.
Usage: python scripts/build_map_data.py DIRECTORY
DIRECTORY contains cities5000.zip, countryInfo.txt, admin1CodesASCII.txt, world.geojson.
See docs/MAPS.md for sources and licenses.
"""
import gzip
import json
import sys
import zipfile
from pathlib import Path
source = Path(sys.argv[1])
root = Path(__file__).resolve().parent.parent
countries = {r.split('\t')[0]: r.split('\t')[4] for r in (source / 'countryInfo.txt').read_text(encoding='utf-8').splitlines() if r and not r.startswith('#')}
regions = {r.split('\t')[0]: r.split('\t')[1] for r in (source / 'admin1CodesASCII.txt').read_text(encoding='utf-8').splitlines() if r}
rows = []
with zipfile.ZipFile(source / 'cities5000.zip') as archive:
    for line in archive.read('cities5000.txt').decode('utf-8').splitlines():
        row = line.split('\t')
        label = ', '.join(dict.fromkeys(part for part in [row[1], regions.get(row[8] + '.' + row[10], ''), countries.get(row[8], row[8])] if part))
        rows.append('\t'.join([row[0], row[4], row[5], label]))
(root / 'internal/places/cities.tsv.gz').write_bytes(gzip.compress('\n'.join(rows).encode('utf-8'), mtime=0))
world = json.loads((source / 'world.geojson').read_text(encoding='utf-8'))
for feature in world['features']:
    feature['properties'] = {}
(root / 'web/public/maps/world.geojson').write_text(json.dumps(world, separators=(',', ':')), encoding='utf-8')
print(f'Bundled {len(rows)} named places and {len(world["features"])} country outlines.')

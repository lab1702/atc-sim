#!/usr/bin/env python3
"""Project DTW OpenStreetMap geometry into the simulator's local metre grid.

Usage: python3 scripts/import-airport.py /path/to/overpass.json
If no input is supplied, fetch the documented public Overpass query. This script
uses only Python's standard library. Network access is needed only to refresh.
"""
import argparse
import collections
import datetime
import json
import math
from pathlib import Path
import urllib.parse
import urllib.request

CENTER = {"lat": 42.2124, "lon": -83.3534}
QUERY = '[out:json][timeout:60];(way["aeroway"](42.18,-83.40,42.25,-83.31);node["aeroway"="gate"](42.18,-83.40,42.25,-83.31););out geom;'
FAA = 'https://aeronav.faa.gov/d-tpp/2609/00119AD.PDF'
LAYOUT = 'https://cdn.metroairport.com/uploads/business_documents/master_plans/DTW_MASTER_PLANS/DTW%20EXIST%20AIRPORT%20LAYOUT%20PLAN_Nov2021%20Update.pdf'
# Published FAA physical runway dimensions in feet; OSM supplies the coordinates.
DIMENSIONS = {
    '03L/21R': (8501, 150), '03R/21L': (10001, 150),
    '04L/22R': (10000, 150), '04R/22L': (12003, 200),
    '09L/27R': (8708, 150), '09R/27L': (8500, 150),
}
GATES = ['A12', 'A20', 'A28', 'A40', 'A50', 'A60', 'A70',
         'A15', 'A25', 'A35', 'A55', 'B2', 'B10', 'C2', 'D2', 'D10', 'D20']


def project(p):
    """WGS84 local equirectangular plane: x east, y north; rounded to 1 cm."""
    return {
        'x': round(math.radians(p['lon'] - CENTER['lon']) * 6371008.8 * math.cos(math.radians(CENTER['lat'])), 2),
        'y': round(math.radians(p['lat'] - CENTER['lat']) * 6371008.8, 2),
    }


def distance(a, b):
    return math.hypot(a['x'] - b['x'], a['y'] - b['y'])


def numeric(value, default):
    try:
        return float(value)
    except (TypeError, ValueError):
        return default


def on_segment(p, a, b):
    dx, dy = b['x'] - a['x'], b['y'] - a['y']
    d2 = dx * dx + dy * dy
    f = max(0, min(1, ((p['x'] - a['x']) * dx + (p['y'] - a['y']) * dy) / d2)) if d2 else 0
    return {'x': round(a['x'] + f * dx, 2), 'y': round(a['y'] + f * dy, 2)}


def connect_stands(taxiways):
    """Join open apron gaps without changing mapped taxiway centerlines.

    OSM aircraft stand lines often stop before the adjacent taxilane. These
    short straight apron movements are explicitly modeled, not map features.
    Insert the projection node into its mapped segment for exact graph joins.
    """
    main = [t for t in taxiways if t['kind'] != 'parking_position']
    joins = []
    for stand in [t for t in taxiways if t['kind'] == 'parking_position']:
        candidates = []
        for p in (stand['points'][0], stand['points'][-1]):
            for lane in main:
                for i, (a, b) in enumerate(zip(lane['points'], lane['points'][1:])):
                    q = on_segment(p, a, b)
                    candidates.append((distance(p, q), p, q, lane, i))
        gap, p, q, lane, i = min(candidates, key=lambda c: c[0])
        if gap > 150:
            raise ValueError(f"Stand {stand['name']} has no plausible apron connection ({gap:.1f} m)")
        if q not in lane['points'][i:i + 2]:
            lane['points'].insert(i + 1, q)
        if gap > 0.01:
            joins.append({'id': f"apron-link-{stand['id']}", 'name': '',
                          'points': [p, q], 'width': 10, 'kind': 'apron_connector',
                          'modeled': True})
    taxiways.extend(joins)
    return len(joins)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('input', nargs='?', type=Path)
    parser.add_argument('--output', type=Path, default=Path(__file__).resolve().parents[1] / 'data' / 'dtw.json')
    args = parser.parse_args()
    if args.input:
        raw = json.loads(args.input.read_text())
    else:
        req = urllib.request.Request('https://overpass-api.de/api/interpreter',
                                     data=urllib.parse.urlencode({'data': QUERY}).encode(),
                                     headers={'User-Agent': 'DTWTowerSim/0.1'})
        with urllib.request.urlopen(req, timeout=80) as response:
            raw = json.load(response)
    elements = raw['elements']
    runways, taxiways, buildings, aprons, gates = [], [], [], [], []
    runway_parts = collections.defaultdict(list)
    gate_labels = {e.get('tags', {}).get('ref'): e for e in elements
                   if e.get('tags', {}).get('aeroway') == 'gate'}
    for e in elements:
        t = e.get('tags', {})
        kind = t.get('aeroway')
        pts = [project(p) for p in e.get('geometry', [])]
        if len(pts) < 2:
            continue
        if kind == 'runway' and t.get('ref') in DIMENSIONS:
            runway_parts[t['ref']].extend(pts)
        elif kind in ('taxiway', 'taxilane') or (kind == 'parking_position' and t.get('ref') in GATES):
            name = t.get('ref', t.get('name', ''))
            taxiways.append({'id': str(e['id']), 'name': name, 'points': pts,
                             'width': numeric(t.get('width'), 10 if kind == 'parking_position' else 22.86),
                             'kind': kind})
            if kind == 'parking_position':
                ref = t['ref']
                # OSM gate nodes usually sit at the jet bridge. The nearest end
                # of its parking-position line is the mapped aircraft stand.
                label = project(gate_labels[ref]) if ref in gate_labels else pts[-1]
                stand = min((pts[0], pts[-1]), key=lambda p: distance(p, label))
                gates.append({'id': ref, **stand, 'osmWay': e['id']})
        elif kind in ('terminal', 'hangar'):
            if t.get('access') == 'no':
                continue
            buildings.append({'name': t.get('name', 'Hangar' if kind == 'hangar' else 'Terminal'),
                              'points': pts, 'height': numeric(t.get('height'), 18 if kind == 'terminal' else 22),
                              'osmWay': e['id']})
        elif kind == 'apron':
            aprons.append({'name': t.get('name', 'Apron'), 'points': pts})
    apron_connectors = connect_stands(taxiways)
    for pair, dims in DIMENSIONS.items():
        pts = runway_parts[pair]
        if not pts:
            raise ValueError(f'Missing runway geometry: {pair}')
        # OSM divides some physical runways into several ways. Use their most
        # distant nodes so a displaced-threshold segment is not lost.
        a, b = max(((a, b) for a in pts for b in pts), key=lambda ab: distance(*ab))
        axis = 'x' if pair.startswith('09') else 'y'
        start, end = sorted((a, b), key=lambda p: p[axis], reverse=True)
        opposite, operational = pair.split('/')
        runways.append({'id': operational, 'opposite': opposite, 'start': start, 'end': end,
                        'width': round(dims[1] * 0.3048, 2), 'length': round(dims[0] * 0.3048, 2)})
    gates.sort(key=lambda g: GATES.index(g['id']))
    # Center of the octagonal tower shaft identifiable in OSM way32481454.
    tower = {**project({'lat': 42.2126426, 'lon': -83.3543714}),
             'height': round((851 - 645) * 0.3048, 2)}
    allpoints = [p for r in runways for p in (r['start'], r['end'])]
    allpoints += [p for t in taxiways + buildings + aprons for p in t['points']]
    airport = {
        'id': 'KDTW', 'name': 'Detroit Metropolitan', 'elevation': 645, 'center': CENTER,
        'bounds': {'minX': math.floor(min(p['x'] for p in allpoints) - 350),
                   'maxX': math.ceil(max(p['x'] for p in allpoints) + 350),
                   'minY': math.floor(min(p['y'] for p in allpoints) - 350),
                   'maxY': math.ceil(max(p['y'] for p in allpoints) + 350)},
        'runways': runways, 'taxiways': taxiways, 'buildings': buildings,
        'aprons': aprons, 'gates': gates, 'tower': tower,
        'sources': [
            {'title': 'OpenStreetMap contributors (ODbL)', 'url': 'https://www.openstreetmap.org/copyright'},
            {'title': 'FAA DTW airport diagram, 03 Sep–01 Oct 2026', 'url': FAA},
            {'title': 'WCAA airport layout plan, November 2021', 'url': LAYOUT},
        ],
        'metadata': {
            'osmSnapshot': raw.get('osm3s', {}).get('timestamp_osm_base'),
            'importedOn': datetime.date.today().isoformat(),
            'projection': 'WGS84 local equirectangular, metres east/north',
            'license': 'OSM-derived geometry: Open Database License (ODbL) 1.0',
            'modeledApronConnectors': apron_connectors,
            'limitations': 'Static simulation layout; no NOTAM or live airport-state feed. Taxiway widths and building heights are simplified; tower height is eye level above the flat 645 ft field datum.',
        },
    }
    args.output.parent.mkdir(parents=True, exist_ok=True)
    args.output.write_text(json.dumps(airport, separators=(',', ':')) + '\n')
    print(f'{args.output}: {len(runways)} runways, {len(taxiways)} surface paths, {len(buildings)} buildings, {len(gates)} stands')


if __name__ == '__main__':
    main()

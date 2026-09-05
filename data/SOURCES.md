# DTW geometry sources

`dtw.json` contains real mapped centerlines and building footprints projected to
metres east (`x`) and north (`y`) of 42.2124°N, 83.3534°W. It is a static layout
for the simulator, not a live airport operational database.

- **OpenStreetMap contributors**, snapshot **2026-07-15 15:22:01 UTC**, retrieved
  2026-09-05 from the public Overpass service. Runway coordinates, taxiway and
  taxilane centerlines, apron polygons, terminal and hangar footprints, and
  aircraft parking-position lines come from this snapshot. Original OSM way IDs
  are retained. Shared geometry nodes remain identical after projection.
  [Map](https://www.openstreetmap.org/#map=14/42.2124/-83.3534) ·
  [Copyright and ODbL license](https://www.openstreetmap.org/copyright).
- **FAA airport diagram AL-119**, effective **03 Sep–01 Oct 2026**, cross-checks
  runway designations, published physical lengths and widths, field elevation,
  overall layout and major taxiway names.
  [FAA diagram](https://aeronav.faa.gov/d-tpp/2609/00119AD.PDF).
- **Wayne County Airport Authority airport layout plan**, November 2021,
  gives the tower cab eye elevation as 851 ft MSL and typical taxiway width as
  75 ft. The simulated flat field is 645 ft MSL, making camera eye height 62.79 m.
  [WCAA airport layout plan](https://cdn.metroairport.com/uploads/business_documents/master_plans/DTW_MASTER_PLANS/DTW%20EXIST%20AIRPORT%20LAYOUT%20PLAN_Nov2021%20Update.pdf).

Runways point from the threshold named by `id` to the opposite end: 21R, 21L,
22R, 22L, 27R and 27L. Segmented OSM runway ways are merged. FAA widths override
OSM width tags where they disagree (notably 03L/21R). Physical pavement ends are
shown; displaced thresholds and declared landing distances are not modeled.

Selected gates retain actual OSM stand labels and use the aircraft end of mapped
parking-position lines, rather than the terminal-side jet-bridge node. Ground
paths include these stand lead-ins. Unnamed paths have an empty name rather than
an invented taxiway designation.

OSM parking lines often stop short of a taxiway. Seventeen short straight apron
connectors join the chosen stand lead-ins to their nearest mapped taxiway
segment. These carry `kind: "apron_connector"` and `modeled: true`; they are
simulation movements across open apron, not surveyed or named taxiway geometry.
The join point is inserted into the mapped segment to preserve graph connectivity.

Building footprints are mapped; heights default to illustrative 18 m for
terminals and 22 m for hangars unless an OSM height exists. Terrain is flat.
Taxiway widths default to 22.86 m, stand lines to 10 m. The tower position is the
mapped octagonal shaft within OSM way 32481454. Runway closure status,
construction, NOTAMs, restrictions, hold markings and current gate assignments
are not reproduced. The September live OSM map differed on part of 09R/27L;
the checked-in July snapshot intentionally retains its documented full layout,
consistent with the cited FAA diagram. No claim of current operational status
is made.

To regenerate from a saved Overpass JSON response:

```sh
python3 scripts/import-airport.py /path/to/overpass.json
```

Omit the input file to fetch fresh OSM data. Fresh geometry must be reviewed
against an appropriate FAA diagram before replacing the snapshot. The importer
contains the exact geographic query and the FAA dimensions used here.

# DTW Metro Tower

![DTW Metro Tower simulator showing flight strips, ground radar with a selected taxi route, a 3D tower view, and aircraft controls](docs/screenshot.jpg)

A single-player air traffic control simulator written in Go, with synchronized
WebGL 2 ground radar and a 3D view from Detroit Metropolitan Airport's control
tower. Each player gets an independent game at the same URL. The simulation runs
in Go; the browser renders its live state and sends aircraft clearances.

## Run

Requires Go 1.24 or newer and a desktop browser with WebGL 2 enabled.

```sh
go run .
```

Open **http://127.0.0.1:8080**. The server binds to your computer's loopback
interface by default. No Node.js installation, package installation, API keys,
or external services are needed to run the simulator.

To build a portable executable with the UI and airport data embedded:

```sh
go build -buildvcs=false -trimpath -o bin/atc-sim .
./bin/atc-sim
```

For UI development, use `go run . -dev` to serve editable files from `web/`;
reload the browser after edits.

Use `-addr 127.0.0.1:8090` to change the listening address.

## Independent games

Multiple players can open the same server URL and play independently. Aircraft,
clearances, pause, speed, traffic flow, resets, and statistics belong to each game.
No account or room code is needed.

- A browser cookie identifies your game. Refreshing the page resumes it, and
  tabs in the same browser profile share it. Use a different browser profile or
  a private window to try another independent game on one computer. Private
  windows may share cookies with other private windows in the same browser.
- A game stops advancing when its last connected tab disconnects. Reconnecting
  resumes it with its previous pause and speed settings. After 30 minutes with
  no connected tabs, the game expires and the next visit starts a new one.
- Games are kept in server memory. Restarting the server starts everyone over;
  open pages reconnect automatically. Cookies must be allowed for the site.
- The default limit is 256 games, including disconnected games awaiting expiry.
  If the server is full, new players wait and retry automatically; existing
  players can continue. Set `-max-sessions` and `-session-timeout` to adjust these
  limits, for example `go run . -max-sessions 100 -session-timeout 15m`.

To let other players connect over your local network:

```sh
go run . -addr 0.0.0.0:8080
```

Share `http://<your-computer-ip>:8080`. For public internet hosting, use an HTTPS
reverse proxy that passes cookies and streams `/api/events` without buffering.
The server has no sign-in requirement; anyone who can reach it can start a game.

## Play

- Click an aircraft or flight strip. A departure is already holding short when
  the session starts, so you can immediately issue a takeoff clearance.
- For a departure at a gate, select a runway and **Taxi to runway**. The aircraft
  follows the taxiway network and holds short. **Line up & wait** is optional;
  **Clear for takeoff** authorizes entering the runway and departing.
- For an arrival, issue **Clear to land** before short final (about 1 NM). Without
  clearance, it goes around. Landing aircraft vacate and taxi back to a stand.
- Use **Hold position** and **Resume taxi** to manage ground traffic. Runway
  reservations, occupancy, intersecting strips and nearby aircraft are checked.
- Airborne aircraft accept heading, altitude and speed instructions. Heading
  uses degrees from true north; altitude is feet MSL; speed is knots. A landing
  clearance returns an aircraft to its assigned approach. Issuing a vector
  cancels any existing landing clearance; clear the aircraft to land again
  when ready.
- Drag and scroll on the ground map to pan and zoom. Drag the tower view to look
  around, scroll to change its field of view, or track the selected aircraft.
  The 2D/3D selector exchanges the main and secondary views.
- Adjust **Traffic flow** for light, normal or busy continuous traffic. Pause
  or accelerate time with the top controls. **Space** pauses; **Escape** clears
  selection. The help dialog contains **Reset session**.

## Implementation

- Standard-library Go HTTP server, embedded assets, fixed simulation steps with
  0.1-second physics substeps, and Server-Sent Events at 10 Hz.
- Go owns aircraft motion, ground routes, clearances, continuous traffic,
  runway reservations and activity history.
- Plain JavaScript ES modules and locally vendored Three.js 0.180.0. Static
  airport geometry is batched into GPU buffers; aircraft positions interpolate
  between server snapshots. Rendering pauses while the tab is hidden.
- No frontend build step or runtime CDN requests. The Three.js MIT license is
  included in `web/vendor/THREE-LICENSE.txt`.

```sh
go test -race ./...
go vet ./...
```

## Airport fidelity

The static dataset contains all six physical runways, mapped taxiway and stand
paths, terminal footprints, aprons and selected real stand labels. It uses an
OpenStreetMap July 2026 snapshot, checked against the FAA September 2026 airport
diagram. Short modeled apron connectors join stand paths to the routing graph.
See [data/SOURCES.md](data/SOURCES.md) for sources, dates, licenses and the
reproducible importer.

This is simulation-lite: aircraft performance and procedures are simplified,
traffic and callsigns are synthetic, and wind is fixed to a southerly operation.
Terrain is flat, building heights are illustrative, and this first version
operates the south/west runway ends. It does not model live traffic, NOTAMs,
weather changes, wake categories, detailed phraseology, displaced thresholds,
or real-world gate availability. It is not an operational training tool.

## License

Original project source code is licensed under the [MIT License](LICENSE).

Airport geometry is © OpenStreetMap contributors, licensed under ODbL 1.0.
See [data/SOURCES.md](data/SOURCES.md) for data sources and attribution.

Vendored Three.js retains its own [MIT license](web/vendor/THREE-LICENSE.txt).

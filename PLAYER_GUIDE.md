# DTW Metro Tower: Player Guide

You are the air traffic controller at Detroit Metropolitan Airport (DTW). Your
job is to help aircraft leave their parking places, reach a runway, take off,
and land without getting in each other's way. You give permission for movements;
the simulated pilots do the flying and follow the ground routes automatically.

This is an open-ended game with continuous traffic. There is no campaign or
finish line. Aim to keep aircraft moving, resolve warnings, and avoid unnecessary
go-arounds. The counters show how many aircraft have landed, departed, or tried
a landing again.

No aviation knowledge is required. The procedures are simplified for play, and
the game is not a real-world air traffic control training tool.

Start with the airport terms and walkthrough below. For a quick reference, jump
to [screen controls](#finding-your-way-around-the-screen),
[departures](#getting-departures-into-the-air),
[arrivals](#bringing-arrivals-down-and-into-a-stand),
[traffic management](#sharing-runways-and-taxiways),
[airborne instructions](#optional-directing-aircraft-in-the-air), or
[troubleshooting](#when-something-seems-stuck).

## The airport in plain language

| Term | What it means in the game |
| --- | --- |
| Aircraft | An airplane you can select and give instructions to. |
| Callsign | A flight's identifying name, such as `DAL1234`. Use it to match an aircraft with its messages. |
| Arrival | An aircraft coming to DTW to land and park. |
| Departure | An aircraft leaving DTW, starting at a parking place or waiting near a runway. |
| Gate / stand | A parking place for an aircraft. The interface uses both names. |
| Terminal / apron | The passenger building and the paved parking area around it. |
| Taxi / taxiway | To taxi means to move slowly on the ground. Taxiways are the paths between parking places and runways. |
| Runway | The long strip of pavement used for takeoff and landing. |
| Clearance | Your permission for an aircraft to do something, issued by clicking a command button. |
| Hold short | Stop at the waiting point before entering a runway. |
| Approach / final | The flight toward a runway for landing. Short final is the last part, very close to the runway. |
| Go-around | Abandon a landing attempt, climb away, and prepare to try again. |
| Vacate | Leave the runway onto a taxiway. Touching down does not mean the runway is clear yet. |

Runway labels identify the direction in which the runway is used. For example,
`21R / 03L` names the two opposite ends of **one physical runway**. `L` and `R`
mean left and right when looking down the runways in that direction. This game
uses the south/west ends: **21R, 21L, 22R, 22L, 27R, and 27L**. Keep the starting
runway assignments while learning.

## Your first few minutes

If the game is not running yet, follow the [README's run instructions](README.md#run).
Once the page says **CONNECTED**:

1. Click **Pause** and set **Traffic flow** to **Light**. Leave the simulation
   at **1× speed**. You can select aircraft and issue clearances while paused;
   movement begins when you resume.
2. In **Flight strips**, select the departure marked **Holding short**. A fresh
   game starts with one waiting at runway **21R**, usually already selected.
3. Leave **Assigned runway** at **21R**. Check the runway status below the map,
   then click **Clear for takeoff**. Look for **Clearance acknowledged.**
   If the command is rejected, read the reason before trying again.
4. Select each **On approach** arrival. Keep its assigned runway and click
   **Clear to land** when the runway is available. You can grant these
   clearances before the aircraft reach the airport.
5. Click **Resume**. Watch the departure enter the runway, accelerate, and
   climb away. The arrivals will line up with their runways and descend without
   heading or altitude instructions from you.
6. Select a departure marked **At gate**, keep its runway assignment, and click
   **Taxi to runway**. Its planned ground route appears in white. It will stop
   automatically at the runway's holding point.
7. When that departure shows **Holding short**, check for arriving traffic and
   runway availability, then use **Clear for takeoff**.
8. Watch a landed arrival slow down, leave the runway, and taxi to a stand.
   This normally happens automatically. Pause whenever you want time to inspect
   the next aircraft or read a warning.

## Finding your way around the screen

**Flight strips** are the aircraft cards on the left. Each shows the callsign,
aircraft model, arrival/departure type, assigned runway (`RWY`), current activity,
and altitude or ground speed. You do not need to memorize aircraft models.
Click a strip to select that flight. **All**, **Arrivals**, and **Departures**
filter the list; hidden flights still continue moving.

**Ground radar** is the overhead map. Departures are mint green, arrivals are
amber, and the selected aircraft's ground route is white. The map shows airborne
aircraft too, so check altitude before assuming two nearby symbols are both on
the ground. Drag to pan, scroll to zoom, or use **+**, **−**, and the reset-view
button. Zoom in to see more taxiway labels. Click an aircraft or its label to
select it; use its flight strip if it is difficult to find.

**Tower view** shows the same aircraft in 3D from the control tower. Drag to
look around and scroll to change the zoom. **Track selected** keeps the tower
camera pointed at your selection; click **Stop tracking** to stop following it.
**2D map** and **3D tower** choose which view fills the main panel.

**Aircraft control** shows the selected flight's current activity, heading,
altitude, speed, clearance, and alerts. Buttons appear or become available when
appropriate for that aircraft. Choosing a runway or stand in a dropdown does
not issue an instruction: you must click the corresponding clearance button.
Read the result below the controls to see whether it was accepted.

**Runways**, below the main view, shows which runways are busy. Hover over a
runway indicator for its available, occupied, or reserved status. **Radio &
activity**, along the bottom, records clearances, completed movements, and
warnings, with the newest messages first. You issue commands with the interface;
you do not need to speak or type radio phrases.

## Getting departures into the air

A normal departure follows this sequence:

**At gate → Taxiing → Holding short → Lined up → Takeoff roll → Climbing out**

1. **Taxi to runway:** Select a departure at a gate, choose its runway, and
   click this button. The game finds a route through the taxiways. You do not
   need to push the aircraft back separately or specify every turn.
2. **Wait for Holding short:** A taxi clearance stops the aircraft before the
   runway. It does not permit takeoff, and you cannot give takeoff clearance
   while it is still taxiing to the holding point.
3. **Clear for takeoff:** When the runway can be used, click this button. From
   the holding point, the aircraft enters and aligns with the runway, then
   accelerates and takes off automatically. You do not need a second command
   when its activity changes to **Lined up**.

**Line up & wait** is an optional intermediate instruction. It puts the aircraft
on the runway facing the takeoff direction, but it will wait there until you
click **Clear for takeoff**. It reserves the runway and prevents another aircraft
from receiving a conflicting clearance. For your first games, using takeoff
clearance directly from the holding point is easier.

Once an aircraft is **Climbing out**, it climbs and flies away automatically.
Eventually it is handed off to departure control, meaning it leaves your part
of the simulation and disappears from your flight strips.

To change a departure's runway while it is at a gate, taxiing, or holding short,
choose the new runway and click **Taxi to runway** again. It must reach the new
holding point before you clear it for takeoff. Changing the dropdown alone does
not move it to another runway.

## Bringing arrivals down and into a stand

A normal arrival follows this sequence:

**On approach → Landing roll → Taxi to gate → Parked and removed from the list**

Select an arrival and click **Clear to land** while it is still approaching.
The game handles its alignment, speed, and descent. Give the clearance while
the aircraft is still more than **1 nautical mile** from the runway, before it
reaches short final. A nautical mile (`NM`) is about 1.85 km or 1.15 ordinary
miles. An arrival still on its automatic approach without clearance on short
final will go around.

A landing clearance reserves the runway immediately, even if the aircraft is
still far away. This blocks conflicting runway clearances, so avoid committing
a shared runway too early when another movement needs to finish first. While
learning, prioritize arrivals that are already close and leave waiting
departures at their holding points.

After touchdown, **Landing roll** means the aircraft is slowing down on the
runway. It then finds a free stand, takes a mapped exit, and starts **Taxi to
gate** automatically. Wait for the runway to be vacated before trying a
conflicting clearance. The arrival disappears from the active list after it
parks; it does not need an unloading command.

While an arrival shows **Taxi to gate**, you can change its destination using
**Destination stand → Taxi to stand**. The new stand must be free, not assigned
to another aircraft, and reachable by the taxiway network. A successful stand
clearance also resumes an aircraft you had held. The stand selector is not
available while the aircraft is still landing.

### Recovering from a go-around

A go-around is a recovery, not the end of your game. It can happen because a
landing clearance was missing, the runway became unavailable, or the aircraft
could not reach a suitable landing position. You can also click **Go around**
for an arrival on approach when you want it to abandon that attempt.

The aircraft climbs along the runway direction to 3,000 feet above the airport,
or keeps its current altitude if already higher. Without manual instructions,
it eventually returns toward another approach and needs landing clearance again.
You can select an available runway and click **Clear to land** even while it
shows **Going around**. The game guides it back toward an approach point; give
it time to turn and return.

## Sharing runways and taxiways

An **occupied** runway has an aircraft on it. A **reserved** runway has been
committed to an aircraft by a landing, lineup, or takeoff clearance. Either can
prevent another clearance. Runways that physically cross each other also affect
one another: an available-looking runway can still be blocked by activity on an
intersecting runway. A nearby arrival can also prevent a takeoff clearance.
The command result names the reason for rejection.

Taxiing aircraft automatically stop for occupied or reserved runway crossings,
nearby ground traffic, and some merging routes. There is no separate runway
crossing button. At a merge where routes join in the same direction, the nearer
aircraft normally leads; aircraft leaving a reserved runway get priority.
Automatic waits are checked continuously, so the aircraft resumes when the path
clears.

**Hold position** lets you stop a taxiing aircraft yourself. **Resume taxi**
releases that manual hold. It does not override an automatic safety stop or
authorize takeoff. An aircraft waiting normally at **Holding short** needs
**Clear for takeoff** or **Line up & wait** to enter its runway.

If two aircraft meet head-on and neither can move, waiting longer may not solve
it. Pause, select each aircraft to inspect its route, and reroute one:

- For a departure, choose another runway and issue **Taxi to runway** again.
- For an arrival already taxiing in, choose a different free **Destination
  stand** and issue **Taxi to stand**.

Check that the new white route avoids the opposing traffic, then resume the
simulation. A manual hold can help sequence traffic before paths become blocked,
but it cannot create space between two aircraft already facing each other.

## Optional: directing aircraft in the air

You can play your first flights without using these controls. A **vector** is
an instruction to fly a particular direction, altitude, or speed. These controls
work for **On approach**, **Going around**, and **Climbing out** aircraft.

| Field | How to read it |
| --- | --- |
| **HDG °** | Heading: the direction to fly, clockwise from true north. `000` is north, `090` east, `180` south, and `270` west. Enter 0–359. |
| **ALT ft** | Altitude in feet above mean sea level (`MSL`), not above the ground. DTW's ground is modeled at 645 ft, so 3,000 ft displayed is about 2,355 ft above the airport. The input accepts 1,200–20,000 ft in 100 ft steps. |
| **SPD kt** | Speed in knots: nautical miles per hour. One knot is about 1.85 km/h or 1.15 mph. The input accepts 100–320 kt in 5 kt steps. |

Edit the fields you want to change, then click **Issue heading / altitude /
speed**. Only fields you edit are sent. The aircraft turns, climbs, descends,
or changes speed gradually; the displayed current values will take time to
reach your targets.

**Any vector to an arrival cancels its landing clearance**, even if you change
only speed. It also switches the aircraft away from automatic approach handling.
Manual instructions continue through a runway flyover; the aircraft will not
automatically land just because it is pointed toward the runway.

When ready to bring it in, select the desired runway and click **Clear to land**
again. This restores the automatic approach. If needed, the aircraft first flies
toward an approach point before turning back to land.

An **Airborne proximity** warning means two aircraft are too close together
both horizontally and in altitude. The warning does not steer them apart.
Pause, inspect their positions and headings, and use vectors to give them
different paths or altitudes. Remember to renew an arrival's landing clearance
when it is ready to return to the approach.

## Pacing, progress, and returning later

- **Pause / Resume:** Stops or starts simulation time. Use the button whenever
  you need thinking time. **Space** toggles pause when you are not interacting
  with a form field or button and the help dialog is closed.
- **1× / 2× / 4× / 8× speed:** Changes how quickly simulation time passes.
  Aircraft move and new traffic arrives faster in real time at higher settings.
- **Traffic flow — Light / Normal / Busy:** Changes how often new traffic
  appears and how many flights can be active. It does not remove existing traffic
  when you lower it. Learn at Light and 1× before increasing either setting.
- **LANDED / DEPARTED / GO-AROUNDS:** Count touchdowns, takeoffs, and go-around
  attempts. A landed aircraft still needs to clear the runway and park. The
  game has no combined points total or automatic game-over screen.
- **Escape:** Clears your aircraft selection during normal play. If a field
  has focus or the help dialog is open, the normal selection shortcut is inactive.
- **? → Reset session:** Immediately starts a fresh game, clearing your traffic,
  clock, and counters and returning to Normal traffic at 1×, unpaused. Opening
  help by itself does not pause the game.

Refreshing the page resumes your current game. Tabs in the same browser profile
share that game, including pause and reset; other players have independent games.
Pause before switching to another tab if you want your traffic to wait.

When your last connected game tab disconnects, your simulation stops advancing.
By default, it is kept for 30 minutes after disconnection, though a server owner
can change that timeout. A server restart or an expired session starts you over.
Games are not permanently saved, and the site needs cookies to remember yours.

## When something seems stuck

| What you see | What to do |
| --- | --- |
| A command is gray or missing | Check the selected aircraft's activity. Takeoff requires a holding point or lineup; stand routing requires an arrival taxiing in; vectors require an airborne aircraft. Also check the connection status. |
| Changing the runway or stand did nothing | Click the matching clearance button after changing the dropdown. |
| Runway occupied, reserved, or arrival on short final | Wait for that movement to clear, or use a suitable alternative runway. A departure must taxi to a different runway before taking off from it. |
| Ground proximity, taxi merge, or crossing held | Let the blocking aircraft move. Automatic waits clear themselves; use rerouting if the aircraft are stuck on opposing paths. |
| Resume taxi is unavailable | The aircraft may be waiting automatically, rather than under your **Hold position** instruction. Read its clearance and alert. |
| No free stand · holding on runway | Taxi waiting departures away from stands to free parking space. The landed aircraft checks again automatically. |
| No connected runway exit · holding | The aircraft could not find a usable mapped route off the runway. It still blocks the runway; use others for traffic. Reset the session if you want to clear the stuck situation. |
| An arrival flies past instead of landing | Read the activity log for a go-around reason. If you gave it a vector, issue **Clear to land** again to restore its approach. |
| RECONNECTING, or an instruction timed out | Wait for the connection to recover, then check the flight's clearance before retrying. |

A useful habit is to check arrivals first, then runway availability, then
departures waiting at holding points, and finally ground traffic. Keep returning
to **All** flight strips so you do not overlook aircraft hidden by a filter.

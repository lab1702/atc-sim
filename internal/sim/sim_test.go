package sim

import (
	"encoding/json"
	"math"
	"os"
	"strings"
	"testing"
)

func number(v float64) *float64  { return &v }
func boolean(v bool) *bool       { return &v }
func textValue(v string) *string { return &v }

func testAirport() Airport {
	a := Airport{ID: "TEST", Name: "Test airport", Elevation: 645}
	a.Runways = []Runway{
		{ID: "21R", Opposite: "03L", Start: Point{0, 1000}, End: Point{0, -1000}, Width: 40, Length: 2000},
		{ID: "27R", Opposite: "09L", Start: Point{1000, 0}, End: Point{-1000, 0}, Width: 40, Length: 2000},
	}
	a.Taxiways = []Taxiway{
		{ID: "A", Points: []Point{{-100, -900}, {-100, 0}, {-100, 200}, {-100, 900}}},
		{ID: "N", Points: []Point{{-100, 900}, {0, 900}, {0, 1000}}},
		{ID: "S", Points: []Point{{-100, -900}, {0, -900}, {0, -1000}}},
		{ID: "E", Points: []Point{{-100, 0}, {0, 0}, {900, 0}, {900, 100}, {1000, 100}}},
		{ID: "G1", Points: []Point{{-100, 200}, {-300, 200}}},
		{ID: "G2", Points: []Point{{-300, 200}, {-300, 400}}},
	}
	a.Gates = []Gate{{ID: "A1", X: -300, Y: 200}, {ID: "A2", X: -300, Y: 400}}
	return a
}

func isolate(s *Simulation, f *flight) {
	s.flights = []*flight{f}
	s.reservations = make(map[int]string)
	s.nextTraffic = math.Inf(1)
}

func tickUntil(t *testing.T, s *Simulation, limit float64, condition func() bool) {
	t.Helper()
	for elapsed := 0.0; elapsed < limit; elapsed += 0.1 {
		if condition() {
			return
		}
		s.Tick(0.1)
	}
	t.Fatalf("condition not reached in %.0fs; state: %+v", limit, s.Snapshot())
}

func TestDepartureTaxiTakeoffAndHandoff(t *testing.T) {
	s := New(testAirport())
	f := s.flights[1]
	isolate(s, f)
	if err := s.Command(Command{AircraftID: f.ID, Action: "takeoff"}); err == nil {
		t.Fatal("takeoff from a stand was accepted")
	}
	if err := s.Command(Command{AircraftID: f.ID, Action: "taxi", Runway: "21R"}); err != nil {
		t.Fatal(err)
	}
	if len(f.Route) < 3 {
		t.Fatal("taxi route must follow the mapped corner")
	}
	tickUntil(t, s, 300, func() bool { return f.Phase == "holdshort" })
	if onRunway(f.Position, s.airport.Runways[0], 30) {
		t.Fatal("holding point intrudes into runway")
	}
	if err := s.Command(Command{AircraftID: f.ID, Action: "takeoff"}); err != nil {
		t.Fatal(err)
	}
	if s.Snapshot().Runways[0].ReservedBy != f.ID {
		t.Fatal("takeoff did not reserve runway")
	}
	tickUntil(t, s, 150, func() bool { return f.Phase == "departure" })
	if s.stats.Departures != 1 {
		t.Fatalf("departures=%d", s.stats.Departures)
	}
	tickUntil(t, s, 400, func() bool { return f.Phase == "complete" })
	if s.Snapshot().Runways[0].ReservedBy != "" {
		t.Fatal("departure did not release runway")
	}
}

func TestArrivalLandsVacatesAndParks(t *testing.T) {
	s := New(testAirport())
	f := s.flights[2]
	isolate(s, f)
	if err := s.Command(Command{AircraftID: f.ID, Action: "land"}); err != nil {
		t.Fatal(err)
	}
	tickUntil(t, s, 400, func() bool { return f.Phase == "landing" })
	if s.stats.Arrivals != 1 || f.Altitude != s.airport.Elevation {
		t.Fatal("touchdown state or counter incorrect")
	}
	tickUntil(t, s, 400, func() bool { return f.Phase == "complete" })
	if s.stats.Arrivals != 1 {
		t.Fatal("arrival counted more than once")
	}
	if s.Snapshot().Runways[0].ReservedBy != "" {
		t.Fatal("runway remains reserved after parking")
	}
	if distance(f.Position, Point{-300, 200}) > 1 {
		t.Fatalf("not parked at the gate: %+v", f.Position)
	}
}

func TestNoLandingClearanceTriggersGoAround(t *testing.T) {
	s := New(testAirport())
	f := s.flights[2]
	isolate(s, f)
	tickUntil(t, s, 200, func() bool { return f.Phase == "goaround" })
	if s.stats.GoArounds != 1 || s.stats.Arrivals != 0 {
		t.Fatal("incorrect go-around counters")
	}
	if f.TargetAltitude <= f.Altitude {
		t.Fatal("go-around does not command a climb")
	}
	tickUntil(t, s, 80, func() bool { return f.Phase == "approach" })
	if f.finalFix == nil {
		t.Fatal("go-around did not rejoin an approach fix")
	}
}

func TestRunwayReservationOppositeAndIntersectionSafety(t *testing.T) {
	s := New(testAirport())
	hold, arrival := s.flights[0], s.flights[2]
	s.flights = []*flight{hold, arrival}
	if err := s.Command(Command{AircraftID: arrival.ID, Action: "land", Runway: "03L"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Command(Command{AircraftID: hold.ID, Action: "takeoff"}); err == nil {
		t.Fatal("opposite direction reservation did not block takeoff")
	}
	if err := s.Command(Command{AircraftID: arrival.ID, Action: "goaround"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Command(Command{AircraftID: hold.ID, Action: "lineup"}); err != nil {
		t.Fatal(err)
	}
	arrival.Phase = "approach"
	if err := s.Command(Command{AircraftID: arrival.ID, Action: "land", Runway: "27R"}); err == nil {
		t.Fatal("intersecting runway reservation did not block landing")
	}
}

func TestOccupiedRunwayAndShortFinalRejectTakeoff(t *testing.T) {
	s := New(testAirport())
	hold, other := s.flights[0], s.flights[1]
	other.Position = Point{0, 300}
	other.Phase = "taxi"
	s.flights = []*flight{hold, other}
	if err := s.Command(Command{AircraftID: hold.ID, Action: "takeoff"}); err == nil {
		t.Fatal("occupied runway accepted takeoff")
	}
	other.Phase = "approach"
	other.Altitude = 1500
	other.Position = Point{0, 3000}
	other.Runway = hold.Runway
	if err := s.Command(Command{AircraftID: hold.ID, Action: "takeoff"}); err == nil {
		t.Fatal("short-final traffic accepted takeoff")
	}
}

func TestControlAndVectorValidationAreAtomic(t *testing.T) {
	s := New(testAirport())
	if err := s.Control(Control{Paused: boolean(true), Rate: number(math.NaN())}); err == nil {
		t.Fatal("NaN rate accepted")
	}
	if s.paused {
		t.Fatal("invalid control partially applied")
	}
	if err := s.Control(Control{Difficulty: textValue("unknown")}); err == nil {
		t.Fatal("invalid difficulty accepted")
	}
	if err := s.Control(Control{Paused: boolean(true), Rate: number(4), Difficulty: textValue("hard")}); err != nil {
		t.Fatal(err)
	}
	s.Tick(10)
	if s.time != 0 || s.rate != 4 || s.difficulty != "hard" {
		t.Fatal("pause, rate, or difficulty incorrect")
	}
	if err := s.Control(Control{Paused: boolean(false)}); err != nil {
		t.Fatal(err)
	}
	s.Tick(1)
	if math.Abs(s.time-1) > 0.001 {
		t.Fatal("Tick applied the playback rate twice")
	}
	f := s.flights[2]
	oldHeading := f.TargetHeading
	if err := s.Command(Command{AircraftID: f.ID, Action: "vector", Heading: number(90), Speed: number(math.Inf(1))}); err == nil {
		t.Fatal("infinite speed accepted")
	}
	if f.TargetHeading != oldHeading {
		t.Fatal("invalid vector partially applied")
	}
	if err := s.Command(Command{AircraftID: f.ID, Action: "vector", Heading: number(360), Altitude: number(4000), Speed: number(210)}); err != nil {
		t.Fatal(err)
	}
	if f.TargetHeading != 0 || f.TargetAltitude != 4000 || f.TargetSpeed != 210 {
		t.Fatal("vector did not set targets")
	}
	previousHeading := f.Heading
	s.Tick(1)
	if absHeadingDifference(f.Heading, previousHeading) > 3.01 {
		t.Fatal("aircraft heading changed instantaneously")
	}
}

func TestGraphFollowsConnectedTaxiwaysAndRejectsDisconnected(t *testing.T) {
	g := buildGraph([]Taxiway{{Points: []Point{{0, 0}, {100, 0}, {100, 100}, {200, 100}}}})
	path, err := g.route(Point{0, 0}, Point{200, 100})
	if err != nil {
		t.Fatal(err)
	}
	length := 0.0
	last := Point{0, 0}
	for _, p := range path {
		length += distance(last, p)
		last = p
	}
	if length < 299 {
		t.Fatalf("route cut across a corner: %.2fm", length)
	}
	g = buildGraph([]Taxiway{{Points: []Point{{0, 0}, {100, 0}}}, {Points: []Point{{1000, 0}, {1100, 0}}}})
	if _, err := g.route(Point{0, 0}, Point{1100, 0}); err == nil {
		t.Fatal("disconnected graph invented a route")
	}
}

func TestGroundProximityStopsAndResumeContinues(t *testing.T) {
	s := New(testAirport())
	f, other := s.flights[0], s.flights[1]
	f.Phase = "taxi"
	f.Position = Point{-100, 300}
	f.Route = []Point{{-100, 900}}
	other.Position = Point{-100, 346}
	s.flights = []*flight{f, other}
	s.nextTraffic = math.Inf(1)
	s.Tick(1)
	if f.Speed != 0 || f.Alert == "" || s.stats.Conflicts != 1 {
		t.Fatal("ground proximity did not stop and alert exactly once")
	}
	before := f.Position
	if err := s.Command(Command{AircraftID: f.ID, Action: "hold"}); err != nil {
		t.Fatal(err)
	}
	other.Position = Point{-300, 400}
	s.Tick(5)
	if f.Position != before {
		t.Fatal("held aircraft moved")
	}
	if err := s.Command(Command{AircraftID: f.ID, Action: "resume"}); err != nil {
		t.Fatal(err)
	}
	s.Tick(5)
	if f.Position == before {
		t.Fatal("resume did not continue taxi")
	}
}

func TestSnapshotCopiesRoutesAndStateStaysFinite(t *testing.T) {
	s := New(testAirport())
	f := s.flights[1]
	if err := s.Command(Command{AircraftID: f.ID, Action: "taxi"}); err != nil {
		t.Fatal(err)
	}
	state := s.Snapshot()
	for i := range state.Aircraft {
		if state.Aircraft[i].ID == f.ID {
			state.Aircraft[i].Route[0].X = math.NaN()
		}
	}
	if !finite(f.Route[0].X) {
		t.Fatal("snapshot aliases mutable route")
	}
	s.Tick(math.NaN())
	s.Tick(math.Inf(1))
	s.Tick(-1)
	for i := 0; i < 1200; i++ {
		s.Tick(0.5)
	}
	for _, a := range s.Snapshot().Aircraft {
		for _, v := range []float64{a.Position.X, a.Position.Y, a.Heading, a.Altitude, a.Speed} {
			if !finite(v) {
				t.Fatalf("non-finite state: %+v", a)
			}
		}
		if a.Heading < 0 || a.Heading >= 360 || a.Speed < 0 || a.Altitude < s.airport.Elevation {
			t.Fatalf("invalid state: %+v", a)
		}
	}
}

func TestContinuousTrafficRemainsBoundedAndResetRestoresSession(t *testing.T) {
	s := New(testAirport())
	if err := s.Control(Control{Difficulty: textValue("hard")}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1800; i++ {
		s.Tick(1)
		if len(s.flights) > 22 || len(s.events) > 60 {
			t.Fatal("continuous traffic or event log exceeded its cap")
		}
	}
	if s.aircraftID <= 4 || s.stats.GoArounds == 0 {
		t.Fatal("continuous traffic did not spawn or exercise missed approaches")
	}
	if err := s.Control(Control{Reset: boolean(true), Paused: boolean(true)}); err != nil {
		t.Fatal(err)
	}
	if s.time != 0 || s.stats != (Stats{}) || len(s.flights) != 4 || !s.paused || len(s.reservations) != 0 {
		t.Fatal("reset did not restore a clean seeded session")
	}
}

func TestDTWGroundRoutesAndPlayableInitialClearances(t *testing.T) {
	data, err := os.ReadFile("../../data/dtw.json")
	if err != nil {
		t.Fatal(err)
	}
	var airport Airport
	if err := json.Unmarshal(data, &airport); err != nil {
		t.Fatal(err)
	}
	s := New(airport)
	if len(s.flights) != 4 {
		t.Fatalf("initial traffic=%d", len(s.flights))
	}
	for _, r := range airport.Runways {
		hold, ok := s.graph.holdPoint(r)
		if !ok {
			t.Fatalf("no holding point runway %s", r.ID)
		}
		for _, g := range airport.Gates {
			path, err := s.graph.route(Point{g.X, g.Y}, hold)
			if err != nil {
				t.Fatalf("stand %s -> runway %s: %v", g.ID, r.ID, err)
			}
			if len(path) < 2 {
				t.Fatalf("stand %s runway %s not a taxiway route", g.ID, r.ID)
			}
		}
	}
	hold, arrival := s.flights[0], s.flights[2]
	if err := s.Command(Command{AircraftID: hold.ID, Action: "takeoff"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Command(Command{AircraftID: arrival.ID, Action: "land"}); err != nil {
		t.Fatal(err)
	}
	s.nextTraffic = math.Inf(1)
	tickUntil(t, s, 240, func() bool { return s.stats.Departures == 1 && s.stats.Arrivals == 1 })
	tickUntil(t, s, 900, func() bool { return arrival.Phase == "complete" })
	if arrival.Gate == "" || len(arrival.Route) != 0 {
		t.Fatal("DTW arrival did not complete taxi to a stand")
	}
	for _, e := range s.events {
		if strings.Contains(e.Message, "takeoff held") {
			t.Fatalf("initial takeoff stalled unexpectedly: %s", e.Message)
		}
	}
}

package sim

import (
	"fmt"
	"math"
	"math/rand"
)

type flight struct {
	Aircraft
	holding            bool
	takeoffAfterLineup bool
	landingCleared     bool
	manualVector       bool
	finalFix           *Point
	goAroundUntil      float64
	completedAt        float64
	landingCounted     bool
	groundWarning      string
}

// Simulation is deterministic for a given command/tick sequence. Its caller
// must serialize access; the HTTP server holds one mutex around these methods.
type Simulation struct {
	airport      Airport
	graph        taxiGraph
	flights      []*flight
	time         float64
	paused       bool
	rate         float64
	difficulty   string
	stats        Stats
	events       []Event
	eventID      int
	aircraftID   int
	nextTraffic  float64
	trafficCount int
	reservations map[int]string
	activeAlerts map[string]bool
	random       *rand.Rand
}

func New(airport Airport) *Simulation {
	s := &Simulation{
		airport: airport, graph: buildGraph(airport.Taxiways), rate: 1,
		difficulty: "normal", nextTraffic: 110,
		reservations: make(map[int]string), activeAlerts: make(map[string]bool),
		random: rand.New(rand.NewSource(42)),
	}
	s.seed()
	return s
}

func (s *Simulation) seed() {
	if len(s.airport.Runways) == 0 {
		return
	}
	r, _ := s.runway("21R")
	if r.ID == "" {
		r = s.airport.Runways[0]
	}
	if f := s.spawnDeparture(); f != nil {
		f.Runway = r.ID
		if p, ok := s.graph.holdPoint(r); ok {
			f.Position = p
			f.Phase = "holdshort"
			f.Heading = heading(p, r.Start)
			f.TargetHeading = f.Heading
			f.Clearance = "Holding short runway " + r.ID + " · ready for departure"
		}
	}
	s.spawnDeparture()
	arrivalRunway, _ := s.runway("21L")
	if arrivalRunway.ID == "" {
		arrivalRunway = r
	}
	s.spawnArrival(arrivalRunway, 14000)
	other, _ := s.runway("22R")
	if other.ID == "" {
		other = arrivalRunway
	}
	s.spawnArrival(other, 25000)
	s.log("DTW tower online. Traffic is simulated; select an aircraft to issue a clearance.", "info")
}

func (s *Simulation) log(message, level string) {
	s.eventID++
	s.events = append(s.events, Event{ID: s.eventID, Time: s.time, Message: message, Level: level})
	if len(s.events) > 60 {
		s.events = append([]Event(nil), s.events[len(s.events)-60:]...)
	}
}

func (s *Simulation) nextFlight(kind string) *flight {
	s.aircraftID++
	prefixes := []string{"DAL", "SKW", "UAL", "AAL", "FFT", "SWA"}
	types := []string{"A320", "E175", "B738", "A321", "A320", "B738"}
	i := (s.aircraftID - 1) % len(prefixes)
	f := &flight{Aircraft: Aircraft{
		ID: fmt.Sprintf("ac-%03d", s.aircraftID), Callsign: fmt.Sprintf("%s%d", prefixes[i], 300+s.random.Intn(1600)), Type: types[i], Kind: kind,
		Altitude: s.airport.Elevation, TargetAltitude: s.airport.Elevation, Route: []Point{},
	}}
	s.flights = append(s.flights, f)
	return f
}

func (s *Simulation) freeGate() (Gate, bool) {
	for _, g := range s.airport.Gates {
		free := true
		for _, f := range s.flights {
			if f.Phase == "complete" {
				continue
			}
			if (f.Gate == g.ID && (f.Phase == "gate" || f.Phase == "taxi-in")) || (f.Altitude < s.airport.Elevation+20 && distance(f.Position, Point{g.X, g.Y}) < 60) {
				free = false
				break
			}
		}
		if free {
			return g, true
		}
	}
	return Gate{}, false
}

func (s *Simulation) spawnDeparture() *flight {
	g, ok := s.freeGate()
	if !ok || len(s.airport.Runways) == 0 {
		return nil
	}
	f := s.nextFlight("departure")
	f.Phase = "gate"
	f.Gate = g.ID
	f.Position = Point{g.X, g.Y}
	f.Runway = s.airport.Runways[0].ID
	f.Heading = 30
	f.TargetHeading = f.Heading
	f.Clearance = "At stand " + g.ID + " · request taxi"
	s.log(f.Callsign+" ready to taxi from stand "+g.ID, "info")
	return f
}

func (s *Simulation) spawnArrival(r Runway, rangeMeters float64) *flight {
	f := s.nextFlight("arrival")
	f.Runway = r.ID
	f.Phase = "approach"
	f.Heading = heading(r.Start, r.End)
	f.TargetHeading = f.Heading
	f.Position = add(r.Start, scale(direction(f.Heading), -rangeMeters))
	f.Speed = 180
	f.TargetSpeed = 180
	f.Altitude = s.airport.Elevation + clamp(rangeMeters*math.Tan(3*math.Pi/180)*3.28084, 1500, 6000)
	f.TargetAltitude = f.Altitude
	f.Clearance = "Inbound runway " + r.ID + " · request landing clearance"
	s.log(f.Callsign+fmt.Sprintf(" inbound %.1f NM for runway %s", rangeMeters/1852, r.ID), "info")
	return f
}

func (s *Simulation) runway(id string) (Runway, int) {
	for i, r := range s.airport.Runways {
		if r.ID == id {
			return r, i
		}
		if r.Opposite == id && id != "" {
			r.ID, r.Opposite = r.Opposite, r.ID
			r.Start, r.End = r.End, r.Start
			return r, i
		}
	}
	return Runway{}, -1
}

func (s *Simulation) release(f *flight) {
	for i, id := range s.reservations {
		if id == f.ID {
			delete(s.reservations, i)
		}
	}
}

func (s *Simulation) runwayStates() []RunwayStatus {
	statuses := make([]RunwayStatus, len(s.airport.Runways))
	for i, r := range s.airport.Runways {
		statuses[i] = RunwayStatus{ID: r.ID, ReservedBy: s.reservations[i]}
		for _, f := range s.flights {
			if f.Phase == "complete" || f.Altitude > s.airport.Elevation+30 {
				continue
			}
			if onRunway(f.Position, r, 15) {
				statuses[i].OccupiedBy = f.ID
				break
			}
		}
	}
	return statuses
}

func (s *Simulation) canUseRunway(index int, aircraftID string) error {
	if index < 0 || index >= len(s.airport.Runways) {
		return fmt.Errorf("unknown runway")
	}
	r := s.airport.Runways[index]
	for i, status := range s.runwayStates() {
		if i != index && !runwaysIntersect(r, s.airport.Runways[i]) {
			continue
		}
		if status.OccupiedBy != "" && status.OccupiedBy != aircraftID {
			return fmt.Errorf("runway %s is occupied by %s", status.ID, s.callsign(status.OccupiedBy))
		}
		if status.ReservedBy != "" && status.ReservedBy != aircraftID {
			return fmt.Errorf("runway %s is reserved for %s", status.ID, s.callsign(status.ReservedBy))
		}
	}
	return nil
}

func (s *Simulation) callsign(id string) string {
	for _, f := range s.flights {
		if f.ID == id {
			return f.Callsign
		}
	}
	return id
}

func (s *Simulation) Snapshot() State {
	state := State{Time: s.time, Paused: s.paused, Rate: s.rate, Difficulty: s.difficulty, Stats: s.stats,
		Aircraft: make([]Aircraft, 0, len(s.flights)), Events: append([]Event{}, s.events...), Runways: s.runwayStates()}
	for _, f := range s.flights {
		a := f.Aircraft
		a.Route = append([]Point{}, f.Route...)
		state.Aircraft = append(state.Aircraft, a)
	}
	return state
}

func (s *Simulation) Control(c Control) error {
	if c.Rate != nil && (!finite(*c.Rate) || *c.Rate < 0.25 || *c.Rate > 8) {
		return fmt.Errorf("simulation rate must be between 0.25 and 8")
	}
	if c.Difficulty != nil && *c.Difficulty != "easy" && *c.Difficulty != "normal" && *c.Difficulty != "hard" {
		return fmt.Errorf("difficulty must be easy, normal, or hard")
	}
	if c.Reset != nil && *c.Reset {
		fresh := New(s.airport)
		*s = *fresh
	}
	if c.Paused != nil {
		s.paused = *c.Paused
	}
	if c.Rate != nil {
		s.rate = *c.Rate
	}
	if c.Difficulty != nil && *c.Difficulty != s.difficulty {
		s.difficulty = *c.Difficulty
		s.nextTraffic = s.time + s.trafficInterval()
		s.log("Traffic intensity set to "+s.difficulty, "info")
	}
	return nil
}

func (s *Simulation) trafficInterval() float64 {
	switch s.difficulty {
	case "easy":
		return 180
	case "hard":
		return 55
	default:
		return 110
	}
}

// Tick accepts simulation seconds; callers apply the selected rate once. Large
// updates are subdivided to keep collision and threshold checks reliable.
func (s *Simulation) Tick(dt float64) {
	if s.paused || !finite(dt) || dt <= 0 {
		return
	}
	dt = math.Min(dt, 60)
	for dt > 0.000001 {
		step := math.Min(dt, 0.1)
		s.tickStep(step)
		dt -= step
	}
}

func (s *Simulation) tickStep(dt float64) {
	s.time += dt
	for _, f := range s.flights {
		f.Alert = ""
		switch f.Phase {
		case "taxi", "taxi-in", "lineup":
			s.tickGround(f, dt)
		case "takeoff":
			s.tickTakeoff(f, dt)
		case "approach", "departure", "goaround":
			s.tickAirborne(f, dt)
		case "landing":
			s.tickLanding(f, dt)
		}
	}
	s.checkAirSeparation()
	remaining := s.flights[:0]
	for _, f := range s.flights {
		if f.Phase == "complete" && s.time-f.completedAt > 8 {
			s.release(f)
			continue
		}
		remaining = append(remaining, f)
	}
	s.flights = remaining
	if s.time >= s.nextTraffic {
		s.nextTraffic = s.time + s.trafficInterval()
		limit := 14
		if s.difficulty == "easy" {
			limit = 10
		} else if s.difficulty == "hard" {
			limit = 22
		}
		if len(s.flights) < limit && len(s.airport.Runways) > 0 {
			s.trafficCount++
			if s.trafficCount%2 == 0 {
				s.spawnDeparture()
			} else {
				ids := []string{"21L", "22R", "22L"}
				r, _ := s.runway(ids[(s.trafficCount/2)%len(ids)])
				if r.ID == "" {
					r = s.airport.Runways[0]
				}
				s.spawnArrival(r, 23000+float64(s.random.Intn(6000)))
			}
		}
	}
}

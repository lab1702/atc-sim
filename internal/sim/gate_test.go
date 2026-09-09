package sim

import (
	"reflect"
	"testing"
)

func gateClearanceSimulation() (*Simulation, *flight) {
	s := New(testAirport())
	f := s.flights[2]
	isolate(s, f)
	f.Phase, f.Gate = "taxi-in", "A1"
	f.Position = Point{0, 0}
	f.Altitude = s.airport.Elevation
	f.Route = []Point{{-100, 0}, {-100, 200}, {-300, 200}}
	f.holding, f.landingCleared = true, true
	s.reservations[0] = f.ID
	return s, f
}

func TestStandReassignmentRetainsRunwayUntilVacated(t *testing.T) {
	s, f := gateClearanceSimulation()
	runway := f.Runway
	if err := s.Command(Command{AircraftID: f.ID, Action: "taxi-gate", Gate: "A2"}); err != nil {
		t.Fatal(err)
	}
	if f.Gate != "A2" || f.holding || f.Phase != "taxi-in" || f.TargetSpeed != 15 || len(f.Route) == 0 {
		t.Fatalf("stand clearance did not resume taxiing: %+v", f)
	}
	if f.Runway != runway || !f.landingCleared || s.reservations[0] != f.ID {
		t.Fatal("reroute changed the landing runway or released its reservation prematurely")
	}
	if !s.gateAvailable(s.airport.Gates[0], "") || s.gateAvailable(s.airport.Gates[1], "") {
		t.Fatal("stand claims did not follow the reassignment")
	}
	tickUntil(t, s, 300, func() bool { return f.Phase == "complete" })
	if len(s.reservations) != 0 || distance(f.Position, Point{-300, 400}) > 1 {
		t.Fatal("arrival did not vacate and park at the reassigned stand")
	}
}

func TestInvalidStandClearancesLeaveStateUnchanged(t *testing.T) {
	for _, test := range []struct {
		name  string
		gate  string
		setup func(*Simulation, *flight)
	}{
		{"unknown", "missing", func(*Simulation, *flight) {}},
		{"assigned", "A2", func(s *Simulation, _ *flight) {
			s.flights = append(s.flights, &flight{Aircraft: Aircraft{ID: "other", Phase: "taxi-in", Gate: "A2", Position: Point{-800, 0}}})
		}},
		{"occupied", "A2", func(s *Simulation, _ *flight) {
			s.flights = append(s.flights, &flight{Aircraft: Aircraft{ID: "other", Phase: "taxi", Position: Point{-300, 400}}})
		}},
		{"unreachable", "remote", func(s *Simulation, _ *flight) {
			s.airport.Gates = append(s.airport.Gates, Gate{ID: "remote", X: 10000, Y: 10000})
		}},
		{"airborne", "A2", func(_ *Simulation, f *flight) { f.Phase = "approach" }},
		{"departure", "A2", func(_ *Simulation, f *flight) { f.Kind, f.Phase = "departure", "taxi" }},
	} {
		t.Run(test.name, func(t *testing.T) {
			s, f := gateClearanceSimulation()
			test.setup(s, f)
			before := s.Snapshot()
			if err := s.Command(Command{AircraftID: f.ID, Action: "taxi-gate", Gate: test.gate}); err == nil {
				t.Fatal("invalid stand clearance was accepted")
			}
			if !reflect.DeepEqual(before, s.Snapshot()) || !f.holding || !f.landingCleared || s.reservations[0] != f.ID {
				t.Fatal("rejected clearance changed the existing route, state, or reservation")
			}
		})
	}
}

func TestArrivalCanKeepItsCurrentStand(t *testing.T) {
	s, f := gateClearanceSimulation()
	if err := s.Command(Command{AircraftID: f.ID, Action: "taxi-gate", Gate: f.Gate}); err != nil {
		t.Fatalf("arrival's own stand claim blocked its clearance: %v", err)
	}
}

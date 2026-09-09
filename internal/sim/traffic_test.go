package sim

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"
)

func TestArrivalsMergeAndReachTheirStands(t *testing.T) {
	raw, err := os.ReadFile("../../data/dtw.json")
	if err != nil {
		t.Fatal(err)
	}
	var airport Airport
	if err := json.Unmarshal(raw, &airport); err != nil {
		t.Fatal(err)
	}
	s := New(airport)
	a, b := s.flights[2], s.flights[3]
	for _, command := range []Command{
		{AircraftID: a.ID, Action: "vector", Speed: number(100)},
		{AircraftID: b.ID, Action: "land", Runway: "21R"},
	} {
		if err := s.Command(command); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 100; i++ {
		s.Tick(1)
	}
	if err := s.Command(Command{AircraftID: a.ID, Action: "land", Runway: "21L"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 17000; i++ {
		s.Tick(0.1)
		if a.Phase == "taxi-in" && b.Phase == "taxi-in" && distance(a.Position, b.Position) < 48 {
			t.Fatalf("arrivals breached ground separation: %v / %v", a.Position, b.Position)
		}
		if a.Phase == "complete" && b.Phase == "complete" {
			if a.Gate == "" || b.Gate == "" || a.Gate == b.Gate {
				t.Fatal("arrivals did not park at separate stands")
			}
			return
		}
	}
	t.Fatalf("arrivals did not finish taxiing: a=%+v b=%+v", a, b)
}

func TestConvergingTaxiRoutesPreserveSeparation(t *testing.T) {
	for _, test := range []struct {
		name   string
		startA Point
		startB Point
		holdA  bool
	}{
		{"right angle", Point{-300, 0}, Point{0, 300}, false},
		{"shallow angle", Point{-500, -50}, Point{-500, 50}, false},
		{"following", Point{-300, 0}, Point{-200, 0}, false},
		{"held leader resumes", Point{-40, 0}, Point{0, 300}, true},
	} {
		for _, reverse := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/reverse=%t", test.name, reverse), func(t *testing.T) {
				s := New(Airport{})
				s.nextTraffic = math.Inf(1)
				a := &flight{Aircraft: Aircraft{ID: "a", Callsign: "A", Phase: "taxi-in", Position: test.startA, Route: []Point{{0, 0}, {300, 0}, {400, -100}}}}
				b := &flight{Aircraft: Aircraft{ID: "b", Callsign: "B", Phase: "taxi-in", Position: test.startB, Route: []Point{{0, 0}, {300, 0}, {400, 100}}}}
				a.holding = test.holdA
				s.flights = []*flight{a, b}
				if reverse {
					s.flights = []*flight{b, a}
				}
				for i := 0; i < 3000; i++ {
					if i == 600 {
						a.holding = false
					}
					s.Tick(0.1)
					if a.Phase != "complete" && b.Phase != "complete" && distance(a.Position, b.Position) < 48 {
						t.Fatalf("aircraft breached ground separation: %v / %v", a.Position, b.Position)
					}
					if a.Phase == "complete" && b.Phase == "complete" {
						return
					}
				}
				t.Fatalf("aircraft did not finish taxiing: a=%+v b=%+v", a, b)
			})
		}
	}
}

func TestTaxiingDepartureCanRerouteAroundOpposingArrival(t *testing.T) {
	raw, err := os.ReadFile("../../data/dtw.json")
	if err != nil {
		t.Fatal(err)
	}
	var airport Airport
	if err := json.Unmarshal(raw, &airport); err != nil {
		t.Fatal(err)
	}
	s := New(airport)
	departure, arrival := s.flights[1], s.flights[2]
	for _, command := range []Command{
		{AircraftID: s.flights[0].ID, Action: "takeoff"},
		{AircraftID: arrival.ID, Action: "land"},
	} {
		if err := s.Command(command); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 200; i++ {
		s.Tick(1)
	}
	if err := s.Command(Command{AircraftID: departure.ID, Action: "taxi", Runway: "21R"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 600; i++ {
		s.Tick(1)
	}
	if departure.Phase != "taxi" || arrival.Phase != "taxi-in" ||
		!strings.Contains(departure.Alert, arrival.Callsign) || !strings.Contains(arrival.Alert, departure.Callsign) {
		t.Fatalf("expected opposing taxi routes: departure=%+v arrival=%+v", departure, arrival)
	}
	beforeDeparture, beforeArrival := departure.Position, arrival.Position
	s.Tick(30)
	if departure.Position != beforeDeparture || arrival.Position != beforeArrival {
		t.Fatal("opposing aircraft did not hold for ground proximity")
	}
	if err := s.Command(Command{AircraftID: departure.ID, Action: "taxi", Runway: "22R"}); err != nil {
		t.Fatalf("taxiing departure could not be rerouted: %v", err)
	}
	tickUntil(t, s, 1200, func() bool { return departure.Phase == "holdshort" && arrival.Phase == "complete" })
	if departure.Runway != "22R" || arrival.Gate == "" {
		t.Fatal("reroute did not let both aircraft reach their destinations")
	}
}

func TestTaxiMergeLetsRunwayOwnerVacate(t *testing.T) {
	for _, reverse := range []bool{false, true} {
		t.Run(fmt.Sprintf("reverse=%t", reverse), func(t *testing.T) {
			s := New(Airport{})
			s.nextTraffic = math.Inf(1)
			r := Runway{ID: "01", Start: Point{-200, 0}, End: Point{400, 0}, Width: 45}
			s.airport.Runways = []Runway{r}
			crossing := s.nextFlight("arrival")
			crossing.Phase = "taxi-in"
			crossing.Position = Point{0, 260}
			crossing.Route = []Point{{0, 12.5}, {0, -500}}
			owner := s.nextFlight("arrival")
			owner.Phase = "taxi-in"
			owner.Position = Point{300, 12.5}
			owner.Route = []Point{{0, 12.5}, {0, -500}}
			owner.Runway, owner.landingCleared = r.ID, true
			s.reservations[0] = owner.ID
			if reverse {
				s.flights = []*flight{owner, crossing}
			}
			for i := 0; i < 3000; i++ {
				s.Tick(0.1)
				if s.reservations[0] == owner.ID && onRunway(crossing.Position, r, 40) {
					t.Fatal("crossing aircraft entered the reserved runway")
				}
				if crossing.Phase != "complete" && owner.Phase != "complete" && distance(crossing.Position, owner.Position) < 48 {
					t.Fatal("aircraft breached ground separation")
				}
				if crossing.Phase == "complete" && owner.Phase == "complete" {
					if len(s.reservations) != 0 {
						t.Fatal("runway reservation was not released")
					}
					return
				}
			}
			t.Fatalf("aircraft did not finish taxiing: crossing=%+v owner=%+v", crossing, owner)
		})
	}
}

func TestOpposingArrivalsCanRerouteToAnotherStand(t *testing.T) {
	raw, err := os.ReadFile("../../data/dtw.json")
	if err != nil {
		t.Fatal(err)
	}
	var airport Airport
	if err := json.Unmarshal(raw, &airport); err != nil {
		t.Fatal(err)
	}
	s := New(airport)
	if err := s.Control(Control{Difficulty: textValue("easy")}); err != nil {
		t.Fatal(err)
	}
	a, b := s.flights[2], s.flights[3]
	for _, command := range []Command{
		{AircraftID: s.flights[0].ID, Action: "takeoff"},
		{AircraftID: s.flights[1].ID, Action: "taxi", Runway: "21R"},
		{AircraftID: a.ID, Action: "land"},
		{AircraftID: b.ID, Action: "vector", Speed: number(320), Altitude: number(1200)},
	} {
		if err := s.Command(command); err != nil {
			t.Fatal(err)
		}
	}
	for i := 0; i < 110; i++ {
		s.Tick(1)
	}
	if err := s.Command(Command{AircraftID: b.ID, Action: "land"}); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 1500; i++ {
		s.Tick(1)
	}
	if a.Phase != "taxi-in" || b.Phase != "taxi-in" || !strings.Contains(a.Alert, b.Callsign) || !strings.Contains(b.Alert, a.Callsign) {
		t.Fatalf("expected opposing taxi routes: a=%+v b=%+v", a, b)
	}
	if err := s.Command(Command{AircraftID: a.ID, Action: "taxi-gate", Gate: "A60"}); err != nil {
		t.Fatalf("arrival could not be rerouted: %v", err)
	}
	for i := 0; i < 12000; i++ {
		s.Tick(0.1)
		if a.Phase != "complete" && b.Phase != "complete" && distance(a.Position, b.Position) < 48 {
			t.Fatal("reroute breached ground separation")
		}
		if a.Phase == "complete" && b.Phase == "complete" {
			if a.Gate != "A60" || b.Gate != "A20" {
				t.Fatal("arrivals did not park at their assigned stands")
			}
			return
		}
	}
	t.Fatalf("reroute did not resolve opposing traffic: a=%+v b=%+v", a, b)
}

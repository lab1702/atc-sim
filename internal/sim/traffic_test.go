package sim

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

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

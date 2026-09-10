package sim

import (
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func TestManualVectorPersistsAcrossShortFinal(t *testing.T) {
	s := New(testAirport())
	f := s.flights[2]
	isolate(s, f)
	assignedHeading := normalizedHeading(f.Heading + 1)
	if err := s.Command(Command{AircraftID: f.ID, Action: "vector", Heading: number(assignedHeading), Altitude: number(6000), Speed: number(180)}); err != nil {
		t.Fatal(err)
	}
	clearance := f.Clearance
	for i := 0; i < 220; i++ {
		s.Tick(1)
		if f.Phase != "approach" || !f.manualVector || f.TargetAltitude != 6000 || f.TargetSpeed != 180 || f.TargetHeading != assignedHeading || f.Clearance != clearance {
			t.Fatalf("automatic approach logic replaced the manual clearance at %ds: %+v", i+1, f)
		}
	}
	r, _ := s.runway(f.Runway)
	along, _ := runwayCoordinates(f.Position, r)
	if along <= 500 || s.stats.GoArounds != 0 || s.stats.Arrivals != 0 {
		t.Fatal("manual vector did not cross the runway without entering an automatic approach")
	}
	if err := s.Command(Command{AircraftID: f.ID, Action: "land"}); err != nil {
		t.Fatal(err)
	}
	if f.manualVector || !f.landingCleared || f.finalFix == nil {
		t.Fatal("landing clearance did not return the vectored aircraft to its approach")
	}
}

func TestGoAroundNeverCommandsDescent(t *testing.T) {
	for _, altitude := range []float64{1200, 3645, 6000} {
		t.Run(fmt.Sprintf("altitude=%.0f", altitude), func(t *testing.T) {
			s := New(testAirport())
			f := s.flights[2]
			isolate(s, f)
			f.Altitude = altitude
			if err := s.Command(Command{AircraftID: f.ID, Action: "goaround"}); err != nil {
				t.Fatal(err)
			}
			if f.TargetAltitude < altitude || f.TargetAltitude < s.airport.Elevation+3000 {
				t.Fatalf("go-around commanded a descent: %+v", f)
			}
			if altitude >= s.airport.Elevation+3000 && !strings.Contains(f.Clearance, "maintain") {
				t.Fatalf("level go-around clearance should say maintain: %s", f.Clearance)
			}
			for i := 0; i < 60; i++ {
				before := f.Altitude
				s.Tick(1)
				if f.Altitude < before {
					t.Fatal("aircraft descended during the go-around")
				}
			}
		})
	}
}

func TestGoAroundVectorCanReceiveLandingClearance(t *testing.T) {
	s := New(testAirport())
	f := s.flights[2]
	isolate(s, f)
	if err := s.Command(Command{AircraftID: f.ID, Action: "goaround"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Command(Command{AircraftID: f.ID, Action: "vector", Speed: number(190)}); err != nil {
		t.Fatal(err)
	}
	// A vector keeps the aircraft under manual control after the normal
	// go-around timer expires. A landing clearance must provide a way back.
	for i := 0; i < 70; i++ {
		s.Tick(1)
	}
	if f.Phase != "goaround" || !f.manualVector {
		t.Fatalf("vector did not retain manual go-around control: %+v", f)
	}
	if err := s.Command(Command{AircraftID: f.ID, Action: "land"}); err != nil {
		t.Fatalf("landing clearance after a go-around vector was rejected: %v", err)
	}
	if f.Phase != "approach" || f.manualVector || !f.landingCleared || f.finalFix == nil {
		t.Fatalf("landing clearance did not restore an automatic approach through a fix: %+v", f)
	}
	if s.Snapshot().Runways[0].ReservedBy != f.ID {
		t.Fatal("new landing clearance did not reserve the runway")
	}
	tickUntil(t, s, 900, func() bool { return f.Phase == "landing" })
	if s.stats.Arrivals != 1 || f.Altitude != s.airport.Elevation {
		t.Fatalf("re-cleared aircraft did not touch down correctly: %+v", s.Snapshot())
	}
}

func TestRejectedGoAroundLandingClearancePreservesVector(t *testing.T) {
	s := New(testAirport())
	f, blocker := s.flights[2], s.flights[1]
	isolate(s, f)
	if err := s.Command(Command{AircraftID: f.ID, Action: "goaround"}); err != nil {
		t.Fatal(err)
	}
	if err := s.Command(Command{AircraftID: f.ID, Action: "vector", Heading: number(90), Speed: number(190)}); err != nil {
		t.Fatal(err)
	}
	blocker.Phase = "holdshort"
	blocker.Position = Point{0, 300}
	s.flights = append(s.flights, blocker)
	before := *f
	runwaysBefore := s.Snapshot().Runways
	err := s.Command(Command{AircraftID: f.ID, Action: "land", Runway: "03L"})
	if err == nil || !strings.Contains(err.Error(), "occupied") {
		t.Fatalf("expected rejection for an occupied runway, got %v", err)
	}
	if !reflect.DeepEqual(*f, before) {
		t.Fatalf("rejected landing clearance changed the manual go-around: before=%+v after=%+v", before, *f)
	}
	if !reflect.DeepEqual(s.Snapshot().Runways, runwaysBefore) {
		t.Fatal("rejected landing clearance changed runway ownership")
	}
}

func rolloutSimulation(a Airport, position Point) (*Simulation, *flight) {
	s := New(a)
	f := s.flights[2]
	isolate(s, f)
	f.Phase = "landing"
	f.Position = position
	f.Altitude = a.Elevation
	f.TargetAltitude = a.Elevation
	f.Speed = 24
	f.TargetSpeed = 24
	f.landingCleared = true
	f.landingCounted = true
	s.reservations[0] = f.ID
	s.stats.Arrivals = 1
	return s, f
}

func TestLandingWithFullStandsHoldsAndRecovers(t *testing.T) {
	s, f := rolloutSimulation(testAirport(), Point{0, -100})
	firstDeparture := s.spawnDeparture()
	if firstDeparture == nil || s.spawnDeparture() == nil {
		t.Fatal("could not fill both stands")
	}
	tickUntil(t, s, 15, func() bool {
		return f.Speed == 0 && strings.Contains(f.Alert, "No free stand")
	})
	stopped := f.Position
	for i := 0; i < 6000; i++ {
		s.Tick(0.1)
	}
	if f.Position != stopped {
		t.Errorf("aircraft crept %.3fm while waiting 600s for a stand: stopped=%+v now=%+v", distance(stopped, f.Position), stopped, f.Position)
	}
	if f.Phase != "landing" || f.Speed != 0 || f.TargetSpeed != 0 {
		t.Errorf("full-stand hold is not stationary: %+v", f)
	}
	if s.Snapshot().Runways[0].ReservedBy != f.ID {
		t.Fatal("holding arrival lost its runway reservation")
	}
	// Let an occupied stand become available through an ordinary taxi command.
	if err := s.Command(Command{AircraftID: firstDeparture.ID, Action: "taxi", Runway: "21R"}); err != nil {
		t.Fatal(err)
	}
	tickUntil(t, s, 100, func() bool { return f.Phase == "taxi-in" })
	if f.Gate != firstDeparture.Gate {
		t.Fatalf("arrival did not claim the newly freed stand: got %q, want %q", f.Gate, firstDeparture.Gate)
	}
	tickUntil(t, s, 300, func() bool { return f.Phase == "complete" })
	if distance(f.Position, Point{-300, 200}) > 1 || s.stats.Arrivals != 1 {
		t.Fatalf("arrival did not park once at the freed stand: %+v", s.Snapshot())
	}
	if s.Snapshot().Runways[0].ReservedBy != "" {
		t.Fatal("parked arrival did not release its runway reservation")
	}
}

func TestLandingWithoutConnectedExitHoldsAtRunwayEnd(t *testing.T) {
	a := testAirport()
	a.Taxiways = []Taxiway{
		{ID: "EXIT", Points: []Point{{0, -900}, {-100, -900}, {-100, -1000}}},
		{ID: "STANDS", Points: []Point{{-2000, 200}, {-2000, 400}}},
	}
	a.Gates = []Gate{{ID: "A1", X: -2000, Y: 200}, {ID: "A2", X: -2000, Y: 400}}
	s, f := rolloutSimulation(a, Point{0, -900})
	tickUntil(t, s, 15, func() bool {
		return f.Speed == 0 && strings.Contains(f.Alert, "No connected runway exit")
	})
	stopped := f.Position
	for i := 0; i < 6000; i++ {
		s.Tick(0.1)
	}
	if f.Position != stopped {
		t.Errorf("aircraft crept %.3fm while waiting 600s for a connected exit: stopped=%+v now=%+v", distance(stopped, f.Position), stopped, f.Position)
	}
	if f.Phase != "landing" || f.Speed != 0 || f.TargetSpeed != 0 || len(f.Route) != 0 {
		t.Errorf("disconnected-exit hold is not stationary: %+v", f)
	}
	if !onRunway(f.Position, a.Runways[0], 0) {
		t.Fatal("aircraft overran the runway while holding for an exit")
	}
}

func TestLandingResumesRolloutToNextExitWhenStandFrees(t *testing.T) {
	a := testAirport()
	a.Runways = a.Runways[:1]
	a.Taxiways = []Taxiway{
		{ID: "EXIT", Points: []Point{{0, -900}, {-100, -900}, {-300, -900}, {-300, -1100}}},
	}
	a.Gates = []Gate{{ID: "A1", X: -300, Y: -900}, {ID: "A2", X: -300, Y: -1100}}
	s, f := rolloutSimulation(a, Point{0, 0})
	occupant := s.spawnDeparture()
	if occupant == nil || s.spawnDeparture() == nil {
		t.Fatal("could not fill both stands")
	}
	tickUntil(t, s, 15, func() bool {
		return f.Speed == 0 && strings.Contains(f.Alert, "No free stand")
	})
	stopped := f.Position
	s.Tick(30)
	if f.Position != stopped || f.Speed != 0 {
		t.Fatal("arrival did not remain stopped while all stands were occupied")
	}
	// The occupied stand clears while the arrival is still too far from
	// the only mapped exit to begin taxiing directly to the stand.
	occupant.Phase = "complete"
	occupant.completedAt = s.time
	s.Tick(1)
	if f.Phase != "landing" || f.Speed <= 0 || f.Position.Y >= stopped.Y {
		t.Fatalf("arrival did not resume its rollout toward the next exit: %+v", f)
	}
	tickUntil(t, s, 150, func() bool { return f.Phase == "taxi-in" })
	if f.Gate != "A1" {
		t.Fatalf("arrival claimed stand %q instead of the newly freed A1", f.Gate)
	}
	tickUntil(t, s, 100, func() bool { return distance(f.Position, Point{0, -900}) < 2 })
	tickUntil(t, s, 100, func() bool { return f.Phase == "complete" })
	if distance(f.Position, Point{-300, -900}) > 1 || s.stats.Arrivals != 1 {
		t.Fatalf("arrival did not park once after resuming rollout: %+v", s.Snapshot())
	}
	if s.Snapshot().Runways[0].ReservedBy != "" {
		t.Fatal("parked arrival did not release its runway reservation")
	}
}

func TestLineupHoldResumePreservesTakeoffClearance(t *testing.T) {
	s := New(testAirport())
	f := s.flights[0]
	isolate(s, f)
	if err := s.Command(Command{AircraftID: f.ID, Action: "takeoff"}); err != nil {
		t.Fatal(err)
	}
	s.Tick(1)
	if f.Phase != "lineup" || f.Speed <= 0 {
		t.Fatalf("takeoff clearance did not start lineup: %+v", f)
	}
	if err := s.Command(Command{AircraftID: f.ID, Action: "hold"}); err != nil {
		t.Fatal(err)
	}
	stopped := f.Position
	s.Tick(30)
	if f.Position != stopped || f.Speed != 0 || f.Phase != "lineup" {
		t.Fatalf("lineup hold did not stop the aircraft: %+v", f)
	}
	if s.Snapshot().Runways[0].ReservedBy != f.ID {
		t.Fatal("lineup hold lost the runway reservation")
	}
	if err := s.Command(Command{AircraftID: f.ID, Action: "resume"}); err != nil {
		t.Fatal(err)
	}
	s.Tick(1)
	if f.Position == stopped {
		t.Fatal("resume did not restart lineup movement")
	}
	tickUntil(t, s, 150, func() bool { return f.Phase == "departure" })
	if s.stats.Departures != 1 {
		t.Fatalf("resuming lineup did not complete the existing takeoff clearance: %+v", s.Snapshot())
	}
}

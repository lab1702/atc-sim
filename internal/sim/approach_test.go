package sim

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestVectoredDTWArrivalRejoinsAndLands(t *testing.T) {
	raw, err := os.ReadFile("../../data/dtw.json")
	if err != nil {
		t.Fatal(err)
	}
	var airport Airport
	if err := json.Unmarshal(raw, &airport); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ heading, duration, speed float64 }{
		{180, 40, 180}, {180, 60, 180}, {240, 40, 180}, {240, 60, 180},
		{240, 70, 100}, {165, 30, 320}, {165, 40, 320},
	} {
		t.Run(fmt.Sprintf("heading_%g_after_%gs_at_%gkt", tc.heading, tc.duration, tc.speed), func(t *testing.T) {
			s := New(airport)
			f := s.flights[2]
			isolate(s, f)
			if err := s.Command(Command{AircraftID: f.ID, Action: "vector", Heading: number(tc.heading), Speed: number(tc.speed)}); err != nil {
				t.Fatal(err)
			}
			for elapsed := 0.0; elapsed < tc.duration; elapsed++ {
				s.Tick(1)
			}
			if err := s.Command(Command{AircraftID: f.ID, Action: "land", Runway: "21L"}); err != nil {
				t.Fatal(err)
			}
			tickUntil(t, s, 900, func() bool { return f.Phase == "landing" })
			if s.stats.Arrivals != 1 || s.stats.GoArounds != 0 {
				t.Fatalf("vectored arrival did not complete its cleared approach: %+v", s.stats)
			}
			tickUntil(t, s, 900, func() bool { return f.Phase == "complete" })
			if len(s.reservations) != 0 {
				t.Fatal("arrival kept its runway reservation after parking")
			}
		})
	}
}

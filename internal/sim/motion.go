package sim

import (
	"fmt"
	"math"
)

func (s *Simulation) tickGround(f *flight, dt float64) {
	if f.holding {
		f.Speed = approach(f.Speed, 0, 8*dt)
		f.TargetSpeed = 0
		return
	}
	if len(f.Route) == 0 {
		f.Speed = 0
		f.TargetSpeed = 0
		switch f.Phase {
		case "taxi":
			f.Phase = "holdshort"
			f.Clearance = "Holding short runway " + f.Runway + " · ready for departure"
			s.log(f.Callsign+" holding short runway "+f.Runway, "info")
		case "lineup":
			r, _ := s.runway(f.Runway)
			f.Heading = heading(r.Start, r.End)
			f.TargetHeading = f.Heading
			if f.takeoffAfterLineup {
				s.beginTakeoff(f, r)
			}
		case "taxi-in":
			f.Phase = "complete"
			f.completedAt = s.time
			f.Clearance = "Parked at stand " + f.Gate
			s.release(f)
			s.log(f.Callsign+" parked at stand "+f.Gate, "success")
		}
		return
	}
	f.TargetSpeed = 15
	if f.Phase == "lineup" {
		f.TargetSpeed = 12
	}
	// Slow for the final stand or hold point, avoiding instant stops.
	if len(f.Route) == 1 {
		f.TargetSpeed = math.Min(f.TargetSpeed, math.Max(3, distance(f.Position, f.Route[0])/2))
	}
	f.Speed = approach(f.Speed, f.TargetSpeed, 4*dt)
	remaining := f.Speed * knotsToMPS * dt
	for remaining > 0 && len(f.Route) > 0 {
		next := f.Route[0]
		dist := distance(f.Position, next)
		if dist < 0.1 {
			f.Position = next
			f.Route = f.Route[1:]
			continue
		}
		step := math.Min(remaining, dist)
		proposed := add(f.Position, scale(sub(next, f.Position), step/dist))
		warning := s.groundBlocked(f, proposed)
		if warning != "" {
			f.Speed = 0
			f.Alert = warning
			if warning != f.groundWarning {
				s.stats.Conflicts++
				s.log(f.Callsign+": "+warning, "warning")
			}
			f.groundWarning = warning
			return
		}
		f.groundWarning = ""
		f.Heading = heading(f.Position, next)
		f.TargetHeading = f.Heading
		f.Position = proposed
		remaining -= step
		if step >= dist-0.01 {
			f.Route = f.Route[1:]
		}
	}
	if f.Phase == "taxi-in" {
		r, _ := s.runway(f.Runway)
		if !onRunway(f.Position, r, 50) && f.landingCleared {
			f.landingCleared = false
			s.release(f)
			s.log(f.Callsign+" runway "+r.ID+" vacated", "success")
		}
	}
}

func (s *Simulation) groundBlocked(f *flight, proposed Point) string {
	for _, other := range s.flights {
		if other.ID == f.ID || other.Phase == "complete" || other.Altitude > s.airport.Elevation+30 {
			continue
		}
		gap := distance(proposed, other.Position)
		if gap < 48 && gap < distance(f.Position, other.Position)-0.001 {
			return "Ground proximity · holding for " + other.Callsign
		}
	}
	for i, r := range s.airport.Runways {
		if !onRunway(proposed, r, 40) {
			continue
		}
		if owner := s.reservations[i]; owner != "" && owner != f.ID {
			return "Runway " + r.ID + " crossing held for " + s.callsign(owner)
		}
		for _, other := range s.flights {
			if other.ID == f.ID || other.Phase == "complete" || other.Altitude > s.airport.Elevation+30 {
				continue
			}
			if onRunway(other.Position, r, 20) {
				return "Runway " + r.ID + " occupied by " + other.Callsign
			}
		}
	}
	return ""
}

func (s *Simulation) beginTakeoff(f *flight, r Runway) {
	f.Phase = "takeoff"
	f.Heading = heading(r.Start, r.End)
	f.TargetHeading = f.Heading
	f.TargetSpeed = 155
	f.TargetAltitude = s.airport.Elevation + 5000
	f.Route = []Point{}
}

func (s *Simulation) tickTakeoff(f *flight, dt float64) {
	r, _ := s.runway(f.Runway)
	// The runway was reserved before entry; a surface incursion still stops
	// the roll at low speed rather than allowing an avoidable collision.
	proposed := add(f.Position, scale(direction(f.Heading), math.Max(2, f.Speed)*knotsToMPS*dt))
	if warning := s.groundBlocked(f, proposed); warning != "" && f.Speed < 80 {
		f.Speed = 0
		f.Alert = warning
		if f.groundWarning != warning {
			s.stats.Conflicts++
			s.log(f.Callsign+": takeoff held · "+warning, "warning")
		}
		f.groundWarning = warning
		return
	}
	f.Speed = approach(f.Speed, 155, 4.5*dt)
	f.Position = add(f.Position, scale(direction(f.Heading), f.Speed*knotsToMPS*dt))
	along, _ := runwayCoordinates(f.Position, r)
	if f.Speed >= 140 && along >= 700 {
		f.Phase = "departure"
		f.Altitude = s.airport.Elevation + 15
		f.TargetSpeed = 230
		f.TargetAltitude = s.airport.Elevation + 5000
		f.Clearance = "Depart runway " + r.ID + " · climb to " + fmt.Sprintf("%.0f ft", f.TargetAltitude)
		s.stats.Departures++
		s.log(f.Callsign+" airborne runway "+r.ID, "success")
	}
}

func (s *Simulation) tickAirborne(f *flight, dt float64) {
	r, index := s.runway(f.Runway)
	if index < 0 {
		return
	}
	if f.Phase == "goaround" && s.time >= f.goAroundUntil && !f.manualVector {
		f.Phase = "approach"
		fix := add(r.Start, scale(direction(heading(r.Start, r.End)), -10500))
		f.finalFix = &fix
		f.Clearance = "Rejoining approach runway " + r.ID + " · request landing clearance"
		s.log(f.Callsign+" rejoining approach runway "+r.ID, "info")
	}
	if f.Phase == "approach" && !f.manualVector {
		aim := r.Start
		if f.finalFix != nil {
			aim = *f.finalFix
			if distance(f.Position, aim) < 350 {
				f.finalFix = nil
				aim = r.Start
			}
		}
		f.TargetHeading = heading(f.Position, aim)
		if f.finalFix != nil {
			f.TargetAltitude = s.airport.Elevation + 3000
			f.TargetSpeed = 185
		} else {
			rangeMeters := distance(f.Position, r.Start)
			f.TargetAltitude = s.airport.Elevation + clamp((rangeMeters-100)*math.Tan(3*math.Pi/180)*3.28084, 0, 6000)
			if f.landingCleared {
				f.TargetSpeed = 145
			} else {
				f.TargetSpeed = 175
			}
		}
	}
	f.Heading = normalizedHeading(f.Heading + clamp(headingDifference(f.TargetHeading, f.Heading), -3*dt, 3*dt))
	f.Speed = approach(f.Speed, f.TargetSpeed, 2.5*dt)
	verticalRate := 2000.0 / 60
	if f.TargetAltitude < f.Altitude {
		verticalRate = 1500.0 / 60
	}
	f.Altitude = approach(f.Altitude, f.TargetAltitude, verticalRate*dt)
	f.Altitude = math.Max(s.airport.Elevation, f.Altitude)
	f.Position = add(f.Position, scale(direction(f.Heading), f.Speed*knotsToMPS*dt))
	if f.Phase == "departure" {
		if f.Altitude > s.airport.Elevation+250 {
			s.release(f)
		}
		if distance(f.Position, r.End) > 22000 {
			f.Phase = "complete"
			f.completedAt = s.time
			f.Clearance = "Handed off to departure control"
			s.release(f)
			s.log(f.Callsign+" handed off to departure control", "success")
		}
		return
	}
	if f.Phase == "approach" {
		along, lateral := runwayCoordinates(f.Position, r)
		if f.finalFix == nil && along > -1600 && along < 300 && lateral < 600 {
			if !f.landingCleared {
				s.goAround(f, "No landing clearance")
				return
			}
			if err := s.canUseRunway(index, f.ID); err != nil {
				s.goAround(f, "Runway unavailable")
				return
			}
		}
		if f.finalFix == nil && along >= 0 && along < 500 && lateral < 200 {
			if f.landingCleared && f.Altitude < s.airport.Elevation+100 && absHeadingDifference(f.Heading, heading(r.Start, r.End)) < 20 {
				f.Phase = "landing"
				f.Position = add(r.Start, scale(direction(heading(r.Start, r.End)), along))
				f.Altitude = s.airport.Elevation
				f.TargetAltitude = s.airport.Elevation
				f.Heading = heading(r.Start, r.End)
				f.TargetHeading = f.Heading
				f.TargetSpeed = 24
				f.Clearance = "Landing rollout runway " + r.ID
				s.stats.Arrivals++
				f.landingCounted = true
				s.log(f.Callsign+" landed runway "+r.ID, "success")
				return
			}
			if along > 100 {
				s.goAround(f, "Unstable approach")
				return
			}
		}
		if f.landingCleared && f.finalFix == nil && along > 500 && distance(f.Position, r.Start) < 3500 {
			s.goAround(f, "Missed runway alignment")
			return
		}
	}
	if distance(f.Position, Point{}) > 65000 {
		f.Phase = "complete"
		f.completedAt = s.time
		s.release(f)
		s.log(f.Callsign+" left the simulated sector", "info")
	}
}

func (s *Simulation) goAround(f *flight, reason string) {
	r, _ := s.runway(f.Runway)
	s.release(f)
	f.landingCleared = false
	f.manualVector = false
	f.finalFix = nil
	f.Phase = "goaround"
	f.goAroundUntil = s.time + 65
	f.TargetHeading = heading(r.Start, r.End)
	f.TargetAltitude = s.airport.Elevation + 3000
	f.TargetSpeed = 210
	f.Clearance = "Go around · runway heading · climb to " + fmt.Sprintf("%.0f ft", f.TargetAltitude)
	f.Alert = reason
	s.stats.GoArounds++
	s.log(f.Callsign+" going around: "+reason, "warning")
}

func (s *Simulation) tickLanding(f *flight, dt float64) {
	r, _ := s.runway(f.Runway)
	along, _ := runwayCoordinates(f.Position, r)
	if f.Speed <= 25 && along >= math.Min(1300, distance(r.Start, r.End)*0.45) {
		// Check stand and exit availability before moving so a held aircraft
		// stays stationary, but can resume when a stand becomes available.
		g, ok := s.freeGate()
		if !ok {
			f.Speed = 0
			f.TargetSpeed = 0
			f.Alert = "No free stand · holding on runway"
			return
		}
		route, err := s.graph.route(f.Position, Point{g.X, g.Y})
		if err == nil {
			f.Route = route
			f.Gate = g.ID
			f.Phase = "taxi-in"
			f.TargetSpeed = 15
			f.Clearance = "Vacate runway " + r.ID + " · taxi to stand " + g.ID
			s.log(f.Callsign+" vacating runway "+r.ID+" for stand "+g.ID, "info")
			return
		}
		// Continue at taxi speed to the next mapped exit, never through grass.
		if along > distance(r.Start, r.End)-180 {
			f.Speed = 0
			f.TargetSpeed = 0
			f.Alert = "No connected runway exit · holding"
			return
		}
	}
	f.TargetSpeed = 24
	f.Speed = approach(f.Speed, f.TargetSpeed, 5.5*dt)
	f.Position = add(f.Position, scale(direction(f.Heading), f.Speed*knotsToMPS*dt))
}

func (s *Simulation) checkAirSeparation() {
	current := make(map[string]bool)
	for i, a := range s.flights {
		if a.Altitude < s.airport.Elevation+200 || a.Phase == "complete" {
			continue
		}
		for _, b := range s.flights[i+1:] {
			if b.Altitude < s.airport.Elevation+200 || b.Phase == "complete" {
				continue
			}
			if distance(a.Position, b.Position) >= 1500 || math.Abs(a.Altitude-b.Altitude) >= 500 {
				continue
			}
			key := a.ID + ":" + b.ID
			current[key] = true
			a.Alert = "Airborne proximity · " + b.Callsign
			b.Alert = "Airborne proximity · " + a.Callsign
			if !s.activeAlerts[key] {
				s.stats.Conflicts++
				s.log(a.Callsign+" / "+b.Callsign+": airborne proximity", "warning")
			}
		}
	}
	s.activeAlerts = current
}

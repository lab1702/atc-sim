package sim

import (
	"fmt"
	"strings"
)

func (s *Simulation) Command(c Command) error {
	var f *flight
	for _, candidate := range s.flights {
		if candidate.ID == c.AircraftID {
			f = candidate
			break
		}
	}
	if f == nil || f.Phase == "complete" {
		return fmt.Errorf("aircraft is no longer active")
	}
	action := strings.ToLower(strings.TrimSpace(c.Action))
	if action == "vector" {
		return s.vector(f, c)
	}
	if action == "hold" {
		if f.Phase != "taxi" && f.Phase != "taxi-in" && f.Phase != "lineup" && f.Phase != "holdshort" {
			return fmt.Errorf("hold position is available while taxiing or holding short")
		}
		f.holding = true
		f.TargetSpeed = 0
		f.Clearance = "Hold position"
		s.log(f.Callsign+", hold position", "info")
		return nil
	}
	if action == "resume" {
		if !f.holding {
			return fmt.Errorf("aircraft is not holding; issue a taxi or runway clearance")
		}
		f.holding = false
		if f.Phase == "holdshort" {
			f.Clearance = "Holding short runway " + f.Runway
		} else {
			f.TargetSpeed = 15
			f.Clearance = "Continue taxi"
		}
		s.log(f.Callsign+", continue taxi", "info")
		return nil
	}
	if action == "goaround" {
		if f.Phase != "approach" {
			return fmt.Errorf("go-around is available for an aircraft on approach")
		}
		s.goAround(f, "Tower instruction")
		return nil
	}
	runwayID := c.Runway
	if runwayID == "" {
		runwayID = f.Runway
	}
	r, index := s.runway(runwayID)
	if index < 0 {
		return fmt.Errorf("unknown runway %q", runwayID)
	}
	switch action {
	case "taxi":
		if f.Kind != "departure" || (f.Phase != "gate" && f.Phase != "taxi" && f.Phase != "holdshort") {
			return fmt.Errorf("taxi clearance requires a departure at a stand, taxiway, or hold point")
		}
		hold, ok := s.graph.holdPoint(r)
		if !ok {
			return fmt.Errorf("no mapped holding point for runway %s", r.ID)
		}
		route, err := s.graph.route(f.Position, hold)
		if err != nil {
			return err
		}
		f.Runway = r.ID
		f.Route = route
		f.Phase = "taxi"
		f.holding = false
		f.TargetSpeed = 15
		f.Clearance = "Taxi to runway " + r.ID + " · hold short"
		s.log(f.Callsign+", taxi to runway "+r.ID+", hold short", "info")
	case "lineup", "takeoff":
		if f.Phase != "holdshort" && f.Phase != "lineup" {
			return fmt.Errorf("aircraft must reach its runway hold point before %s clearance", action)
		}
		if r.ID != f.Runway {
			return fmt.Errorf("taxi to runway %s before requesting entry", r.ID)
		}
		if err := s.canUseRunway(index, f.ID); err != nil {
			return err
		}
		if action == "takeoff" {
			for _, other := range s.flights {
				_, otherIndex := s.runway(other.Runway)
				if other.ID != f.ID && other.Phase == "approach" && otherIndex == index && distance(other.Position, r.Start) < 3500 {
					return fmt.Errorf("%s is on short final; hold for arriving traffic", other.Callsign)
				}
			}
		}
		if f.Phase == "holdshort" {
			lineup := add(r.Start, scale(direction(heading(r.Start, r.End)), 120))
			route, err := s.graph.route(f.Position, lineup)
			if err != nil {
				return err
			}
			f.Route = route
			f.Phase = "lineup"
		}
		s.reservations[index] = f.ID
		f.holding = false
		f.TargetSpeed = 12
		f.takeoffAfterLineup = action == "takeoff"
		if action == "takeoff" {
			f.Clearance = "Cleared for takeoff runway " + r.ID
			if len(f.Route) == 0 {
				s.beginTakeoff(f, r)
			}
		} else {
			f.Clearance = "Line up and wait runway " + r.ID
		}
		s.log(f.Callsign+", "+strings.ToLower(f.Clearance), "success")
	case "land":
		if f.Phase != "approach" && f.Phase != "goaround" {
			return fmt.Errorf("landing clearance requires an aircraft on approach or going around")
		}
		if err := s.canUseRunway(index, f.ID); err != nil {
			return err
		}
		// Obtain the new reservation before releasing the old one so a failed
		// reassignment leaves the current clearance intact.
		s.release(f)
		s.reservations[index] = f.ID
		changedRunway := f.Runway != r.ID
		returningFromGoAround := f.Phase == "goaround"
		f.Phase = "approach"
		f.Runway = r.ID
		f.landingCleared = true
		f.manualVector = false
		along, lateral := runwayCoordinates(f.Position, r)
		if returningFromGoAround || changedRunway || along > -700 || lateral > 1400 || absHeadingDifference(f.Heading, heading(r.Start, r.End)) > 65 {
			fix := add(r.Start, scale(direction(heading(r.Start, r.End)), -9000))
			f.finalFix = &fix
		}
		f.TargetSpeed = 145
		f.Clearance = "Cleared to land runway " + r.ID
		s.log(f.Callsign+", cleared to land runway "+r.ID, "success")
	default:
		return fmt.Errorf("unknown command %q", c.Action)
	}
	return nil
}

func absHeadingDifference(a, b float64) float64 {
	d := headingDifference(a, b)
	if d < 0 {
		return -d
	}
	return d
}

func (s *Simulation) vector(f *flight, c Command) error {
	if f.Phase != "approach" && f.Phase != "departure" && f.Phase != "goaround" {
		return fmt.Errorf("heading, altitude, and speed commands require an airborne aircraft")
	}
	if c.Heading == nil && c.Altitude == nil && c.Speed == nil {
		return fmt.Errorf("provide a heading, altitude, or speed")
	}
	if c.Heading != nil && (!finite(*c.Heading) || *c.Heading < 0 || *c.Heading > 360) {
		return fmt.Errorf("heading must be between 0 and 360 degrees")
	}
	if c.Altitude != nil && (!finite(*c.Altitude) || *c.Altitude < s.airport.Elevation+500 || *c.Altitude > 20000) {
		return fmt.Errorf("altitude must be between %.0f and 20000 feet MSL", s.airport.Elevation+500)
	}
	if c.Speed != nil && (!finite(*c.Speed) || *c.Speed < 100 || *c.Speed > 320) {
		return fmt.Errorf("airspeed must be between 100 and 320 knots")
	}
	parts := []string{}
	if c.Heading != nil {
		f.TargetHeading = normalizedHeading(*c.Heading)
		parts = append(parts, fmt.Sprintf("heading %03.0f", f.TargetHeading))
	}
	if c.Altitude != nil {
		f.TargetAltitude = *c.Altitude
		parts = append(parts, fmt.Sprintf("maintain %.0f ft", f.TargetAltitude))
	}
	if c.Speed != nil {
		f.TargetSpeed = *c.Speed
		parts = append(parts, fmt.Sprintf("speed %.0f kt", f.TargetSpeed))
	}
	f.manualVector = true
	f.finalFix = nil
	if f.landingCleared {
		s.release(f)
		f.landingCleared = false
	}
	f.Clearance = strings.Join(parts, " · ")
	s.log(f.Callsign+", "+strings.Join(parts, ", "), "info")
	return nil
}

package sim

import (
	"container/heap"
	"fmt"
	"math"
	"sort"
)

const knotsToMPS = 0.514444

func distance(a, b Point) float64         { return math.Hypot(a.X-b.X, a.Y-b.Y) }
func add(a, b Point) Point                { return Point{a.X + b.X, a.Y + b.Y} }
func sub(a, b Point) Point                { return Point{a.X - b.X, a.Y - b.Y} }
func scale(p Point, f float64) Point      { return Point{p.X * f, p.Y * f} }
func dot(a, b Point) float64              { return a.X*b.X + a.Y*b.Y }
func clamp(x, low, high float64) float64  { return math.Max(low, math.Min(high, x)) }
func normalizedHeading(h float64) float64 { return math.Mod(math.Mod(h, 360)+360, 360) }
func heading(a, b Point) float64 {
	return normalizedHeading(math.Atan2(b.X-a.X, b.Y-a.Y) * 180 / math.Pi)
}
func headingDifference(a, b float64) float64 { return math.Mod(a-b+540, 360) - 180 }
func direction(h float64) Point {
	return Point{math.Sin(h * math.Pi / 180), math.Cos(h * math.Pi / 180)}
}
func approach(x, target, maxDelta float64) float64 { return x + clamp(target-x, -maxDelta, maxDelta) }
func finite(x float64) bool                        { return !math.IsNaN(x) && !math.IsInf(x, 0) }

func project(p, a, b Point) Point {
	d := sub(b, a)
	den := dot(d, d)
	if den < 0.001 {
		return a
	}
	return add(a, scale(d, clamp(dot(sub(p, a), d)/den, 0, 1)))
}

func runwayCoordinates(p Point, r Runway) (along, lateral float64) {
	d := sub(r.End, r.Start)
	l := distance(r.Start, r.End)
	if l < 1 {
		return 0, distance(p, r.Start)
	}
	u := scale(d, 1/l)
	v := sub(p, r.Start)
	return dot(v, u), math.Abs(v.X*u.Y - v.Y*u.X)
}

func onRunway(p Point, r Runway, margin float64) bool {
	along, lateral := runwayCoordinates(p, r)
	return along >= -margin && along <= distance(r.Start, r.End)+margin && lateral <= r.Width/2+margin
}

func runwaysIntersect(a, b Runway) bool {
	// Treat strips with near-touching ends as intersecting too.
	u, v := sub(a.End, a.Start), sub(b.End, b.Start)
	w := sub(b.Start, a.Start)
	cross := u.X*v.Y - u.Y*v.X
	if math.Abs(cross) > 0.01 {
		t, q := (w.X*v.Y-w.Y*v.X)/cross, (w.X*u.Y-w.Y*u.X)/cross
		if t >= 0 && t <= 1 && q >= 0 && q <= 1 {
			return true
		}
	}
	margin := (a.Width+b.Width)/2 + 20
	return distance(a.Start, project(a.Start, b.Start, b.End)) < margin || distance(a.End, project(a.End, b.Start, b.End)) < margin || distance(b.Start, project(b.Start, a.Start, a.End)) < margin || distance(b.End, project(b.End, a.Start, a.End)) < margin
}

type graphEdge struct {
	to   int
	cost float64
}
type graphSegment struct{ a, b int }
type taxiGraph struct {
	points   []Point
	edges    [][]graphEdge
	segments []graphSegment
}

func buildGraph(taxiways []Taxiway) taxiGraph {
	g := taxiGraph{}
	indices := make(map[string]int)
	node := func(p Point) int {
		// OSM ways share coordinates. A sub-metre key absorbs JSON rounding.
		key := fmt.Sprintf("%.0f:%.0f", p.X*2, p.Y*2)
		if i, exists := indices[key]; exists {
			return i
		}
		i := len(g.points)
		indices[key] = i
		g.points = append(g.points, p)
		g.edges = append(g.edges, nil)
		return i
	}
	for _, way := range taxiways {
		for i := 1; i < len(way.Points); i++ {
			a, b := node(way.Points[i-1]), node(way.Points[i])
			cost := distance(g.points[a], g.points[b])
			if a == b || cost < 0.1 {
				continue
			}
			g.edges[a] = append(g.edges[a], graphEdge{b, cost})
			g.edges[b] = append(g.edges[b], graphEdge{a, cost})
			g.segments = append(g.segments, graphSegment{a, b})
		}
	}
	return g
}

type snap struct {
	segment  int
	point    Point
	distance float64
}

func (g taxiGraph) nearby(p Point) []snap {
	candidates := make([]snap, 0, len(g.segments))
	for i, e := range g.segments {
		q := project(p, g.points[e.a], g.points[e.b])
		candidates = append(candidates, snap{i, q, distance(p, q)})
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].distance < candidates[j].distance })
	if len(candidates) == 0 {
		return candidates
	}
	limit := math.Min(350, candidates[0].distance+65)
	n := 0
	for n < len(candidates) && n < 12 && candidates[n].distance <= limit {
		n++
	}
	return candidates[:n]
}

type queueItem struct {
	node int
	cost float64
}
type pathQueue []queueItem

func (q pathQueue) Len() int           { return len(q) }
func (q pathQueue) Less(i, j int) bool { return q[i].cost < q[j].cost }
func (q pathQueue) Swap(i, j int)      { q[i], q[j] = q[j], q[i] }
func (q *pathQueue) Push(x any)        { *q = append(*q, x.(queueItem)) }
func (q *pathQueue) Pop() any          { old := *q; x := old[len(old)-1]; *q = old[:len(old)-1]; return x }

// route uses taxiway edges, with short connectors from stand/hold positions to
// the closest mapped line. It never substitutes a straight route if disconnected.
func (g taxiGraph) route(start, end Point) ([]Point, error) {
	starts, ends := g.nearby(start), g.nearby(end)
	if len(starts) == 0 || len(ends) == 0 {
		return nil, fmt.Errorf("no mapped taxiway connects this position")
	}
	points := append([]Point(nil), g.points...)
	edges := append([][]graphEdge(nil), g.edges...)
	appendNode := func(p Point) int { i := len(points); points = append(points, p); edges = append(edges, nil); return i }
	connect := func(a, b int, cost float64) {
		edges[a] = append(append([]graphEdge(nil), edges[a]...), graphEdge{b, cost})
		edges[b] = append(append([]graphEdge(nil), edges[b]...), graphEdge{a, cost})
	}
	startID, endID := appendNode(start), appendNode(end)
	type attached struct{ id, segment int }
	attachments := []attached{}
	for group, candidates := range [][]snap{starts, ends} {
		endpoint := startID
		if group == 1 {
			endpoint = endID
		}
		for _, c := range candidates {
			n := appendNode(c.point)
			e := g.segments[c.segment]
			// Penalize off-line connectors so mapped lead-in lines win.
			connect(endpoint, n, c.distance*5)
			connect(n, e.a, distance(c.point, points[e.a]))
			connect(n, e.b, distance(c.point, points[e.b]))
			for _, old := range attachments {
				if old.segment == c.segment {
					connect(n, old.id, distance(c.point, points[old.id]))
				}
			}
			attachments = append(attachments, attached{n, c.segment})
		}
	}
	distances, prev := make([]float64, len(points)), make([]int, len(points))
	for i := range distances {
		distances[i] = math.Inf(1)
		prev[i] = -1
	}
	distances[startID] = 0
	queue := &pathQueue{{startID, 0}}
	heap.Init(queue)
	for queue.Len() > 0 {
		cur := heap.Pop(queue).(queueItem)
		if cur.cost > distances[cur.node] {
			continue
		}
		if cur.node == endID {
			break
		}
		for _, e := range edges[cur.node] {
			cost := cur.cost + e.cost
			if cost < distances[e.to] {
				distances[e.to] = cost
				prev[e.to] = cur.node
				heap.Push(queue, queueItem{e.to, cost})
			}
		}
	}
	if math.IsInf(distances[endID], 1) {
		return nil, fmt.Errorf("no connected taxiway route; select another runway")
	}
	path := []Point{}
	for at := endID; at != startID; at = prev[at] {
		if at < 0 {
			return nil, fmt.Errorf("taxiway route reconstruction failed")
		}
		path = append(path, points[at])
	}
	for i, j := 0, len(path)-1; i < j; i, j = i+1, j-1 {
		path[i], path[j] = path[j], path[i]
	}
	result := []Point{}
	last := start
	for _, p := range path {
		if distance(last, p) > 0.5 {
			result = append(result, p)
			last = p
		}
	}
	return result, nil
}

func (g taxiGraph) holdPoint(r Runway) (Point, bool) {
	aim := add(r.Start, scale(direction(heading(r.Start, r.End)), 110))
	best := math.Inf(1)
	point := Point{}
	// Prefer the hold line on a mapped runway access segment. Interpolating
	// this point retains a paved route instead of cutting across the infield.
	for _, e := range g.segments {
		a, b := g.points[e.a], g.points[e.b]
		insideA, insideB := onRunway(a, r, 70), onRunway(b, r, 70)
		if insideA == insideB {
			continue
		}
		if insideA {
			a, b = b, a
		}
		outside, inside := a, b
		for k := 0; k < 25; k++ {
			mid := scale(add(outside, inside), 0.5)
			if onRunway(mid, r, 70) {
				inside = mid
			} else {
				outside = mid
			}
		}
		p := outside
		along, lateral := runwayCoordinates(p, r)
		if lateral < r.Width/2+60 || along < -150 || along > 600 {
			continue
		}
		score := distance(p, aim)
		if score < best {
			best = score
			point = p
		}
	}
	if best < 800 {
		return point, true
	}
	for _, e := range g.segments {
		for _, p := range []Point{project(aim, g.points[e.a], g.points[e.b]), g.points[e.a], g.points[e.b]} {
			along, lateral := runwayCoordinates(p, r)
			if lateral < r.Width/2+60 || along < -250 || along > 550 {
				continue
			}
			score := distance(p, aim)
			if score < best {
				best = score
				point = p
			}
		}
	}
	return point, best < 800
}

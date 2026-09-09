package sim

// Airport geometry uses local metres east and north of Center. Aircraft altitudes
// are feet MSL, speeds are knots, and headings are degrees clockwise from north.
type Point struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type Airport struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	Elevation float64 `json:"elevation"`
	Center    struct {
		Lat float64 `json:"lat"`
		Lon float64 `json:"lon"`
	} `json:"center"`
	Bounds struct {
		MinX float64 `json:"minX"`
		MaxX float64 `json:"maxX"`
		MinY float64 `json:"minY"`
		MaxY float64 `json:"maxY"`
	} `json:"bounds"`
	Runways   []Runway   `json:"runways"`
	Taxiways  []Taxiway  `json:"taxiways"`
	Buildings []Building `json:"buildings"`
	Aprons    []Building `json:"aprons"`
	Gates     []Gate     `json:"gates"`
	Tower     struct {
		X      float64 `json:"x"`
		Y      float64 `json:"y"`
		Height float64 `json:"height"`
	} `json:"tower"`
	Sources []Source `json:"sources"`
}

type Runway struct {
	ID       string  `json:"id"`
	Opposite string  `json:"opposite"`
	Start    Point   `json:"start"`
	End      Point   `json:"end"`
	Width    float64 `json:"width"`
	Length   float64 `json:"length"`
}

type Taxiway struct {
	ID     string  `json:"id"`
	Name   string  `json:"name"`
	Points []Point `json:"points"`
	Width  float64 `json:"width"`
}

type Building struct {
	Name   string  `json:"name"`
	Points []Point `json:"points"`
	Height float64 `json:"height"`
}

type Gate struct {
	ID string  `json:"id"`
	X  float64 `json:"x"`
	Y  float64 `json:"y"`
}

type Source struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type Aircraft struct {
	ID             string  `json:"id"`
	Callsign       string  `json:"callsign"`
	Type           string  `json:"type"`
	Kind           string  `json:"kind"`
	Phase          string  `json:"phase"`
	Position       Point   `json:"position"`
	Heading        float64 `json:"heading"`
	Altitude       float64 `json:"altitude"`
	Speed          float64 `json:"speed"`
	TargetHeading  float64 `json:"targetHeading"`
	TargetAltitude float64 `json:"targetAltitude"`
	TargetSpeed    float64 `json:"targetSpeed"`
	Runway         string  `json:"runway"`
	Gate           string  `json:"gate"`
	Route          []Point `json:"route"`
	Clearance      string  `json:"clearance"`
	Alert          string  `json:"alert"`
}

type State struct {
	Time       float64        `json:"time"`
	Paused     bool           `json:"paused"`
	Rate       float64        `json:"rate"`
	Difficulty string         `json:"difficulty"`
	Aircraft   []Aircraft     `json:"aircraft"`
	Events     []Event        `json:"events"`
	Stats      Stats          `json:"stats"`
	Runways    []RunwayStatus `json:"runways"`
}

type Event struct {
	ID      int     `json:"id"`
	Time    float64 `json:"time"`
	Message string  `json:"message"`
	Level   string  `json:"level"`
}

type Stats struct {
	Departures int `json:"departures"`
	Arrivals   int `json:"arrivals"`
	GoArounds  int `json:"goArounds"`
	Conflicts  int `json:"conflicts"`
}

type RunwayStatus struct {
	ID         string `json:"id"`
	OccupiedBy string `json:"occupiedBy"`
	ReservedBy string `json:"reservedBy"`
}

type Command struct {
	AircraftID string   `json:"aircraftId"`
	Action     string   `json:"action"`
	Runway     string   `json:"runway,omitempty"`
	Gate       string   `json:"gate,omitempty"`
	Heading    *float64 `json:"heading,omitempty"`
	Altitude   *float64 `json:"altitude,omitempty"`
	Speed      *float64 `json:"speed,omitempty"`
}

type Control struct {
	Paused     *bool    `json:"paused,omitempty"`
	Rate       *float64 `json:"rate,omitempty"`
	Difficulty *string  `json:"difficulty,omitempty"`
	Reset      *bool    `json:"reset,omitempty"`
}

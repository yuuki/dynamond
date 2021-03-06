package model

type Metric struct {
	Name       string       `json:"name"`
	Datapoints []*Datapoint `json:"datapoints"`
}

type Datapoint struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}

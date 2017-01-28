package series

import (
	"encoding/json"
	"fmt"
	"math"
	"testing"

	"github.com/kylelemons/godebug/pretty"
)

func TestNewSeries(t *testing.T) {
	name := "server1.loadavg5"
	values := []float64{0.1, 0.2, 0.3}
	start, step := int64(10000), 60

	s := NewSeries(name, values, start, step)

	if s.Name() != "server1.loadavg5" {
		t.Fatalf("\nExpected: %+v\nActual:   %+v", name, s.Name())
	}
	if diff := pretty.Compare(s.Values(), values); diff != "" {
		t.Fatalf("diff: (-actual +expected)\n%s", diff)
	}
	if s.Start() != start {
		t.Fatalf("\nExpected: %+v\nActual:   %+v", start, s.Start())
	}
	if s.End() != 10120 {
		t.Fatalf("\nExpected: %+v\nActual:   %+v", 10120, s.End())
	}
	if s.Step() != step {
		t.Fatalf("\nExpected: %+v\nActual:   %+v", step, s.Step())
	}
	if s.Len() != 3 {
		t.Fatalf("\nExpected: %+v\nActual:   %+v", 3, s.Len())
	}
}

func TestSeriesAlias(t *testing.T) {
	s := NewSeries("server1.loadavg5", []float64{}, 100, 60)
	if s.Alias() != "server1.loadavg5" {
		t.Fatalf("\nExpected: %+v\nActual:   %+v", "server1.loadavg5", s.Alias())
	}
	s.SetAlias("func(server1.loadavg5)")
	if s.Alias() != "func(server1.loadavg5)" {
		t.Fatalf("\nExpected: %+v\nActual:   %+v", "func(server1.loadavg5)", s.Alias())
	}
	s = NewSeries("server1.loadavg5", []float64{}, 100, 60).SetAliasWith("func2(server1.loadavg5)")
	if s.Alias() != "func2(server1.loadavg5)" {
		t.Fatalf("\nExpected: %+v\nActual:   %+v", "func2(server1.loadavg5)", s.Alias())
	}
}

func TestMarshalJSON(t *testing.T) {
	s := NewSeries("server1.loadavg5", []float64{0.1, 0.2, 0.3, math.NaN(), 0.5}, 1000, 60)
	s.SetAlias("func(server1.loadavg5)")
	j, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("%s", err)
	}
	expected := "{\"target\":\"func(server1.loadavg5)\",\"datapoints\":[[0.1,1000],[0.2,1060],[0.3,1120],[null,1180],[0.5,1240]]}"
	if got := fmt.Sprintf("%s", j); got != expected {
		t.Fatalf("\nExpected: %+v\nActual:   %+v", expected, got)
	}
}

func TestSeriesPoints(t *testing.T) {
	s := NewSeries("server1.loadavg5", []float64{0.1, 0.2, 0.3, 0.4, 0.5}, 0, 60)
	got := s.Points()
	expected := DataPoints{
		NewDataPoint(0, 0.1),
		NewDataPoint(60, 0.2),
		NewDataPoint(120, 0.3),
		NewDataPoint(180, 0.4),
		NewDataPoint(240, 0.5),
	}
	if diff := pretty.Compare(got, expected); diff != "" {
		t.Fatalf("diff: (-actual +expected)\n%s", diff)
	}
}

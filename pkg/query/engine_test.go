package query

import (
	"fmt"
	"testing"
	"time"

	"github.com/fortytw2/leaktest"
	"github.com/kylelemons/godebug/pretty"
	"github.com/pkg/errors"

	. "github.com/yuuki/diamondb/pkg/model"
	"github.com/yuuki/diamondb/pkg/storage"
)

func TestEvalTargets(t *testing.T) {
	tests := []struct {
		desc     string
		targets  []string
		mockFunc func(string, time.Time, time.Time) (SeriesSlice, error)
		expected SeriesSlice
		err      error
	}{
		{
			"one target",
			[]string{"server1.loadavg5"},
			func(name string, start, end time.Time) (SeriesSlice, error) {
				if name != "server1.loadavg5" {
					return nil, errors.Errorf("unexpected name: %s", name)
				}
				return SeriesSlice{
					NewSeries("server1.loadavg5", []float64{10.0}, 1000, 60),
				}, nil
			},
			SeriesSlice{
				NewSeries("server1.loadavg5", []float64{10.0}, 1000, 60),
			},
			nil,
		},
		{
			"three targets",
			[]string{"server1.loadavg5", "server2.loadavg5", "server3.loadavg5"},
			func(name string, start, end time.Time) (SeriesSlice, error) {
				switch name {
				case "server1.loadavg5":
					return SeriesSlice{
						NewSeries("server1.loadavg5", []float64{10.0}, 1000, 60),
					}, nil
				case "server2.loadavg5":
					return SeriesSlice{
						NewSeries("server2.loadavg5", []float64{11.0}, 1000, 60),
					}, nil
				case "server3.loadavg5":
					return SeriesSlice{
						NewSeries("server3.loadavg5", []float64{12.0}, 1000, 60),
					}, nil
				default:
					return nil, errors.Errorf("unexpected name %s", name)
				}
			},
			SeriesSlice{
				NewSeries("server1.loadavg5", []float64{10.0}, 1000, 60),
				NewSeries("server2.loadavg5", []float64{11.0}, 1000, 60),
				NewSeries("server3.loadavg5", []float64{12.0}, 1000, 60),
			},
			nil,
		},
		{
			"return one goroutine error",
			[]string{"server1.loadavg5", "server2.loadavg5", "server3.loadavg5"},
			func(name string, start, end time.Time) (SeriesSlice, error) {
				switch name {
				case "server1.loadavg5":
					return SeriesSlice{
						NewSeries("server1.loadavg5", []float64{10.0}, 1000, 60),
					}, nil
				case "server2.loadavg5":
					return nil, errors.New("some accident occur")
				case "server3.loadavg5":
					return SeriesSlice{
						NewSeries("server3.loadavg5", []float64{12.0}, 1000, 60),
					}, nil
				default:
					return nil, errors.Errorf("unexpected name %s", name)
				}
			},
			SeriesSlice{},
			errors.New("some accident occur"),
		},
	}

	for _, tc := range tests {
		fakefetcher := &storage.FakeReadWriter{
			FakeFetch: tc.mockFunc,
		}
		got, err := EvalTargets(
			fakefetcher,
			tc.targets,
			time.Unix(0, 0),
			time.Unix(120, 0),
		)
		if err != nil {
			if errors.Cause(err).Error() != errors.Cause(tc.err).Error() {
				t.Fatalf("err: %s", err)
			}
		}
		if diff := pretty.Compare(got, tc.expected); diff != "" {
			t.Fatalf("desc: %s diff: (-actual +expected)\n%s", tc.desc, diff)
		}
	}
}

func TestEvalTarget_Func(t *testing.T) {
	fakefetcher := &storage.FakeReadWriter{
		FakeFetch: func(name string, start, end time.Time) (SeriesSlice, error) {
			return SeriesSlice{
				NewSeries("server1.loadavg5", []float64{10.0, 11.0}, 1000, 60),
			}, nil
		},
	}

	seriesSlice, err := EvalTarget(
		fakefetcher,
		"alias(server1.loadavg5,\"server01.loadavg5\")",
		time.Unix(0, 0),
		time.Unix(120, 0),
	)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	s1 := NewSeries("server1.loadavg5", []float64{10.0, 11.0}, 1000, 60).SetAliasWith(
		"server01.loadavg5",
	)
	expected := SeriesSlice{s1}
	if diff := pretty.Compare(seriesSlice, expected); diff != "" {
		t.Fatalf("diff: (-actual +expected)\n%s", diff)
	}
}

func TestEvalTarget_FuncNest(t *testing.T) {
	fakefetcher := &storage.FakeReadWriter{
		FakeFetch: func(name string, start, end time.Time) (SeriesSlice, error) {
			return SeriesSlice{
				NewSeries("server1.loadavg5", []float64{10.0, 11.0}, 1000, 60),
			}, nil
		},
	}

	seriesSlice, err := EvalTarget(
		fakefetcher,
		"alias(alias(server1.loadavg5,\"server01.loadavg5\"),\"server001.loadavg5\")",
		time.Unix(0, 0),
		time.Unix(120, 0),
	)

	if err != nil {
		t.Fatalf("err: %s", err)
	}
	s1 := NewSeries("server1.loadavg5", []float64{10.0, 11.0}, 1000, 60).SetAliasWith(
		"server001.loadavg5",
	)
	expected := SeriesSlice{s1}
	if diff := pretty.Compare(seriesSlice, expected); diff != "" {
		t.Fatalf("diff: (-actual +expected)\n%s", diff)
	}
}

func TestEvalTarget_GroupSeries(t *testing.T) {
	expected := SeriesSlice{
		NewSeries("server1.loadavg5", []float64{10.0, 11.0}, 1000, 60),
		NewSeries("server2.loadavg5", []float64{12.0, 13.0}, 1000, 60),
	}
	fakefetcher := &storage.FakeReadWriter{
		FakeFetch: func(name string, start, end time.Time) (SeriesSlice, error) {
			if name != "server1.loadavg5,server2.loadavg5" {
				return nil, errors.Errorf("unexpected name: %s", name)
			}
			return expected, nil
		},
	}
	got, err := EvalTarget(
		fakefetcher,
		"server{1,2}.loadavg5",
		time.Unix(0, 0),
		time.Unix(120, 0),
	)
	if err != nil {
		t.Fatalf("err: %s", err)
	}
	if diff := pretty.Compare(got, expected); diff != "" {
		t.Fatalf("diff: (-actual +expected)\n%s", diff)
	}
}

func TestInvokeSubExprs_Leak(t *testing.T) {
	defer leaktest.Check(t)()

	ff := &storage.FakeReadWriter{
		FakeFetch: func(name string, start, end time.Time) (SeriesSlice, error) {
			time.Sleep(10 * time.Millisecond)
			ss := SeriesSlice{NewSeries(name, []float64{10.0}, 1, 60)}
			return ss, nil
		},
	}
	exprs := []Expr{
		SeriesListExpr{Literal: "server1.loadavg5"},
		SeriesListExpr{Literal: "server2.loadavg5"},
		BoolExpr{Literal: true}, // mix expr other than SeriesListExpr.
	}
	// goto infinite loop if test failures
	_, err := invokeSubExprs(ff, exprs, time.Unix(1, 0), time.Unix(10, 0))
	if err != nil {
		t.Fatalf("should not raise error: %s", err)
	}
}

func TestInvokeSubExprs_ErrLeak(t *testing.T) {
	defer leaktest.Check(t)()

	ff := &storage.FakeReadWriter{
		FakeFetch: func(name string, start, end time.Time) (SeriesSlice, error) {
			time.Sleep(10 * time.Millisecond)
			if name == "server1.loadavg5" {
				return nil, fmt.Errorf("dummy err: name=%q", name)
			}
			ss := SeriesSlice{NewSeries(name, []float64{10.0}, 1, 60)}
			return ss, nil
		},
	}
	exprs := []Expr{
		SeriesListExpr{Literal: "server1.loadavg5"},
		SeriesListExpr{Literal: "server2.loadavg5"},
		NumberExpr{Literal: 10},
	}
	_, err := invokeSubExprs(ff, exprs, time.Unix(1, 0), time.Unix(10, 0))
	if err == nil {
		t.Fatal("should raise error but got nil")
	}
}

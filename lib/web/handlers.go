package web

import (
	"net/http"
	"net/url"
	"time"

	"github.com/pkg/errors"

	"github.com/yuuki/diamondb/lib/config"
	"github.com/yuuki/diamondb/lib/env"
	"github.com/yuuki/diamondb/lib/errutil"
	"github.com/yuuki/diamondb/lib/log"
	"github.com/yuuki/diamondb/lib/query"
	"github.com/yuuki/diamondb/lib/timeparser"
)

const (
	// DayTime is one day period.
	DayTime = time.Duration(24*60*60) * time.Second
)

// PingHandler returns a HTTP handler for the endpoint to ping storage.
func PingHandler(env *env.Env) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := env.Fetcher.Ping(); err != nil {
			unavaliableError(w, errors.Cause(err).Error())
			return
		}
		ok(w, "PONG")
		return
	})
}

// InspectHandler returns a HTTP handler for the endpoint to inspect information.
func InspectHandler(env *env.Env) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		renderJSON(w, http.StatusOK, config.Config)
		return
	})
}

// RenderHandler returns a HTTP handler for the endpoint to read data.
func RenderHandler(env *env.Env) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		until := time.Now().Round(time.Second)
		from := until.Add(-DayTime)

		if v := r.FormValue("from"); v != "" {
			t, err := timeparser.ParseAtTime(url.QueryEscape(v))
			if err != nil {
				log.Println(err)
				errutil.PrintStackTrace(err)
				badRequest(w, errors.Cause(err).Error())
				return
			}
			from = t
		}
		if v := r.FormValue("until"); v != "" {
			t, err := timeparser.ParseAtTime(url.QueryEscape(v))
			if err != nil {
				log.Println(err)
				errutil.PrintStackTrace(err)
				badRequest(w, errors.Cause(err).Error())
				return
			}
			until = t
		}

		targets := r.Form["target"]
		if len(targets) < 1 {
			badRequest(w, "no targets requested")
			return
		}

		seriesSlice, err := query.EvalTargets(env.Fetcher, targets, from, until)
		if err != nil {
			log.Println(err)
			errutil.PrintStackTrace(err)

			switch err := errors.Cause(err).(type) {
			case *query.ParserError, *query.UnsupportedFunctionError, *query.ArgumentError:
				badRequest(w, err.Error())
			default:
				serverError(w, err.Error())
			}
			return
		}
		renderJSON(w, http.StatusOK, seriesSlice)
	})
}

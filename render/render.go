// Package render is a middleware for Martini that provides easy JSON serialization and HTML template rendering.
//
//  package main
//
//  import (
//    "github.com/codegangsta/martini"
//    "github.com/codegangsta/martini-contrib/render"
//  )
//
//  func main() {
//    m := martini.Classic()
//    m.Use(render.Renderer("templates"))
//
//    m.Get("/html", func(r render.Render) {
//      r.HTML(200, "mytemplate", nil)
//    })
//
//    m.Get("/json", func(r render.Render) {
//      r.JSON(200, "hello world")
//    })
//
//    m.Run()
//  }
package render

import (
	"bytes"
	"encoding/json"
	"github.com/codegangsta/martini"
	"html/template"
	"io/ioutil"
	"net/http"
	"os"
	"path/filepath"
)

const (
	ContentType = "Content-Type"
	ContentJSON = "application/json"
	ContentHTML = "text/html"
)

// Render is a service that can be injected into a Martini handler. Render provides functions for easily writing JSON and
// HTML templates out to a http Response.
type Render interface {
	// JSON writes the given status and JSON serialized version of the given value to the http.ResponseWriter.
	JSON(status int, v interface{})
	// HTML renders a html template specified by the name and writes the result and given status to the http.ResponseWriter.
	HTML(status int, name string, v interface{})
	// Error is a convenience function that writes an http status to the http.ResponseWriter.
	Error(status int)
}

type RenderConfig struct {
	Directory string
	Extension string
	Layout string
}

// Renderer is a Middleware that maps a render.Render service into the Martini handler chain. Renderer will compile templates
// globbed in the given dir. Templates must have the .tmpl extension to be compiled.
//
// If MARTINI_ENV is set to "" or "development" then templates will be recompiled on every request. For more performance, set the
// MARTINI_ENV environment variable to "production"
func Renderer(cfg RenderConfig) martini.Handler {
	t := compile(cfg)

	return func(res http.ResponseWriter, c martini.Context) {
		// recompile for easy development
		if martini.Env == martini.Dev {
			t = compile(cfg)
		}
		c.MapTo(&renderer{res, cfg, t}, (*Render)(nil))
	}
}

func compile(cfg RenderConfig) map[string]*template.Template {
	tmplMap := make(map[string]*template.Template)

	filepath.Walk(cfg.Directory, func(path string, info os.FileInfo, err error) error {
		r, err := filepath.Rel(cfg.Directory, path)
		if err != nil {
			return err
		}

		ext := filepath.Ext(r)
		name := (r[0 : len(r)-len(ext)])
		if ext == cfg.Extension {
			if name == cfg.Layout {
				// We don't parse the layout file
				return nil
			}

			t := template.New(name)

			buf, err := ioutil.ReadFile(path)
			if err != nil {
				panic(err)
			}

			tmpl := t.New(filepath.ToSlash(name))

			// Bomb out if parse fails. We don't want any silent server starts.
			if cfg.Layout == "" {
				// If a layout isn't specified, parse as normal
				template.Must(tmpl.Parse(string(buf)))
			} else {
				// If we do have a layout specified, include that in the parse
				template.Must(tmpl.ParseFiles(filepath.Join(cfg.Directory, cfg.Layout + cfg.Extension), path))
			}

			// XXX In production this should only run once, but in development this is run
			// with every request. Should we lock before adding the template to the map?
			tmplMap[name] = t
		}

		return nil
	})

	return tmplMap
}

type renderer struct {
	http.ResponseWriter
	cfg RenderConfig
	t map[string]*template.Template
}

func (r *renderer) JSON(status int, v interface{}) {
	result, err := json.Marshal(v)
	if err != nil {
		http.Error(r, err.Error(), 500)
		return
	}

	// json rendered fine, write out the result
	r.Header().Set(ContentType, ContentJSON)
	r.WriteHeader(status)
	r.Write(result)
}

func (r *renderer) HTML(status int, name string, binding interface{}) {
	var buf bytes.Buffer
	var tmpl string

	// If a layout is being used, we want to execute the layout template
	// which will pull in the other templates compiled into this object
	if r.cfg.Layout != "" {
		tmpl = r.cfg.Layout
	} else {
		tmpl = name
	}

	if err := r.t[name].ExecuteTemplate(&buf, tmpl, binding); err != nil {
		http.Error(r, err.Error(), 500)
		return
	}

	// template rendered fine, write out the result
	r.Header().Set(ContentType, ContentHTML)
	r.WriteHeader(status)
	r.Write(buf.Bytes())
}

func (r *renderer) Error(status int) {
	r.WriteHeader(status)
}

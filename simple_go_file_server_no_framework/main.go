package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"regexp"
	"strings"
	// "time"
)

type route struct {
	method       string
	pattern      *regexp.Regexp
	innerHandler http.HandlerFunc
	paramsKeys   []string
}

type router struct {
	routes []route
}

// A wrapper for logging
func (r *route) handler(w http.ResponseWriter, req *http.Request) {
	fmt.Sprintln(req.Method, " ", req.URL)
	r.innerHandler(w, req)

	requestString := fmt.Sprint(req.Method, " ", req.URL)
	fmt.Println("received ", requestString)
	// start := time.Now()
	r.innerHandler(NewResponseWriter(w), req)
	// w.Time = time.Since(start).Milliseconds()
	fmt.Printf("%s resolved with %s\n", requestString, w)
}

func (r *router) addRoute(method, endpoint string, handler http.HandlerFunc) {
	// path params
	pathParamPattern := regexp.MustCompile(":([a-z]+)")
	matches := pathParamPattern.FindAllStringSubmatch(endpoint, -1)
	paramKeys := []string{}

	if len(matches) > 0 {
		// replace path parameter definition with regex pattern to capture any string
		endpoint = pathParamPattern.ReplaceAllLiteralString(endpoint, "([^/]+)")
		// store the names of path parameters, to later be used as context keys
		for i := 0; i < len(matches); i++ {
			paramKeys = append(paramKeys, matches[i][1])
		}
	}

	route := route{method, regexp.MustCompile("^" + endpoint + "$"), handler, paramKeys}
	r.routes = append(r.routes, route)
}

// Convenience methods
func (r *router) GET(pattern string, handler http.HandlerFunc) {
	r.addRoute(http.MethodGet, pattern, handler)
}

func (r *router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	var allow []string
	for _, route := range r.routes {
		matches := route.pattern.FindStringSubmatch(req.URL.Path)
		if len(matches) > 0 {
			if req.Method != route.method {
				allow = append(allow, route.method)
				continue
			}
			route.handler(
				w,
				buildContext(req, route.paramsKeys, matches[1:]),
			)
			return
		}
	}
	if len(allow) > 0 {
		w.Header().Set("Allow", strings.Join(allow, ", "))
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	http.NotFound(w, req)
}

// This is used to avoid context key collisions
// it serves as a domain for the context keys
type ContextKey string

// Returns a shallow-copy of the request with an updated context,
// including path parameters
func buildContext(req *http.Request, paramKeys, paramValues []string) *http.Request {
	ctx := req.Context()
	for i := 0; i < len(paramKeys); i++ {
		ctx = context.WithValue(ctx, ContextKey(paramKeys[i]), paramValues[i])
	}
	return req.WithContext(ctx)
}

type ResponseWriter struct {
	Status int
	Body   string
	Time   int64
	http.ResponseWriter
}

// Converts http.ResponseWriter into ResponseWriter
func NewResponseWriter(w http.ResponseWriter) *ResponseWriter {
	return &ResponseWriter{ResponseWriter: w}
}

func (w *ResponseWriter) WriteHeader(code int) {
	w.Status = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *ResponseWriter) Write(body []byte) (int, error) {
	w.Body = string(body)
	return w.ResponseWriter.Write(body)
}

// Overwrite the string method to see what the log looks like
func (w *ResponseWriter) String() string {
	out := fmt.Sprintf("status %d (took %dms)", w.Status, w.Time)
	if w.Body != "" {
		out = fmt.Sprintf("%s\n\tresponse: %s", out, w.Body)
	}
	return out
}

// Convenience methods to response
func (w *ResponseWriter) StringResponse(code int, response string) {
	w.WriteHeader(code)
	w.Write([]byte(response))
}

func (w *ResponseWriter) JSONResponse(code int, responseObject any) {
	w.WriteHeader(code)
	response, err := json.Marshal(responseObject)
	if err != nil {
		w.StringResponse(http.StatusBadRequest, err.Error())
	}
	w.Header().Set("content-type", "application/json")
	w.Write(response)
}
func newRouter() *router {
	return &router{routes: []route{}}
}

func main() {
	router := newRouter()
	router.GET("/ping/:id/:otherid", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Println(r.Context().Value(ContextKey("id")))
		fmt.Println(r.Context().Value(ContextKey("otherid")))
		fmt.Println(r.FormValue("name"))
		

	}))

	l, err := net.Listen("tcp", ":8081")
	if err != nil {
		fmt.Printf("error starting server: %s\n", err)
	}
	fmt.Println("server started on ", l.Addr().String())
	if err := http.Serve(l, router); err != nil {
		fmt.Printf("server closed: %s\n", err)
	}
	os.Exit(1)
}

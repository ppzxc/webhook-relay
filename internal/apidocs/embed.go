package apidocs

import (
	_ "embed"
	"net/http"
)

//go:embed openapi.yaml
var OpenAPISpec []byte

//go:embed asyncapi.yaml
var AsyncAPISpec []byte

//go:embed redoc.standalone.js
var RedocJS []byte

func OpenAPIHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.WriteHeader(http.StatusOK)
	w.Write(OpenAPISpec) //nolint:errcheck
}

func AsyncAPIHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.WriteHeader(http.StatusOK)
	w.Write(AsyncAPISpec) //nolint:errcheck
}

func RedocHTMLHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	w.Write([]byte(`<!DOCTYPE html><html><head><meta charset="utf-8"><title>webhook-relay API</title></head><body><redoc spec-url="/docs/openapi"></redoc><script>`)) //nolint:errcheck
	w.Write(RedocJS)                                                                                                                                                   //nolint:errcheck
	w.Write([]byte(`</script></body></html>`))                                                                                                                         //nolint:errcheck
}

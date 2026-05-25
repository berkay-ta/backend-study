package httpapi

import (
	"net/http"
	"os"
	"path/filepath"

	"github.com/iberkayC/case1back/internal/platform/apperror"
)

const swaggerHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>League API Docs</title>
  <link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui.css">
  <style>
    body { margin: 0; background: #fff; }
    .swagger-ui .topbar { display: none; }
  </style>
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5/swagger-ui-bundle.js"></script>
  <script>
    window.ui = SwaggerUIBundle({
      url: "/openapi.yaml",
      dom_id: "#swagger-ui",
      deepLinking: true,
      displayRequestDuration: true,
      syntaxHighlight: { theme: "agate" }
    });
  </script>
</body>
</html>`

func swaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(swaggerHTML))
}

func openAPISpec(w http.ResponseWriter, r *http.Request) {
	path, ok := findOpenAPISpec()
	if !ok {
		writeError(w, r, apperror.NotFound(apperror.CodeNotFound, "openapi spec not found"))
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	http.ServeFile(w, r, path)
}

func findOpenAPISpec() (string, bool) {
	for _, path := range []string{
		filepath.Join("api", "openapi.yaml"),
		filepath.Join("api-spec", "openapi.yaml"),
		filepath.Join("..", "..", "api", "openapi.yaml"),
	} {
		if st, err := os.Stat(path); err == nil && !st.IsDir() {
			return path, true
		}
	}
	return "", false
}

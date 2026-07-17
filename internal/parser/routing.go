package parser

import (
	"regexp"
)

type APIRoute struct {
	Method string // 'GET', 'POST', 'PUT', 'DELETE'
	Path   string // '/api/payment/checkout'
	File   string
	Line   int
}

type APICallSite struct {
	URLPattern string // 'https://checkout-service/api/payment/checkout' or '/api/payment/checkout'
	File       string
	Line       int
}

type ServiceLinkMatch struct {
	Route APIRoute
	Call  APICallSite
}

// ExtractRoutes parses files for REST routes using AST or regex pattern fallbacks
func ExtractRoutes(fileContent string, file string) []APIRoute {
	var routes []APIRoute
	// Match Python FastAPI / Flask route decorators e.g. @app.get("/items") or @app.route("/items", methods=["POST"])
	reFastAPI := regexp.MustCompile(`@(?:app|router|blueprint)\.(get|post|put|delete)\(['"]([^'"]+)['"]`)
	matchesFast := reFastAPI.FindAllStringSubmatchIndex(fileContent, -1)

	for _, match := range matchesFast {
		method := fileContent[match[2]:match[3]]
		path := fileContent[match[4]:match[5]]
		line := countLines(fileContent, match[0])
		routes = append(routes, APIRoute{
			Method: method,
			Path:   path,
			File:   file,
			Line:   line,
		})
	}

	// Flask style routes: @app.route('/items', methods=['POST'])
	reFlask := regexp.MustCompile(`@(?:app|router|blueprint)\.route\(['"]([^'"]+)['"](?:\s*,\s*methods\s*=\s*\[['"]([^'"]+)['"]\])?`)
	matchesFlask := reFlask.FindAllStringSubmatchIndex(fileContent, -1)
	for _, match := range matchesFlask {
		path := fileContent[match[2]:match[3]]
		method := "GET" // default Flask method is GET
		if len(match) > 5 && match[4] != -1 {
			method = fileContent[match[4]:match[5]]
		}
		line := countLines(fileContent, match[0])
		routes = append(routes, APIRoute{
			Method: method,
			Path:   path,
			File:   file,
			Line:   line,
		})
	}

	return routes
}

// ExtractCallSites parses files for REST API client call sites (e.g. requests.get("/api/payment"))
func ExtractCallSites(fileContent string, file string) []APICallSite {
	var calls []APICallSite
	// Match requests.get("url"), requests.post("url"), http.Get("url"), fetch("url"), axios.get("url"), etc.
	reCalls := regexp.MustCompile(`(?:requests|http|axios|fetch)\.(?:get|post|put|delete|Get|Post|Put|Delete)\(\s*['"]([^'"]+)['"]`)
	matches := reCalls.FindAllStringSubmatchIndex(fileContent, -1)

	for _, match := range matches {
		url := fileContent[match[2]:match[3]]
		line := countLines(fileContent, match[0])
		calls = append(calls, APICallSite{
			URLPattern: url,
			File:       file,
			Line:       line,
		})
	}
	return calls
}

// MatchEndpoints compares API callsites in caller services to API routes in callee services
func MatchEndpoints(routes []APIRoute, calls []APICallSite) []ServiceLinkMatch {
	var matches []ServiceLinkMatch

	for _, route := range routes {
		for _, call := range calls {
			cleanCallURL := cleanURL(call.URLPattern)
			if cleanCallURL == route.Path {
				matches = append(matches, ServiceLinkMatch{
					Route: route,
					Call:  call,
				})
			}
		}
	}
	return matches
}

func cleanURL(rawURL string) string {
	// Remove protocol + host + port (e.g., 'http://localhost:8080/api/xyz' -> '/api/xyz')
	re := regexp.MustCompile(`^(?:https?://[^/]+)?(.*)$`)
	res := re.FindStringSubmatch(rawURL)
	if len(res) > 1 {
		return res[1]
	}
	return rawURL
}

func countLines(content string, offset int) int {
	line := 1
	for i := 0; i < offset && i < len(content); i++ {
		if content[i] == '\n' {
			line++
		}
	}
	return line
}

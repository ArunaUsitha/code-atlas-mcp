package parser

import (
	"bufio"
	"strings"
)

type IaCResource struct {
	Type      string // 'docker_image', 'k8s_service'
	Name      string
	DependsOn []string
}

// ParseDockerfile parses ENV and ARG parameters in Dockerfile config
func ParseDockerfile(content string) []string {
	var envKeys []string
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "ENV ") || strings.HasPrefix(line, "ARG ") {
			parts := strings.Fields(line[4:])
			if len(parts) > 0 {
				eqIdx := strings.Index(parts[0], "=")
				if eqIdx != -1 {
					envKeys = append(envKeys, parts[0][:eqIdx])
				} else {
					envKeys = append(envKeys, parts[0])
				}
			}
		}
	}
	return envKeys
}

// ParseK8sYaml is a placeholder for Kubernetes YAML dependency mapping
func ParseK8sYaml(content string) []IaCResource {
	var resources []IaCResource
	scanner := bufio.NewScanner(strings.NewReader(content))
	var currentName string
	var currentKind string
	var dependencies []string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "kind:") {
			currentKind = strings.TrimSpace(strings.TrimPrefix(line, "kind:"))
		} else if strings.HasPrefix(line, "name:") {
			currentName = strings.TrimSpace(strings.TrimPrefix(line, "name:"))
		} else if strings.HasPrefix(line, "dependsOn:") || strings.Contains(line, "serviceName:") {
			// Extract service dependencies
			parts := strings.Fields(line)
			if len(parts) > 1 {
				dependencies = append(dependencies, parts[len(parts)-1])
			}
		}

		// When a block ends, create resource (simple logic for mock structure)
		if line == "---" && currentName != "" {
			resources = append(resources, IaCResource{
				Type:      "k8s_" + strings.ToLower(currentKind),
				Name:      currentName,
				DependsOn: dependencies,
			})
			currentName = ""
			currentKind = ""
			dependencies = nil
		}
	}
	if currentName != "" {
		resources = append(resources, IaCResource{
			Type:      "k8s_" + strings.ToLower(currentKind),
			Name:      currentName,
			DependsOn: dependencies,
		})
	}
	return resources
}

package epub

import "strings"

func hasProperty(props, target string) bool {
	for _, token := range strings.Fields(props) {
		if token == target {
			return true
		}
	}
	return false
}

func addProperty(props, target string) string {
	if target == "" {
		return props
	}
	if hasProperty(props, target) {
		return props
	}
	if strings.TrimSpace(props) == "" {
		return target
	}
	return props + " " + target
}

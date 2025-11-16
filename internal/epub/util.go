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

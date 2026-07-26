package gdamdb

import (
	"os"
	"strings"
)

// DefaultAPIURL is the public registry API. Unlike the Supabase key this
// replaces, it is not a secret and needs no build-time injection.
const DefaultAPIURL = "https://api.gdam.dev"

func defaultAPIURL() string {
	if value := strings.TrimSpace(os.Getenv("GDAM_API_URL")); value != "" {
		return value
	}
	return DefaultAPIURL
}

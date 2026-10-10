package commands

import (
	"context"
	"errors"
	"testing"
)

func TestPublishRejectsLegacyCredentialsBeforeNetwork(t *testing.T) {
	t.Setenv("GDAM_SECRET_KEY", "gdam_sk_old-secret")
	for _, key := range []string{"", "gdam_sk_old-secret"} {
		t.Setenv("GDAM_API_KEY", key)
		err := Publish(context.Background(), PublishOptions{Spec: "@owner/addon", TagName: "v1"})
		if !errors.Is(err, ErrUserInput) {
			t.Fatalf("legacy key reached network: %v", err)
		}
	}
}

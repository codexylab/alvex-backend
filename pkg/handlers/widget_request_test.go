package handlers

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestValidateWidgetMessageAndImage(t *testing.T) {
	validImage := "data:image/png;base64," + base64.StdEncoding.EncodeToString([]byte("png-bytes"))
	tests := []struct {
		name    string
		message string
		image   string
		wantErr bool
	}{
		{name: "text", message: "Hello"},
		{name: "valid image", image: validImage},
		{name: "empty", wantErr: true},
		{name: "long message", message: strings.Repeat("a", widgetMessageMaxRunes+1), wantErr: true},
		{name: "svg rejected", image: "data:image/svg+xml;base64,PHN2Zz4=", wantErr: true},
		{name: "remote URL rejected", image: "https://example.com/image.png", wantErr: true},
		{name: "invalid base64", image: "data:image/png;base64,%%%", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateWidgetMessage(test.message, test.image)
			if (err != nil) != test.wantErr {
				t.Fatalf("wantErr=%v, got %v", test.wantErr, err)
			}
		})
	}
}

func TestAllowedWidgetReactions(t *testing.T) {
	for _, reaction := range []string{"", "👍", "❤️", "😊"} {
		if !isAllowedReaction(reaction) {
			t.Fatalf("expected %q to be allowed", reaction)
		}
	}
	if isAllowedReaction("<script>") {
		t.Fatal("unexpected reaction accepted")
	}
}

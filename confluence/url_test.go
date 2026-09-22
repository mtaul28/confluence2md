// SPDX-License-Identifier: Apache-2.0

package confluence

import "testing"

func TestParsePageURL(t *testing.T) {
	tests := []struct {
		name     string
		url      string
		wantBase string
		wantRef  PageRef
	}{
		{
			name:     "cloud page",
			url:      "https://example.atlassian.net/wiki/spaces/ENG/pages/123456/Some+Page",
			wantBase: "https://example.atlassian.net/wiki",
			wantRef:  PageRef{ID: "123456"},
		},
		{
			name:     "cloud page without title",
			url:      "https://example.atlassian.net/wiki/spaces/ENG/pages/123456",
			wantBase: "https://example.atlassian.net/wiki",
			wantRef:  PageRef{ID: "123456"},
		},
		{
			name:     "cloud edit url",
			url:      "https://example.atlassian.net/wiki/spaces/ENG/pages/edit-v2/123456",
			wantBase: "https://example.atlassian.net/wiki",
			wantRef:  PageRef{ID: "123456"},
		},
		{
			name:     "server viewpage",
			url:      "https://confluence.example.com/pages/viewpage.action?pageId=987",
			wantBase: "https://confluence.example.com",
			wantRef:  PageRef{ID: "987"},
		},
		{
			name:     "server viewpage with context path",
			url:      "https://intranet.example.com/confluence/pages/viewpage.action?pageId=987",
			wantBase: "https://intranet.example.com/confluence",
			wantRef:  PageRef{ID: "987"},
		},
		{
			name:     "server display link",
			url:      "https://confluence.example.com/display/OPS/Runbook+%26+Notes",
			wantBase: "https://confluence.example.com",
			wantRef:  PageRef{SpaceKey: "OPS", Title: "Runbook & Notes"},
		},
		{
			name:     "data center spaces link",
			url:      "http://localhost:8090/spaces/DEV/pages/42/Home",
			wantBase: "http://localhost:8090",
			wantRef:  PageRef{ID: "42"},
		},
		{
			name:     "surrounding whitespace",
			url:      "  https://confluence.example.com/pages/viewpage.action?pageId=5\n",
			wantBase: "https://confluence.example.com",
			wantRef:  PageRef{ID: "5"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, ref, err := ParsePageURL(tt.url)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if base != tt.wantBase {
				t.Errorf("base = %q, want %q", base, tt.wantBase)
			}
			if ref != tt.wantRef {
				t.Errorf("ref = %+v, want %+v", ref, tt.wantRef)
			}
		})
	}
}

func TestParsePageURL_Errors(t *testing.T) {
	bad := []string{
		"",
		"not a url",
		"ftp://example.com/pages/viewpage.action?pageId=1",
		"https://example.atlassian.net/wiki/x/AbCdE",
		"https://example.atlassian.net/wiki/spaces/ENG/overview",
		"https://confluence.example.com/display/OPS",
	}
	for _, u := range bad {
		if _, _, err := ParsePageURL(u); err == nil {
			t.Errorf("ParsePageURL(%q) expected an error", u)
		}
	}
}

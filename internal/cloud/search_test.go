package cloud

import "testing"

func TestDashboardSearchURLTargetsExactItem(t *testing.T) {
	tests := []struct {
		name  string
		path  string
		key   string
		value string
		want  string
	}{
		{name: "note", path: "/dashboard/profile-notes", key: "note", value: "note/roof", want: "/dashboard/profile-notes?note=note%2Froof"},
		{name: "app", path: "/dashboard/settings/apps", key: "app", value: "weather station", want: "/dashboard/settings/apps?app=weather+station"},
		{name: "member", path: "/dashboard/settings/people", key: "member", value: "owner@example.com", want: "/dashboard/settings/people?member=owner%40example.com"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := dashboardSearchURL(test.path, test.key, test.value); got != test.want {
				t.Fatalf("dashboardSearchURL() = %q, want %q", got, test.want)
			}
		})
	}
}

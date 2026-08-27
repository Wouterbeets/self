package main

import "testing"

func TestBrowseURL(t *testing.T) {
	cases := []struct {
		args []string
		want string
	}{
		{nil, "http://127.0.0.1:8377"},
		{[]string{}, "http://127.0.0.1:8377"},
		{[]string{"menu"}, "http://127.0.0.1:8377/view/menu"},
		{[]string{"echo", "one", "two"}, "http://127.0.0.1:8377/view/echo/one/two"},
		{[]string{"a b"}, "http://127.0.0.1:8377/view/a%20b"},
	}
	for _, c := range cases {
		got := browseURL("8377", c.args)
		if got != c.want {
			t.Errorf("browseURL(%q)=%q want %q", c.args, got, c.want)
		}
	}
}
